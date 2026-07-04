package m68kemu

func init() {
	registerInstructions(
		instructionRegistration{swapInstruction, 0x4840, 0xfff8, 0, constantCycles(4)},
		instructionRegistration{extInstruction, 0x4880, 0xfff8, 0, constantCycles(4)},
		instructionRegistration{extInstruction, 0x48c0, 0xfff8, 0, constantCycles(4)},
	)
	registerInstructions(instructionRegistration{tasInstruction, 0x4ac0, 0xffc0, eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex | eaMaskAbsoluteShort | eaMaskAbsoluteLong, clrTstCycles})

	for _, exg := range []struct {
		match uint16
		cycle uint32
	}{
		{0xc140, 6},
		{0xc148, 6},
		{0xc188, 8},
	} {
		registerExgInstruction(exg.match, constantCycles(exg.cycle))
	}

	registerInstructions(
		instructionRegistration{illegalInstruction, 0x4afc, 0xffff, 0, constantCycles(4)},
		instructionRegistration{nop, 0x4e71, 0xffff, 0, constantCycles(4)},
		instructionRegistration{trapv, 0x4e76, 0xffff, 0, constantCycles(4)},
		instructionRegistration{resetInstruction, 0x4e70, 0xffff, 0, constantCycles(132)},
		instructionRegistration{stop, 0x4e72, 0xffff, 0, constantCycles(4)},
		instructionRegistration{movec68000, 0x4e7a, 0xffff, 0, constantCycles(4)},
		instructionRegistration{movec68000, 0x4e7b, 0xffff, 0, constantCycles(4)},
		instructionRegistration{oriToCcr, 0x003c, 0xffff, 0, constantCycles(20)},
		instructionRegistration{oriToSr, 0x007c, 0xffff, 0, constantCycles(20)},
		instructionRegistration{andiToCcr, 0x023c, 0xffff, 0, constantCycles(20)},
		instructionRegistration{andiToSr, 0x027c, 0xffff, 0, constantCycles(20)},
		instructionRegistration{eoriToCcr, 0x0a3c, 0xffff, 0, constantCycles(20)},
		instructionRegistration{eoriToSr, 0x0a7c, 0xffff, 0, constantCycles(20)},
	)

	const controlSourceMask = eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskImmediate |
		eaMaskPCDisplacement | eaMaskPCIndex
	const controlDestinationMask = eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong

	registerInstructions(
		instructionRegistration{moveFromSr, 0x40c0, 0xffc0, controlDestinationMask, moveControlCycles},
		instructionRegistration{moveToCcr, 0x44c0, 0xffc0, controlSourceMask, moveControlCycles},
		instructionRegistration{moveToSr, 0x46c0, 0xffc0, controlSourceMask, moveControlCycles},
		instructionRegistration{rte, 0x4e73, 0xffff, 0, constantCycles(20)},
		instructionRegistration{rtr, 0x4e77, 0xffff, 0, constantCycles(20)},
	)
}

// MOVEC is not implemented on a plain 68000. The opcode traps immediately as
// ILLEGAL before consuming the extension word that names the control register.
func movec68000(cpu *CPU) error {
	return cpu.exceptionWithCycles(XIllegal, exceptionCyclesIllegal)
}

func trapv(cpu *CPU) error {
	if cpu.regs.SR&srOverflow == 0 {
		return nil
	}

	return cpu.exceptionWithCycles(7, exceptionCyclesTrapV)
}

func resetInstruction(cpu *CPU) error {
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
	}

	cpu.bus.Reset()
	cpu.interrupts.Reset()
	return nil
}

func stop(cpu *CPU) error {
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
	}

	newSR, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	cpu.setSR(uint16(newSR))
	cpu.stopped = true
	return nil
}

func clrTstCycles(opcode uint16) uint32 {
	size := operandSizeFromOpcode(opcode)
	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7
	return 4 + eaAccessCycles(mode, reg, size)
}

func logicalCcrOp(cpu *CPU, op func(uint16, uint16) uint16) error {
	imm, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	current := cpu.regs.SR
	newCcr := op(current&0xff, uint16(imm)) & 0xff
	cpu.setSR((current & 0xff00) | newCcr)
	return nil
}

func logicalSrOp(cpu *CPU, op func(uint16, uint16) uint16) error {
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
	}
	imm, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	cpu.setSR(op(cpu.regs.SR, uint16(imm)))
	return nil
}

func oriToCcr(cpu *CPU) error  { return logicalCcrOp(cpu, func(a, b uint16) uint16 { return a | b }) }
func oriToSr(cpu *CPU) error   { return logicalSrOp(cpu, func(a, b uint16) uint16 { return a | b }) }
func andiToCcr(cpu *CPU) error { return logicalCcrOp(cpu, func(a, b uint16) uint16 { return a & b }) }
func andiToSr(cpu *CPU) error  { return logicalSrOp(cpu, func(a, b uint16) uint16 { return a & b }) }
func eoriToCcr(cpu *CPU) error { return logicalCcrOp(cpu, func(a, b uint16) uint16 { return a ^ b }) }
func eoriToSr(cpu *CPU) error  { return logicalSrOp(cpu, func(a, b uint16) uint16 { return a ^ b }) }

