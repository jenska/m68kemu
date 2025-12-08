package emu

const (
	srSupervisor = uint16(0x2000)
)

func init() {
	RegisterInstruction(trap, 0x4e40, 0xfff0, 0)
}

// trap handles TRAP #n instructions by stacking the exception frame and
// loading the handler address from the vector table.
func trap(cpu *CPU) error {
	return cpu.Exception(XTrap)
}
