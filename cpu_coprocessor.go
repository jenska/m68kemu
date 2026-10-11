package m68kemu

import (
	"encoding/binary"
	"errors"
)

// Coprocessor is a coprocessor attached through the MC68020 coprocessor
// interface with WithCoprocessor, such as the MC68881/MC68882 FPU of package
// fpu. The CPU decodes the F-line instructions with the coprocessor's ID,
// evaluates their effective addresses and moves operands as bytes in memory
// format; the coprocessor decodes and executes its commands.
type Coprocessor interface {
	// Decode checks the command word of a general instruction (cpGEN) and
	// returns the data register (0-7) whose value it needs, for example for a
	// dynamic register list, or -1.
	Decode(cmd uint16) (register int, err error)
	// Operands returns the number of bytes the command reads from and
	// writes to its effective address, given the value of that register.
	Operands(cmd uint16, dynamic uint32) (in, out int)
	// Execute runs the command with its source operand and returns its
	// result operand. in is only valid during the call.
	Execute(cmd uint16, dynamic uint32, in []byte) ([]byte, error)
	// Condition evaluates a conditional predicate for cpBcc, cpScc, cpDBcc
	// and cpTRAPcc.
	Condition(predicate uint16) (bool, error)
	// Save returns the coprocessor's state frame for cpSAVE, starting with a
	// long whose bits 23-16 hold the size of the rest of the frame.
	Save() []byte
	// Restore loads a frame for cpRESTORE.
	Restore(frame []byte) error
	// PendingException reports an exception the coprocessor wants the CPU to
	// take before its next instruction.
	PendingException() (vector int, ok bool)
	// AcknowledgeException tells the coprocessor that the CPU took it.
	AcknowledgeException()
	// SetInstructionAddress passes the address of the instruction about to
	// run.
	SetInstructionAddress(address uint32)
}

// CoprocessorTiming is implemented by a Coprocessor that reports how long its
// commands take. The CPU then models the concurrency of the MC68020
// coprocessor interface: a general instruction (cpGEN) that returns nothing to
// the CPU lets the CPU go on with the next instructions while the coprocessor
// computes, and the next instruction for that coprocessor waits until it has
// finished. A command that returns an operand, such as FMOVE FPn,<ea>, keeps
// the CPU waiting for its result. Without CoprocessorTiming every coprocessor
// instruction costs only its CPU cycles.
type CoprocessorTiming interface {
	// Cycles returns the time the general instruction cmd takes once its
	// operand has arrived, in CPU clock cycles. dynamic is the value of the
	// data register Decode named.
	Cycles(cmd uint16, dynamic uint32) uint32
}

// ErrCoprocessorUnsupported is returned by NewCPU for a coprocessor on a
// model without a coprocessor interface (before the MC68020) or with an ID
// outside 0-7.
var ErrCoprocessorUnsupported = errors.New("m68kemu: coprocessor not supported")

// WithCoprocessor attaches c with coprocessor ID id (0-7) to an MC68020 or
// later. The MC68881/MC68882 FPU conventionally uses ID 1.
func WithCoprocessor(id int, c Coprocessor) Option {
	return func(cfg *cpuConfig) {
		cfg.coprocessors = append(cfg.coprocessors, attachedCoprocessor{id, c})
	}
}

type attachedCoprocessor struct {
	id int
	c  Coprocessor
}

// coprocessorInstruction runs an F-line instruction for the coprocessor its
// bits 11-9 name; without one it raises the line-F exception.
func coprocessorInstruction(cpu *cpu) error {
	id := (cpu.regs.IR >> 9) & 7
	cp := cpu.coprocessors[id]
	instructionPC := cpu.currentOpcodeAddress(cpu.regs.PC - 2)
	if cp == nil {
		return cpu.opcodeException(XLineF, instructionPC)
	}
	if busy := cpu.coprocessorBusy[id]; busy > cpu.cycles {
		cpu.addCycles(uint32(busy - cpu.cycles)) // wait for the previous command
	}
	kind := (cpu.regs.IR >> 6) & 7
	if kind <= 3 { // cpSAVE and cpRESTORE do not take pending exceptions
		if vector, ok := cp.PendingException(); ok {
			cp.AcknowledgeException()
			cpu.overrideInstructionCycles(exceptionCyclesIllegal)
			return cpu.raiseExceptionWithPC(uint32(vector), cpu.regs.SR|srSupervisor, instructionPC, instructionPC)
		}
	}
	cp.SetInstructionAddress(instructionPC)

	var err error
	switch kind {
	case 0:
		err = cpu.cpGeneral(cp, id)
	case 1:
		err = cpu.cpConditional(cp)
	case 2, 3:
		err = cpu.cpBranch(cp, kind == 3)
	case 4:
		err = cpu.cpSave(cp)
	case 5:
		err = cpu.cpRestore(cp)
	default:
		return cpu.opcodeException(XLineF, instructionPC)
	}
	if errors.Is(err, errCoprocessorRejected) {
		return cpu.opcodeException(XLineF, instructionPC)
	}
	return err
}

