package m68kemu

import "math/bits"

func registerMoves(b *tableBuilder) {
	registerMove(b, 0x1000, moveCycleCalculator(Byte))
	registerMove(b, 0x3000, moveCycleCalculator(Word))
	registerMove(b, 0x2000, moveCycleCalculator(Long))
	registerMoveA(b, 0x3000, moveaw, moveAddressCycleCalculator(Word))
	registerMoveA(b, 0x2000, moveal, moveAddressCycleCalculator(Long))
	b.add(moveq, 0x7000, 0xf100, 0, constantCycles(4))
}

const moveSourceEAMask = eaMaskDataRegister |
	eaMaskAddressRegister |
	eaMaskIndirect |
	eaMaskPostIncrement |
	eaMaskPreDecrement |
	eaMaskDisplacement |
	eaMaskIndex |
	eaMaskAbsoluteShort |
	eaMaskAbsoluteLong |
	eaMaskImmediate |
	eaMaskPCDisplacement |
	eaMaskPCIndex

func moveq(cpu *CPU) error {
	value := uint32(int32(int8(cpu.regs.IR)))
	*dx(cpu) = value

	updateNZClearVC(cpu, uint32(value), Long)
	return nil
}

func registerMoveA(b *tableBuilder, base uint16, handler instruction, calc cycleCalculator) {
	const dstMode = uint16(1)
	for dstReg := range uint16(8) {
		match := base | (dstReg << 9) | (dstMode << 6)
		b.add(handler, match, 0xffc0, moveSourceEAMask, calc)
	}
}

func registerMove(b *tableBuilder, base uint16, calc cycleCalculator) {
	for dstMode := range uint16(8) {
		// Address register destinations are handled by MOVEA.
		if dstMode == 1 {
			continue
		}
		for dstReg := range uint16(8) {
			// Destination must be data alterable: mode 7 only has the two
			// absolute forms; registers 2-7 are PC-relative, immediate or
			// unassigned.
			if dstMode == 7 && dstReg > 1 {
				continue
			}
			match := base | (dstReg << 9) | (dstMode << 6)
			b.add(move, match, 0xffc0, moveSourceEAMask, calc)
		}
	}
}

// moveSizes maps the size field of MOVE, bits 13-12, to the operand size.
var moveSizes = [4]Size{1: Byte, 2: Long, 3: Word}

func move(cpu *CPU) error {
	size := moveSizes[(cpu.regs.IR>>12)&3]
	src, err := cpu.resolveSrcEA(size)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	dst, err := cpu.resolveDstEA(size)
	if err != nil {
		return err
	}

	if err := dst.write(value); err != nil {
		return err
	}

	updateNZClearVC(cpu, value, size)
	return nil
}

func moveaw(cpu *CPU) error {
	src, err := cpu.resolveSrcEA(Word)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	// Destination is always an address register encoded in bits 11..9.
	*ax(cpu) = uint32(int32(int16(value)))
	return nil
}

func moveal(cpu *CPU) error {
	src, err := cpu.resolveSrcEA(Long)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	// Destination is always an address register encoded in bits 11..9.
	*ax(cpu) = value
	return nil
}

// moveCycleCalculator times MOVE (tables 8-2, 8-3).
func moveCycleCalculator(size Size) cycleCalculator {
	return func(opcode uint16) uint32 {
		srcMode, srcReg := eaFields(opcode)
		dstMode, dstReg := (opcode>>6)&0x7, (opcode>>9)&0x7
		return 4 + eaAccessCycles(srcMode, srcReg, size) + moveDestinationCycles(dstMode, dstReg, size)
	}
}

// moveAddressCycleCalculator times MOVEA (tables 8-2, 8-3).
func moveAddressCycleCalculator(size Size) cycleCalculator {
	return func(opcode uint16) uint32 {
		mode, reg := eaFields(opcode)
		return 4 + eaAccessCycles(mode, reg, size)
	}
}

