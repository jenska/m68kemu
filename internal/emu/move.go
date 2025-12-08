package emu

func init() {
	RegisterInstruction(moveaw, 0x3040, 0xf040, 0x0fff)
	RegisterInstruction(moveal, 0x2040, 0xf040, 0x0fff)
}

func moveaw(cpu *CPU) error {
	src, err := cpu.ResolveSrcEA(Word)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	// Destination is always an address register encoded in bits 11..9.
	reg := (cpu.ir >> 9) & 0x7
	cpu.regs.A[reg] = uint32(int32(int16(value)))
	return nil
}

func moveal(cpu *CPU) error {
	src, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	// Destination is always an address register encoded in bits 11..9.
	reg := (cpu.ir >> 9) & 0x7
	cpu.regs.A[reg] = value
	return nil
}
