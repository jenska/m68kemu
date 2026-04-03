package m68kemu

import (
	"fmt"
	"math/bits"
)

func init() {
	registerMove(moveb, 0x1000, moveCycleCalculator(Byte))
	registerMove(movew, 0x3000, moveCycleCalculator(Word))
	registerMove(movel, 0x2000, moveCycleCalculator(Long))
	registerMoveA(0x3000, moveaw, moveAddressCycleCalculator(Word))
	registerMoveA(0x2000, moveal, moveAddressCycleCalculator(Long))
	registerInstruction(moveq, 0x7000, 0xf100, 0, constantCycles(4))
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

func moveq(cpu *cpu) error {
	value := int32(int8(cpu.regs.IR))
	*dx(cpu) = value

	updateNZClearVC(cpu, uint32(value), Long)
	return nil
}

func registerMoveA(base uint16, handler instruction, calc cycleCalculator) {
	const dstMode = uint16(1)
	for dstReg := uint16(0); dstReg < 8; dstReg++ {
		match := base | (dstReg << 9) | (dstMode << 6)
		registerInstruction(handler, match, 0xffc0, moveSourceEAMask, calc)
	}
}

func registerMove(ins instruction, base uint16, calc cycleCalculator) {
	for dstMode := uint16(0); dstMode < 8; dstMode++ {
		// Address register destinations are handled by MOVEA.
		if dstMode == 1 {
			continue
		}
		for dstReg := uint16(0); dstReg < 8; dstReg++ {
			// Destination must be alterable: exclude PC relative and immediate forms.
			if dstMode == 7 && (dstReg == 2 || dstReg == 3 || dstReg == 4) {
				continue
			}
			match := base | (dstReg << 9) | (dstMode << 6)
			registerInstruction(ins, match, 0xffc0, moveSourceEAMask, calc)
		}
	}
}

func moveb(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Byte)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	dst, err := cpu.ResolveDstEA(Byte)
	if err != nil {
		return err
	}

	if err := dst.write(value); err != nil {
		return err
	}

	updateNZClearVC(cpu, value, Byte)
	return nil
}

func movew(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Word)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	dst, err := cpu.ResolveDstEA(Word)
	if err != nil {
		return err
	}

	if err := dst.write(value); err != nil {
		return err
	}

	updateNZClearVC(cpu, value, Word)
	return nil
}

func movel(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return err
	}
	value, err := src.read()
	if err != nil {
		return err
	}

	dst, err := cpu.ResolveDstEA(Long)
	if err != nil {
		return err
	}

	if err := dst.write(value); err != nil {
		return err
	}

	updateNZClearVC(cpu, value, Long)
	return nil
}

func moveaw(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Word)
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

func moveal(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Long)
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

func moveCycles(ir uint16, size Size) uint32 {
	srcMode := (ir >> 3) & 0x7
	srcReg := ir & 0x7
	dstMode := (ir >> 6) & 0x7
	dstReg := (ir >> 9) & 0x7

	return 4 + eaAccessCycles(srcMode, srcReg, size) + eaAccessCycles(dstMode, dstReg, size)
}

func moveAddressCycles(ir uint16, size Size) uint32 {
	srcMode := (ir >> 3) & 0x7
	srcReg := ir & 0x7
	return 4 + eaAccessCycles(srcMode, srcReg, size)
}

func moveCycleCalculator(size Size) cycleCalculator {
	return func(opcode uint16) uint32 {
		return moveCycles(opcode, size)
	}
}

func moveAddressCycleCalculator(size Size) cycleCalculator {
	return func(opcode uint16) uint32 {
		return moveAddressCycles(opcode, size)
	}
}

// MOVEM: move multiple registers to/from memory
func init() {
	// Register-to-memory: MOVEM.<size> <register list>,<ea>
	for _, sizeBits := range []uint16{2 << 6, 3 << 6} {
		registerMovemInstruction(0x4800, sizeBits, movemToMemory, eaMaskIndirect|eaMaskPreDecrement|eaMaskDisplacement|
			eaMaskIndex|eaMaskAbsoluteShort|eaMaskAbsoluteLong)
		registerMovemInstruction(0x4c00, sizeBits, movemToRegisters, eaMaskIndirect|eaMaskPostIncrement|
			eaMaskDisplacement|eaMaskIndex|eaMaskAbsoluteShort|eaMaskAbsoluteLong|eaMaskPCDisplacement|eaMaskPCIndex)
	}
}

