package m68kemu

func init() {
	addSubEAMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskImmediate |
		eaMaskPCDisplacement | eaMaskPCIndex
	addSubAlterableMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong

	// ADD <ea>,Dn
	for opmode := uint16(0); opmode <= 2; opmode++ {
		match := uint16(0xd000) | (opmode << 6)
		registerInstruction(add, match, 0xf1c0, addSubEAMask, addCycleCalculator(opmode, false))
	}

	// ADD Dn,<ea>
	for opmode := uint16(4); opmode <= 6; opmode++ {
		match := uint16(0xd000) | (opmode << 6)
		registerInstruction(add, match, 0xf1c0, addSubAlterableMask, addCycleCalculator(opmode, true))
	}

	// SUB <ea>,Dn
	for opmode := uint16(0); opmode <= 2; opmode++ {
		match := uint16(0x9000) | (opmode << 6)
		registerInstruction(sub, match, 0xf1c0, addSubEAMask, addCycleCalculator(opmode, false))
	}

	// SUB Dn,<ea>
	for opmode := uint16(4); opmode <= 6; opmode++ {
		match := uint16(0x9000) | (opmode << 6)
		registerInstruction(sub, match, 0xf1c0, addSubAlterableMask, addCycleCalculator(opmode, true))
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

	alterableMask := eaMaskDataRegister | eaMaskAddressRegister | eaMaskIndirect |
		eaMaskPostIncrement | eaMaskPreDecrement | eaMaskDisplacement |
		eaMaskIndex | eaMaskAbsoluteShort | eaMaskAbsoluteLong
	registerInstruction(addq, 0x5000, 0xf100, alterableMask, addqSubqCycleCalculator())
	registerInstruction(subq, 0x5100, 0xf100, alterableMask, addqSubqCycleCalculator())

	divMulMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskPCDisplacement | eaMaskPCIndex
	registerInstruction(divu, 0x80c0, 0xf1c0, divMulMask, constantCycles(140))
	registerInstruction(divs, 0x81c0, 0xf1c0, divMulMask, constantCycles(158))
	registerInstruction(mulu, 0xc0c0, 0xf1c0, divMulMask, constantCycles(70))
	registerInstruction(muls, 0xc1c0, 0xf1c0, divMulMask, constantCycles(70))
}

func add(cpu *cpu) error {
	opmode := (cpu.regs.IR >> 6) & 0x7
	size := operandSizeFromOpmode(opmode)

	if opmode >= 4 {
		// Destination comes from the lower six bits.
		dst, err := cpu.ResolveSrcEA(size)
		if err != nil {
			return err
		}
		dstVal, err := dst.read()
		if err != nil {
			return err
		}

		src := *udx(cpu) & size.mask()
		result, flags := addWithFlags(src, dstVal, size)
		if err := dst.write(result); err != nil {
			return err
		}
		cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
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
	result, flags := addWithFlags(srcVal, *dst&size.mask(), size)
	*dst = (*dst & ^size.mask()) | result

	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
	return nil
}

func sub(cpu *cpu) error {
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
		result, flags := subWithFlags(src, dstVal, size)
		if err := dst.write(result); err != nil {
			return err
		}
		cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
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
	result, flags := subWithFlags(srcVal, *dst&size.mask(), size)
	*dst = (*dst & ^size.mask()) | result

	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
	return nil
}

func addq(cpu *cpu) error {
	size := operandSizeFromOpcode(cpu.regs.IR)
	quick := uint32((cpu.regs.IR >> 9) & 0x7)
	if quick == 0 {
		quick = 8
	}

	mode := (cpu.regs.IR >> 3) & 0x7
	reg := cpu.regs.IR & 0x7

	if mode == 1 { // address register direct
		value := cpu.regs.A[reg]
		cpu.regs.A[reg] = value + quick
		return nil
	}

	dst, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	dstVal, err := dst.read()
	if err != nil {
		return err
	}

	result, flags := addWithFlags(quick, dstVal, size)
	if err := dst.write(result); err != nil {
		return err
	}
	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
	return nil
}

func subq(cpu *cpu) error {
	size := operandSizeFromOpcode(cpu.regs.IR)
	quick := uint32((cpu.regs.IR >> 9) & 0x7)
	if quick == 0 {
		quick = 8
	}

	mode := (cpu.regs.IR >> 3) & 0x7
	reg := cpu.regs.IR & 0x7

	if mode == 1 { // address register direct
		value := cpu.regs.A[reg]
		cpu.regs.A[reg] = value - quick
		return nil
	}

	dst, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	dstVal, err := dst.read()
	if err != nil {
		return err
	}

	result, flags := subWithFlags(quick, dstVal, size)
	if err := dst.write(result); err != nil {
		return err
	}
	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
	return nil
}

func addWithFlags(src, dst uint32, size Size) (uint32, uint16) {
	mask := size.mask()
	sign := size.signBit()
	src &= mask
	dst &= mask
	res := (src + dst) & mask

	var sr uint16
	if res == 0 {
		sr |= srZero
	}
	if res&sign != 0 {
		sr |= srNegative
	}
	if ((^(dst ^ src)) & (res ^ dst) & sign) != 0 {
		sr |= srOverflow
	}
	if (((src & dst) | ((src | dst) & ^res)) & sign) != 0 {
		sr |= srCarry | srExtend
	}
	return res, sr
}

func subWithFlags(src, dst uint32, size Size) (uint32, uint16) {
	mask := size.mask()
	sign := size.signBit()
	src &= mask
	dst &= mask
	res := (dst - src) & mask

	var sr uint16
	if res == 0 {
		sr |= srZero
	}
	if res&sign != 0 {
		sr |= srNegative
	}
	if ((dst ^ src) & (dst ^ res) & sign) != 0 {
		sr |= srOverflow
	}
	if src > dst {
		sr |= srCarry | srExtend
	}
	return res, sr
}

func addCycleCalculator(opmode uint16, toEA bool) cycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		if toEA {
			return 8 + eaAccessCycles(mode, reg, operandSizeFromOpmode(opmode))
		}
		return 4 + eaAccessCycles(mode, reg, operandSizeFromOpmode(opmode))
	}
}

func addqSubqCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		size := Size((opcode >> 6) & 0x3)

		if mode == 0 {
			return 4 + eaAccessCycles(mode, reg, size)
		}
		if mode == 1 {
			return 8 + eaAccessCycles(mode, reg, size)
		}
		return 8 + eaAccessCycles(mode, reg, size)
	}
}

func divu(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Word)
	if err != nil {
		return err
	}
	divisor, err := src.read()
	if err != nil {
		return err
	}
	if divisor == 0 {
		return cpu.exception(XDivByZero)
	}

	dividend := uint32(*dx(cpu))
	quotient := dividend / divisor
	if quotient > 0xffff {
		cpu.regs.SR = (cpu.regs.SR & ^uint16(srCarry)) | srOverflow
		return nil
	}
	remainder := dividend % divisor
	result := (remainder << 16) | (quotient & 0xffff)
	*dx(cpu) = int32(result)

	var flags uint16
	if quotient == 0 {
		flags |= srZero
	}
	if quotient&0x8000 != 0 {
		flags |= srNegative
	}
	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
	return nil
}

func divs(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Word)
	if err != nil {
		return err
	}
	divisorRaw, err := src.read()
	if err != nil {
		return err
	}
	divisor := int32(int16(divisorRaw))
	if divisor == 0 {
		return cpu.exception(XDivByZero)
	}

	dividend := *dx(cpu)
	quotient := dividend / divisor
	if quotient > 0x7fff || quotient < -0x8000 {
		cpu.regs.SR = (cpu.regs.SR & ^uint16(srCarry)) | srOverflow
		return nil
	}
	remainder := dividend % divisor
	result := (uint32(uint16(remainder)) << 16) | (uint32(uint16(quotient)) & 0xffff)
	*dx(cpu) = int32(result)

	var flags uint16
	if quotient == 0 {
		flags |= srZero
	}
	if quotient&0x8000 != 0 {
		flags |= srNegative
	}
	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
	return nil
}

func mulu(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Word)
	if err != nil {
		return err
	}
	op, err := src.read()
	if err != nil {
		return err
	}

	result := uint32(uint16(op)) * uint32(uint16(*dx(cpu)))
	*dx(cpu) = int32(result)

	var flags uint16
	if result == 0 {
		flags |= srZero
	}
	if uint32(result)&0x80000000 != 0 {
		flags |= srNegative
	}
	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
	return nil
}

func muls(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Word)
	if err != nil {
		return err
	}
	opRaw, err := src.read()
	if err != nil {
		return err
	}

	result := int32(int16(opRaw)) * int32(int16(*dx(cpu)))
	*dx(cpu) = result

	var flags uint16
	if result == 0 {
		flags |= srZero
	}
	if uint32(result)&0x80000000 != 0 {
		flags |= srNegative
	}
	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
	return nil
}

func adda(cpu *cpu) error {
	size := Word
	if (cpu.regs.IR>>6)&0x7 == 7 {
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

	*ax(cpu) += value
	return nil
}

func suba(cpu *cpu) error {
	size := Word
	if (cpu.regs.IR>>6)&0x7 == 7 {
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

	*ax(cpu) -= value
	return nil
}

func addaSubaCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		size := operandSizeFromOpcode(opcode)
		return 8 + eaAccessCycles(mode, reg, size)
	}
}