// MOVEM: move multiple registers to/from memory
func registerMovem(b *tableBuilder) {
	// Register-to-memory: MOVEM.<size> <register list>,<ea>
	// The handlers add the whole time, which depends on the register list.
	for _, sizeBits := range []uint16{2 << 6, 3 << 6} {
		b.add(movemToMemory, 0x4800|sizeBits, 0xffc0, eaMaskIndirect|eaMaskPreDecrement|eaMaskDisplacement|
			eaMaskIndex|eaMaskAbsoluteShort|eaMaskAbsoluteLong, nil)
		b.add(movemToRegisters, 0x4c00|sizeBits, 0xffc0, eaMaskIndirect|eaMaskPostIncrement|
			eaMaskDisplacement|eaMaskIndex|eaMaskAbsoluteShort|eaMaskAbsoluteLong|eaMaskPCDisplacement|eaMaskPCIndex, nil)
	}
}

// movemSize returns the MOVEM operand size from opcode bit 6.
func movemSize(opcode uint16) Size {
	if opcode&0x0040 != 0 {
		return Long
	}
	return Word
}

func movemRegisterOrder(mask uint16, reverse bool) []int {
	if reverse {
		mask = bits.Reverse16(mask)
	}

	order := make([]int, 0, 16)

	if reverse {
		for reg := 15; reg >= 0; reg-- {
			if mask&(1<<reg) != 0 {
				order = append(order, reg)
			}
		}
		return order
	}

	for reg := range 16 {
		if mask&(1<<reg) != 0 {
			order = append(order, reg)
		}
	}
	return order
}

func movemReadAddress(cpu *CPU, mode, reg uint16) (uint32, error) {
	switch mode {
	case 2, 3, 4: // (An), (An)+, -(An)
		return cpu.regs.A[reg], nil
	case 5: // (d16,An)
		ext, err := cpu.popPc(Word)
		return uint32(int32(cpu.regs.A[reg]) + int32(int16(ext))), err
	case 6: // (d8,An,Xn)
		return cpu.indexedAddress(cpu.regs.A[reg])
	case 7:
		switch reg {
		case 0: // (xxx).W
			ext, err := cpu.popPc(Word)
			return uint32(int32(int16(ext))), err
		case 1: // (xxx).L
			return cpu.popPc(Long)
		case 2: // (d16,PC)
			// The base is the displacement word's own address, not the PC
			// after it; MOVEM's register mask precedes it.
			pc := cpu.regs.PC
			ext, err := cpu.popPc(Word)
			return uint32(int32(pc) + int32(int16(ext))), err
		case 3: // (d8,PC,Xn)
			return cpu.indexedAddress(cpu.regs.PC)
		}
	}
	return 0, cpu.exception(XIllegal)
}

func movemToRegisters(cpu *CPU) error {
	opcode := cpu.regs.IR
	size := movemSize(opcode)

	mask, err := cpu.popPc(Word)
	if err != nil {
		return err
	}

	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7
	addr, err := movemReadAddress(cpu, mode, reg)
	if err != nil {
		return err
	}

	regs := movemRegisterOrder(uint16(mask), false)
	cpu.addCycles(movemBaseCycles(mode, reg, true) + movemRegisterCycles(size, len(regs)))

	sizeBytes := uint32(size)

	for _, r := range regs {
		if mode == 4 { // predecrement reads from decremented address
			addr -= sizeBytes
		}

		value, err := cpu.read(size, addr)
		if err != nil {
			return err
		}

		if size == Word {
			value = uint32(int32(int16(value)))
		}
		*generalRegister(cpu, uint16(r)) = value

		if mode != 4 { // all memory-to-register modes read sequentially
			addr += sizeBytes
		}
	}

	if mode == 3 {
		// If the base register was loaded from memory, it must not be overwritten by the incremented address.
		if (mask>>uint(8+reg))&1 == 0 {
			cpu.regs.A[reg] = addr
		}
	}

	return nil
}

