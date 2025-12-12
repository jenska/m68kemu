package emu

func init() {
	registerAdd()
	registerSubq()
}

func registerAdd() {
	addEAMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskImmediate |
		eaMaskPCDisplacement | eaMaskPCIndex

	addAlterableMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong

	// ADD <ea>,Dn
	for opmode := uint16(0); opmode <= 2; opmode++ {
		match := uint16(0xd000) | (opmode << 6)
		RegisterInstruction(add, match, 0xf1c0, addEAMask, addCycleCalculator(opmode, false))
	}

	// ADD Dn,<ea>
	for opmode := uint16(4); opmode <= 6; opmode++ {
		match := uint16(0xd000) | (opmode << 6)
		RegisterInstruction(add, match, 0xf1c0, addAlterableMask, addCycleCalculator(opmode, true))
	}
}

func registerSubq() {
	// SUBQ on 68000 supports alterable destinations; we keep it to data registers
	// for now to cover loop counters and keep flag behavior simple.
	RegisterInstruction(subq, 0x5100, 0xf100, eaMaskDataRegister, subqCycleCalculator())
}

func operandSizeFromOpmode(opmode uint16) Size {
	switch opmode {
	case 0, 4:
		return Byte
	case 1, 5:
		return Word
	case 2, 6:
		return Long
	default:
		return Byte
	}
}

func add(cpu *CPU) error {
	opmode := (cpu.regs.IR >> 6) & 0x7
	size := operandSizeFromOpmode(opmode)
	toEA := opmode >= 4

	if toEA {
		// Destination comes from the lower six bits.
		dst, err := cpu.ResolveSrcEA(size)
		if err != nil {
			return err
		}
		dstVal, err := dst.read()
		if err != nil {
			return err
		}

		src := *udx(cpu) & maskForSize(size)
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
	result, flags := addWithFlags(srcVal, *dst&maskForSize(size), size)
	*dst = (*dst & ^maskForSize(size)) | result

	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
	return nil
}

func subq(cpu *CPU) error {
	size := (cpu.regs.IR >> 6) & 0x3
	destReg := cpu.regs.IR & 0x7
	decrement := uint32(cpu.regs.IR>>9) & 0x7
	if decrement == 0 {
		decrement = 8
	}

	value := uint32(cpu.regs.D[destReg])
	mask := maskForSize(Size(size))
	result, flags := subWithFlags(decrement, value&mask, Size(size))
	cpu.regs.D[destReg] = int32((value & ^mask) | result)
	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
	return nil
}

func maskForSize(size Size) uint32 {
	switch size {
	case Byte:
		return 0xff
	case Word:
		return 0xffff
	case Long:
		return 0xffffffff
	}
	return 0xffffffff
}

func signBit(size Size) uint32 {
	switch size {
	case Byte:
		return 0x80
	case Word:
		return 0x8000
	default:
		return 0x80000000
	}
}

func addWithFlags(src, dst uint32, size Size) (uint32, uint16) {
	mask := maskForSize(size)
	sign := signBit(size)
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
	mask := maskForSize(size)
	sign := signBit(size)
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

func addCycleCalculator(opmode uint16, toEA bool) CycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		if toEA {
			return 8 + eaAccessCycles(mode, reg, operandSizeFromOpmode(opmode))
		}
		return 4 + eaAccessCycles(mode, reg, operandSizeFromOpmode(opmode))
	}
}

func subqCycleCalculator() CycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		size := Size((opcode >> 6) & 0x3)
		base := uint32(4)
		if mode != 0 {
			base = 8
		}
		return base + eaAccessCycles(mode, reg, size)
	}
}
