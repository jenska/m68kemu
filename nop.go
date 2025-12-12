package m68kemu

func init() {
	RegisterInstruction(nop, 0x4e71, 0xffff, 0, constantCycles(4))
}

// nop implements the 68000 NOP instruction (opcode 0x4E71).
// It performs no operation and leaves all condition codes unchanged.
func nop(cpu *cpu) error {
	return nil
}
