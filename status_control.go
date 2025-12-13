package m68kemu

func init() {
	alterableNoAddr := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong

	for size := uint16(0); size < 3; size++ {
		match := uint16(0x4200) | (size << 6)
		registerInstruction(clr, match, 0xffc0, alterableNoAddr, clrTstCycleCalculator())

		match = uint16(0x4a00) | (size << 6)
		registerInstruction(tst, match, 0xffc0, alterableNoAddr|eaMaskPCDisplacement|eaMaskPCIndex|eaMaskImmediate, clrTstCycleCalculator())
	}

	addaSubaMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskPCDisplacement | eaMaskPCIndex | eaMaskImmediate

	for opmode := uint16(3); opmode <= 7; opmode += 4 { // 3=word, 7=long
		match := uint16(0xd000) | (opmode << 6)
		registerInstruction(adda, match, 0xf1c0, addaSubaMask, addaSubaCycleCalculator())

		match = uint16(0x9000) | (opmode << 6)
		registerInstruction(suba, match, 0xf1c0, addaSubaMask, addaSubaCycleCalculator())
	}

	registerInstruction(trapv, 0x4e76, 0xffff, 0, constantCycles(4))
	registerInstruction(resetInstruction, 0x4e70, 0xffff, 0, constantCycles(132))
	registerInstruction(stop, 0x4e72, 0xffff, 0, constantCycles(4))
}

func clr(cpu *cpu) error {
	size := operandSizeFromOpcode(cpu.regs.IR)

	dst, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}

	if err := dst.write(0); err != nil {
		return err
	}

	cpu.regs.SR &^= srNegative | srZero | srOverflow | srCarry
	cpu.regs.SR |= srZero
	return nil
}

func tst(cpu *cpu) error {
	size := operandSizeFromOpcode(cpu.regs.IR)

	src, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	cpu.regs.SR &^= srNegative | srZero | srOverflow | srCarry
	if isZeroForSize(value, size) {
		cpu.regs.SR |= srZero
	} else if isNegativeForSize(value, size) {
		cpu.regs.SR |= srNegative
	}

	return nil
}

func adda(cpu *cpu) error {
	opmode := (cpu.regs.IR >> 6) & 0x7
	size := Word
	if opmode == 7 {
		size = Long
	}

	src, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	if size == Word {
		value = uint32(int32(int16(value)))
	}

	dst := ax(cpu)
	*dst += value
	return nil
}

func suba(cpu *cpu) error {
	opmode := (cpu.regs.IR >> 6) & 0x7
	size := Word
	if opmode == 7 {
		size = Long
	}

	src, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	if size == Word {
		value = uint32(int32(int16(value)))
	}

	dst := ax(cpu)
	*dst -= value
	return nil
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

func addaSubaCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		size := operandSizeFromOpcode(opcode)
		return 8 + eaAccessCycles(mode, reg, size)
	}
}

func isZeroForSize(value uint32, size Size) bool {
	return value&maskForSize(size) == 0
}

func isNegativeForSize(value uint32, size Size) bool {
	return value&signBit(size) != 0
}
