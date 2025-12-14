package m68kemu

// Subroutine control flow: JSR/RTS
func init() {
	const controlAlterableMask = eaMaskIndirect | eaMaskDisplacement | eaMaskIndex | eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskPCDisplacement | eaMaskPCIndex

	registerInstruction(jsr, 0x4e80, 0xffc0, controlAlterableMask, jsrCycleCalculator())
	registerInstruction(rts, 0x4e75, 0xffff, 0, constantCycles(16))
}

func jsr(cpu *cpu) error {
	target, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return err
	}

	returnAddr := cpu.regs.PC
	if err := cpu.push(Long, returnAddr); err != nil {
		return err
	}

	cpu.regs.PC = target.computedAddress()
	return nil
}

func rts(cpu *cpu) error {
	addr, err := cpu.pop(Long)
	if err != nil {
		return err
	}
	cpu.regs.PC = addr
	return nil
}

func jsrCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		return 16 + eaAccessCycles(mode, reg, Long)
	}
}
