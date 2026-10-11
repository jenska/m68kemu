package m68kemu

import "math/bits"

// Instructions the MC68020 adds to the MC68010 set. Branches with a 32-bit
// displacement are handled by branch; CALLM and RTM are not implemented and
// raise the illegal-instruction exception.

func register68020(b *tableBuilder) {
	if b.model < M68020 {
		return
	}
	const (
		dataMask = eaMaskDataRegister | eaMaskIndirect | eaMaskPostIncrement | eaMaskPreDecrement |
			eaMaskDisplacement | eaMaskIndex | eaMaskAbsoluteShort | eaMaskAbsoluteLong |
			eaMaskImmediate | eaMaskPCDisplacement | eaMaskPCIndex
		controlMask          = eaMaskIndirect | eaMaskDisplacement | eaMaskIndex | eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskPCDisplacement | eaMaskPCIndex
		controlAlterableMask = controlMask &^ (eaMaskPCDisplacement | eaMaskPCIndex)
		memoryAlterableMask  = controlAlterableMask | eaMaskPostIncrement | eaMaskPreDecrement
	)

	// Bcc, BRA and BSR with an 8-bit displacement of $FF, which selects a
	// 32-bit displacement; registerBranches filled these slots with branch.
	b.replace(branchLong, 0x60ff, 0xf0ff, 0, constantCycles(8))
	b.add(linkLong, 0x4808, 0xfff8, 0, constantCycles(16))
	b.add(extb, 0x49c0, 0xfff8, 0, constantCycles(4))
	b.add(chkLong, 0x4100, 0xf1c0, dataMask, constantCycles(10))
	b.add(mulLong, 0x4c00, 0xffc0, dataMask, constantCycles(44))
	b.add(divLong, 0x4c40, 0xffc0, dataMask, constantCycles(84))

	// TST.W/L An, and TST and CMPI with PC-relative operands.
	b.add(tst, 0x4a48, 0xfff8, 0, tstCycleCalculator())
	b.add(tst, 0x4a88, 0xfff8, 0, tstCycleCalculator())
	for size := range uint16(3) {
		b.add(cmpi, 0x0c00|size<<6, 0xffc0, eaMaskPCDisplacement|eaMaskPCIndex, cmpiCycleCalculator())
	}

	for cond := range uint16(16) {
		for _, reg := range []uint16{2, 3, 4} { // word operand, long operand, none
			b.add(trapcc, 0x50f8|cond<<8|reg, 0xffff, 0, constantCycles(4))
		}
	}

	for _, m := range []uint16{0, 8} { // Dx,Dy and -(Ax),-(Ay)
		// OR Dn,<ea> fills these slots with register operands the
		// MC68000 does not have.
		b.replace(pack, 0x8140|m, 0xf1f8, 0, constantCycles(6))
		b.replace(unpk, 0x8180|m, 0xf1f8, 0, constantCycles(8))
	}

	for op := range uint16(8) {
		mask := uint16(eaMaskDataRegister | controlAlterableMask)
		if bitFieldReadOnly(op) {
			mask = eaMaskDataRegister | controlMask
		}
		b.add(bitField, 0xe8c0|op<<8, 0xffc0, mask, constantCycles(12))
	}

	for size := uint16(1); size <= 3; size++ {
		b.add(cas, 0x08c0|size<<9, 0xffc0, memoryAlterableMask, constantCycles(16))
	}
	b.add(cas2, 0x0cfc, 0xffff, 0, constantCycles(24))
	// F-line: the coprocessor interface. Types 6 and 7 are undefined.
	for kind := range uint16(6) {
		b.add(coprocessorInstruction, 0xf000|kind<<6, 0xf1c0, 0, constantCycles(4))
	}
	b.add(cas2, 0x0efc, 0xffff, 0, constantCycles(24))
	for size := range uint16(3) {
		b.add(chk2, 0x00c0|size<<9, 0xffc0, controlMask, constantCycles(18))
	}
}

