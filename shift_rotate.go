package m68kemu

import "fmt"

func init() {
	RegisterInstruction(shiftRotate, 0xe000, 0xf000, 0, shiftRotateCycleCalculator)
}

func shiftRotate(cpu *cpu) error {
	opcode := cpu.regs.IR

	if (opcode>>6)&0x3 == 0x3 {
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

	if registerCount {
		cpu.addCycles(uint32(count * 2))
	}

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
	updateShiftRotateFlags(cpu, result, int(size)*8, flags)
	return nil
}

func shiftRotateMemory(cpu *cpu) error {
	opcode := cpu.regs.IR
	logical := ((opcode >> 9) & 0x1) != 0
	left := ((opcode >> 8) & 0x1) != 0

	ea, err := cpu.ResolveSrcEA(Word)
	if err != nil {
		return err
	}

	val, err := ea.read()
	if err != nil {
		return err
	}

	var (
		result uint32
		flags  shiftRotateFlags
	)
	switch {
	case !logical && !left:
		result, flags = asr(uint32(val), 1, 16)
	case !logical && left:
		result, flags = asl(uint32(val), 1, 16)
	case logical && !left:
		result, flags = lsr(uint32(val), 1, 16)
	case logical && left:
		result, flags = lsl(uint32(val), 1, 16)
	}

	if err := ea.write(result); err != nil {
		return err
	}

	updateShiftRotateFlags(cpu, result, 16, flags)
	return nil
}

type shiftRotateFlags struct {
	carryOut     uint32
	changeCarry  bool
	extendOut    bool
	changeExtend bool
	overflow     bool
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
			return asl(value, count, width)
		}
		return asr(value, count, width)
	case 1: // logical shift
		if left {
			return lsl(value, count, width)
		}
		return lsr(value, count, width)
	case 2: // rotate with extend
		if left {
			return roxl(value, count, width, extend)
		}
		return roxr(value, count, width, extend)
	case 3: // rotate
		if left {
			return rol(value, count, width)
		}
		return ror(value, count, width)
	default:
		return value, shiftRotateFlags{}
	}
}

func asl(value uint32, count, width int) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask
	var carry uint32
	for i := 0; i < count; i++ {
		carry = (value >> (width - 1)) & 1
		value = (value << 1) & mask
	}
	newMsb := (value >> (width - 1)) & 1
	return value, shiftRotateFlags{
		carryOut:     carry,
		changeCarry:  true,
		extendOut:    carry != 0,
		changeExtend: true,
		overflow:     (carry ^ newMsb) != 0,
	}
}

func asr(value uint32, count, width int) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask
	sign := value & (1 << (width - 1))
	var carry uint32
	for i := 0; i < count; i++ {
		carry = value & 1
		value >>= 1
		if sign != 0 {
			value |= 1 << (width - 1)
		}
	}
	return value & mask, shiftRotateFlags{carryOut: carry, changeCarry: true, extendOut: carry != 0, changeExtend: true}
}

func lsl(value uint32, count, width int) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask
	var carry uint32
	for i := 0; i < count; i++ {
		carry = (value >> (width - 1)) & 1
		value = (value << 1) & mask
	}
	return value, shiftRotateFlags{carryOut: carry, changeCarry: true, extendOut: carry != 0, changeExtend: true}
}

func lsr(value uint32, count, width int) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask
	var carry uint32
	for i := 0; i < count; i++ {
		carry = value & 1
		value >>= 1
	}
	return value, shiftRotateFlags{carryOut: carry, changeCarry: true, extendOut: carry != 0, changeExtend: true}
}

func roxl(value uint32, count, width int, extend bool) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask
	shift := count % (width + 1)
	var carry uint32
	for i := 0; i < shift; i++ {
		carry = b2i(extend)
		extend = (value>>(width-1))&1 != 0
		value = ((value << 1) | carry) & mask
	}
	return value, shiftRotateFlags{carryOut: b2i(extend), changeCarry: true, extendOut: extend, changeExtend: true}
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
	return value, shiftRotateFlags{carryOut: b2i(extend), changeCarry: true, extendOut: extend, changeExtend: true}
}

func rol(value uint32, count, width int) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask
	shift := count % width
	if shift == 0 {
		return value, shiftRotateFlags{carryOut: ^uint32(0)}
	}
	carry := (value >> (width - shift)) & 1
	result := ((value << shift) | (value >> (width - shift))) & mask
	return result, shiftRotateFlags{carryOut: carry, changeCarry: true}
}

func ror(value uint32, count, width int) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask
	shift := count % width
	if shift == 0 {
		return value, shiftRotateFlags{carryOut: ^uint32(0)}
	}
	carry := (value >> (shift - 1)) & 1
	result := ((value >> shift) | (value << (width - shift))) & mask
	return result, shiftRotateFlags{carryOut: carry, changeCarry: true}
}

func updateShiftRotateFlags(cpu *cpu, result uint32, width int, flags shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	result &= mask

	cpu.regs.SR &^= srZero | srNegative | srOverflow
	if result == 0 {
		cpu.regs.SR |= srZero
	}
	if result&(1<<(width-1)) != 0 {
		cpu.regs.SR |= srNegative
	}

	if flags.changeCarry {
		cpu.regs.SR &^= srCarry
		if flags.carryOut != 0 {
			cpu.regs.SR |= srCarry
		}
	}
	if flags.changeExtend {
		cpu.regs.SR &^= srExtend
		if flags.extendOut {
			cpu.regs.SR |= srExtend
		}
	}
	if flags.overflow {
		cpu.regs.SR |= srOverflow
	}
}

func b2i(v bool) uint32 {
	if v {
		return 1
	}
	return 0
}

func shiftMemoryCycles(ir uint16) uint32 {
	mode := (ir >> 3) & 0x7
	reg := ir & 0x7
	return 8 + eaAccessCycles(mode, reg, Word)
}

func shiftRegisterCycleCalculator(opcode uint16) uint32 {
	operation := int((opcode >> 3) & 0x7)
	registerCount := operation >= 4
	if registerCount {
		return 6
	}

	countField := int((opcode >> 9) & 0x7)
	if countField == 0 {
		countField = 8
	}
	return 6 + uint32(countField*2)
}

func shiftMemoryCycleCalculator(opcode uint16) uint32 {
	return shiftMemoryCycles(opcode)
}

func shiftRotateCycleCalculator(opcode uint16) uint32 {
	if (opcode>>6)&0x7 == 0x7 {
		return shiftMemoryCycleCalculator(opcode)
	}
	return shiftRegisterCycleCalculator(opcode)
}
