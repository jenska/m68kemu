package m68kemu

func init() {
	addSubEAMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskImmediate |
		eaMaskPCDisplacement | eaMaskPCIndex
	addSubWordLongEAMask := addSubEAMask | eaMaskAddressRegister
	addSubAlterableMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong

	// ADD.B <ea>,Dn
	registerInstruction(add, 0xd000, 0xf1c0, addSubEAMask, addCycleCalculator(0, false))

	// ADD.W/L <ea>,Dn
	for opmode := uint16(1); opmode <= 2; opmode++ {
		match := uint16(0xd000) | (opmode << 6)
		registerInstruction(add, match, 0xf1c0, addSubWordLongEAMask, addCycleCalculator(opmode, false))
	}

	// ADD Dn,<ea>
	for opmode := uint16(4); opmode <= 6; opmode++ {
		match := uint16(0xd000) | (opmode << 6)
		registerInstruction(add, match, 0xf1c0, addSubAlterableMask, addCycleCalculator(opmode, true))
	}

	// SUB.B <ea>,Dn
	registerInstruction(sub, 0x9000, 0xf1c0, addSubEAMask, addCycleCalculator(0, false))

	// SUB.W/L <ea>,Dn
	for opmode := uint16(1); opmode <= 2; opmode++ {
		match := uint16(0x9000) | (opmode << 6)
		registerInstruction(sub, match, 0xf1c0, addSubWordLongEAMask, addCycleCalculator(opmode, false))
	}

	// SUB Dn,<ea>
	for opmode := uint16(4); opmode <= 6; opmode++ {
		match := uint16(0x9000) | (opmode << 6)
		registerInstruction(sub, match, 0xf1c0, addSubAlterableMask, addCycleCalculator(opmode, true))
	}

	addaSubaMask := eaMaskDataRegister | eaMaskAddressRegister | eaMaskIndirect | eaMaskPostIncrement |
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

	for size := uint16(0); size < 3; size++ {
		registerInstruction(addq, 0x5000|(size<<6), 0xf1c0, alterableMask, addqSubqCycleCalculator())
		registerInstruction(subq, 0x5100|(size<<6), 0xf1c0, alterableMask, addqSubqCycleCalculator())
	}

	divMulMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskPCDisplacement | eaMaskPCIndex |
		eaMaskImmediate
	registerInstruction(divu, 0x80c0, 0xf1c0, divMulMask, constantCycles(140))
	registerInstruction(divs, 0x81c0, 0xf1c0, divMulMask, constantCycles(158))
	registerInstruction(mulu, 0xc0c0, 0xf1c0, divMulMask, constantCycles(70))
	registerInstruction(muls, 0xc1c0, 0xf1c0, divMulMask, constantCycles(70))

	alterableNoAddr := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong

	for size := uint16(0); size < 3; size++ {
		registerInstruction(addi, uint16(0x0600)|(size<<6), 0xffc0, alterableNoAddr, arithmeticImmediateCycleCalculator())
		registerInstruction(subi, uint16(0x0400)|(size<<6), 0xffc0, alterableNoAddr, arithmeticImmediateCycleCalculator())
	}

	for size := uint16(0); size < 3; size++ {
		match := uint16(0x4400) | (size << 6)
		registerInstruction(negInstruction, match, 0xffc0, alterableNoAddr, clrTstCycleCalculator())
	}
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
		replaceStatusFlags(cpu, statusMaskNZVCX, flags)
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

	replaceStatusFlags(cpu, statusMaskNZVCX, flags)
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
		replaceStatusFlags(cpu, statusMaskNZVCX, flags)
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

	replaceStatusFlags(cpu, statusMaskNZVCX, flags)
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
	replaceStatusFlags(cpu, statusMaskNZVCX, flags)
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
	replaceStatusFlags(cpu, statusMaskNZVCX, flags)
	return nil
}

func addi(cpu *cpu) error {
	return arithmeticImmediate(cpu, addWithFlags)
}

func subi(cpu *cpu) error {
	return arithmeticImmediate(cpu, subWithFlags)
}

