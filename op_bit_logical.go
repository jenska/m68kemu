package m68kemu

import "fmt"

func registerLogical(b *tableBuilder) {
	logicalSourceMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskPCDisplacement | eaMaskPCIndex |
		eaMaskImmediate
	logicalDestinationMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong

	// AND <ea>,Dn
	for opmode := uint16(0); opmode <= 2; opmode++ {
		match := uint16(0xc000) | (opmode << 6)
		registerLogicalInstruction(b, andInstruction, match, 0xf1c0, logicalSourceMask, logicalCycleCalculator(opmode, false))
	}

	// AND Dn,<ea>
	for opmode := uint16(4); opmode <= 6; opmode++ {
		match := uint16(0xc000) | (opmode << 6)
		registerLogicalInstruction(b, andInstruction, match, 0xf1c0, logicalDestinationMask, logicalCycleCalculator(opmode, true))
	}

	// OR <ea>,Dn
	for opmode := uint16(0); opmode <= 2; opmode++ {
		match := uint16(0x8000) | (opmode << 6)
		registerLogicalInstruction(b, orInstruction, match, 0xf1c0, logicalSourceMask, logicalCycleCalculator(opmode, false))
	}

	// OR Dn,<ea>
	for opmode := uint16(4); opmode <= 6; opmode++ {
		match := uint16(0x8000) | (opmode << 6)
		registerLogicalInstruction(b, orInstruction, match, 0xf1c0, logicalDestinationMask, logicalCycleCalculator(opmode, true))
	}

	// EOR Dn,<ea>
	for size := range uint16(3) {
		match := uint16(0xb100) | (size << 6)
		registerLogicalInstruction(b, eorInstruction, match, 0xf1c0, logicalDestinationMask, logicalCycleCalculator(size, true))
	}

	// Immediate logical operations.
	immediateMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong
	for size := range uint16(3) {
		b.add(oriImmediate, uint16(0x0000)|(size<<6), 0xffc0, immediateMask, logicalImmediateCycleCalculator())
		b.add(andiImmediate, uint16(0x0200)|(size<<6), 0xffc0, immediateMask, logicalImmediateCycleCalculator())
		b.add(eoriImmediate, uint16(0x0a00)|(size<<6), 0xffc0, immediateMask, logicalImmediateCycleCalculator())
	}

	// NOT <ea>
	notMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement | eaMaskPreDecrement |
		eaMaskDisplacement | eaMaskIndex | eaMaskAbsoluteShort | eaMaskAbsoluteLong
	for size := range uint16(3) {
		match := uint16(0x4600) | (size << 6)
		b.add(notInstruction, match, 0xffc0, notMask, singleOperandCycleCalculator())
	}
}

// registerLogicalInstruction behaves like tableBuilder.add but skips opcode slots that
// are already reserved by other instructions (for example, ABCD).
func registerLogicalInstruction(b *tableBuilder, ins instruction, match, mask uint16, eaMask uint16, calc cycleCalculator) {
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
			if b.set.handlers[index] == nil {
				b.set.handlers[index] = ins
				if calc != nil {
					b.set.cycles[index] = calc(index)
				}
			}
		}

		value = ((value | mask) + 1) & ^mask
		if value == 0 {
			break
		}
	}
}

// logicalCycleCalculator times AND/OR <ea>,Dn and AND/OR/EOR Dn,<ea>
// (table 8-4).
func logicalCycleCalculator(size uint16, toEA bool) cycleCalculator {
	return func(opcode uint16) uint32 {
		mode, reg := eaFields(opcode)
		operandSize := operandSizeFromOpmode(size)
		if toEA {
			return readModifyWriteCycles(mode, reg, operandSize, 4, 8)
		}
		return dataToRegisterCycles(mode, reg, operandSize)
	}
}

// logicalImmediateCycleCalculator times ANDI/ORI/EORI (table 8-5).
func logicalImmediateCycleCalculator() cycleCalculator {
	return func(opcode uint16) uint32 {
		mode, reg := eaFields(opcode)
		return immediateCycles(mode, reg, operandSizeFromOpcode(opcode))
	}
}

func andInstruction(cpu *cpu) error {
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
		updateNZClearVC(cpu, result, size)
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

	updateNZClearVC(cpu, result, size)
	return nil
}

func orInstruction(cpu *cpu) error {
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
		updateNZClearVC(cpu, result, size)
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

	updateNZClearVC(cpu, result, size)
	return nil
}

func eorInstruction(cpu *cpu) error {
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

	updateNZClearVC(cpu, result, size)
	return nil
}

func logicalImmediate(cpu *cpu, op func(uint32, uint32) uint32) error {
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

	updateNZClearVC(cpu, result, size)
	return nil
}

func oriImmediate(cpu *cpu) error {
	return logicalImmediate(cpu, func(a, b uint32) uint32 { return a | b })
}