func registerMovemInstruction(match, sizeBits uint16, ins instruction, eaMask uint16) {
	for mode := uint16(0); mode < 8; mode++ {
		for reg := uint16(0); reg < 8; reg++ {
			opcode := match | sizeBits | (mode << 3) | reg
			if !validEA(opcode, eaMask) {
				continue
			}
			if opcodeTable[opcode] != nil {
				panic(fmt.Errorf("instruction 0x%04x already registered", opcode))
			}
			opcodeTable[opcode] = ins
		}
	}
}

func movemSize(opcode uint16) (Size, bool) {
	switch (opcode >> 6) & 0x3 {
	case 2:
		return Word, true
	case 3:
		return Long, true
	default:
		return 0, false
	}
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

	for reg := 0; reg < 16; reg++ {
		if mask&(1<<reg) != 0 {
			order = append(order, reg)
		}
	}
	return order
}

func movemReadAddress(cpu *cpu, mode, reg uint16) (uint32, error) {
	var addr uint32

	switch mode {
	case 2: // (An)
		addr = cpu.regs.A[reg]
	case 3: // (An)+
		addr = cpu.regs.A[reg]
	case 4: // -(An)
		addr = cpu.regs.A[reg]
	case 5: // (d16,An)
		ext, err := cpu.popPc(Word)
		if err != nil {
			return 0, err
		}
		addr = uint32(int32(cpu.regs.A[reg]) + int32(int16(ext)))
	case 6: // (d8,An,Xn)
		a := cpu.regs.A[reg]
		extAddr, err := ix68000(cpu, a)
		if err != nil {
			return 0, err
		}
		addr = extAddr
	case 7:
		switch reg {
		case 0: // (xxx).W
			ext, err := cpu.popPc(Word)
			if err != nil {
				return 0, err
			}
			addr = uint32(int32(int16(ext)))
		case 1: // (xxx).L
			ext, err := cpu.popPc(Long)
			if err != nil {
				return 0, err
			}
			addr = ext
		case 2: // (d16,PC)
			ext, err := cpu.popPc(Word)
			if err != nil {
				return 0, err
			}
			addr = uint32(int32(cpu.regs.PC) + int32(int16(ext)))
		case 3: // (d8,PC,Xn)
			pc := cpu.regs.PC
			extAddr, err := ix68000(cpu, pc)
			if err != nil {
				return 0, err
			}
			addr = extAddr
		default:
			return 0, cpu.exception(XIllegal)
		}
	default:
		return 0, cpu.exception(XIllegal)
	}

	return addr, nil
}