// errCoprocessorRejected reports a command or predicate the coprocessor does
// not implement; the CPU takes the line-F exception.
var errCoprocessorRejected = errors.New("m68kemu: coprocessor rejected the instruction")

func (cpu *cpu) cpGeneral(cp Coprocessor, id uint16) error {
	cmd, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	register, err := cp.Decode(uint16(cmd))
	if err != nil {
		return errCoprocessorRejected
	}
	var dynamic uint32
	if register >= 0 {
		dynamic = uint32(cpu.regs.D[register&7])
	}
	in, out := cp.Operands(uint16(cmd), dynamic)
	var location cpOperand
	if in > 0 || out > 0 {
		if location, err = cpu.cpOperandLocation(max(in, out)); err != nil {
			return err
		}
		if location.kind == cpOperandRegister && max(in, out) > 4 {
			return errCoprocessorRejected // a register holds at most a long
		}
	}
	var operand []byte
	if in > 0 {
		if cap(cpu.cpOperand) < in {
			cpu.cpOperand = make([]byte, in)
		}
		operand = cpu.cpOperand[:in]
		if err := location.readInto(cpu, operand); err != nil {
			return err
		}
	}
	result, err := cp.Execute(uint16(cmd), dynamic, operand)
	if err != nil {
		return errCoprocessorRejected
	}
	if timing := cpu.coprocessorTimings[id]; timing != nil {
		busy := timing.Cycles(uint16(cmd), dynamic)
		if out > 0 {
			cpu.addCycles(busy)
		} else {
			cpu.coprocessorBusy[id] = cpu.cycles + uint64(busy)
		}
	}
	if out > 0 {
		return location.write(cpu, result)
	}
	return nil
}

// cpConditional runs cpScc, cpDBcc and cpTRAPcc.
func (cpu *cpu) cpConditional(cp Coprocessor) error {
	condition, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	mode, reg := (cpu.regs.IR>>3)&7, cpu.regs.IR&7
	switch {
	case mode == 1: // cpDBcc
		basePC := cpu.regs.PC
		displacement, err := cpu.popPc(Word)
		if err != nil {
			return err
		}
		ok, err := cp.Condition(uint16(condition) & 0x3f)
		if err != nil {
			return errCoprocessorRejected
		}
		if ok {
			return nil
		}
		counter := uint16(cpu.regs.D[reg]) - 1
		cpu.regs.D[reg] = cpu.regs.D[reg]&^0xffff | int32(counter)
		if counter != 0xffff {
			cpu.regs.PC = uint32(int32(basePC) + int32(int16(displacement)))
		}
		return nil
	case mode == 7 && reg >= 2 && reg <= 4: // cpTRAPcc
		cpu.regs.PC += [...]uint32{2, 4, 0}[reg-2]
		ok, err := cp.Condition(uint16(condition) & 0x3f)
		if err != nil {
			return errCoprocessorRejected
		}
		if !ok {
			return nil
		}
		return cpu.synchronousException(7, cpu.regs.PC)
	}
	dst, err := cpu.ResolveSrcEA(Byte) // cpScc
	if err != nil {
		return err
	}
	ok, err := cp.Condition(uint16(condition) & 0x3f)
	if err != nil {
		return errCoprocessorRejected
	}
	if ok {
		return dst.write(0xff)
	}
	return dst.write(0)
}

// cpBranch runs cpBcc with a word or long displacement.
func (cpu *cpu) cpBranch(cp Coprocessor, long bool) error {
	basePC := cpu.regs.PC
	size := Word
	if long {
		size = Long
	}
	displacement, err := cpu.popPc(size)
	if err != nil {
		return err
	}
	if !long {
		displacement = uint32(int32(int16(displacement)))
	}
	ok, err := cp.Condition(cpu.regs.IR & 0x3f)
	if err != nil {
		return errCoprocessorRejected
	}
	if ok {
		cpu.regs.PC = basePC + displacement
	}
	return nil
}

// cpSave writes the coprocessor's state frame to the effective address, a
// control alterable mode or -(An).
func (cpu *cpu) cpSave(cp Coprocessor) error {
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
	}
	frame := cp.Save()
	location, err := cpu.cpOperandLocation(len(frame))
	if err != nil {
		return err
	}
	return location.write(cpu, frame)
}

// cpRestore reads a state frame from the effective address, a control mode
// or (An)+: its first long, then as many bytes as that long gives. A frame
// the coprocessor rejects raises the format-error exception.
func (cpu *cpu) cpRestore(cp Coprocessor) error {
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
	}
	location, err := cpu.cpOperandLocation(0)
	if err != nil {
		return err
	}
	header, err := location.read(cpu, 4)
	if err != nil {
		return err
	}
	rest, err := location.read(cpu, int(header[1]))
	if err != nil {
		return err
	}
	if err := cp.Restore(append(header, rest...)); err != nil {
		return cpu.exceptionWithCycles(XFormatError, exceptionCyclesIllegal)
	}
	return nil
}

