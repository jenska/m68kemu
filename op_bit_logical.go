package m68kemu

import "fmt"

func init() {
	logicalSourceMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskPCDisplacement | eaMaskPCIndex |
		eaMaskImmediate
	logicalDestinationMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong

	registerLogicalOpmodes(
		opmodeRegistration{andInstruction, 0xc000, 0, 2, 0xf1c0, logicalSourceMask, logicalCycles},
		opmodeRegistration{andInstruction, 0xc000, 4, 6, 0xf1c0, logicalDestinationMask, logicalCycles},
		opmodeRegistration{orInstruction, 0x8000, 0, 2, 0xf1c0, logicalSourceMask, logicalCycles},
		opmodeRegistration{orInstruction, 0x8000, 4, 6, 0xf1c0, logicalDestinationMask, logicalCycles},
		opmodeRegistration{eorInstruction, 0xb100, 0, 2, 0xf1c0, logicalDestinationMask, logicalCycles},
	)

	// Immediate logical operations.
	immediateMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong
	registerOpmodes(
		opmodeRegistration{oriImmediate, 0x0000, 0, 2, 0xffc0, immediateMask, immediateEACycles},
		opmodeRegistration{andiImmediate, 0x0200, 0, 2, 0xffc0, immediateMask, immediateEACycles},
		opmodeRegistration{eoriImmediate, 0x0a00, 0, 2, 0xffc0, immediateMask, immediateEACycles},
	)

	// NOT <ea>
	notMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement | eaMaskPreDecrement |
		eaMaskDisplacement | eaMaskIndex | eaMaskAbsoluteShort | eaMaskAbsoluteLong
	registerOpmodes(opmodeRegistration{notInstruction, 0x4600, 0, 2, 0xffc0, notMask, clrTstCycles})
}

// registerLogicalInstruction behaves like registerInstruction but skips opcode slots that
// are already reserved by other instructions (for example, ABCD).
func registerLogicalInstruction(ins instruction, match, mask uint16, eaMask uint16, calc cycleCalculator) {
	for value := uint16(0); ; {
		index := match | value
		if (index & 0xf1f8) == 0xc100 { // Reserved for ABCD.
			value = ((value | mask) + 1) & ^mask
			if value == 0 {
				break
			}
			continue
		}

		if validEA(index, eaMask) {
			if opcodeTable[index] == nil {
				opcodeTable[index] = ins
				if calc != nil {
					opcodeCycleTable[index] = calc(index)
				}
			}
		}

		value = ((value | mask) + 1) & ^mask
		if value == 0 {
			break
		}
	}
}

func registerLogicalOpmodes(regs ...opmodeRegistration) {
	for _, reg := range regs {
		for opmode := reg.first; opmode <= reg.last; opmode++ {
			registerLogicalInstruction(reg.ins, reg.base|(opmode<<6), reg.mask, reg.ea, reg.calc)
		}
	}
}

func logicalCycles(opcode uint16) uint32 {
	opmode := (opcode >> 6) & 0x7
	base := uint32(4)
	if opmode >= 4 {
		base = 8
	}
	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7
	return base + eaAccessCycles(mode, reg, operandSizeFromOpmode(opmode))
}

func logicalUpdateFlags(cpu *CPU, result uint32, size Size) {
	updateNZClearVC(cpu, result, size)
}

func andInstruction(cpu *CPU) error {
	opmode := (cpu.regs.IR >> 6) & 0x7
	size := operandSizeFromOpmode(opmode)

	if opmode >= 4 {
		dst, err := cpu.ResolveSrcEA(size)
		if err != nil {
			return err
		}
		dstVal, err := dst.read()
		if err != nil {
			return err
		}

		src := *udx(cpu) & size.mask()
		result := (src & dstVal) & size.mask()
		if err := dst.write(result); err != nil {
			return err
		}
		logicalUpdateFlags(cpu, result, size)
		return nil
	}

	src, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	srcVal, err := src.read()
	if err != nil {
		return err
	}

	dst := udx(cpu)
	result := (srcVal & (*dst & size.mask())) & size.mask()
	*dst = (*dst & ^size.mask()) | result

	logicalUpdateFlags(cpu, result, size)
	return nil
}

