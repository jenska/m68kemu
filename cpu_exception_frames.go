package m68kemu

// Exception stack frames of the MC68010 and later. Every frame ends with a
// format/vector word: the format in bits 15-12, the vector offset (vector*4)
// in bits 11-0. Format $0 is the four-word frame of most exceptions. Bus and
// address errors push the 29-word format $8 frame on the MC68010 and the
// 46-word format $B frame from the MC68020 on, which also has format $1
// (throwaway), $2 (with the instruction address), $9 (coprocessor
// mid-instruction) and $A (short bus fault).

// frameSize returns the size in bytes of a frame of the given format on the
// CPU's model, or 0 if the model does not have the format.
func (cpu *CPU) frameSize(format uint16) uint32 {
	switch {
	case format == 0:
		return 8
	case cpu.model == M68010:
		if format == 8 {
			return 58
		}
	case format == 1:
		return 8
	case format == 2:
		return 12
	case format == 9:
		return 20
	case format == 0xa:
		return 32
	case format == 0xb:
		return 92
	}
	return 0
}

// usesFormat2 reports whether the exception with vector pushes a format $2
// frame on the MC68020 and later: divide by zero, CHK and CHK2, TRAPcc and
// TRAPV, and trace.
func usesFormat2(vector uint32) bool {
	switch vector {
	case XDivByZero, 6, 7, 9:
		return true
	}
	return false
}

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
		if f.instructionFetch() {
			word |= 1 << 13
		} else {
			word |= 1 << 12
		}
	}
	return word
}

// instructionFetch reports whether the fault was an instruction fetch.
func (f faultInfo) instructionFetch() bool {
	return accessContext{functionCode: f.functionCode, notInstruction: f.notInstruction, write: f.write}.instructionFetch()
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

// specialStatusWord020 returns the MC68020/MC68030 special status word for
// the fault: FB and RB (bits 14 and 12) for an instruction fetch, DF (8) for a
// data access, RW (6) for a read, the size in bits 5-4 (byte 01, word 10,
// long 00) and the function code in bits 2-0.
func (f faultInfo) specialStatusWord020() uint16 {
	word := f.functionCode & 7
	if f.instructionFetch() {
		word |= 1<<14 | 1<<12
	} else {
		word |= 1 << 8
	}
	if !f.write {
		word |= 1 << 6
	}
	switch f.size {
	case Byte:
		word |= 1 << 4
	case Word:
		word |= 2 << 4
	}
	return word
}

// frameField is one word or long of an exception stack frame.
type frameField struct {
	size   Size
	offset uint32
	value  uint32
}

// raiseBusFault takes a bus or address error on the MC68010 and later.
// Instead of saving the internal state of the interrupted instruction, as the
// real CPU does, it restores the registers saved when the instruction started
// and stacks the instruction's address, so RTE runs the whole instruction
// again. The MC68010 pushes a format $8 frame, later models format $B.
func (cpu *CPU) raiseBusFault(vector uint32) error {
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

	frame := ExceptionStackFrame{
		SR:           originalSR,
		PC:           pc,
		VectorOffset: uint16(vector << 2),
		FaultAddress: fault.address,
	}
	var format uint16
	var fields []frameField
	if cpu.model >= M68020 {
		format = 0xb
		frame.Format = ExceptionStackFrameFormatB
		frame.StatusWord = fault.specialStatusWord020()
		fields = []frameField{{Word, 10, uint32(frame.StatusWord)}}
		if frame.StatusWord&(1<<8) != 0 { // data fault
			frame.DataOutput = fault.value
			fields = append(fields, frameField{Long, 16, fault.address}, frameField{Long, 24, fault.value})
		} else {
			frame.StageBAddress = fault.address
			fields = append(fields, frameField{Long, 36, fault.address})
		}
	} else {
		format = 8
		frame.Format = ExceptionStackFrameFormat8
		frame.StatusWord = fault.specialStatusWord()
		frame.DataOutput = uint32(fault.dataOutputBuffer())
		fields = []frameField{{Word, 8, uint32(frame.StatusWord)}, {Long, 10, fault.address}, {Word, 16, frame.DataOutput}}
	}
	size := cpu.frameSize(format)
	sp := cpu.regs.A[7] - size
	cpu.regs.A[7] = sp
	frame.StackPointer = sp

	// Reserved words, input buffers and internal information stay zero.
	for offset := uint32(8); offset < size; offset += 2 {
		if err := cpu.writeSystemData(Word, sp+offset, 0); err != nil {
			return err
		}
	}
	fields = append(fields, frameField{Word, 0, uint32(originalSR)}, frameField{Long, 2, pc},
		frameField{Word, 6, uint32(formatWord(format, vector))})
	for _, w := range fields {
		if err := cpu.writeSystemData(w.size, sp+w.offset, w.value); err != nil {
			return err
		}
	}

	handler, err := cpu.readVector(vector << 2)
	if err != nil {
		return err
	}
	cpu.regs.PC = handler
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

// rteFormatted is RTE on the MC68010 and later: it pops a frame of a format
// the model has and raises the format-error exception for any other.
// Returning from a bus fault frame reruns the instruction that faulted (see
// raiseBusFault). A throwaway frame (format $1) is popped, its SR selects the
// stack, and RTE continues with the frame there.
func rteFormatted(cpu *CPU) error {
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
	}
	for {
		again, err := cpu.popFrame()
		if err != nil || !again {
			return err
		}
	}
}

// popFrame pops one exception frame for RTE and reports whether it was a
// throwaway frame, after which RTE continues.
func (cpu *CPU) popFrame() (bool, error) {
	sp := cpu.regs.A[7]
	sr, err := cpu.read(Word, sp)
	if err != nil {
		return false, err
	}
	pc, err := cpu.read(Long, sp+2)
	if err != nil {
		return false, err
	}
	format, err := cpu.read(Word, sp+6)
	if err != nil {
		return false, err
	}
	size := cpu.frameSize(uint16(format >> 12))
	if size == 0 {
		return false, cpu.exceptionWithCycles(XFormatError, exceptionCyclesIllegal)
	}
	cpu.regs.A[7] = sp + size
	cpu.setSR(uint16(sr))
	if format>>12 == 1 {
		return true, nil
	}
	cpu.regs.PC = pc
	return false, nil
}
