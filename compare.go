package m68kemu

func init() {
	cmpEAMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskImmediate |
		eaMaskPCDisplacement | eaMaskPCIndex

	for opmode := uint16(0); opmode <= 2; opmode++ {
		match := uint16(0xb000) | (opmode << 6)
		registerInstruction(cmpInstruction, match, 0xf1c0, cmpEAMask, addCycleCalculator(opmode, false))
	}

	cmpiMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong
	for size := uint16(0); size < 3; size++ {
		match := uint16(0x0c00) | (size << 6)
		registerInstruction(cmpi, match, 0xffc0, cmpiMask, cmpiCycleCalculator())
	}

	for size := uint16(0); size < 3; size++ {
		match := uint16(0xb108) | (size << 6)
		registerInstruction(cmpm, match, 0xf1f8, 0, cmpmCycleCalculator())
	}
}

func cmpInstruction(cpu *cpu) error {
	size := operandSizeFromOpcode(cpu.regs.IR)

	src, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	srcVal, err := src.read()
	if err != nil {
		return err
	}

	dst := udx(cpu)
	_, flags := subWithFlags(srcVal, *dst&size.mask(), size)
	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry)) | (flags & (srNegative | srZero | srOverflow | srCarry))
	return nil
}

func cmpi(cpu *cpu) error {
	size := operandSizeFromOpcode(cpu.regs.IR)

	immSize := size
	if size == Byte {
		immSize = Word
	}

	srcVal, err := cpu.popPc(immSize)
	if err != nil {
		return err
	}
	srcVal &= size.mask()

	dst, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	dstVal, err := dst.read()
	if err != nil {
		return err
	}

	_, flags := subWithFlags(srcVal, dstVal, size)
	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry)) | (flags & (srNegative | srZero | srOverflow | srCarry))
	return nil
}

func cmpm(cpu *cpu) error {
	size := operandSizeFromOpcode(cpu.regs.IR)
	srcReg := cpu.regs.IR & 0x7
	dstReg := (cpu.regs.IR >> 9) & 0x7

	srcAddr := cpu.regs.A[srcReg]
	dstAddr := cpu.regs.A[dstReg]

	srcVal, err := cpu.read(size, srcAddr)
	if err != nil {
		return err
	}
	dstVal, err := cpu.read(size, dstAddr)
	if err != nil {
		return err
	}

	cpu.regs.A[srcReg] += uint32(size)
	cpu.regs.A[dstReg] += uint32(size)

	_, flags := subWithFlags(srcVal, dstVal, size)
	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry)) | (flags & (srNegative | srZero | srOverflow | srCarry))
	return nil
}

func cmpiCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		size := operandSizeFromOpcode(opcode)
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		return 8 + eaAccessCycles(mode, reg, size)
	}
}

func cmpmCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		switch operandSizeFromOpcode(opcode) {
		case Byte, Word:
			return 12
		case Long:
			return 20
		default:
			return 0
		}
	}
}
