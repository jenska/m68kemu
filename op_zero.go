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
	if size.isZero(value) {
		cpu.regs.SR |= srZero
	} else if size.isNegative(value) {
		cpu.regs.SR |= srNegative
	}

	return nil
}