// valid68020Addition reports whether op is one of the opcode words the
// MC68020 adds to the MC68010 map.
func valid68020Addition(op uint16) bool {
	mode := (op >> 3) & 7
	switch {
	case op&0xfff8 == 0x4808, op&0xfff8 == 0x49c0: // LINK.L, EXTB.L
		return true
	case op&0xf1c0 == 0x4100: // CHK.L
		return specIn(op, specData)
	case op&0xff80 == 0x4c00: // MULx.L, DIVx.L
		return specIn(op, specData)
	case op&0xffc0 == 0x4a40 || op&0xffc0 == 0x4a80: // TST.W/L
		return mode == 1 || specIn(op, specPCDisp|specPCIndex|specImm)
	case op&0xff00 == 0x4a00 && (op>>6)&3 == 0: // TST.B
		return specIn(op, specPCDisp|specPCIndex|specImm)
	case op&0xff00 == 0x0c00 && (op>>6)&3 != 3: // CMPI
		return specIn(op, specPCDisp|specPCIndex)
	case op&0xf0ff == 0x50fa, op&0xf0ff == 0x50fb, op&0xf0ff == 0x50fc: // TRAPcc
		return true
	case op&0xf1f0 == 0x8140, op&0xf1f0 == 0x8180: // PACK, UNPK
		return true
	case op&0xf8c0 == 0xe8c0: // bit fields
		if bitFieldReadOnly((op >> 8) & 7) {
			return mode == 0 || specIn(op, specControl)
		}
		return mode == 0 || specIn(op, specControl&^(specPCDisp|specPCIndex))
	case op == 0x0cfc || op == 0x0efc: // CAS2, before CAS whose pattern it matches
		return true
	case op&0xf9c0 == 0x08c0 && (op>>9)&3 != 0: // CAS
		return specIn(op, specMemoryAlterable)
	case op&0xf9c0 == 0x00c0 && (op>>9)&3 != 3: // CHK2, CMP2
		return specIn(op, specControl)
	case op&0xf000 == 0xf000: // coprocessor interface
		return (op>>6)&7 <= 5
	}
	return false
}

// branchLong is Bcc, BRA and BSR with a 32-bit displacement. It has its own
// handler so that branch, run by every other branch on every model, does not
// test for it.
func branchLong(cpu *cpu) error {
	cond := (cpu.regs.IR >> 8) & 0xf
	basePC := cpu.regs.PC
	displacement, err := cpu.popPc(Long)
	if err != nil {
		return err
	}
	if cond != 0x0 && cond != 0x1 && !conditionTrue(cpu, cond) {
		cpu.addCycles(4)
		return nil
	}
	cpu.addCycles(2)
	if cond == 0x1 { // BSR
		if err := cpu.push(Long, cpu.regs.PC); err != nil {
			return err
		}
	}
	cpu.regs.PC = basePC + displacement
	return nil
}

// bitFieldReadOnly reports whether bit-field operation op (opcode bits 10-8)
// only reads its field: BFTST, BFEXTU, BFEXTS and BFFFO.
func bitFieldReadOnly(op uint16) bool {
	return op == 0 || op == 1 || op == 3 || op == 5
}

func linkLong(cpu *cpu) error {
	reg := cpu.regs.IR & 7
	displacement, err := cpu.popPc(Long)
	if err != nil {
		return err
	}
	if err := cpu.push(Long, cpu.regs.A[reg]); err != nil {
		return err
	}
	cpu.regs.A[reg] = cpu.regs.A[7]
	cpu.regs.A[7] += displacement
	return nil
}

func extb(cpu *cpu) error {
	d := &cpu.regs.D[cpu.regs.IR&7]
	*d = int32(int8(*d))
	updateNZClearVC(cpu, uint32(*d), Long)
	return nil
}

func chkLong(cpu *cpu) error {
	src, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return err
	}
	bound, err := src.read()
	if err != nil {
		return err
	}
	value := int32(*udx(cpu))
	replaceStatusFlags(cpu, statusMaskNZVC, 0)
	switch {
	case value < 0:
		cpu.regs.SR |= srNegative
		return cpu.exceptionWithCycles(6, exceptionCyclesCHK)
	case value > int32(bound):
		return cpu.exceptionWithCycles(6, exceptionCyclesCHK)
	case value == int32(bound):
		cpu.regs.SR |= srZero
	}
	return nil
}