func andiImmediate(cpu *cpu) error {
	return logicalImmediate(cpu, func(a, b uint32) uint32 { return a & b })
}

func eoriImmediate(cpu *cpu) error {
	return logicalImmediate(cpu, func(a, b uint32) uint32 { return a ^ b })
}

func notInstruction(cpu *cpu) error {
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

	updateNZClearVC(cpu, result, size)
	return nil
}

func registerBitOps(b *tableBuilder) {
	// Valid bit-instruction destinations include data registers plus the memory
	// forms this emulator supports, including PC-relative operands.
	bitOperandMask := eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement |
		eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex |
		eaMaskAbsoluteShort | eaMaskAbsoluteLong |
		eaMaskPCDisplacement | eaMaskPCIndex

	// Dynamic bit number (from Dx) uses opcodes with bit 11 clear and op type
	// in bits 8-6. BTST Dn,#<data> tests an immediate; opcodesFor
	// drops the immediate and PC-relative slots of BCHG/BCLR/BSET.
	for op := range uint16(4) {
		match := uint16(0x0100) | ((op + 4) << 6)
		b.add(bitDynamic, match, 0xf1c0, bitOperandMask|eaMaskImmediate, bitCycleCalculator(false, op))
	}

	// Static bit number (immediate) uses opcodes with bit 11 set and op type in bits 8-6.
	for op := range uint16(4) {
		match := uint16(0x0800) | (op << 6)
		b.add(bitImmediate, match, 0xffc0, bitOperandMask, bitCycleCalculator(true, op))
	}
}

// bitCycleCalculator times BTST/BCHG/BCLR/BSET (table 8-8). For a data
// register destination the table holds the time for bit numbers 0-15;
// bitOperation adds 2 for bits 16-31 (except BTST).
func bitCycleCalculator(immediate bool, op uint16) cycleCalculator {
	return func(opcode uint16) uint32 {
		mode, reg := eaFields(opcode)
		if mode == 0 {
			var cycles uint32
			switch op {
			case 0: // BTST
				cycles = 6
			case 2: // BCLR
				cycles = 8
			default: // BCHG, BSET
				cycles = 6
			}
			if immediate {
				cycles += 4
			}
			return cycles
		}

		cycles := uint32(4) // BTST
		if op != 0 {
			cycles = 8 // BCHG, BCLR, BSET write the byte back
		}
		if immediate {
			cycles += 4
		}
		return cycles + eaAccessCycles(mode, reg, Byte)
	}
}

func bitDynamic(cpu *cpu) error {
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

func bitImmediate(cpu *cpu) error {
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

func bitOperation(cpu *cpu, bitNumber uint32, mode uint16, dst modifier) error {
	op := (cpu.regs.IR >> 6) & 0x7
	opType := op & 0x3 // 0=BTST, 1=BCHG, 2=BCLR, 3=BSET

	cpu.regs.SR &^= srZero

	if mode == 0 { // data register destination (long size)
		if opType != 0 && bitNumber&31 >= 16 {
			cpu.addCycles(2) // modifying the upper word takes 2 cycles more
		}
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

func registerShiftRotate(b *tableBuilder) {
	// Register forms: size field 00-10, every count/register combination.
	for size := range uint16(3) {
		b.add(shiftRotate, 0xe000|(size<<6), 0xf0c0, 0, shiftRotateCycleCalculator)
	}
	// Memory forms: size field 11, bit 11 clear (the 68020 bit-field
	// instructions use the rest), and a memory alterable operand.
	const memoryAlterable = eaMaskIndirect | eaMaskPostIncrement | eaMaskPreDecrement |
		eaMaskDisplacement | eaMaskIndex | eaMaskAbsoluteShort | eaMaskAbsoluteLong
	b.add(shiftRotate, 0xe0c0, 0xf8c0, memoryAlterable, shiftRotateCycleCalculator)
}

func shiftRotate(cpu *cpu) error {
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

func shiftRotateMemory(cpu *cpu) error {
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

// shiftRegisterCycleCalculator times a register shift or rotate (table 8-7):
// 6 (byte/word) or 8 (long) plus 2 per bit shifted. A count taken from a
// register is added by shiftRotate.
func shiftRegisterCycleCalculator(opcode uint16) uint32 {
	cycles := uint32(6)
	if (opcode>>6)&0x3 == 2 {
		cycles = 8
	}
	if (opcode>>5)&0x1 == 1 { // count in a data register
		return cycles
	}
	count := uint32((opcode >> 9) & 0x7)
	if count == 0 {
		count = 8
	}
	return cycles + 2*count
}

func shiftMemoryCycleCalculator(opcode uint16) uint32 {
	return shiftMemoryCycles(opcode)
}

func shiftRotateCycleCalculator(opcode uint16) uint32 {
	if (opcode>>6)&0x3 == 0x3 { // size field 11 selects the memory form
		return shiftMemoryCycleCalculator(opcode)
	}
	return shiftRegisterCycleCalculator(opcode)
}
