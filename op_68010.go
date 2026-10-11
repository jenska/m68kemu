package m68kemu

// Instructions the MC68010 adds to the MC68000 set. registerSystem picks the
// model-dependent variants of MOVEC and MOVE from SR.

func register68010(b *tableBuilder) {
	if b.model < M68010 {
		return
	}
	const dataAlterableMask = eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong
	const memoryAlterableMask = dataAlterableMask &^ eaMaskDataRegister

	// MC68010 User's Manual: MOVEC takes 10 cycles to a general register and
	// 12 to a control register, RTD 16.
	b.add(movec, 0x4e7a, 0xffff, 0, constantCycles(10))
	b.add(movec, 0x4e7b, 0xffff, 0, constantCycles(12))
	b.add(rtd, 0x4e74, 0xffff, 0, constantCycles(16))
	b.add(moveFromCcr, 0x42c0, 0xffc0, dataAlterableMask, moveFromSrCycleCalculator())
	for size := range uint16(3) {
		b.add(moves, 0x0e00|size<<6, 0xffc0, memoryAlterableMask, movesCycleCalculator())
	}
}

// generalRegister returns Dn (r = 0-7) or An (r = 8-15), as named by the
// register field of a MOVEC or MOVES extension word.
func generalRegister(cpu *CPU, r uint16) *uint32 {
	if r&8 != 0 {
		return &cpu.regs.A[r&7]
	}
	return &cpu.regs.D[r&7]
}

// movec copies a control register to a general register (MOVEC Rc,Rn, $4E7A)
// or back (MOVEC Rn,Rc, $4E7B). A control register the model does not have
// raises the illegal-instruction exception.
func movec(cpu *CPU) error {
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
	}
	ext, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	rn := generalRegister(cpu, uint16(ext>>12))
	toControl := cpu.regs.IR&1 != 0

	reg, ok := cpu.controlRegister(uint16(ext & 0xfff))
	if !ok {
		// Give back the extension word, so the stacked PC is the same as for
		// an unassigned opcode word.
		cpu.regs.PC -= uint32(Word)
		return cpu.exceptionWithCycles(XIllegal, exceptionCyclesIllegal)
	}
	if toControl {
		reg.set(*rn)
	} else {
		*rn = reg.get()
	}
	return nil
}

// moves moves between a general register and memory in the address space
// selected by SFC (reads) or DFC (writes). Byte and word loads into an address
// register are sign-extended. The bus does not distinguish address spaces, so
// the function code only shows in bus traces and fault status words.
func moves(cpu *CPU) error {
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
	}
	ext, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	size := operandSizeFromOpcode(cpu.regs.IR)
	operand, err := cpu.resolveSrcEA(size)
	if err != nil {
		return err
	}
	address := operand.computedAddress()
	rn := generalRegister(cpu, uint16(ext>>12))

	if ext&0x0800 != 0 {
		ctx := accessContext{functionCode: uint16(cpu.regs.DFC), notInstruction: true, write: true}
		return cpu.writeCtx(size, address, *rn&size.mask(), ctx)
	}
	value, err := cpu.readCtx(size, address, accessContext{functionCode: uint16(cpu.regs.SFC), notInstruction: true})
	if err != nil {
		return err
	}
	switch {
	case ext&0x8000 == 0:
		*rn = *rn&^size.mask() | value
	case size == Byte:
		*rn = uint32(int32(int8(value)))
	case size == Word:
		*rn = uint32(int32(int16(value)))
	default:
		*rn = value
	}
	return nil
}

// movesCycleCalculator estimates MOVES as an extension word fetch plus the
// operand access; the MC68000 tables have no entry to reuse.
func movesCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		mode, reg := eaFields(opcode)
		return 8 + eaAccessCycles(mode, reg, operandSizeFromOpcode(opcode))
	}
}

// rtd pops the return address and then adds the sign-extended displacement
// to the stack pointer, releasing the caller's arguments.
func rtd(cpu *CPU) error {
	disp, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	addr, err := cpu.pop(Long)
	if err != nil {
		return err
	}
	cpu.regs.A[7] += uint32(int32(int16(disp)))
	cpu.regs.PC = addr
	return nil
}

// moveFromCcr stores the CCR as a word with the upper byte zero.
func moveFromCcr(cpu *CPU) error {
	dst, err := cpu.resolveSrcEA2(Word)
	if err != nil {
		return err
	}
	return dst.write(uint32(cpu.regs.SR & 0xff))
}

// moveFromSrPrivileged is MOVE SR,<ea> on the MC68010 and later, where it is
// a privileged instruction.
func moveFromSrPrivileged(cpu *CPU) error {
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
	}
	return moveFromSr(cpu)
}

// controlAccess reads and writes one MOVEC control register.
type controlAccess struct {
	get func() uint32
	set func(uint32)
}

// CACR bits that hold state; the others are write-only actions (clear the
// cache or an entry) or unused, and read as zero.
const (
	cacrBits68020 = 0x0003 // E, F
	cacrBits68030 = 0x3313 // EI, FI, IBE, ED, FD, DBE, WA
)

// controlRegister returns the MOVEC control register with the given code, if
// the model has it: SFC, DFC, USP and VBR from the MC68010 on; CACR, CAAR,
// MSP and ISP from the MC68020 on.
func (cpu *CPU) controlRegister(code uint16) (controlAccess, bool) {
	r := &cpu.regs
	stack := func(slot *uint32) controlAccess {
		return controlAccess{
			get: func() uint32 { return *cpu.stackPointer(slot) },
			set: func(v uint32) { *cpu.stackPointer(slot) = v },
		}
	}
	switch code {
	case 0x000:
		return controlAccess{func() uint32 { return uint32(r.SFC) }, func(v uint32) { r.SFC = uint8(v & 7) }}, true
	case 0x001:
		return controlAccess{func() uint32 { return uint32(r.DFC) }, func(v uint32) { r.DFC = uint8(v & 7) }}, true
	case 0x800:
		return stack(&r.USP), true
	case 0x801:
		return controlAccess{func() uint32 { return r.VBR }, func(v uint32) { r.VBR = v }}, true
	}
	if cpu.model < M68020 {
		return controlAccess{}, false
	}
	switch code {
	case 0x002:
		bits := uint32(cacrBits68020)
		if cpu.model >= M68030 {
			bits = cacrBits68030
		}
		return controlAccess{func() uint32 { return r.CACR }, func(v uint32) { r.CACR = v & bits }}, true
	case 0x802:
		return controlAccess{func() uint32 { return r.CAAR }, func(v uint32) { r.CAAR = v }}, true
	case 0x803:
		return stack(&r.MSP), true
	case 0x804:
		return stack(&r.SSP), true
	}
	return controlAccess{}, false
}
