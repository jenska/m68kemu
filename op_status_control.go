package m68kemu

func init() {
	registerInstruction(trapv, 0x4e76, 0xffff, 0, constantCycles(4))
	registerInstruction(resetInstruction, 0x4e70, 0xffff, 0, constantCycles(132))
	registerInstruction(stop, 0x4e72, 0xffff, 0, constantCycles(4))

	registerInstruction(oriToCcr, 0x003c, 0xffff, 0, constantCycles(20))
	registerInstruction(oriToSr, 0x007c, 0xffff, 0, constantCycles(20))
	registerInstruction(andiToCcr, 0x023c, 0xffff, 0, constantCycles(20))
	registerInstruction(andiToSr, 0x027c, 0xffff, 0, constantCycles(20))
	registerInstruction(eoriToCcr, 0x0a3c, 0xffff, 0, constantCycles(20))
	registerInstruction(eoriToSr, 0x0a7c, 0xffff, 0, constantCycles(20))

	const controlSourceMask = eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskImmediate |
		eaMaskPCDisplacement | eaMaskPCIndex
	const controlDestinationMask = eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong

	registerInstruction(moveFromSr, 0x40c0, 0xffc0, controlDestinationMask, moveControlCycleCalculator(Word))
	registerInstruction(moveToCcr, 0x44c0, 0xffc0, controlSourceMask, moveControlCycleCalculator(Byte))
	registerInstruction(moveToSr, 0x46c0, 0xffc0, controlSourceMask, moveControlCycleCalculator(Word))

	registerInstruction(rte, 0x4e73, 0xffff, 0, constantCycles(20))
	registerInstruction(rtr, 0x4e77, 0xffff, 0, constantCycles(20))
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

func clrTstCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		size := operandSizeFromOpcode(opcode)
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		return 4 + eaAccessCycles(mode, reg, size)
	}
}

func logicalCcrOp(cpu *cpu, op func(uint16, uint16) uint16) error {
	imm, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	current := cpu.regs.SR
	newCcr := op(current&0xff, uint16(imm)) & 0xff
	cpu.setSR((current & 0xff00) | newCcr)
	return nil
}

func logicalSrOp(cpu *cpu, op func(uint16, uint16) uint16) error {
	if cpu.regs.SR&srSupervisor == 0 {
		return cpu.exception(XPrivViolation)
	}
	imm, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	cpu.setSR(op(cpu.regs.SR, uint16(imm)))
	return nil
}

func oriToCcr(cpu *cpu) error  { return logicalCcrOp(cpu, func(a, b uint16) uint16 { return a | b }) }
func oriToSr(cpu *cpu) error   { return logicalSrOp(cpu, func(a, b uint16) uint16 { return a | b }) }
func andiToCcr(cpu *cpu) error { return logicalCcrOp(cpu, func(a, b uint16) uint16 { return a & b }) }
func andiToSr(cpu *cpu) error  { return logicalSrOp(cpu, func(a, b uint16) uint16 { return a & b }) }
func eoriToCcr(cpu *cpu) error { return logicalCcrOp(cpu, func(a, b uint16) uint16 { return a ^ b }) }
func eoriToSr(cpu *cpu) error  { return logicalSrOp(cpu, func(a, b uint16) uint16 { return a ^ b }) }

func moveControlCycleCalculator(size Size) cycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		return 12 + eaAccessCycles(mode, reg, size)
	}
}

func moveFromSr(cpu *cpu) error {
	dst, err := cpu.ResolveSrcEA2(Word)
	if err != nil {
		return err
	}
	return dst.write(uint32(cpu.regs.SR))
}

func moveToCcr(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Byte)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}
	current := cpu.regs.SR & 0xff00
	cpu.setSR(current | uint16(value&0xff))
	return nil
}

func moveToSr(cpu *cpu) error {
	if cpu.regs.SR&srSupervisor == 0 {
		return cpu.exception(XPrivViolation)
	}
	src, err := cpu.ResolveSrcEA(Word)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}
	cpu.setSR(uint16(value))
	return nil
}

func rte(cpu *cpu) error {
	if cpu.regs.SR&srSupervisor == 0 {
		return cpu.exception(XPrivViolation)
	}
	newSR, err := cpu.pop(Word)
	if err != nil {
		return err
	}
	pc, err := cpu.pop(Long)
	if err != nil {
		return err
	}
	cpu.setSR(uint16(newSR))
	cpu.regs.PC = pc
	return nil
}

func rtr(cpu *cpu) error {
	ccr, err := cpu.pop(Word)
	if err != nil {
		return err
	}
	pc, err := cpu.pop(Long)
	if err != nil {
		return err
	}
	preserved := cpu.regs.SR & 0xff00
	cpu.setSR(preserved | uint16(ccr&0xff))
	cpu.regs.PC = pc
	return nil
}
