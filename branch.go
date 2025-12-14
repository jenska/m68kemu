package m68kemu

func init() {
	// BRA/Bcc with 8- or 16-bit displacement (no 32-bit on 68000)
	for cond := uint16(0); cond < 16; cond++ {
		match := uint16(0x6000) | (cond << 8)
		registerInstruction(branch, match, 0xff00, 0, constantCycles(10))
	}
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

	taken := false
	switch cond {
	case 0x0: // BRA
		taken = true
	case 0x1: // BSR
		taken = true
	case 0x2: // BHI (C=0 and Z=0)
		taken = (cpu.regs.SR & (srCarry | srZero)) == 0
	case 0x3: // BLS (C=1 or Z=1)
		taken = (cpu.regs.SR & (srCarry | srZero)) != 0
	case 0x4: // BCC
		taken = (cpu.regs.SR & srCarry) == 0
	case 0x5: // BCS
		taken = (cpu.regs.SR & srCarry) != 0
	case 0x6: // BNE
		taken = (cpu.regs.SR & srZero) == 0
	case 0x7: // BEQ
		taken = (cpu.regs.SR & srZero) != 0
	case 0x8: // BVC
		taken = (cpu.regs.SR & srOverflow) == 0
	case 0x9: // BVS
		taken = (cpu.regs.SR & srOverflow) != 0
	case 0xa: // BPL
		taken = (cpu.regs.SR & srNegative) == 0
	case 0xb: // BMI
		taken = (cpu.regs.SR & srNegative) != 0
	case 0xc: // BGE
		taken = ((cpu.regs.SR & srNegative) >> 3) == ((cpu.regs.SR & srOverflow) >> 1)
	case 0xd: // BLT
		taken = ((cpu.regs.SR & srNegative) >> 3) != ((cpu.regs.SR & srOverflow) >> 1)
	case 0xe: // BGT
		taken = (cpu.regs.SR&srZero) == 0 && ((cpu.regs.SR&srNegative)>>3) == ((cpu.regs.SR&srOverflow)>>1)
	case 0xf: // BLE
		taken = (cpu.regs.SR&srZero) != 0 || ((cpu.regs.SR&srNegative)>>3) != ((cpu.regs.SR&srOverflow)>>1)
	}

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
