package emu

// addCycles increments the CPU cycle counter using a uint32 input to keep call
// sites close to the 68k reference values while storing the counter as a wider
// type.
func (cpu *CPU) addCycles(c uint32) {
	cpu.cycles += uint64(c)
}

// Cycles returns the total number of cycles executed since the last reset.
func (cpu *CPU) Cycles() uint64 {
	return cpu.cycles
}

func eaAccessCycles(mode, reg uint16, size Size) uint32 {
	switch mode {
	case 0: // Dn
		return 0
	case 1: // An
		return 0
	case 2: // (An)
		return 4
	case 3: // (An)+
		return 4
	case 4: // -(An)
		return 6
	case 5: // (d16,An)
		return 8
	case 6: // (d8,An,Xn)
		return 10
	case 7:
		switch reg {
		case 0: // (xxx).W
			return 8
		case 1: // (xxx).L
			return 12
		case 2: // (d16,PC)
			return 8
		case 3: // (d8,PC,Xn)
			return 10
		case 4: // #<data>
			if size == Long {
				return 8
			}
			return 4
		}
	}
	return 0
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

func leaPeaCycles(ir uint16, base uint32) uint32 {
	mode := (ir >> 3) & 0x7
	reg := ir & 0x7
	return base + eaAccessCycles(mode, reg, Long)
}

func shiftRegisterCycles(count int) uint32 {
	return 6 + uint32(count*2)
}

func shiftMemoryCycles(ir uint16) uint32 {
	mode := (ir >> 3) & 0x7
	reg := ir & 0x7
	return 8 + eaAccessCycles(mode, reg, Word)
}
