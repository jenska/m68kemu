package m68kemu

func init() {
	// BRA/Bcc with 8- or 16-bit displacement (no 32-bit on 68000)
	for cond := uint16(0); cond < 16; cond++ {
		match := uint16(0x6000) | (cond << 8)
		registerInstruction(branch, match, 0xff00, 0, constantCycles(10))
	}

	for cond := uint16(0); cond < 16; cond++ {
		match := uint16(0x50c8) | (cond << 8)
		registerInstruction(dbcc, match, 0xfff8, 0, constantCycles(12))
	}

	// Scc
	registerInstruction(scc, 0x50c0, 0xf0c0, eaMaskDataRegister|eaMaskIndirect|
		eaMaskPostIncrement|eaMaskPreDecrement|eaMaskDisplacement|eaMaskIndex|
		eaMaskAbsoluteShort|eaMaskAbsoluteLong, sccCycleCalculator())
}

func branch(cpu *cpu) error {
	cond := (cpu.regs.IR >> 8) & 0xf
	displacement := int32(int8(cpu.regs.IR))
	basePC := cpu.regs.PC

	if displacement == 0 {
		ext, err := cpu.popPc(Word)
		if err != nil {
			return err
		}
		displacement = int32(int16(ext))
	}

	taken := cond == 0x0 || cond == 0x1 || conditionTrue(cpu, cond)

	if taken {
		if cond == 0x1 { // BSR pushes return address
			if err := cpu.push(Long, cpu.regs.PC); err != nil {
				return err
			}
		}
		cpu.regs.PC = uint32(int32(basePC) + displacement)
	}
	return nil
}

func dbcc(cpu *cpu) error {
	cond := (cpu.regs.IR >> 8) & 0xf
	basePC := cpu.regs.PC

	displacement, err := cpu.popPc(Word)
	if err != nil {
		return err
	}

	if conditionTrue(cpu, cond) {
		return nil
	}

	reg := cpu.regs.IR & 0x7
	counter := uint16(cpu.regs.D[reg]) - 1
	cpu.regs.D[reg] = (cpu.regs.D[reg] &^ 0xffff) | int32(counter)

	if counter != 0xffff {
		cpu.regs.PC = uint32(int32(basePC) + int32(int16(displacement)))
	}

	return nil
}

func conditionTrue(cpu *cpu, cond uint16) bool {
	switch cond {
	case 0x0: // True (BRA/BSR)
		return true
	case 0x1: // False
		return false
	case 0x2: // HI (C=0 and Z=0)
		return (cpu.regs.SR & (srCarry | srZero)) == 0
	case 0x3: // LS (C=1 or Z=1)
		return (cpu.regs.SR & (srCarry | srZero)) != 0
	case 0x4: // CC
		return (cpu.regs.SR & srCarry) == 0
	case 0x5: // CS
		return (cpu.regs.SR & srCarry) != 0
	case 0x6: // NE
		return (cpu.regs.SR & srZero) == 0
	case 0x7: // EQ
		return (cpu.regs.SR & srZero) != 0
	case 0x8: // VC
		return (cpu.regs.SR & srOverflow) == 0
	case 0x9: // VS
		return (cpu.regs.SR & srOverflow) != 0
	case 0xa: // PL
		return (cpu.regs.SR & srNegative) == 0
	case 0xb: // MI
		return (cpu.regs.SR & srNegative) != 0
	case 0xc: // GE
		return ((cpu.regs.SR & srNegative) >> 3) == ((cpu.regs.SR & srOverflow) >> 1)
	case 0xd: // LT
		return ((cpu.regs.SR & srNegative) >> 3) != ((cpu.regs.SR & srOverflow) >> 1)
	case 0xe: // GT
		return (cpu.regs.SR&srZero) == 0 && ((cpu.regs.SR&srNegative)>>3) == ((cpu.regs.SR&srOverflow)>>1)
	case 0xf: // LE
		return (cpu.regs.SR&srZero) != 0 || ((cpu.regs.SR&srNegative)>>3) != ((cpu.regs.SR&srOverflow)>>1)
	}

	return false
}

func scc(cpu *cpu) error {
	cond := (cpu.regs.IR >> 8) & 0xf

	dst, err := cpu.ResolveSrcEA(Byte)
	if err != nil {
		return err
	}

	if cond == 0 || conditionTrue(cpu, cond) {
		return dst.write(0xff)
	}

	return dst.write(0x00)
}

func sccCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		if mode == 0 {
			return 6
		}
		return 8 + eaAccessCycles(mode, reg, Byte)
	}
}

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
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
	}
	reg := cpu.regs.IR & 0x7
	cpu.regs.USP = cpu.regs.A[reg]
	return nil
}

func moveFromUsp(cpu *cpu) error {
	if ok, err := cpu.requireSupervisor(); err != nil || !ok {
		return err
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

	replaceStatusFlags(cpu, statusMaskNZVC, 0)

	if value < 0 {
		cpu.regs.SR |= srNegative
		return cpu.exceptionWithCycles(6, chkExceptionCycles(cpu.regs.IR))
	}

	if value > upper {
		cpu.regs.SR |= srCarry
		return cpu.exceptionWithCycles(6, chkExceptionCycles(cpu.regs.IR))
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

func chkExceptionCycles(opcode uint16) uint32 {
	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7
	return exceptionCyclesCHK + eaAccessCycles(mode, reg, Word)
}

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