// mulLong is MULU.L and MULS.L: Dl * <ea> into Dl (32-bit result, V on
// overflow) or Dh:Dl (64-bit result).
func mulLong(cpu *cpu) error {
	ext, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	src, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return err
	}
	factor, err := src.read()
	if err != nil {
		return err
	}
	dl := generalRegister(cpu, uint16(ext>>12)&7)
	dh := generalRegister(cpu, uint16(ext)&7)
	signed, wide := ext&0x0800 != 0, ext&0x0400 != 0

	var hi, lo uint32
	var overflow bool
	if signed {
		p := int64(int32(*dl)) * int64(int32(factor))
		hi, lo = uint32(uint64(p)>>32), uint32(p)
		overflow = p != int64(int32(lo))
	} else {
		hi, lo = bits.Mul32(*dl, factor)
		overflow = hi != 0
	}

	var flags uint16
	if wide {
		*dh, *dl = hi, lo
		if hi&0x80000000 != 0 {
			flags |= srNegative
		}
		if hi|lo == 0 {
			flags |= srZero
		}
	} else {
		*dl = lo
		_, flags = nzFlagsOf(lo)
		if overflow {
			flags |= srOverflow
		}
	}
	replaceStatusFlags(cpu, statusMaskNZVC, flags)
	return nil
}

// divLong is DIVU.L, DIVS.L, DIVUL.L and DIVSL.L: Dq (or Dr:Dq) / <ea> into
// Dq, with the remainder in Dr unless Dr is Dq.
func divLong(cpu *cpu) error {
	ext, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	src, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return err
	}
	divisor, err := src.read()
	if err != nil {
		return err
	}
	dq := generalRegister(cpu, uint16(ext>>12)&7)
	dr := generalRegister(cpu, uint16(ext)&7)
	signed, wide := ext&0x0800 != 0, ext&0x0400 != 0

	if divisor == 0 {
		replaceStatusFlags(cpu, srCarry, 0)
		return cpu.exceptionWithCycles(XDivByZero, exceptionCyclesDivByZero)
	}

	var q, r uint32
	if signed {
		dividend := int64(int32(*dq))
		if wide {
			dividend = int64(uint64(*dr)<<32 | uint64(*dq))
		}
		quotient, remainder := dividend/int64(int32(divisor)), dividend%int64(int32(divisor))
		if quotient != int64(int32(quotient)) {
			replaceStatusFlags(cpu, srOverflow|srCarry, srOverflow)
			return nil
		}
		q, r = uint32(quotient), uint32(remainder)
	} else {
		dividend := uint64(*dq)
		if wide {
			dividend |= uint64(*dr) << 32
		}
		if dividend/uint64(divisor) > 0xffffffff {
			replaceStatusFlags(cpu, srOverflow|srCarry, srOverflow)
			return nil
		}
		q, r = uint32(dividend/uint64(divisor)), uint32(dividend%uint64(divisor))
	}
	*dr = r
	*dq = q // written last: with Dr = Dq only the quotient remains
	_, flags := nzFlagsOf(q)
	replaceStatusFlags(cpu, statusMaskNZVC, flags)
	return nil
}

// nzFlagsOf returns the N and Z flags of a long result.
func nzFlagsOf(v uint32) (uint32, uint16) {
	var flags uint16
	if v&0x80000000 != 0 {
		flags |= srNegative
	}
	if v == 0 {
		flags |= srZero
	}
	return v, flags
}

// trapcc is TRAPcc with no, a word or a long operand, which it skips.
func trapcc(cpu *cpu) error {
	switch cpu.regs.IR & 7 {
	case 2:
		cpu.regs.PC += 2
	case 3:
		cpu.regs.PC += 4
	}
	if !conditionTrue(cpu, (cpu.regs.IR>>8)&0xf) {
		return nil
	}
	cpu.overrideInstructionCycles(exceptionCyclesTrapV)
	return cpu.synchronousException(7, cpu.regs.SR|srSupervisor, cpu.regs.PC)
}

// predecrementByte returns the address of a byte operand -(An), decrementing
// An by 1, or by 2 for A7 so the stack stays word-aligned.
func predecrementByte(cpu *cpu, reg uint16) uint32 {
	if reg == 7 {
		cpu.regs.A[7] -= 2
	} else {
		cpu.regs.A[reg]--
	}
	return cpu.regs.A[reg]
}

// pack converts two unpacked BCD digits (a word: digit, digit) plus the
// adjustment into one packed byte.
func pack(cpu *cpu) error {
	adj, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	ry, rx := (cpu.regs.IR>>9)&7, cpu.regs.IR&7
	if cpu.regs.IR&8 == 0 {
		v := (uint32(cpu.regs.D[rx]) + adj) & 0xffff
		d := generalRegister(cpu, ry)
		*d = *d&^0xff | (v>>4)&0xf0 | v&0x0f
		return nil
	}
	lo, err := cpu.read(Byte, predecrementByte(cpu, rx))
	if err != nil {
		return err
	}
	hi, err := cpu.read(Byte, predecrementByte(cpu, rx))
	if err != nil {
		return err
	}
	v := (hi<<8 | lo + adj) & 0xffff
	return cpu.write(Byte, predecrementByte(cpu, ry), (v>>4)&0xf0|v&0x0f)
}

