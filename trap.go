package m68kemu

func init() {
	RegisterInstruction(trap, 0x4e40, 0xfff0, 0, constantCycles(34))
}

// trap handles TRAP #n instructions by stacking the exception frame and
// loading the handler address from the vector table.
func trap(cpu *cpu) error {
	return cpu.Exception(XTrap + uint32(cpu.regs.IR&0x000f))
}
