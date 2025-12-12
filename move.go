package m68kemu

func init() {
	registerMove(moveb, 0x1000, moveCycleCalculator(Byte))
	registerMove(movew, 0x3000, moveCycleCalculator(Word))
	registerMove(movel, 0x2000, moveCycleCalculator(Long))
	registerMoveA(0x3000, moveaw, moveAddressCycleCalculator(Word))
	registerMoveA(0x2000, moveal, moveAddressCycleCalculator(Long))
	RegisterInstruction(moveq, 0x7000, 0xf100, 0, constantCycles(4))
}

const moveSourceEAMask = eaMaskDataRegister |
	eaMaskAddressRegister |
	eaMaskIndirect |
	eaMaskPostIncrement |
	eaMaskPreDecrement |
	eaMaskDisplacement |
	eaMaskIndex |
	eaMaskAbsoluteShort |
	eaMaskAbsoluteLong |
	eaMaskImmediate |
	eaMaskPCDisplacement |
	eaMaskPCIndex

func moveq(cpu *cpu) error {
	value := int32(int8(cpu.regs.IR))
	*dx(cpu) = value

	cpu.regs.SR &^= srNegative | srZero | srOverflow | srCarry
	if value == 0 {
		cpu.regs.SR |= srZero
	} else if value < 0 {
		cpu.regs.SR |= srNegative
	}

	return nil
}

func registerMoveA(base uint16, handler Instruction, calc CycleCalculator) {
	const dstMode = uint16(1)
	for dstReg := uint16(0); dstReg < 8; dstReg++ {
		match := base | (dstReg << 9) | (dstMode << 6)
		RegisterInstruction(handler, match, 0xffc0, moveSourceEAMask, calc)
	}
}

func registerMove(ins Instruction, base uint16, calc CycleCalculator) {
	for dstMode := uint16(0); dstMode < 8; dstMode++ {
		// Address register destinations are handled by MOVEA.
		if dstMode == 1 {
			continue
		}
		for dstReg := uint16(0); dstReg < 8; dstReg++ {
			// Destination must be alterable: exclude PC relative and immediate forms.
			if dstMode == 7 && (dstReg == 2 || dstReg == 3 || dstReg == 4) {
				continue
			}
			match := base | (dstReg << 9) | (dstMode << 6)
			RegisterInstruction(ins, match, 0xffc0, moveSourceEAMask, calc)
		}
	}
}

func moveb(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Byte)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	dst, err := cpu.ResolveDstEA(Byte)
	if err != nil {
		return err
	}

	if err := dst.write(value); err != nil {
		return err
	}

	cpu.regs.SR &^= srNegative | srZero | srOverflow | srCarry
	signed := int32(int8(value))
	if signed == 0 {
		cpu.regs.SR |= srZero
	} else if signed < 0 {
		cpu.regs.SR |= srNegative
	}

	return nil
}

func movew(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Word)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	dst, err := cpu.ResolveDstEA(Word)
	if err != nil {
		return err
	}

	if err := dst.write(value); err != nil {
		return err
	}

	cpu.regs.SR &^= srNegative | srZero | srOverflow | srCarry
	signed := int32(int16(value))
	if signed == 0 {
		cpu.regs.SR |= srZero
	} else if signed < 0 {
		cpu.regs.SR |= srNegative
	}

	return nil
}

func movel(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	dst, err := cpu.ResolveDstEA(Long)
	if err != nil {
		return err
	}

	if err := dst.write(value); err != nil {
		return err
	}

	cpu.regs.SR &^= srNegative | srZero | srOverflow | srCarry
	signed := int32(value)
	if signed == 0 {
		cpu.regs.SR |= srZero
	} else if signed < 0 {
		cpu.regs.SR |= srNegative
	}

	return nil
}

func moveaw(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Word)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	// Destination is always an address register encoded in bits 11..9.
	*ax(cpu) = uint32(int32(int16(value)))
	return nil
}

func moveal(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	// Destination is always an address register encoded in bits 11..9.
	*ax(cpu) = value
	return nil
}

func moveCycles(ir uint16, size Size) uint32 {
	srcMode := (ir >> 3) & 0x7
	srcReg := ir & 0x7
	dstMode := (ir >> 6) & 0x7
	dstReg := (ir >> 9) & 0x7

	return 4 + eaAccessCycles(srcMode, srcReg, size) + eaAccessCycles(dstMode, dstReg, size)
}

func moveAddressCycles(ir uint16, size Size) uint32 {
	srcMode := (ir >> 3) & 0x7
	srcReg := ir & 0x7
	return 4 + eaAccessCycles(srcMode, srcReg, size)
}

func moveCycleCalculator(size Size) CycleCalculator {
	return func(opcode uint16) uint32 {
		return moveCycles(opcode, size)
	}
}

func moveAddressCycleCalculator(size Size) CycleCalculator {
	return func(opcode uint16) uint32 {
		return moveAddressCycles(opcode, size)
	}
}