func orInstruction(cpu *CPU) error {
	opmode := (cpu.regs.IR >> 6) & 0x7
	size := operandSizeFromOpmode(opmode)

	if opmode >= 4 {
		dst, err := cpu.ResolveSrcEA(size)
		if err != nil {
			return err
		}
		dstVal, err := dst.read()
		if err != nil {
			return err
		}

		src := *udx(cpu) & size.mask()
		result := (src | dstVal) & size.mask()
		if err := dst.write(result); err != nil {
			return err
		}
		logicalUpdateFlags(cpu, result, size)
		return nil
	}

	src, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	srcVal, err := src.read()
	if err != nil {
		return err
	}

	dst := udx(cpu)
	result := (srcVal | (*dst & size.mask())) & size.mask()
	*dst = (*dst & ^size.mask()) | result

	logicalUpdateFlags(cpu, result, size)
	return nil
}

func eorInstruction(cpu *CPU) error {
	size := operandSizeFromOpcode(cpu.regs.IR)

	dst, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	dstVal, err := dst.read()
	if err != nil {
		return err
	}

	src := *udx(cpu) & size.mask()
	result := (src ^ dstVal) & size.mask()
	if err := dst.write(result); err != nil {
		return err
	}

	logicalUpdateFlags(cpu, result, size)
	return nil
}

func logicalImmediate(cpu *CPU, op func(uint32, uint32) uint32) error {
	size := operandSizeFromOpcode(cpu.regs.IR)
	immSize := size
	if size == Byte {
		immSize = Word
	}

	srcVal, err := cpu.popPc(immSize)
	if err != nil {
		return err
	}
	srcVal &= size.mask()

	dst, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	dstVal, err := dst.read()
	if err != nil {
		return err
	}

	result := op(srcVal, dstVal) & size.mask()
	if err := dst.write(result); err != nil {
		return err
	}

	logicalUpdateFlags(cpu, result, size)
	return nil
}

func oriImmediate(cpu *CPU) error {
	return logicalImmediate(cpu, func(a, b uint32) uint32 { return a | b })
}

func andiImmediate(cpu *CPU) error {
	return logicalImmediate(cpu, func(a, b uint32) uint32 { return a & b })
}

func eoriImmediate(cpu *CPU) error {
	return logicalImmediate(cpu, func(a, b uint32) uint32 { return a ^ b })
}

func notInstruction(cpu *CPU) error {
	size := operandSizeFromOpcode(cpu.regs.IR)

	dst, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	value, err := dst.read()
	if err != nil {
		return err
	}

	result := (^value) & size.mask()
	if err := dst.write(result); err != nil {
		return err
	}

	logicalUpdateFlags(cpu, result, size)
	return nil
}

func init() {
	// Valid bit-instruction destinations include data registers plus the memory
	// forms this emulator supports, including PC-relative operands.
	bitOperandMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong |
		eaMaskPCDisplacement | eaMaskPCIndex

	registerOpmodes(
		opmodeRegistration{bitDynamic, 0x0100, 4, 7, 0xf1c0, bitOperandMask, bitCycles},
		opmodeRegistration{bitImmediate, 0x0800, 0, 3, 0xffc0, bitOperandMask, bitCycles},
	)
}

func bitCycles(opcode uint16) uint32 {
	immediate := opcode&0x0800 != 0
	op := (opcode >> 6) & 0x3
	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7

	if mode == 0 {
		if op == 0 {
			if immediate {
				return 8
			}
			return 4
		}
		if immediate {
			return 12
		}
		return 8
	}

	ea := eaAccessCycles(mode, reg, Byte)
	if op == 0 {
		if immediate {
			return 12 + ea
		}
		return 8 + ea
	}
	if immediate {
		return 16 + ea
	}
	return 12 + ea
}

func bitDynamic(cpu *CPU) error {
	index := (cpu.regs.IR >> 9) & 0x7
	bitNumber := uint32(cpu.regs.D[index])
	mode := (cpu.regs.IR >> 3) & 0x7

	var dst modifier
	if mode != 0 {
		var err error
		dst, err = cpu.ResolveSrcEA(Byte)
		if err != nil {
			return err
		}
	}

	return bitOperation(cpu, bitNumber, mode, dst)
}

func bitImmediate(cpu *CPU) error {
	mode := (cpu.regs.IR >> 3) & 0x7

	imm, err := cpu.popPc(Word)
	if err != nil {
		return err
	}

	var dst modifier
	if mode != 0 {
		var err error
		dst, err = cpu.ResolveSrcEA(Byte)
		if err != nil {
			return err
		}
	}

	return bitOperation(cpu, imm, mode, dst)
}

