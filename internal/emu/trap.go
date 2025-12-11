package emu

func init() {
	RegisterInstruction(trap, 0x4e40, 0xfff0, 0)
}

// trap handles TRAP #n instructions by stacking the exception frame and
// loading the handler address from the vector table.
func trap(cpu *CPU) error {
	cpu.addCycles(34)
	return cpu.Exception(XTrap + uint32(cpu.regs.IR&0x000f))
}