func movemToRegisters(cpu *cpu) error {
	opcode := cpu.regs.IR
	size, ok := movemSize(opcode)
	if !ok {
		return cpu.exception(XIllegal)
	}

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
	// 12 cycles base + 4 cycles per register transferred
	cpu.addCycles(12 + 4*uint32(len(regs)))

	sizeBytes := uint32(size)

	for _, r := range regs {
		if mode == 4 { // predecrement reads from decremented address
			addr -= sizeBytes
		}

		value, err := cpu.read(size, addr)
		if err != nil {
			return err
		}

		if r < 8 {
			if size == Word {
				cpu.regs.D[r] = int32(int16(value))
			} else {
				cpu.regs.D[r] = int32(value)
			}
		} else {
			if size == Word {
				cpu.regs.A[r-8] = uint32(int32(int16(value)))
			} else {
				cpu.regs.A[r-8] = value
			}
		}

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

func movemToMemory(cpu *cpu) error {
	opcode := cpu.regs.IR
	size, ok := movemSize(opcode)
	if !ok {
		return cpu.exception(XIllegal)
	}

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
	// 8 cycles base + 4 cycles per register transferred
	cpu.addCycles(8 + 4*uint32(len(regs)))

	sizeBytes := uint32(size)

	for _, r := range regs {
		if mode == 4 { // predecrement destination
			addr -= sizeBytes
		}

		var value uint32
		if r < 8 {
			value = uint32(cpu.regs.D[r])
		} else {
			value = cpu.regs.A[r-8]
		}

		if err := cpu.write(size, addr, value); err != nil {
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

func init() {
	registerInstruction(movep, 0x0108, 0xf1f8, 0, movepCycleCalculator)
	registerInstruction(movep, 0x0148, 0xf1f8, 0, movepCycleCalculator)
	registerInstruction(movep, 0x0188, 0xf1f8, 0, movepCycleCalculator)
	registerInstruction(movep, 0x01c8, 0xf1f8, 0, movepCycleCalculator)
}

func movep(cpu *cpu) error {
	opcode := cpu.regs.IR
	size := Word
	if opcode&0x0080 != 0 {
		size = Long
	}
	toRegister := opcode&0x0040 != 0

	disp, err := cpu.popPc(Word)
	if err != nil {
		return err
	}

	addr := uint32(int32(cpu.regs.A[opcode&0x7]) + int32(int16(disp)))
	reg := (opcode >> 9) & 0x7
	mask := size.mask()

	var value uint32
	if toRegister {
		if size == Word {
			high, err := cpu.read(Byte, addr)
			if err != nil {
				return err
			}
			low, err := cpu.read(Byte, addr+2)
			if err != nil {
				return err
			}
			value = (high << 8) | low
		} else {
			b3, err := cpu.read(Byte, addr)
			if err != nil {
				return err
			}
			b2, err := cpu.read(Byte, addr+2)
			if err != nil {
				return err
			}
			b1, err := cpu.read(Byte, addr+4)
			if err != nil {
				return err
			}
			b0, err := cpu.read(Byte, addr+6)
			if err != nil {
				return err
			}
			value = (b3 << 24) | (b2 << 16) | (b1 << 8) | b0
		}
		cpu.regs.D[reg] = (cpu.regs.D[reg] & ^int32(mask)) | int32(value&mask)
	} else {
		value = uint32(cpu.regs.D[reg]) & mask
		if size == Word {
			if err := cpu.write(Byte, addr, (value>>8)&0xff); err != nil {
				return err
			}
			if err := cpu.write(Byte, addr+2, value&0xff); err != nil {
				return err
			}
		} else {
			if err := cpu.write(Byte, addr, (value>>24)&0xff); err != nil {
				return err
			}
			if err := cpu.write(Byte, addr+2, (value>>16)&0xff); err != nil {
				return err
			}
			if err := cpu.write(Byte, addr+4, (value>>8)&0xff); err != nil {
				return err
			}
			if err := cpu.write(Byte, addr+6, value&0xff); err != nil {
				return err
			}
		}
	}

	updateNZClearVC(cpu, value, size)
	return nil
}

func movepCycleCalculator(opcode uint16) uint32 {
	if opcode&0x0080 != 0 {
		return 24
	}
	return 16
}

func init() {
	const leaPeaAddressMask = eaMaskIndirect | eaMaskPostIncrement | eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex | eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskPCDisplacement | eaMaskPCIndex

	registerInstruction(lea, 0x41c0, 0xf1c0, leaPeaAddressMask, leaPeaCycleCalculator(4))
	registerInstruction(pea, 0x4840, 0xffc0, leaPeaAddressMask, leaPeaCycleCalculator(8))
}

func lea(cpu *cpu) error {
	mode := (cpu.regs.IR >> 3) & 0x7
	reg := cpu.regs.IR & 0x7
	if mode < 2 || (mode == 7 && reg == 4) {
		return fmt.Errorf("invalid addressing mode for LEA")
	}

	src, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return err
	}

	*ax(cpu) = src.computedAddress()
	return nil
}

func pea(cpu *cpu) error {
	mode := (cpu.regs.IR >> 3) & 0x7
	reg := cpu.regs.IR & 0x7
	if mode < 2 || (mode == 7 && reg == 4) {
		return fmt.Errorf("invalid addressing mode for PEA")
	}

	src, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return err
	}

	return cpu.push(Long, src.computedAddress())
}

func leaPeaCycles(ir uint16, base uint32) uint32 {
	mode := (ir >> 3) & 0x7
	reg := ir & 0x7
	return base + eaAccessCycles(mode, reg, Long)
}

func leaPeaCycleCalculator(base uint32) cycleCalculator {
	return func(opcode uint16) uint32 {
		return leaPeaCycles(opcode, base)
	}
}