func arithmeticImmediate(cpu *cpu, op func(src, dst uint32, size Size) (uint32, uint16)) error {
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

	result, flags := op(srcVal, dstVal, size)
	if err := dst.write(result); err != nil {
		return err
	}
	replaceStatusFlags(cpu, statusMaskNZVCX, flags)
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

func divExceptionCycles(opcode uint16) uint32 {
	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7
	return exceptionCyclesDivByZero + eaAccessCycles(mode, reg, Word)
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

func arithmeticImmediateCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		size := operandSizeFromOpcode(opcode)
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
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
		return cpu.exceptionWithCycles(XDivByZero, divExceptionCycles(cpu.regs.IR))
	}

	dividend := uint32(*dx(cpu))
	quotient := dividend / divisor
	if quotient > 0xffff {
		replaceStatusFlags(cpu, statusMaskNZVC, srOverflow)
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
	replaceStatusFlags(cpu, statusMaskNZVC, flags)
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
		return cpu.exceptionWithCycles(XDivByZero, divExceptionCycles(cpu.regs.IR))
	}

	dividend := *dx(cpu)
	quotient := dividend / divisor
	if quotient > 0x7fff || quotient < -0x8000 {
		replaceStatusFlags(cpu, statusMaskNZVC, srOverflow)
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
	replaceStatusFlags(cpu, statusMaskNZVC, flags)
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
	replaceStatusFlags(cpu, statusMaskNZVC, flags)
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
	replaceStatusFlags(cpu, statusMaskNZVC, flags)
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

func negInstruction(cpu *cpu) error {
	size := operandSizeFromOpcode(cpu.regs.IR)

	dst, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	value, err := dst.read()
	if err != nil {
		return err
	}

	result, flags := subWithFlags(value, 0, size)
	if err := dst.write(result); err != nil {
		return err
	}

	replaceStatusFlags(cpu, statusMaskNZVCX, flags)
	return nil
}

func init() {
	cmpEAMask := eaMaskDataRegister | eaMaskAddressRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskImmediate |
		eaMaskPCDisplacement | eaMaskPCIndex

	for opmode := uint16(0); opmode <= 2; opmode++ {
		match := uint16(0xb000) | (opmode << 6)
		registerInstruction(cmpInstruction, match, 0xf1c0, cmpEAMask, addCycleCalculator(opmode, false))
	}
	for _, opmode := range []uint16{3, 7} {
		match := uint16(0xb000) | (opmode << 6)
		registerInstruction(cmpa, match, 0xf1c0, cmpEAMask, cmpaCycleCalculator())
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
	replaceStatusFlags(cpu, statusMaskNZVC, flags)
	return nil
}

func cmpa(cpu *cpu) error {
	size := cmpaOperandSize(cpu.regs.IR)

	src, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	srcVal, err := src.read()
	if err != nil {
		return err
	}
	if size == Word {
		srcVal = uint32(int32(int16(srcVal)))
	}

	dst := *ax(cpu)
	_, flags := subWithFlags(srcVal, dst, Long)
	replaceStatusFlags(cpu, statusMaskNZVC, flags)
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
	replaceStatusFlags(cpu, statusMaskNZVC, flags)
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

	cpu.regs.A[srcReg] += addressRegisterStep(srcReg, size)
	cpu.regs.A[dstReg] += addressRegisterStep(dstReg, size)

	_, flags := subWithFlags(srcVal, dstVal, size)
	replaceStatusFlags(cpu, statusMaskNZVC, flags)
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

func cmpaOperandSize(opcode uint16) Size {
	if (opcode>>6)&0x7 == 7 {
		return Long
	}
	return Word
}

func cmpaCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		size := cmpaOperandSize(opcode)
		return 8 + eaAccessCycles(mode, reg, size)
	}
}

func init() {
	// ADDX and SUBX operate on either data registers or pre-decrement
	// memory operands depending on bit 3 of the opcode.
	registerExtendInstruction(addx, 0xd100, addxSubxCycleCalculator)
	registerExtendInstruction(subx, 0x9100, addxSubxCycleCalculator)

	for size := uint16(0); size < 3; size++ {
		match := uint16(0x4000) | (size << 6)
		registerInstruction(negx, match, 0xffc0, eaMaskDataRegister|eaMaskIndirect|
			eaMaskPostIncrement|eaMaskPreDecrement|eaMaskDisplacement|
			eaMaskIndex|eaMaskAbsoluteShort|eaMaskAbsoluteLong, clrTstCycleCalculator())
	}
}

func addx(cpu *cpu) error {
	size := operandSizeFromOpcode(cpu.regs.IR)

	src, dst, err := extendOperands(cpu, size)
	if err != nil {
		return err
	}

	prevZero := cpu.regs.SR&srZero != 0
	result, flags := addWithExtend(src.value, dst.value, size, cpu.regs.SR&srExtend != 0)

	if err := dst.write(result); err != nil {
		return err
	}

	flags = updateExtendZeroFlag(flags, size, result, prevZero)
	replaceStatusFlags(cpu, statusMaskNZVCX, flags)
	return nil
}

func subx(cpu *cpu) error {
	size := operandSizeFromOpcode(cpu.regs.IR)

	src, dst, err := extendOperands(cpu, size)
	if err != nil {
		return err
	}

	prevZero := cpu.regs.SR&srZero != 0
	result, flags := subWithExtend(src.value, dst.value, size, cpu.regs.SR&srExtend != 0)

	if err := dst.write(result); err != nil {
		return err
	}

	flags = updateExtendZeroFlag(flags, size, result, prevZero)
	replaceStatusFlags(cpu, statusMaskNZVCX, flags)
	return nil
}

func negx(cpu *cpu) error {
	size := operandSizeFromOpcode(cpu.regs.IR)

	operand, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	value, err := operand.read()
	if err != nil {
		return err
	}

	prevZero := cpu.regs.SR&srZero != 0
	result, flags := subWithExtend(value, 0, size, cpu.regs.SR&srExtend != 0)

	if err := operand.write(result); err != nil {
		return err
	}

	flags = updateExtendZeroFlag(flags, size, result, prevZero)
	replaceStatusFlags(cpu, statusMaskNZVCX, flags)
	return nil
}

type extendOperand struct {
	value uint32
	write func(uint32) error
}

func extendOperands(cpu *cpu, size Size) (extendOperand, extendOperand, error) {
	if (cpu.regs.IR>>3)&0x1 == 0 {
		src := dy(cpu)
		dst := udx(cpu)
		return extendOperand{value: *src}, extendOperand{
			value: *dst,
			write: func(v uint32) error {
				mask := size.mask()
				*dst = (*dst & ^mask) | (v & mask)
				return nil
			},
		}, nil
	}

	srcReg := y(cpu.regs.IR)
	dstReg := x(cpu.regs.IR)
	srcAddr := cpu.regs.A[srcReg] - addressRegisterStep(srcReg, size)
	dstAddr := cpu.regs.A[dstReg] - addressRegisterStep(dstReg, size)
	cpu.regs.A[srcReg] = srcAddr
	cpu.regs.A[dstReg] = dstAddr

	srcVal, err := cpu.read(size, srcAddr)
	if err != nil {
		return extendOperand{}, extendOperand{}, err
	}
	dstVal, err := cpu.read(size, dstAddr)
	if err != nil {
		return extendOperand{}, extendOperand{}, err
	}

	return extendOperand{value: srcVal}, extendOperand{
		value: dstVal,
		write: func(v uint32) error { return cpu.write(size, dstAddr, v) },
	}, nil
}

func registerExtendInstruction(op instruction, base uint16, calc cycleCalculator) {
	for size := uint16(0); size < 3; size++ {
		for dst := uint16(0); dst < 8; dst++ {
			for src := uint16(0); src < 8; src++ {
				for mode := uint16(0); mode <= 1; mode++ {
					opcode := base | (dst << 9) | (size << 6) | (mode << 3) | src
					opcodeTable[opcode] = op
					if calc != nil {
						opcodeCycleTable[opcode] = calc(opcode)
					}
				}
			}
		}
	}
}

func addWithExtend(src, dst uint32, size Size, extend bool) (uint32, uint16) {
	operand2 := (src & size.mask()) + boolToUint32(extend)
	result := (dst & size.mask()) + operand2
	masked := result & size.mask()

	var sr uint16
	if masked == 0 {
		sr |= srZero
	}
	if masked&size.signBit() != 0 {
		sr |= srNegative
	}

	if ((^(dst ^ operand2)) & (masked ^ dst) & size.signBit()) != 0 {
		sr |= srOverflow
	}
	if result&^size.mask() != 0 {
		sr |= srCarry | srExtend
	}
	return masked, sr
}

func subWithExtend(src, dst uint32, size Size, extend bool) (uint32, uint16) {
	operand2 := (src & size.mask()) + boolToUint32(extend)
	result := (dst & size.mask()) - operand2
	masked := result & size.mask()

	var sr uint16
	if masked == 0 {
		sr |= srZero
	}
	if masked&size.signBit() != 0 {
		sr |= srNegative
	}

	if ((dst ^ operand2) & (dst ^ masked) & size.signBit()) != 0 {
		sr |= srOverflow
	}
	if (dst & size.mask()) < operand2 {
		sr |= srCarry | srExtend
	}
	return masked, sr
}

func updateExtendZeroFlag(flags uint16, size Size, result uint32, prevZero bool) uint16 {
	flags &^= srZero
	if size.isZero(result) && prevZero {
		flags |= srZero
	}
	return flags
}

func boolToUint32(v bool) uint32 {
	if v {
		return 1
	}
	return 0
}

func addxSubxCycleCalculator(opcode uint16) uint32 {
	if (opcode>>3)&0x1 == 0 {
		return 4
	}
	return 18
}

func init() {
	registerInstruction(abcd, 0xc100, 0xf1f8, 0, abcdCycleCalculator)
	registerInstruction(abcd, 0xc108, 0xf1f8, 0, abcdCycleCalculator)
	registerInstruction(sbcd, 0x8100, 0xf1f8, 0, sbcdCycleCalculator)
	registerInstruction(sbcd, 0x8108, 0xf1f8, 0, sbcdCycleCalculator)
	registerInstruction(nbcd, 0x4800, 0xfff8, 0, nbcdCycleCalculator)
	registerInstruction(nbcd, 0x4820, 0xfff8, 0, nbcdCycleCalculator)
}

func abcd(cpu *cpu) error {
	src, dst, err := bcdOperands(cpu)
	if err != nil {
		return err
	}

	prevZero := cpu.regs.SR&srZero != 0
	result, carry := bcdAdd(src.value, dst.value, cpu.regs.SR&srExtend != 0)
	if err := dst.write(result); err != nil {
		return err
	}

	updateBCDFlags(cpu, result, carry, prevZero, true)

	return nil
}

func sbcd(cpu *cpu) error {
	src, dst, err := bcdOperands(cpu)
	if err != nil {
		return err
	}

	prevZero := cpu.regs.SR&srZero != 0
	result, borrow := bcdSub(src.value, dst.value, cpu.regs.SR&srExtend != 0)
	if err := dst.write(result); err != nil {
		return err
	}

	updateBCDFlags(cpu, result, borrow, prevZero, true)

	return nil
}

func nbcd(cpu *cpu) error {
	operand, err := bcdDestination(cpu)
	if err != nil {
		return err
	}

	prevZero := cpu.regs.SR&srZero != 0
	result, borrow := bcdSub(operand.value, 0, cpu.regs.SR&srExtend != 0)
	if err := operand.write(result); err != nil {
		return err
	}

	updateBCDFlags(cpu, result, borrow, prevZero, false)

	return nil
}

func updateBCDFlags(cpu *cpu, result byte, carry bool, prevZero bool, propagateZero bool) {
	cpu.regs.SR &^= srCarry | srOverflow | srNegative
	if carry {
		cpu.regs.SR |= srCarry | srExtend
	} else {
		cpu.regs.SR &^= srExtend
	}

	zero := result == 0
	if propagateZero {
		zero = zero && prevZero
	}
	if zero {
		cpu.regs.SR |= srZero
	} else {
		cpu.regs.SR &^= srZero
	}
}

func bcdAdd(src, dst byte, extend bool) (byte, bool) {
	// Calculate binary sum of low nibbles to detect half-carry
	lo := (int(src) & 0x0F) + (int(dst) & 0x0F) + int(boolToUint32(extend))
	sum := int(src) + int(dst) + int(boolToUint32(extend))

	// Adjust if low nibble overflowed (>9) or half-carry occurred
	if lo > 9 {
		sum += 0x06
	}

	carry := sum > 0x99
	if carry {
		sum += 0x60
	}

	return byte(sum), carry
}

func bcdSub(src, dst byte, extend bool) (byte, bool) {
	e := 0
	if extend {
		e = 1
	}

	low := (int(dst) & 0x0f) - (int(src) & 0x0f) - e
	diff := int(dst) - int(src) - e

	if low < 0 {
		diff -= 0x06
	}

	borrow := diff < 0
	if borrow {
		diff -= 0x60
	}

	return byte(diff), borrow
}

type bcdOperand struct {
	value byte
	write func(byte) error
}

type bcdSourceDest struct {
	value byte
	write func(byte) error
}

func bcdOperands(cpu *cpu) (bcdOperand, bcdOperand, error) {
	if (cpu.regs.IR>>3)&0x1 == 0 {
		srcReg := dy(cpu)
		dstReg := udx(cpu)
		return bcdOperand{value: byte(*srcReg & 0xff)}, bcdOperand{
			value: byte(*dstReg & 0xff),
			write: func(v byte) error {
				*dstReg = (*dstReg & 0xffffff00) | uint32(v)
				return nil
			},
		}, nil
	}

	sourceReg := y(cpu.regs.IR)
	destReg := x(cpu.regs.IR)
	sourceAddr := cpu.regs.A[sourceReg] - addressRegisterStep(sourceReg, Byte)
	destAddr := cpu.regs.A[destReg] - addressRegisterStep(destReg, Byte)
	cpu.regs.A[sourceReg] = sourceAddr
	cpu.regs.A[destReg] = destAddr

	srcValue, err := cpu.read(Byte, sourceAddr)
	if err != nil {
		return bcdOperand{}, bcdOperand{}, err
	}
	dstValue, err := cpu.read(Byte, destAddr)
	if err != nil {
		return bcdOperand{}, bcdOperand{}, err
	}

	return bcdOperand{value: byte(srcValue)}, bcdOperand{
		value: byte(dstValue),
		write: func(v byte) error {
			return cpu.write(Byte, destAddr, uint32(v))
		},
	}, nil
}

func bcdDestination(cpu *cpu) (bcdSourceDest, error) {
	mode := (cpu.regs.IR >> 3) & 0x7
	reg := y(cpu.regs.IR)

	if mode == 0 {
		dstReg := dy(cpu)
		return bcdSourceDest{
			value: byte(*dstReg & 0xff),
			write: func(v byte) error {
				*dstReg = (*dstReg & 0xffffff00) | uint32(v)
				return nil
			},
		}, nil
	}

	if mode != 4 {
		return bcdSourceDest{}, cpu.exception(XIllegal)
	}

	addr := cpu.regs.A[reg] - addressRegisterStep(reg, Byte)
	cpu.regs.A[reg] = addr

	value, err := cpu.read(Byte, addr)
	if err != nil {
		return bcdSourceDest{}, err
	}

	return bcdSourceDest{
		value: byte(value),
		write: func(v byte) error { return cpu.write(Byte, addr, uint32(v)) },
	}, nil
}

func abcdCycleCalculator(opcode uint16) uint32 {
	if (opcode>>3)&0x1 == 0 {
		return 6
	}
	return 18
}

func sbcdCycleCalculator(opcode uint16) uint32 {
	if (opcode>>3)&0x1 == 0 {
		return 6
	}
	return 18
}

func nbcdCycleCalculator(opcode uint16) uint32 {
	if (opcode>>3)&0x1 == 0 {
		return 6
	}
	return 8
}

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

	updateNZClearVC(cpu, 0, size)
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

	updateNZClearVC(cpu, value, size)
	return nil
}