func movemToMemory(cpu *CPU) error {
	opcode := cpu.regs.IR
	size := movemSize(opcode)

	mask, err := cpu.popPc(Word)
	if err != nil {
		return err
	}

	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7
	addr, err := movemReadAddress(cpu, mode, reg)
	if err != nil {
		return err
	}

	reverse := mode == 4
	regs := movemRegisterOrder(uint16(mask), reverse)
	cpu.addCycles(movemBaseCycles(mode, reg, false) + movemRegisterCycles(size, len(regs)))

	sizeBytes := uint32(size)

	for _, r := range regs {
		if mode == 4 { // predecrement destination
			addr -= sizeBytes
		}

		if err := cpu.write(size, addr, *generalRegister(cpu, uint16(r))); err != nil {
			return err
		}

		if mode != 4 {
			addr += sizeBytes
		}
	}

	if mode == 4 {
		cpu.regs.A[reg] = addr
	}

	return nil
}

func registerMovep(b *tableBuilder) {
	b.add(movep, 0x0108, 0xf1f8, 0, movepCycleCalculator)
	b.add(movep, 0x0148, 0xf1f8, 0, movepCycleCalculator)
	b.add(movep, 0x0188, 0xf1f8, 0, movepCycleCalculator)
	b.add(movep, 0x01c8, 0xf1f8, 0, movepCycleCalculator)
}

func movep(cpu *CPU) error {
	opcode := cpu.regs.IR
	// Opmode (bits 8-6): 100/101 word/long memory to register, 110/111
	// word/long register to memory.
	size := Word
	if opcode&0x0040 != 0 {
		size = Long
	}
	toRegister := opcode&0x0080 == 0

	disp, err := cpu.popPc(Word)
	if err != nil {
		return err
	}

	addr := uint32(int32(cpu.regs.A[opcode&0x7]) + int32(int16(disp)))
	reg := (opcode >> 9) & 0x7
	mask := size.mask()

	// The bytes go to every other address, most significant first.
	n := uint32(size)
	var value uint32
	if toRegister {
		for i := range n {
			b, err := cpu.read(Byte, addr+2*i)
			if err != nil {
				return err
			}
			value = value<<8 | b
		}
		cpu.regs.D[reg] = cpu.regs.D[reg]&^mask | value&mask
	} else {
		value = cpu.regs.D[reg] & mask
		for i := range n {
			if err := cpu.write(Byte, addr+2*i, (value>>(8*(n-1-i)))&0xff); err != nil {
				return err
			}
		}
	}

	updateNZClearVC(cpu, value, size)
	return nil
}

func movepCycleCalculator(opcode uint16) uint32 {
	if opcode&0x0040 != 0 { // long
		return 24
	}
	return 16
}

func registerLeaPea(b *tableBuilder) {
	const leaPeaAddressMask = eaMaskIndirect | eaMaskPostIncrement | eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex | eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskPCDisplacement | eaMaskPCIndex

	b.add(lea, 0x41c0, 0xf1c0, leaPeaAddressMask, leaPeaCycleCalculator(0))
	b.add(pea, 0x4840, 0xffc0, leaPeaAddressMask, leaPeaCycleCalculator(8))
}

func lea(cpu *CPU) error {
	src, err := cpu.resolveSrcEA(Long)
	if err != nil {
		return err
	}

	*ax(cpu) = src.computedAddress()
	return nil
}

func pea(cpu *CPU) error {
	src, err := cpu.resolveSrcEA(Long)
	if err != nil {
		return err
	}

	return cpu.push(Long, src.computedAddress())
}

// leaPeaCycleCalculator returns LEA's time per mode plus extra (8 for PEA's
// push), table 8-10.
func leaPeaCycleCalculator(extra uint32) cycleCalculator {
	return func(opcode uint16) uint32 {
		mode, reg := eaFields(opcode)
		return leaCycles(mode, reg) + extra
	}
}
