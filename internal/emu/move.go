package emu

func init() {
	registerMoveA(Word, 0x3000, moveaw)
	registerMoveA(Long, 0x2000, moveal)
	registerMove(Byte, 0x1000)
	registerMove(Word, 0x3000)
	registerMove(Long, 0x2000)
}

func registerMoveA(size *Size, base uint16, handler Instruction) {
	dstMode := uint16(1)
	for dstReg := uint16(0); dstReg < 8; dstReg++ {
		for srcMode := uint16(0); srcMode < 8; srcMode++ {
			for srcReg := uint16(0); srcReg < 8; srcReg++ {
				opcode := base | (dstReg << 9) | (dstMode << 6) | (srcMode << 3) | srcReg
				if !validEA(opcode, 0x0fff) {
					continue
				}
				instructions[opcode] = handler
			}
		}
	}
}

func registerMove(size *Size, base uint16) {
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
					if instructions[opcode] != nil {
						continue
					}
					instructions[opcode] = func(cpu *CPU) error {
						return move(cpu, size)
					}
				}
			}
		}
	}
}

func move(cpu *CPU, size *Size) error {
	src, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}
	dst, origIR, err := cpu.ResolveMoveDstEA(size)
	if err != nil {
		return err
	}
	defer func() { cpu.ir = origIR }()
	if err := dst.write(value); err != nil {
		return err
	}
	cpu.ir = origIR
	cpu.regs.SR &^= srNegative | srZero | srOverflow | srCarry
	masked := int32(value & size.mask)
	if masked == 0 {
		cpu.regs.SR |= srZero
	}
	if size.IsNegative(masked) {
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
	reg := (cpu.ir >> 9) & 0x7
	cpu.regs.A[reg] = uint32(int32(int16(value)))
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
	reg := (cpu.ir >> 9) & 0x7
	cpu.regs.A[reg] = value
	return nil
}
