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

// CycleCalculator builds a static cycle count for a given opcode. Results are
// stored in OpcodeCycleTable during instruction registration and can be looked
// up at execution time for fixed-cost instructions.
type CycleCalculator func(opcode uint16) uint32

func opcodeCycles(opcode uint16) uint32 {
	return OpcodeCycleTable[opcode]
}

var instructionCycleTable = struct {
	Move          uint32
	MoveAddress   uint32
	Lea           uint32
	Pea           uint32
	ShiftRegister uint32
	ShiftMemory   uint32
}{
	Move:          4,
	MoveAddress:   4,
	Lea:           4,
	Pea:           8,
	ShiftRegister: 6,
	ShiftMemory:   8,
}

var (
	eaCycleTable = [8][8]uint32{
		{0, 0, 0, 0, 0, 0, 0, 0},         // Dn
		{0, 0, 0, 0, 0, 0, 0, 0},         // An
		{4, 4, 4, 4, 4, 4, 4, 4},         // (An)
		{4, 4, 4, 4, 4, 4, 4, 4},         // (An)+
		{6, 6, 6, 6, 6, 6, 6, 6},         // -(An)
		{8, 8, 8, 8, 8, 8, 8, 8},         // (d16,An)
		{10, 10, 10, 10, 10, 10, 10, 10}, // (d8,An,Xn)
		{8, 12, 8, 10, 0, 0, 0, 0},       // (xxx).W, (xxx).L, (d16,PC), (d8,PC,Xn), #<data>
	}

	immediateCycleTable = map[Size]uint32{
		Byte: 4,
		Word: 4,
		Long: 8,
	}
)

func eaAccessCycles(mode, reg uint16, size Size) uint32 {
	if mode == 7 && reg == 4 { // #<data>
		return immediateCycleTable[size]
	}

	return eaCycleTable[mode][reg]
}

func moveCycles(ir uint16, size Size) uint32 {
	srcMode := (ir >> 3) & 0x7
	srcReg := ir & 0x7
	dstMode := (ir >> 6) & 0x7
	dstReg := (ir >> 9) & 0x7

	return instructionCycleTable.Move + eaAccessCycles(srcMode, srcReg, size) + eaAccessCycles(dstMode, dstReg, size)
}

func moveAddressCycles(ir uint16, size Size) uint32 {
	srcMode := (ir >> 3) & 0x7
	srcReg := ir & 0x7
	return instructionCycleTable.MoveAddress + eaAccessCycles(srcMode, srcReg, size)
}

func leaPeaCycles(ir uint16, base uint32) uint32 {
	mode := (ir >> 3) & 0x7
	reg := ir & 0x7
	return base + eaAccessCycles(mode, reg, Long)
}

func shiftRegisterCycles(count int) uint32 {
	return instructionCycleTable.ShiftRegister + uint32(count*2)
}

func shiftMemoryCycles(ir uint16) uint32 {
	mode := (ir >> 3) & 0x7
	reg := ir & 0x7
	return instructionCycleTable.ShiftMemory + eaAccessCycles(mode, reg, Word)
}

func constantCycles(c uint32) CycleCalculator {
	return func(uint16) uint32 {
		return c
	}
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

func leaPeaCycleCalculator(base uint32) CycleCalculator {
	return func(opcode uint16) uint32 {
		return leaPeaCycles(opcode, base)
	}
}

func shiftRegisterCycleCalculator(opcode uint16) uint32 {
	operation := int((opcode >> 3) & 0x7)
	registerCount := operation >= 4
	if registerCount {
		return instructionCycleTable.ShiftRegister
	}

	countField := int((opcode >> 9) & 0x7)
	if countField == 0 {
		countField = 8
	}
	return instructionCycleTable.ShiftRegister + uint32(countField*2)
}

func shiftMemoryCycleCalculator(opcode uint16) uint32 {
	return shiftMemoryCycles(opcode)
}

func shiftRotateCycleCalculator(opcode uint16) uint32 {
	if (opcode>>6)&0x7 == 0x7 {
		return shiftMemoryCycleCalculator(opcode)
	}
	return shiftRegisterCycleCalculator(opcode)
}

func abcdCycleCalculator(opcode uint16) uint32 {
	if (opcode>>3)&0x1 == 0 {
		return 6
	}
	return 18
}

func sbcdCycleCalculator(opcode uint16) uint32 {
	if (opcode>>3)&0x1 == 0 {
		return 6
	}
	return 18
}

func nbcdCycleCalculator(opcode uint16) uint32 {
	if (opcode>>3)&0x1 == 0 {
		return 6
	}
	return 8
}