// unpk converts a packed BCD byte into two unpacked digits (a word) plus the
// adjustment.
func unpk(cpu *cpu) error {
	adj, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	ry, rx := (cpu.regs.IR>>9)&7, cpu.regs.IR&7
	unpack := func(b uint32) uint32 { return ((b&0xf0)<<4 | b&0x0f) + adj }
	if cpu.regs.IR&8 == 0 {
		v := unpack(uint32(cpu.regs.D[rx]) & 0xff)
		d := generalRegister(cpu, ry)
		*d = *d&^0xffff | v&0xffff
		return nil
	}
	b, err := cpu.read(Byte, predecrementByte(cpu, rx))
	if err != nil {
		return err
	}
	v := unpack(b)
	if err := cpu.write(Byte, predecrementByte(cpu, ry), v); err != nil {
		return err
	}
	return cpu.write(Byte, predecrementByte(cpu, ry), v>>8)
}

// bitField runs BFTST, BFEXTU, BFCHG, BFEXTS, BFCLR, BFFFO, BFSET and BFINS.
// The field starts offset bits from the most significant bit of a data
// register (modulo 32) or of the byte at the effective address (offset may
// be negative there) and is 1 to 32 bits wide.
func bitField(cpu *cpu) error {
	ext, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	op := (cpu.regs.IR >> 8) & 7
	offset := int32((ext >> 6) & 31)
	if ext&0x0800 != 0 {
		offset = cpu.regs.D[(ext>>6)&7]
	}
	width := ext & 31
	if ext&0x0020 != 0 {
		width = uint32(cpu.regs.D[ext&7]) & 31
	}
	if width == 0 {
		width = 32
	}
	dn := generalRegister(cpu, uint16(ext>>12)&7)
	mask := uint64(1)<<width - 1

	// The field is held right-aligned in field; get and put read and write
	// the bits it occupies.
	var get func() (uint64, error)
	var put func(uint64) error
	if (cpu.regs.IR>>3)&7 == 0 {
		reg := generalRegister(cpu, cpu.regs.IR&7)
		shift := uint32(offset) & 31
		get = func() (uint64, error) {
			return uint64(bits.RotateLeft32(*reg, int(shift+width))) & mask, nil
		}
		put = func(v uint64) error {
			placed := bits.RotateLeft32(uint32(v&mask), -int(shift+width))
			fieldBits := bits.RotateLeft32(uint32(mask), -int(shift+width))
			*reg = *reg&^fieldBits | placed
			return nil
		}
	} else {
		operand, err := cpu.ResolveSrcEA(Byte)
		if err != nil {
			return err
		}
		address := operand.computedAddress() + uint32(offset>>3)
		bit := uint32(offset) & 7
		n := (bit + width + 7) / 8
		top := 8*n - bit - width // bits right of the field in the bytes read
		get = func() (uint64, error) {
			var v uint64
			for i := range n {
				b, err := cpu.read(Byte, address+i)
				if err != nil {
					return 0, err
				}
				v = v<<8 | uint64(b)
			}
			return v >> top & mask, nil
		}
		put = func(field uint64) error {
			var v uint64
			for i := range n {
				b, err := cpu.read(Byte, address+i)
				if err != nil {
					return err
				}
				v = v<<8 | uint64(b)
			}
			v = v&^(mask<<top) | (field&mask)<<top
			for i := range n {
				if err := cpu.write(Byte, address+i, uint32(v>>(8*(n-1-i)))); err != nil {
					return err
				}
			}
			return nil
		}
	}

	field, err := get()
	if err != nil {
		return err
	}
	flagsOf := func(v uint64) uint16 {
		var flags uint16
		if v>>(width-1)&1 != 0 {
			flags |= srNegative
		}
		if v&mask == 0 {
			flags |= srZero
		}
		return flags
	}
	flags := flagsOf(field)
	switch op {
	case 1: // BFEXTU
		*dn = uint32(field)
	case 2: // BFCHG
		err = put(^field)
	case 3: // BFEXTS
		*dn = uint32(int32(uint32(field)<<(32-width)) >> (32 - width))
	case 4: // BFCLR
		err = put(0)
	case 5: // BFFFO
		first := uint32(bits.LeadingZeros64(field << (64 - width)))
		*dn = uint32(offset) + min(first, width)
	case 6: // BFSET
		err = put(mask)
	case 7: // BFINS
		flags = flagsOf(uint64(*dn) & mask)
		err = put(uint64(*dn))
	}
	replaceStatusFlags(cpu, statusMaskNZVC, flags)
	return err
}

