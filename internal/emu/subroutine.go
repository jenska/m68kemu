package emu

// Subroutine control flow: JSR/RTS
func init() {
	registerSubroutine()
}

func registerSubroutine() {
	const controlAlterableMask = eaMaskIndirect | eaMaskDisplacement | eaMaskIndex | eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskPCDisplacement | eaMaskPCIndex

	RegisterInstruction(jsr, 0x4e80, 0xffc0, controlAlterableMask, jsrCycleCalculator())
	RegisterInstruction(rts, 0x4e75, 0xffff, 0, constantCycles(16))
}

func jsr(cpu *CPU) error {
	target, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return err
	}

	returnAddr := cpu.regs.PC
	if err := cpu.Push(Long, returnAddr); err != nil {
		return err
	}

	cpu.regs.PC = target.computedAddress()
	return nil
}

func rts(cpu *CPU) error {
	addr, err := cpu.Pop(Long)
	if err != nil {
		return err
	}
	cpu.regs.PC = addr
	return nil
}

func jsrCycleCalculator() CycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		return 16 + eaAccessCycles(mode, reg, Long)
	}
}
