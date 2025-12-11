package emu

func init() {
	RegisterInstruction(nop, 0x4e71, 0xffff, 0)
}

// nop implements the 68000 NOP instruction (opcode 0x4E71).
// It performs no operation and leaves all condition codes unchanged.
func nop(_ *CPU) error {
	return nil
}