// casSize returns the operand size of CAS from opcode bits 10-9.
func casSize(ir uint16) Size {
	return [...]Size{0, Byte, Word, Long}[(ir>>9)&3]
}

// cas compares Dc with the operand; if they are equal it writes Du to the
// operand, otherwise it loads the operand into Dc.
func cas(cpu *cpu) error {
	ext, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	size := casSize(cpu.regs.IR)
	operand, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	value, err := operand.read()
	if err != nil {
		return err
	}
	dc := generalRegister(cpu, uint16(ext)&7)
	du := generalRegister(cpu, uint16(ext>>6)&7)
	_, flags := subWithFlags(*dc&size.mask(), value, size)
	replaceStatusFlags(cpu, statusMaskNZVC, flags)
	if flags&srZero != 0 {
		return operand.write(*du & size.mask())
	}
	*dc = *dc&^size.mask() | value
	return nil
}

// cas2 is CAS with two operands addressed by registers: both are compared,
// and only if both match are both updated.
func cas2(cpu *cpu) error {
	ext1, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	ext2, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	size := Word
	if cpu.regs.IR&0x0200 != 0 {
		size = Long
	}
	addr1 := *generalRegister(cpu, uint16(ext1>>12))
	addr2 := *generalRegister(cpu, uint16(ext2>>12))
	dc1, du1 := generalRegister(cpu, uint16(ext1)&7), generalRegister(cpu, uint16(ext1>>6)&7)
	dc2, du2 := generalRegister(cpu, uint16(ext2)&7), generalRegister(cpu, uint16(ext2>>6)&7)

	v1, err := cpu.read(size, addr1)
	if err != nil {
		return err
	}
	v2, err := cpu.read(size, addr2)
	if err != nil {
		return err
	}
	_, flags := subWithFlags(*dc1&size.mask(), v1, size)
	if flags&srZero != 0 {
		_, flags = subWithFlags(*dc2&size.mask(), v2, size)
	}
	replaceStatusFlags(cpu, statusMaskNZVC, flags)
	if flags&srZero != 0 {
		if err := cpu.write(size, addr1, *du1&size.mask()); err != nil {
			return err
		}
		return cpu.write(size, addr2, *du2&size.mask())
	}
	*dc1 = *dc1&^size.mask() | v1
	*dc2 = *dc2&^size.mask() | v2
	return nil
}

// chk2 is CMP2 and CHK2: it compares a register with a lower and an upper
// bound at the effective address. Z reports a match with either bound, C a
// value outside them; CHK2 then takes the CHK exception. Bounds and data
// registers are sign-extended, and a lower bound above the upper one wraps,
// so the same test serves signed and unsigned ranges.
func chk2(cpu *cpu) error {
	ext, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	size := [...]Size{Byte, Word, Long}[(cpu.regs.IR>>9)&3]
	operand, err := cpu.ResolveSrcEA(size)
	if err != nil {
		return err
	}
	address := operand.computedAddress()
	lowerRaw, err := cpu.read(size, address)
	if err != nil {
		return err
	}
	upperRaw, err := cpu.read(size, address+uint32(size))
	if err != nil {
		return err
	}
	extend := func(v uint32) int32 {
		switch size {
		case Byte:
			return int32(int8(v))
		case Word:
			return int32(int16(v))
		}
		return int32(v)
	}
	lower, upper := extend(lowerRaw), extend(upperRaw)
	value := int32(*generalRegister(cpu, uint16(ext>>12)))
	if ext&0x8000 == 0 {
		value = extend(uint32(value))
	}

	var outside bool
	if lower <= upper {
		outside = value < lower || value > upper
	} else {
		outside = value < lower && value > upper
	}
	var flags uint16
	if value == lower || value == upper {
		flags |= srZero
	}
	if outside {
		flags |= srCarry
	}
	replaceStatusFlags(cpu, srZero|srCarry, flags)
	if outside && ext&0x0800 != 0 {
		return cpu.exceptionWithCycles(6, exceptionCyclesCHK)
	}
	return nil
}
