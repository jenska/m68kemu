package m68kemu

func init() {
	// Data alterable destinations for bit instructions (data register or memory,
	// excluding address registers, PC-relative modes, and immediates).
	dataAlterable := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong

	// Dynamic bit number (from Dx) uses opcodes with bit 11 clear and op type in bits 8-6.
	for op := uint16(0); op < 4; op++ {
		match := uint16(0x0100) | ((op + 4) << 6)
		registerInstruction(bitDynamic, match, 0xf1c0, dataAlterable, bitCycleCalculator(false, op))
	}

	// Static bit number (immediate) uses opcodes with bit 11 set and op type in bits 8-6.
	for op := uint16(0); op < 4; op++ {
		match := uint16(0x0800) | (op << 6)
		registerInstruction(bitImmediate, match, 0xffc0, dataAlterable, bitCycleCalculator(true, op))
	}
}

func bitCycleCalculator(immediate bool, op uint16) cycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7

		// Data register destination
		if mode == 0 {
			switch op {
			case 0: // BTST
				if immediate {
					return 8
				}
				return 4
			default: // BCHG, BCLR, BSET
				if immediate {
					return 12
				}
				return 8
			}
		}

		ea := eaAccessCycles(mode, reg, Byte)
		switch op {
		case 0: // BTST
			if immediate {
				return 12 + ea
			}
			return 8 + ea
		default: // BCHG, BCLR, BSET
			if immediate {
				return 16 + ea
			}
			return 12 + ea
		}
	}
}

func bitDynamic(cpu *cpu) error {
	index := (cpu.regs.IR >> 9) & 0x7
	bitNumber := uint32(cpu.regs.D[index])
	mode := (cpu.regs.IR >> 3) & 0x7

	var dst modifier
	if mode != 0 {
		var err error
		dst, err = cpu.ResolveSrcEA(Byte)
		if err != nil {
			return err
		}
	}

	return bitOperation(cpu, bitNumber, mode, dst)
}

func bitImmediate(cpu *cpu) error {
	mode := (cpu.regs.IR >> 3) & 0x7

	var dst modifier
	if mode != 0 {
		var err error
		dst, err = cpu.ResolveSrcEA(Byte)
		if err != nil {
			return err
		}
	}

	imm, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	return bitOperation(cpu, imm, mode, dst)
}

func bitOperation(cpu *cpu, bitNumber uint32, mode uint16, dst modifier) error {
	op := (cpu.regs.IR >> 6) & 0x7
	opType := op & 0x3 // 0=BTST, 1=BCHG, 2=BCLR, 3=BSET

	cpu.regs.SR &^= srZero

	if mode == 0 { // data register destination (long size)
		dst := dy(cpu)
		mask := uint32(1) << (bitNumber & 31)
		previousSet := (*dst & mask) != 0

		switch opType {
		case 1: // BCHG
			*dst ^= mask
		case 2: // BCLR
			*dst &^= mask
		case 3: // BSET
			*dst |= mask
		}

		if !previousSet {
			cpu.regs.SR |= srZero
		}
		return nil
	}

	if dst == nil {
		var err error
		dst, err = cpu.ResolveSrcEA(Byte)
		if err != nil {
			return err
		}
	}

	value, err := dst.read()
	if err != nil {
		return err
	}

	mask := uint32(1) << (bitNumber & 7)
	previousSet := (value & mask) != 0

	switch opType {
	case 1: // BCHG
		value ^= mask
	case 2: // BCLR
		value &^= mask
	case 3: // BSET
		value |= mask
	}

	if opType != 0 {
		if err := dst.write(value); err != nil {
			return err
		}
	}

	if !previousSet {
		cpu.regs.SR |= srZero
	}
	return nil
}