func moveControlCycles(opcode uint16) uint32 {
	size := Word
	if opcode&0xffc0 == 0x44c0 {
		size = Byte
	}
	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7
	return 12 + eaAccessCycles(mode, reg, size)
}

func moveFromSr(cpu *CPU) error {
	dst, err := cpu.ResolveSrcEA2(Word)
	if err != nil {
		return err
	}
	return dst.write(uint32(cpu.regs.SR))
}

func moveToCcr(cpu *CPU) error {
	src, err := cpu.ResolveSrcEA(Byte)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}
	current := cpu.regs.SR & 0xff00
	cpu.setSR(current | uint16(value&0xff))
	return nil
}

func moveToSr(cpu *CPU) error {
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
	}
	src, err := cpu.ResolveSrcEA(Word)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}
	cpu.setSR(uint16(value))
	return nil
}

func rte(cpu *CPU) error {
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
	}
	// On a 68000, RTE restores the standard SR+PC exception frame.
	// Bus/address error handlers must first discard or rewrite the extra
	// group-0 fault information before executing RTE.
	newSR, err := cpu.pop(Word)
	if err != nil {
		return err
	}
	pc, err := cpu.pop(Long)
	if err != nil {
		return err
	}
	cpu.setSR(uint16(newSR))
	cpu.regs.PC = pc
	return nil
}

func rtr(cpu *CPU) error {
	ccr, err := cpu.pop(Word)
	if err != nil {
		return err
	}
	pc, err := cpu.pop(Long)
	if err != nil {
		return err
	}
	preserved := cpu.regs.SR & 0xff00
	cpu.setSR(preserved | uint16(ccr&0xff))
	cpu.regs.PC = pc
	return nil
}

// nop implements the 68000 NOP instruction (opcode 0x4E71).
// It performs no operation and leaves all condition codes unchanged.
func nop(cpu *CPU) error {
	return nil
}

func swapInstruction(cpu *CPU) error {
	reg := dy(cpu)
	value := *reg
	result := (value << 16) | ((value >> 16) & 0xffff)
	*reg = result

	updateNZClearVC(cpu, result, Long)
	return nil
}

func extInstruction(cpu *CPU) error {
	opcode := cpu.regs.IR
	sizeBits := (opcode >> 6) & 0x3
	dst := dy(cpu)

	var (
		result uint32
		size   Size
	)

	switch sizeBits {
	case 0x2: // EXT.W: byte to word
		size = Word
		result = uint32(int16(int8(*dst)))
	case 0x3: // EXT.L: word to long
		size = Long
		result = uint32(int32(int16(*dst)))
	default:
		return nil
	}

	mask := size.mask()
	*dst = (*dst & ^mask) | (result & mask)

	updateNZClearVC(cpu, result, size)
	return nil
}

func exgInstruction(cpu *CPU) error {
	opcode := cpu.regs.IR
	rx := y(cpu.regs.IR)
	ry := x(cpu.regs.IR)

	switch opcode & 0x01c0 {
	case 0x0140: // Dx,Dy or Ax,Ay
		if opcode&0x0008 == 0 { // Dx,Dy
			dxReg := &cpu.regs.D[ry]
			dyReg := &cpu.regs.D[rx]
			*dxReg, *dyReg = *dyReg, *dxReg
			return nil
		}

		axReg := &cpu.regs.A[ry]
		ayReg := &cpu.regs.A[rx]
		*axReg, *ayReg = *ayReg, *axReg
		return nil

	case 0x0180: // Dx,Ay
		dxReg := &cpu.regs.D[ry]
		ayReg := &cpu.regs.A[rx]
		temp := uint32(*dxReg)
		*dxReg = int32(*ayReg)
		*ayReg = temp
		return nil
	}

	return nil
}

func tasInstruction(cpu *CPU) error {
	dst, err := cpu.ResolveSrcEA(Byte)
	if err != nil {
		return err
	}

	value, err := dst.read()
	if err != nil {
		return err
	}

	updateNZClearVC(cpu, value, Byte)

	return dst.write(value | 0x80)
}

func registerExgInstruction(match uint16, calc cycleCalculator) {
	for rx := range uint16(8) {
		for ry := range uint16(8) {
			opcode := match | (ry << 9) | rx
			opcodeTable[opcode] = exgInstruction
			if calc != nil {
				opcodeCycleTable[opcode] = calc(opcode)
			}
		}
	}
}

func illegalInstruction(cpu *CPU) error {
	return cpu.exceptionWithCycles(XIllegal, exceptionCyclesIllegal)
}

func init() {
	registerInstructions(instructionRegistration{trap, 0x4e40, 0xfff0, 0, constantCycles(34)})
}

// trap handles TRAP #n instructions by stacking the exception frame and
// loading the handler address from the vector table.
func trap(cpu *CPU) error {
	return cpu.trapException(XTrap + uint32(cpu.regs.IR&0x000f))
}
