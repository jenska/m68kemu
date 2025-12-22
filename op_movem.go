package m68kemu

import (
	"fmt"
	"math/bits"
)

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
			mask := size.mask()
			cpu.regs.D[r] = (cpu.regs.D[r] & ^int32(mask)) | int32(value&mask)
		} else {
			cpu.regs.A[r-8] = value
		}

		if mode == 3 { // postincrement source
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

		if mode == 3 {
			addr += sizeBytes
		}
	}

	if mode == 3 || mode == 4 {
		cpu.regs.A[reg] = addr
	}

	return nil
}
