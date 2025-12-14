package m68kemu

func init() {
	logicalSourceMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskPCDisplacement | eaMaskPCIndex
	logicalDestinationMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong

	// AND <ea>,Dn
	for opmode := uint16(0); opmode <= 2; opmode++ {
		match := uint16(0xc000) | (opmode << 6)
		registerLogicalInstruction(andInstruction, match, 0xf1c0, logicalSourceMask, logicalCycleCalculator(opmode, false))
	}

	// AND Dn,<ea>
	for opmode := uint16(4); opmode <= 6; opmode++ {
		match := uint16(0xc000) | (opmode << 6)
		registerLogicalInstruction(andInstruction, match, 0xf1c0, logicalDestinationMask, logicalCycleCalculator(opmode, true))
	}

	// OR <ea>,Dn
	for opmode := uint16(0); opmode <= 2; opmode++ {
		match := uint16(0x8000) | (opmode << 6)
		registerLogicalInstruction(orInstruction, match, 0xf1c0, logicalSourceMask, logicalCycleCalculator(opmode, false))
	}

	// OR Dn,<ea>
	for opmode := uint16(4); opmode <= 6; opmode++ {
		match := uint16(0x8000) | (opmode << 6)
		registerLogicalInstruction(orInstruction, match, 0xf1c0, logicalDestinationMask, logicalCycleCalculator(opmode, true))
	}

	// EOR Dn,<ea>
	for size := uint16(0); size < 3; size++ {
		match := uint16(0xb100) | (size << 6)
		registerLogicalInstruction(eorInstruction, match, 0xf1c0, logicalDestinationMask, logicalCycleCalculator(size, true))
	}

	// Immediate logical operations.
	immediateMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong
	for size := uint16(0); size < 3; size++ {
		registerInstruction(oriImmediate, uint16(0x0000)|(size<<6), 0xffc0, immediateMask, logicalImmediateCycleCalculator())
		registerInstruction(andiImmediate, uint16(0x0200)|(size<<6), 0xffc0, immediateMask, logicalImmediateCycleCalculator())
		registerInstruction(eoriImmediate, uint16(0x0a00)|(size<<6), 0xffc0, immediateMask, logicalImmediateCycleCalculator())
	}

	// NOT <ea>
	notMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement | eaMaskPreDecrement |
		eaMaskDisplacement | eaMaskIndex | eaMaskAbsoluteShort | eaMaskAbsoluteLong
	for size := uint16(0); size < 3; size++ {
		match := uint16(0x4600) | (size << 6)
		registerInstruction(notInstruction, match, 0xffc0, notMask, clrTstCycleCalculator())
	}
}

// registerLogicalInstruction behaves like registerInstruction but skips opcode slots that
// are already reserved by other instructions (for example, ABCD).
func registerLogicalInstruction(ins instruction, match, mask uint16, eaMask uint16, calc cycleCalculator) {
	for value := uint16(0); ; {
		index := match | value
		if (index & 0xf1f8) == 0xc100 { // Reserved for ABCD.
			value = ((value | mask) + 1) & ^mask
			if value == 0 {
				break
			}
			continue
		}

		if validEA(index, eaMask) {
			if opcodeTable[index] == nil {
				opcodeTable[index] = ins
				if calc != nil {
					opcodeCycleTable[index] = calc(index)
				}
			}
		}

		value = ((value | mask) + 1) & ^mask
		if value == 0 {
			break
		}
	}
}

func logicalCycleCalculator(size uint16, toEA bool) cycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		base := uint32(4)
		if toEA {
			base = 8
		}
		return base + eaAccessCycles(mode, reg, operandSizeFromOpmode(size))
	}
}

func logicalImmediateCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		size := operandSizeFromOpcode(opcode)
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		return 8 + eaAccessCycles(mode, reg, size)
	}
}

func logicalUpdateFlags(cpu *cpu, result uint32, size Size) {
	cpu.regs.SR &^= srNegative | srZero | srOverflow | srCarry
	if size.isZero(result) {
		cpu.regs.SR |= srZero
	} else if size.isNegative(result) {
		cpu.regs.SR |= srNegative
	}
}

func andInstruction(cpu *cpu) error {
	opmode := (cpu.regs.IR >> 6) & 0x7
	size := operandSizeFromOpmode(opmode)

	if opmode >= 4 {
		dst, err := cpu.ResolveSrcEA(size)
		if err != nil {
			return err
		}
		dstVal, err := dst.read()
		if err != nil {
			return err
		}

		src := *udx(cpu) & size.mask()
		result := (src & dstVal) & size.mask()
		if err := dst.write(result); err != nil {
			return err
		}
		logicalUpdateFlags(cpu, result, size)
		return nil
	}

	src, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	srcVal, err := src.read()
	if err != nil {
		return err
	}

	dst := udx(cpu)
	result := (srcVal & (*dst & size.mask())) & size.mask()
	*dst = (*dst & ^size.mask()) | result

	logicalUpdateFlags(cpu, result, size)
	return nil
}

func orInstruction(cpu *cpu) error {
	opmode := (cpu.regs.IR >> 6) & 0x7
	size := operandSizeFromOpmode(opmode)

	if opmode >= 4 {
		dst, err := cpu.ResolveSrcEA(size)
		if err != nil {
			return err
		}
		dstVal, err := dst.read()
		if err != nil {
			return err
		}

		src := *udx(cpu) & size.mask()
		result := (src | dstVal) & size.mask()
		if err := dst.write(result); err != nil {
			return err
		}
		logicalUpdateFlags(cpu, result, size)
		return nil
	}

	src, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	srcVal, err := src.read()
	if err != nil {
		return err
	}

	dst := udx(cpu)
	result := (srcVal | (*dst & size.mask())) & size.mask()
	*dst = (*dst & ^size.mask()) | result

	logicalUpdateFlags(cpu, result, size)
	return nil
}

func eorInstruction(cpu *cpu) error {
	size := operandSizeFromOpcode(cpu.regs.IR)

	dst, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	dstVal, err := dst.read()
	if err != nil {
		return err
	}

	src := *udx(cpu) & size.mask()
	result := (src ^ dstVal) & size.mask()
	if err := dst.write(result); err != nil {
		return err
	}

	logicalUpdateFlags(cpu, result, size)
	return nil
}

func logicalImmediate(cpu *cpu, op func(uint32, uint32) uint32) error {
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

	result := op(srcVal, dstVal) & size.mask()
	if err := dst.write(result); err != nil {
		return err
	}

	logicalUpdateFlags(cpu, result, size)
	return nil
}

func oriImmediate(cpu *cpu) error {
	return logicalImmediate(cpu, func(a, b uint32) uint32 { return a | b })
}
func andiImmediate(cpu *cpu) error {
	return logicalImmediate(cpu, func(a, b uint32) uint32 { return a & b })
}
func eoriImmediate(cpu *cpu) error {
	return logicalImmediate(cpu, func(a, b uint32) uint32 { return a ^ b })
}

func notInstruction(cpu *cpu) error {
	size := operandSizeFromOpcode(cpu.regs.IR)

	dst, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	value, err := dst.read()
	if err != nil {
		return err
	}

	result := (^value) & size.mask()
	if err := dst.write(result); err != nil {
		return err
	}

	logicalUpdateFlags(cpu, result, size)
	return nil
}
