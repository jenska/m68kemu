package m68kemu

func init() {
	registerInstruction(trapv, 0x4e76, 0xffff, 0, constantCycles(4))
	registerInstruction(resetInstruction, 0x4e70, 0xffff, 0, constantCycles(132))
	registerInstruction(stop, 0x4e72, 0xffff, 0, constantCycles(4))
	registerInstruction(movec68000, 0x4e7a, 0xffff, 0, constantCycles(4))
	registerInstruction(movec68000, 0x4e7b, 0xffff, 0, constantCycles(4))

	registerInstruction(oriToCcr, 0x003c, 0xffff, 0, constantCycles(20))
	registerInstruction(oriToSr, 0x007c, 0xffff, 0, constantCycles(20))
	registerInstruction(andiToCcr, 0x023c, 0xffff, 0, constantCycles(20))
	registerInstruction(andiToSr, 0x027c, 0xffff, 0, constantCycles(20))
	registerInstruction(eoriToCcr, 0x0a3c, 0xffff, 0, constantCycles(20))
	registerInstruction(eoriToSr, 0x0a7c, 0xffff, 0, constantCycles(20))

	const controlSourceMask = eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskImmediate |
		eaMaskPCDisplacement | eaMaskPCIndex
	const controlDestinationMask = eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong

	registerInstruction(moveFromSr, 0x40c0, 0xffc0, controlDestinationMask, moveControlCycleCalculator(Word))
	registerInstruction(moveToCcr, 0x44c0, 0xffc0, controlSourceMask, moveControlCycleCalculator(Byte))
	registerInstruction(moveToSr, 0x46c0, 0xffc0, controlSourceMask, moveControlCycleCalculator(Word))

	registerInstruction(rte, 0x4e73, 0xffff, 0, constantCycles(20))
	registerInstruction(rtr, 0x4e77, 0xffff, 0, constantCycles(20))
}

// MOVEC is not implemented on a plain 68000. The opcode traps immediately as
// ILLEGAL before consuming the extension word that names the control register.
func movec68000(cpu *cpu) error {
	return cpu.exceptionWithCycles(XIllegal, exceptionCyclesIllegal)
}

func trapv(cpu *cpu) error {
	if cpu.regs.SR&srOverflow == 0 {
		return nil
	}

	return cpu.exceptionWithCycles(7, exceptionCyclesTrapV)
}

func resetInstruction(cpu *cpu) error {
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
	}

	cpu.bus.Reset()
	cpu.interrupts.Reset()
	return nil
}

func stop(cpu *cpu) error {
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

func clrTstCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		size := operandSizeFromOpcode(opcode)
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		return 4 + eaAccessCycles(mode, reg, size)
	}
}

func logicalCcrOp(cpu *cpu, op func(uint16, uint16) uint16) error {
	imm, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	current := cpu.regs.SR
	newCcr := op(current&0xff, uint16(imm)) & 0xff
	cpu.setSR((current & 0xff00) | newCcr)
	return nil
}

func logicalSrOp(cpu *cpu, op func(uint16, uint16) uint16) error {
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

func oriToCcr(cpu *cpu) error  { return logicalCcrOp(cpu, func(a, b uint16) uint16 { return a | b }) }
func oriToSr(cpu *cpu) error   { return logicalSrOp(cpu, func(a, b uint16) uint16 { return a | b }) }
func andiToCcr(cpu *cpu) error { return logicalCcrOp(cpu, func(a, b uint16) uint16 { return a & b }) }
func andiToSr(cpu *cpu) error  { return logicalSrOp(cpu, func(a, b uint16) uint16 { return a & b }) }
func eoriToCcr(cpu *cpu) error { return logicalCcrOp(cpu, func(a, b uint16) uint16 { return a ^ b }) }
func eoriToSr(cpu *cpu) error  { return logicalSrOp(cpu, func(a, b uint16) uint16 { return a ^ b }) }

func moveControlCycleCalculator(size Size) cycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		return 12 + eaAccessCycles(mode, reg, size)
	}
}

func moveFromSr(cpu *cpu) error {
	dst, err := cpu.ResolveSrcEA2(Word)
	if err != nil {
		return err
	}
	return dst.write(uint32(cpu.regs.SR))
}

func moveToCcr(cpu *cpu) error {
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

func moveToSr(cpu *cpu) error {
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

func rte(cpu *cpu) error {
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

func rtr(cpu *cpu) error {
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

func init() {
	registerInstruction(swapInstruction, 0x4840, 0xfff8, 0, constantCycles(4))
	registerInstruction(extInstruction, 0x4880, 0xfff8, 0, constantCycles(4))
	registerInstruction(extInstruction, 0x48c0, 0xfff8, 0, constantCycles(4))
	registerInstruction(tasInstruction, 0x4ac0, 0xffc0, eaMaskDataRegister|eaMaskIndirect|eaMaskPostIncrement|
		eaMaskPreDecrement|eaMaskDisplacement|eaMaskIndex|eaMaskAbsoluteShort|eaMaskAbsoluteLong, clrTstCycleCalculator())

	registerExgInstruction(0xc140, constantCycles(6))
	registerExgInstruction(0xc148, constantCycles(6))
	registerExgInstruction(0xc188, constantCycles(8))

	registerInstruction(illegalInstruction, 0x4afc, 0xffff, 0, constantCycles(4))
	registerInstruction(nop, 0x4e71, 0xffff, 0, constantCycles(4))
}

// nop implements the 68000 NOP instruction (opcode 0x4E71).
// It performs no operation and leaves all condition codes unchanged.
func nop(cpu *cpu) error {
	return nil
}

func swapInstruction(cpu *cpu) error {
	reg := dy(cpu)
	value := *reg
	result := (value << 16) | ((value >> 16) & 0xffff)
	*reg = result

	updateNZClearVC(cpu, result, Long)
	return nil
}

func extInstruction(cpu *cpu) error {
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

func exgInstruction(cpu *cpu) error {
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

func tasInstruction(cpu *cpu) error {
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
	for rx := uint16(0); rx < 8; rx++ {
		for ry := uint16(0); ry < 8; ry++ {
			opcode := match | (ry << 9) | rx
			opcodeTable[opcode] = exgInstruction
			if calc != nil {
				opcodeCycleTable[opcode] = calc(opcode)
			}
		}
	}
}

func illegalInstruction(cpu *cpu) error {
	return cpu.exceptionWithCycles(XIllegal, exceptionCyclesIllegal)
}

func init() {
	registerInstruction(trap, 0x4e40, 0xfff0, 0, constantCycles(34))
}

// trap handles TRAP #n instructions by stacking the exception frame and
// loading the handler address from the vector table.
func trap(cpu *cpu) error {
	return cpu.exception(XTrap + uint32(cpu.regs.IR&0x000f))
}
