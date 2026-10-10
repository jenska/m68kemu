package m68kemu

// Exception stack frames of the MC68010. Every frame ends with a format/vector
// word: the format in bits 15-12, the vector offset (vector*4) in bits 11-0.
// Format $0 is the four-word frame of all exceptions except bus and address
// errors, which push the 29-word format $8 frame.

const (
	format0FrameSize uint32 = 8
	format8FrameSize uint32 = 58
)

// formatWord returns the format/vector word for format and vector.
func formatWord(format uint16, vector uint32) uint16 {
	return format<<12 | uint16(vector<<2)&0x0fff
}

// specialStatusWord returns the MC68010 special status word for the fault:
// IF (bit 13) for an instruction fetch, DF (12) for any other read, HB (10)
// for a byte at an even address, BY (9) for a byte transfer, RW (8) for a
// read, and the function code in bits 2-0. RR (15) and RM (11) stay clear.
func (f faultInfo) specialStatusWord() uint16 {
	word := f.functionCode & 7
	if f.size == Byte {
		word |= 1 << 9
		if f.address&1 == 0 {
			word |= 1 << 10
		}
	}
	if !f.write {
		word |= 1 << 8
		fc := f.functionCode & 7
		if !f.notInstruction && (fc == functionCodeUserProgram || fc == functionCodeSupervisorProg) {
			word |= 1 << 13
		} else {
			word |= 1 << 12
		}
	}
	return word
}

// dataOutputBuffer returns the word the faulted write cycle was driving: the
// whole value of a byte or word write, the high word of a long write.
func (f faultInfo) dataOutputBuffer() uint16 {
	if !f.write {
		return 0
	}
	if f.size == Long {
		return uint16(f.value >> 16)
	}
	return uint16(f.value)
}

// raiseFormat8Exception takes a bus or address error on the MC68010. Instead
// of saving the internal state of the interrupted instruction, as the real
// CPU does, it restores the registers saved when the instruction started and
// stacks the instruction's address, so RTE runs the whole instruction again.
func (cpu *cpu) raiseFormat8Exception(vector uint32) error {
	fault := cpu.fault
	pc := cpu.currentOpcodeAddress(fault.pc)
	cpu.regs = cpu.restartRegs
	cpu.regs.IR = fault.ir

	originalSR := cpu.regs.SR
	cpu.inException = true
	defer func() {
		cpu.inException = false
	}()
	cpu.setSR(originalSR | srSupervisor)

	sp := cpu.regs.A[7] - format8FrameSize
	cpu.regs.A[7] = sp
	ssw := fault.specialStatusWord()
	dob := fault.dataOutputBuffer()

	// The reserved words, the input buffers and the 16 words of internal
	// information stay zero.
	for offset := uint32(14); offset < format8FrameSize; offset += 2 {
		if err := cpu.writeSystemData(Word, sp+offset, 0); err != nil {
			return err
		}
	}
	for _, w := range []struct {
		size   Size
		offset uint32
		value  uint32
	}{
		{Word, 0, uint32(originalSR)},
		{Long, 2, pc},
		{Word, 6, uint32(formatWord(8, vector))},
		{Word, 8, uint32(ssw)},
		{Long, 10, fault.address},
		{Word, 16, uint32(dob)},
	} {
		if err := cpu.writeSystemData(w.size, sp+w.offset, w.value); err != nil {
			return err
		}
	}

	handler, err := cpu.readVector(vector << 2)
	if err != nil {
		return err
	}
	cpu.regs.PC = handler
	frame := ExceptionStackFrame{
		Format:       ExceptionStackFrameFormat8,
		StackPointer: sp,
		StatusWord:   ssw,
		FaultAddress: fault.address,
		SR:           originalSR,
		PC:           pc,
		VectorOffset: uint16(vector << 2),
		DataOutput:   dob,
	}
	cpu.dispatchException(ExceptionInfo{
		Vector:        vector,
		PC:            pc,
		NewPC:         handler,
		Opcode:        fault.ir,
		OpcodeAddress: pc,
		FaultAddress:  fault.address,
		FaultValid:    fault.valid,
		SR:            originalSR,
		NewSR:         cpu.regs.SR,
		StackPointer:  sp,
		Frame:         frame,
		FrameValid:    true,
		InterruptMask: cpu.interruptMask(),
		Group0:        true,
	})
	return nil
}

// rteFormatted is RTE on the MC68010: it pops a format $0 or $8 frame and
// raises the format-error exception for any other format. Returning from a
// format $8 frame reruns the instruction that faulted (see
// raiseFormat8Exception).
func rteFormatted(cpu *cpu) error {
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
	}
	sp := cpu.regs.A[7]
	sr, err := cpu.read(Word, sp)
	if err != nil {
		return err
	}
	pc, err := cpu.read(Long, sp+2)
	if err != nil {
		return err
	}
	format, err := cpu.read(Word, sp+6)
	if err != nil {
		return err
	}
	var size uint32
	switch format >> 12 {
	case 0:
		size = format0FrameSize
	case 8:
		size = format8FrameSize
	default:
		return cpu.exceptionWithCycles(XFormatError, exceptionCyclesIllegal)
	}
	cpu.regs.A[7] = sp + size
	cpu.setSR(uint16(sr))
	cpu.regs.PC = pc
	return nil
}