// cpOperand is where a coprocessor operand lives: a data or address register,
// immediate data in the instruction stream, or memory.
type cpOperand struct {
	register *uint32
	address  uint32
	kind     int
	// a7 marks (A7)+, which steps by 2 for a byte to keep the stack
	// aligned.
	a7 bool
}

const (
	cpOperandMemory = iota
	cpOperandRegister
	cpOperandImmediate
)

// cpOperandLocation evaluates the instruction's effective address for an
// operand of size bytes. (An)+ and -(An) step by the whole operand; a
// cpRESTORE through (An)+ passes size 0 and steps as it reads.
func (cpu *cpu) cpOperandLocation(size int) (cpOperand, error) {
	mode, reg := (cpu.regs.IR>>3)&7, cpu.regs.IR&7
	switch {
	case mode == 0:
		return cpOperand{register: generalRegister(cpu, reg), kind: cpOperandRegister}, nil
	case mode == 1:
		return cpOperand{register: &cpu.regs.A[reg], kind: cpOperandRegister}, nil
	case mode == 7 && reg == 4:
		return cpOperand{kind: cpOperandImmediate}, nil
	case mode == 3: // (An)+: next steps the register
		return cpOperand{register: &cpu.regs.A[reg], a7: reg == 7}, nil
	case mode == 4: // -(An)
		step := uint32(size)
		if size == 1 && reg == 7 {
			step = 2
		}
		cpu.regs.A[reg] -= step
		return cpOperand{address: cpu.regs.A[reg]}, nil
	}
	operand, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return cpOperand{}, err
	}
	return cpOperand{address: operand.computedAddress()}, nil
}

// read returns the next n bytes of the operand.
func (o *cpOperand) read(cpu *cpu, n int) ([]byte, error) {
	b := make([]byte, n)
	if err := o.readInto(cpu, b); err != nil {
		return nil, err
	}
	return b, nil
}

// readInto fills b with the next len(b) bytes of the operand.
func (o *cpOperand) readInto(cpu *cpu, b []byte) error {
	n := len(b)
	switch o.kind {
	case cpOperandRegister:
		var w [4]byte
		binary.BigEndian.PutUint32(w[:], *o.register)
		copy(b, w[4-min(n, 4):])
		return nil
	case cpOperandImmediate:
		if n == 1 {
			v, err := cpu.popPc(Word)
			b[0] = byte(v)
			return err
		}
		for i := 0; i < n; i += 2 {
			v, err := cpu.popPc(Word)
			if err != nil {
				return err
			}
			binary.BigEndian.PutUint16(b[i:], uint16(v))
		}
		return nil
	}
	address := o.next(n)
	for i := 0; i < n; {
		size := memoryChunk(n - i)
		v, err := cpu.read(size, address+uint32(i))
		if err != nil {
			return err
		}
		putBigEndian(b[i:], size, v)
		i += int(size)
	}
	return nil
}

// write stores b as the operand.
func (o *cpOperand) write(cpu *cpu, b []byte) error {
	switch o.kind {
	case cpOperandRegister:
		var w [4]byte
		binary.BigEndian.PutUint32(w[:], *o.register)
		copy(w[4-min(len(b), 4):], b)
		*o.register = binary.BigEndian.Uint32(w[:])
		return nil
	case cpOperandImmediate:
		return errCoprocessorRejected
	}
	address := o.next(len(b))
	for i := 0; i < len(b); {
		size := memoryChunk(len(b) - i)
		if err := cpu.write(size, address+uint32(i), getBigEndian(b[i:], size)); err != nil {
			return err
		}
		i += int(size)
	}
	return nil
}

// next returns the address of the next n bytes of a memory operand and, for
// (An)+, steps the register past them.
func (o *cpOperand) next(n int) uint32 {
	if o.register == nil {
		address := o.address
		o.address += uint32(n)
		return address
	}
	address := *o.register
	step := uint32(n)
	if n == 1 && o.a7 {
		step = 2
	}
	*o.register += step
	return address
}

// memoryChunk returns the largest access that fits in n remaining bytes.
func memoryChunk(n int) Size {
	switch {
	case n >= 4:
		return Long
	case n >= 2:
		return Word
	}
	return Byte
}

func putBigEndian(b []byte, size Size, v uint32) {
	switch size {
	case Long:
		binary.BigEndian.PutUint32(b, v)
	case Word:
		binary.BigEndian.PutUint16(b, uint16(v))
	default:
		b[0] = byte(v)
	}
}

func getBigEndian(b []byte, size Size) uint32 {
	switch size {
	case Long:
		return binary.BigEndian.Uint32(b)
	case Word:
		return uint32(binary.BigEndian.Uint16(b))
	}
	return uint32(b[0])
}
