package emu

import "fmt"

func init() {
	RegisterInstruction(shiftRotate, 0xe000, 0xf000, 0)
}

func shiftRotate(cpu *CPU) error {
	opcode := cpu.regs.IR

	if (opcode>>6)&0x7 == 0x3 {
		return shiftRotateMemory(cpu)
	}

	countField := int((opcode >> 9) & 0x7)
	left := (opcode>>8)&0x1 == 1
	sizeField := (opcode >> 6) & 0x3
	operation := int((opcode >> 3) & 0x7)
	register := int(opcode & 0x7)

	registerCount := false
	if operation >= 4 {
		operation -= 4
		registerCount = true
	}

	var count int
	if registerCount {
		count = int(cpu.regs.D[countField] & 0x3f)
	} else {
		count = countField
		if count == 0 {
			count = 8
		}
	}

	cpu.addCycles(shiftRegisterCycles(count))

	var size Size
	var mask uint32
	switch sizeField {
	case 0:
		size, mask = Byte, 0xff
	case 1:
		size, mask = Word, 0xffff
	case 2:
		size, mask = Long, 0xffffffff
	default:
		return fmt.Errorf("illegal instruction %04x", opcode)
	}

	value := cpu.regs.D[register] & int32(mask)
	result, flags := doShiftRotate(uint32(value), count, int(size)*8, operation, left, cpu.regs.SR&srExtend != 0)

	cpu.regs.D[register] = (cpu.regs.D[register] & ^int32(mask)) | int32(result)
	updateShiftRotateFlags(cpu, result, int(size)*8, flags, count)
	return nil
}

func shiftRotateMemory(cpu *CPU) error {
	opcode := cpu.regs.IR
	op := int((opcode >> 9) & 0x7)

	cpu.addCycles(shiftMemoryCycles(opcode))

	ea, err := cpu.ResolveSrcEA(Word)
	if err != nil {
		return err
	}

	val, err := ea.read()
	if err != nil {
		return err
	}

	result, flags := doShiftRotate(uint32(val), 1, 16, op, (op&0x4) != 0, cpu.regs.SR&srExtend != 0)

	if err := ea.write(result); err != nil {
		return err
	}

	updateShiftRotateFlags(cpu, result, 16, flags, 1)
	return nil
}

type shiftRotateFlags struct {
	carryOut  uint32
	extendOut bool
	overflow  bool
}

func doShiftRotate(value uint32, count int, width int, operation int, left bool, extend bool) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask

	if count == 0 {
		return value, shiftRotateFlags{carryOut: ^uint32(0), extendOut: extend}
	}

	switch operation {
	case 0: // arithmetic shift
		if left {
			return asl(value, count, width, extend)
		}
		return asr(value, count, width, extend)
	case 1: // logical shift
		if left {
			return lsl(value, count, width, extend)
		}
		return lsr(value, count, width, extend)
	case 2: // rotate with extend
		if left {
			return roxl(value, count, width, extend)
		}
		return roxr(value, count, width, extend)
	case 3: // rotate
		if left {
			return rol(value, count, width), shiftRotateFlags{}
		}
		return ror(value, count, width), shiftRotateFlags{}
	default:
		return value, shiftRotateFlags{}
	}
}

func asl(value uint32, count, width int, extend bool) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	max := int64(1<<(width-1)) - 1
	min := -int64(1 << (width - 1))

	full := int64(signExtend(value, width)) << count
	result := uint32(full) & mask
	carry := (value >> (uint(count) % uint(width))) & 1
	if count > width {
		carry = 0
	}
	overflow := full > max || full < min
	return result, shiftRotateFlags{carryOut: carry, extendOut: carry != 0, overflow: overflow}
}

func asr(value uint32, count, width int, extend bool) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask

	signed := signExtend(value, width)
	carry := (value >> (count - 1)) & 1
	if count >= width {
		carry = (value >> (width - 1)) & 1
	}
	result := uint32(signed >> count)
	result &= mask
	return result, shiftRotateFlags{carryOut: carry, extendOut: carry != 0}
}

func lsl(value uint32, count, width int, extend bool) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask

	shift := count % width
	carry := (value >> (width - shift)) & 1
	if shift == 0 {
		carry = 0
	}
	result := (value << shift) & mask
	return result, shiftRotateFlags{carryOut: carry, extendOut: carry != 0}
}

func lsr(value uint32, count, width int, extend bool) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask

	shift := count % width
	carry := (value >> (shift - 1)) & 1
	if shift == 0 {
		carry = 0
	}
	result := value >> shift
	return result, shiftRotateFlags{carryOut: carry, extendOut: carry != 0}
}

func roxl(value uint32, count, width int, extend bool) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask

	shift := count % (width + 1)
	for i := 0; i < shift; i++ {
		carry := extend
		extend = (value>>(width-1))&1 != 0
		value = ((value << 1) | b2i(carry)) & mask
	}
	return value, shiftRotateFlags{carryOut: b2i(extend), extendOut: extend}
}

func roxr(value uint32, count, width int, extend bool) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask

	shift := count % (width + 1)
	for i := 0; i < shift; i++ {
		carry := extend
		extend = value&1 != 0
		value = (value >> 1) | (b2i(carry) << (width - 1))
	}
	return value, shiftRotateFlags{carryOut: b2i(extend), extendOut: extend}
}

func rol(value uint32, count, width int) uint32 {
	mask := uint32((1 << width) - 1)
	value &= mask
	shift := count % width
	return ((value << shift) | (value >> (width - shift))) & mask
}

func ror(value uint32, count, width int) uint32 {
	mask := uint32((1 << width) - 1)
	value &= mask
	shift := count % width
	return ((value >> shift) | (value << (width - shift))) & mask
}

func updateShiftRotateFlags(cpu *CPU, result uint32, width int, flags shiftRotateFlags, count int) {
	mask := uint32((1 << width) - 1)
	result &= mask

	// When count is zero for register operations, C and X remain unchanged.
	if count == 0 {
		cpu.regs.SR &^= srOverflow
		cpu.regs.SR &^= srNegative | srZero
		if result == 0 {
			cpu.regs.SR |= srZero
		}
		if result&(1<<(width-1)) != 0 {
			cpu.regs.SR |= srNegative
		}
		return
	}

	cpu.regs.SR &^= srCarry | srZero | srNegative | srOverflow
	if result == 0 {
		cpu.regs.SR |= srZero
	}
	if result&(1<<(width-1)) != 0 {
		cpu.regs.SR |= srNegative
	}
	if flags.carryOut != ^uint32(0) {
		if flags.carryOut != 0 {
			cpu.regs.SR |= srCarry
		}
		if flags.extendOut {
			cpu.regs.SR |= srExtend
		} else {
			cpu.regs.SR &^= srExtend
		}
	}
	if flags.overflow {
		cpu.regs.SR |= srOverflow
	}
}

func signExtend(value uint32, width int) int32 {
	shift := 32 - width
	return int32(value<<shift) >> shift
}

func b2i(v bool) uint32 {
	if v {
		return 1
	}
	return 0
}