func bitOperation(cpu *CPU, bitNumber uint32, mode uint16, dst modifier) error {
	op := (cpu.regs.IR >> 6) & 0x7
	opType := op & 0x3 // 0=BTST, 1=BCHG, 2=BCLR, 3=BSET

	cpu.regs.SR &^= srZero

	if mode == 0 { // data register destination (long size)
		dst := dy(cpu)
		mask := uint32(1) << (bitNumber & 31)
		previousSet := (*dst & mask) != 0

		switch opType {
		case 1: // BCHG
			*dst ^= mask
		case 2: // BCLR
			*dst &^= mask
		case 3: // BSET
			*dst |= mask
		}

		if !previousSet {
			cpu.regs.SR |= srZero
		}
		return nil
	}

	if dst == nil {
		var err error
		dst, err = cpu.ResolveSrcEA(Byte)
		if err != nil {
			return err
		}
	}

	value, err := dst.read()
	if err != nil {
		return err
	}

	mask := uint32(1) << (bitNumber & 7)
	previousSet := (value & mask) != 0

	switch opType {
	case 1: // BCHG
		value ^= mask
	case 2: // BCLR
		value &^= mask
	case 3: // BSET
		value |= mask
	}

	if opType != 0 {
		if err := dst.write(value); err != nil {
			return err
		}
	}

	if !previousSet {
		cpu.regs.SR |= srZero
	}
	return nil
}

func init() {
	registerInstructions(instructionRegistration{shiftRotate, 0xe000, 0xf000, 0, shiftRotateCycles})
}

func shiftRotate(cpu *CPU) error {
	opcode := cpu.regs.IR

	if (opcode>>6)&0x3 == 0x3 {
		return shiftRotateMemory(cpu)
	}

	countField := int((opcode >> 9) & 0x7)
	left := (opcode>>8)&0x1 == 1
	sizeField := (opcode >> 6) & 0x3
	operation := (opcode >> 3) & 0x7
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

func shiftRotateMemory(cpu *CPU) error {
	opcode := cpu.regs.IR
	operation := (opcode >> 9) & 0x3
	left := ((opcode >> 8) & 0x1) != 0

	ea, err := cpu.ResolveSrcEA(Word)
	if err != nil {
		return err
	}

	val, err := ea.read()
	if err != nil {
		return err
	}

	result, flags := doShiftRotate(uint32(val), 1, 16, operation, left, cpu.regs.SR&srExtend != 0)
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

func doShiftRotate(value uint32, count int, width int, operation uint16, left bool, extend bool) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask

	if count == 0 {
		c := uint32(0)
		if operation == 2 { // ROXL/ROXR sets C=X
			c = b2i(extend)
		}
		return value, shiftRotateFlags{carryOut: c, changeCarry: true, extendOut: extend}
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
	overflow := false
	for range count {
		carry = (value >> (width - 1)) & 1
		value = (value << 1) & mask
		newMsb := (value >> (width - 1)) & 1
		if carry != newMsb {
			overflow = true
		}
	}
	return value, shiftRotateFlags{
		carryOut:     carry,
		changeCarry:  true,
		extendOut:    carry != 0,
		changeExtend: true,
		overflow:     overflow,
	}
}

func asr(value uint32, count, width int) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask
	sign := value & (1 << (width - 1))
	var carry uint32
	for range count {
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
	for range count {
		carry = (value >> (width - 1)) & 1
		value = (value << 1) & mask
	}
	return value, shiftRotateFlags{carryOut: carry, changeCarry: true, extendOut: carry != 0, changeExtend: true}
}

func lsr(value uint32, count, width int) (uint32, shiftRotateFlags) {
	mask := uint32((1 << width) - 1)
	value &= mask
	var carry uint32
	for range count {
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
	for range shift {
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
	for range shift {
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
		carry := value & 1
		return value, shiftRotateFlags{carryOut: carry, changeCarry: true}
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
		carry := (value >> (width - 1)) & 1
		return value, shiftRotateFlags{carryOut: carry, changeCarry: true}
	}
	carry := (value >> (shift - 1)) & 1
	result := ((value >> shift) | (value << (width - shift))) & mask
	return result, shiftRotateFlags{carryOut: carry, changeCarry: true}
}

func updateShiftRotateFlags(cpu *CPU, result uint32, width int, flags shiftRotateFlags) {
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

func shiftRegisterCycles(opcode uint16) uint32 {
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

func shiftRotateCycles(opcode uint16) uint32 {
	if (opcode>>6)&0x7 == 0x7 {
		return shiftMemoryCycles(opcode)
	}
	return shiftRegisterCycles(opcode)
}
