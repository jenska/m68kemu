package m68kemu

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
	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
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
	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
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
	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
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

	srcAddr := *ay(cpu) - uint32(size)
	dstAddr := *ax(cpu) - uint32(size)
	*ay(cpu) = srcAddr
	*ax(cpu) = dstAddr

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
