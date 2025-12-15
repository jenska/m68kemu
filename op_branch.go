package m68kemu

func init() {
	// BRA/Bcc with 8- or 16-bit displacement (no 32-bit on 68000)
	for cond := uint16(0); cond < 16; cond++ {
		match := uint16(0x6000) | (cond << 8)
		registerInstruction(branch, match, 0xff00, 0, constantCycles(10))
	}

	for cond := uint16(0); cond < 16; cond++ {
		match := uint16(0x50c8) | (cond << 8)
		registerInstruction(dbcc, match, 0xfff8, 0, constantCycles(12))
	}

	// Scc
	registerInstruction(scc, 0x50c0, 0xf0c0, eaMaskDataRegister|eaMaskIndirect|
		eaMaskPostIncrement|eaMaskPreDecrement|eaMaskDisplacement|eaMaskIndex|
		eaMaskAbsoluteShort|eaMaskAbsoluteLong, sccCycleCalculator())
}

func branch(cpu *cpu) error {
	cond := (cpu.regs.IR >> 8) & 0xf
	displacement := int32(int8(cpu.regs.IR))

	if displacement == 0 {
		ext, err := cpu.popPc(Word)
		if err != nil {
			return err
		}
		displacement = int32(int16(ext))
	}

	taken := cond == 0x0 || cond == 0x1 || conditionTrue(cpu, cond)

	if taken {
		if cond == 0x1 { // BSR pushes return address
			if err := cpu.push(Long, cpu.regs.PC); err != nil {
				return err
			}
		}
		cpu.regs.PC = uint32(int32(cpu.regs.PC) + displacement)
	}
	return nil
}

func dbcc(cpu *cpu) error {
	cond := (cpu.regs.IR >> 8) & 0xf

	displacement, err := cpu.popPc(Word)
	if err != nil {
		return err
	}

	if conditionTrue(cpu, cond) {
		return nil
	}

	reg := cpu.regs.IR & 0x7
	counter := uint16(cpu.regs.D[reg]) - 1
	cpu.regs.D[reg] = (cpu.regs.D[reg] &^ 0xffff) | int32(counter)

	if counter != 0xffff {
		cpu.regs.PC = uint32(int32(cpu.regs.PC) + int32(int16(displacement)))
	}

	return nil
}

func conditionTrue(cpu *cpu, cond uint16) bool {
	switch cond {
	case 0x0: // True (BRA/BSR)
		return true
	case 0x1: // False
		return false
	case 0x2: // HI (C=0 and Z=0)
		return (cpu.regs.SR & (srCarry | srZero)) == 0
	case 0x3: // LS (C=1 or Z=1)
		return (cpu.regs.SR & (srCarry | srZero)) != 0
	case 0x4: // CC
		return (cpu.regs.SR & srCarry) == 0
	case 0x5: // CS
		return (cpu.regs.SR & srCarry) != 0
	case 0x6: // NE
		return (cpu.regs.SR & srZero) == 0
	case 0x7: // EQ
		return (cpu.regs.SR & srZero) != 0
	case 0x8: // VC
		return (cpu.regs.SR & srOverflow) == 0
	case 0x9: // VS
		return (cpu.regs.SR & srOverflow) != 0
	case 0xa: // PL
		return (cpu.regs.SR & srNegative) == 0
	case 0xb: // MI
		return (cpu.regs.SR & srNegative) != 0
	case 0xc: // GE
		return ((cpu.regs.SR & srNegative) >> 3) == ((cpu.regs.SR & srOverflow) >> 1)
	case 0xd: // LT
		return ((cpu.regs.SR & srNegative) >> 3) != ((cpu.regs.SR & srOverflow) >> 1)
	case 0xe: // GT
		return (cpu.regs.SR&srZero) == 0 && ((cpu.regs.SR&srNegative)>>3) == ((cpu.regs.SR&srOverflow)>>1)
	case 0xf: // LE
		return (cpu.regs.SR&srZero) != 0 || ((cpu.regs.SR&srNegative)>>3) != ((cpu.regs.SR&srOverflow)>>1)
	}

	return false
}

func scc(cpu *cpu) error {
	cond := (cpu.regs.IR >> 8) & 0xf

	dst, err := cpu.ResolveSrcEA(Byte)
	if err != nil {
		return err
	}

	if cond == 0 || conditionTrue(cpu, cond) {
		return dst.write(0xff)
	}

	return dst.write(0x00)
}

func sccCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		if mode == 0 {
			return 6
		}
		return 8 + eaAccessCycles(mode, reg, Byte)
	}
}
