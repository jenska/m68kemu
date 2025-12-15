package m68kemu

func init() {
	alterableNoAddr := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong

	for size := uint16(0); size < 3; size++ {
		match := uint16(0x4400) | (size << 6)
		registerInstruction(negInstruction, match, 0xffc0, alterableNoAddr, clrTstCycleCalculator())
	}

	registerInstruction(swapInstruction, 0x4840, 0xfff8, 0, constantCycles(4))
	registerInstruction(extInstruction, 0x4880, 0xfff8, 0, constantCycles(4))
	registerInstruction(extInstruction, 0x48c0, 0xfff8, 0, constantCycles(4))
	registerInstruction(tasInstruction, 0x4ac0, 0xffc0, eaMaskDataRegister|eaMaskIndirect|eaMaskPostIncrement|
		eaMaskPreDecrement|eaMaskDisplacement|eaMaskIndex|eaMaskAbsoluteShort|eaMaskAbsoluteLong, clrTstCycleCalculator())

	registerExgInstruction(0xc140, constantCycles(6))
	registerExgInstruction(0xc148, constantCycles(6))
	registerExgInstruction(0xc188, constantCycles(8))

	// ILLEGAL
	registerInstruction(illegalInstruction, 0x4afc, 0xffff, 0, constantCycles(4))
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

	cpu.regs.SR = (cpu.regs.SR &^ (srNegative | srZero | srOverflow | srCarry | srExtend)) | flags
	return nil
}

func swapInstruction(cpu *cpu) error {
	reg := dy(cpu)
	value := *reg
	result := (value << 16) | ((value >> 16) & 0xffff)
	*reg = result

	updateNzClearVc(cpu, result, Long)
	return nil
}

func extInstruction(cpu *cpu) error {
	opcode := cpu.regs.IR
	sizeBits := (opcode >> 6) & 0x3
	dst := dy(cpu)

	var (
		result uint32
		size   Size
	)

	switch sizeBits {
	case 0x2: // EXT.W: byte to word
		size = Word
		result = uint32(int16(int8(*dst)))
	case 0x3: // EXT.L: word to long
		size = Long
		result = uint32(int32(int16(*dst)))
	default:
		return nil
	}

	mask := size.mask()
	*dst = (*dst & ^mask) | (result & mask)

	updateNzClearVc(cpu, result, size)
	return nil
}

func exgInstruction(cpu *cpu) error {
	opcode := cpu.regs.IR
	rx := y(cpu.regs.IR)
	ry := x(cpu.regs.IR)

	switch opcode & 0x01c0 {
	case 0x0140: // Dx,Dy or Ax,Ay
		if opcode&0x0008 == 0 { // Dx,Dy
			dxReg := &cpu.regs.D[ry]
			dyReg := &cpu.regs.D[rx]
			*dxReg, *dyReg = *dyReg, *dxReg
			return nil
		}

		axReg := &cpu.regs.A[ry]
		ayReg := &cpu.regs.A[rx]
		*axReg, *ayReg = *ayReg, *axReg
		return nil

	case 0x0180: // Dx,Ay
		dxReg := &cpu.regs.D[ry]
		ayReg := &cpu.regs.A[rx]
		temp := uint32(*dxReg)
		*dxReg = int32(*ayReg)
		*ayReg = temp
		return nil
	}

	return nil
}

func tasInstruction(cpu *cpu) error {
	dst, err := cpu.ResolveSrcEA(Byte)
	if err != nil {
		return err
	}

	value, err := dst.read()
	if err != nil {
		return err
	}

	updateNzClearVc(cpu, value, Byte)

	return dst.write(value | 0x80)
}

func updateNzClearVc(cpu *cpu, result uint32, size Size) {
	cpu.regs.SR &^= srNegative | srZero | srOverflow | srCarry
	if size.isZero(result) {
		cpu.regs.SR |= srZero
	} else if size.isNegative(result) {
		cpu.regs.SR |= srNegative
	}
}

func registerExgInstruction(match uint16, calc cycleCalculator) {
	for rx := uint16(0); rx < 8; rx++ {
		for ry := uint16(0); ry < 8; ry++ {
			opcode := match | (ry << 9) | rx
			opcodeTable[opcode] = exgInstruction
			if calc != nil {
				opcodeCycleTable[opcode] = calc(opcode)
			}
		}
	}
}

func illegalInstruction(cpu *cpu) error {
	return cpu.exception(XIllegal)
}
