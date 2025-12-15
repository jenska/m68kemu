package m68kemu

// Jump and link-related instructions.
func init() {
	const controlAlterableMask = eaMaskIndirect | eaMaskDisplacement | eaMaskIndex | eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskPCDisplacement | eaMaskPCIndex

	registerInstruction(jmp, 0x4ec0, 0xffc0, controlAlterableMask, jmpCycleCalculator())
	registerInstruction(linkInstruction, 0x4e50, 0xfff8, 0, constantCycles(16))
	registerInstruction(unlkInstruction, 0x4e58, 0xfff8, 0, constantCycles(12))
	registerMoveUsp()
	registerInstruction(chkInstruction, 0x4180, 0xf1c0, chkEAMask, chkCycleCalculator())
}

func jmp(cpu *cpu) error {
	target, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return err
	}

	cpu.regs.PC = target.computedAddress()
	return nil
}

func jmpCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		return 4 + eaAccessCycles(mode, reg, Long)
	}
}

func registerMoveUsp() {
	for reg := uint16(0); reg < 8; reg++ {
		toUSP := uint16(0x4e60) | reg
		fromUSP := uint16(0x4e68) | reg
		opcodeTable[toUSP] = moveToUsp
		opcodeCycleTable[toUSP] = 4
		opcodeTable[fromUSP] = moveFromUsp
		opcodeCycleTable[fromUSP] = 4
	}
}

func moveToUsp(cpu *cpu) error {
	if cpu.regs.SR&srSupervisor == 0 {
		return cpu.exception(XPrivViolation)
	}
	reg := cpu.regs.IR & 0x7
	cpu.regs.USP = cpu.regs.A[reg]
	return nil
}

func moveFromUsp(cpu *cpu) error {
	if cpu.regs.SR&srSupervisor == 0 {
		return cpu.exception(XPrivViolation)
	}
	reg := cpu.regs.IR & 0x7
	cpu.regs.A[reg] = cpu.regs.USP
	return nil
}

func linkInstruction(cpu *cpu) error {
	reg := cpu.regs.IR & 0x7
	displacement, err := cpu.popPc(Word)
	if err != nil {
		return err
	}

	if err := cpu.push(Long, cpu.regs.A[reg]); err != nil {
		return err
	}

	cpu.regs.A[reg] = cpu.regs.A[7]
	cpu.regs.A[7] = uint32(int32(cpu.regs.A[7]) + int32(int16(displacement)))
	return nil
}

func unlkInstruction(cpu *cpu) error {
	reg := cpu.regs.IR & 0x7
	cpu.regs.A[7] = cpu.regs.A[reg]
	value, err := cpu.pop(Long)
	if err != nil {
		return err
	}
	cpu.regs.A[reg] = value
	return nil
}

const chkEAMask = eaMaskDataRegister |
	eaMaskIndirect |
	eaMaskPostIncrement |
	eaMaskPreDecrement |
	eaMaskDisplacement |
	eaMaskIndex |
	eaMaskAbsoluteShort |
	eaMaskAbsoluteLong |
	eaMaskPCDisplacement |
	eaMaskPCIndex

func chkInstruction(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Word)
	if err != nil {
		return err
	}
	bound, err := src.read()
	if err != nil {
		return err
	}

	reg := udx(cpu)
	value := int32(int16(*reg))
	upper := int32(int16(bound))

	cpu.regs.SR &^= srNegative | srZero | srOverflow | srCarry

	if value < 0 {
		cpu.regs.SR |= srNegative
		return cpu.exception(6)
	}

	if value > upper {
		cpu.regs.SR |= srCarry
		return cpu.exception(6)
	}

	if value == upper {
		cpu.regs.SR |= srZero
	}
	return nil
}

func chkCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		return 10 + eaAccessCycles(mode, reg, Word)
	}
}
