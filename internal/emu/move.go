package emu

func init() {
	// TODO replace with RegisterInstruction
	registerMove(moveb, 0x1000)
	registerMove(movew, 0x3000)
	registerMove(movel, 0x2000)
	registerMoveA(0x3000, moveaw)
	registerMoveA(0x2000, moveal)

	RegisterInstruction(moveq, 0x7000, 0xf100, 0)
}

func moveq(cpu *CPU) error {
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

func registerMoveA(base uint16, handler Instruction) {
	dstMode := uint16(1)
	for dstReg := uint16(0); dstReg < 8; dstReg++ {
		for srcMode := uint16(0); srcMode < 8; srcMode++ {
			for srcReg := uint16(0); srcReg < 8; srcReg++ {
				opcode := base | (dstReg << 9) | (dstMode << 6) | (srcMode << 3) | srcReg
				if !validEA(opcode, 0x0fff) {
					continue
				}
				InstructionTable[opcode] = handler
			}
		}
	}

}

func registerMove(ins Instruction, base uint16) {
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
			for srcMode := uint16(0); srcMode < 8; srcMode++ {
				for srcReg := uint16(0); srcReg < 8; srcReg++ {
					opcode := base | (dstReg << 9) | (dstMode << 6) | (srcMode << 3) | srcReg
					if !validEA(opcode, 0x0fff) {
						continue
					}
					if InstructionTable[opcode] != nil {
						continue
					}
					InstructionTable[opcode] = ins
				}
			}
		}
	}
}

func moveb(cpu *CPU) error {
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

func movew(cpu *CPU) error {
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

func movel(cpu *CPU) error {
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

func moveaw(cpu *CPU) error {
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

func moveal(cpu *CPU) error {
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
