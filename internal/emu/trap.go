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
	vector := uint32(cpu.ir & 0x000f)
	vectorNumber := uint32(32) + vector
	vectorOffset := vectorNumber << 2

	originalSR := cpu.regs.SR

	if err := cpu.Push(Word, uint32(originalSR)); err != nil {
		return err
	}
	if err := cpu.Push(Long, cpu.regs.PC); err != nil {
		return err
	}
	if err := cpu.Push(Word, vectorOffset); err != nil {
		return err
	}

	cpu.regs.SR |= srSupervisor

	if handler, err := cpu.bus.ReadLongFrom(vectorOffset); err == nil {
		cpu.regs.PC = handler
		return nil
	} else {
		return err
	}
}
