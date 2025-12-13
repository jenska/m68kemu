package m68kemu

func init() {
	registerInstruction(trapv, 0x4e76, 0xffff, 0, constantCycles(4))
	registerInstruction(resetInstruction, 0x4e70, 0xffff, 0, constantCycles(132))
	registerInstruction(stop, 0x4e72, 0xffff, 0, constantCycles(4))
}

func trapv(cpu *cpu) error {
	if cpu.regs.SR&srOverflow == 0 {
		return nil
	}

	return cpu.exception(7)
}

func resetInstruction(cpu *cpu) error {
	if cpu.regs.SR&srSupervisor == 0 {
		return cpu.exception(XPrivViolation)
	}

	cpu.bus.Reset()
	cpu.interrupts = NewInterruptController()
	return nil
}

func stop(cpu *cpu) error {
	if cpu.regs.SR&srSupervisor == 0 {
		return cpu.exception(XPrivViolation)
	}

	newSR, err := cpu.read(Word, cpu.regs.PC)
	if err != nil {
		return err
	}
	cpu.regs.PC += uint32(Word)
	cpu.setSR(uint16(newSR))
	cpu.stopped = true
	return nil
}

func operandSizeFromOpcode(ir uint16) Size {
	switch (ir >> 6) & 0x3 {
	case 0:
		return Byte
	case 1:
		return Word
	default:
		return Long
	}
}

func clrTstCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		size := operandSizeFromOpcode(opcode)
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		return 4 + eaAccessCycles(mode, reg, size)
	}
}
