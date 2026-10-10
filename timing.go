package m68kemu

// Instruction execution times of the MC68000 with a zero-wait bus, from the
// M68000 Family Programmer's Reference / M68000 User's Manual, section 8.
// Every figure includes the instruction's own bus cycles (opcode and extension
// word fetches, operand reads and writes); bus wait states come on top.
//
// The calculators here produce the static part stored in opcodeSet.cycles.
// Instructions whose time depends on run-time values (taken branches, bit
// numbers, shift counts, MULU/MULS/DIVU/DIVS operands, MOVEM register lists)
// add the dynamic part in their handlers.

func eaFields(opcode uint16) (mode, reg uint16) {
	return (opcode >> 3) & 0x7, opcode & 0x7
}

// isRegisterOrImmediate reports whether an effective address is Dn, An or
// #<data>, the operands for which several long operations cost 2 extra cycles.
func isRegisterOrImmediate(mode, reg uint16) bool {
	return mode == 0 || mode == 1 || (mode == 7 && reg == 4)
}

// moveDestinationCycles is the destination part of a MOVE (tables 8-2, 8-3).
// A predecrement destination costs no more than (An): the decrement overlaps
// the write.
func moveDestinationCycles(mode, reg uint16, size Size) uint32 {
	if mode == 4 {
		mode = 2
	}
	return eaAccessCycles(mode, reg, size)
}

// dataToRegisterCycles is ADD/SUB/AND/OR <ea>,Dn (table 8-4).
func dataToRegisterCycles(mode, reg uint16, size Size) uint32 {
	if size != Long {
		return 4 + eaAccessCycles(mode, reg, size)
	}
	if isRegisterOrImmediate(mode, reg) {
		return 8 + eaAccessCycles(mode, reg, size)
	}
	return 6 + eaAccessCycles(mode, reg, size)
}

// readModifyWriteCycles is the time of an operation that reads, modifies and
// writes back its destination: ADD/SUB/AND/OR/EOR Dn,<ea> (table 8-4) and
// the memory forms of CLR/NEG/NEGX/NOT (table 8-6) and ADDQ/SUBQ (table 8-5).
// A data register destination costs regB/regL instead.
func readModifyWriteCycles(mode, reg uint16, size Size, regBW, regL uint32) uint32 {
	if mode == 0 {
		if size == Long {
			return regL
		}
		return regBW
	}
	if size == Long {
		return 12 + eaAccessCycles(mode, reg, size)
	}
	return 8 + eaAccessCycles(mode, reg, size)
}

// immediateCycles is ADDI/SUBI/ANDI/ORI/EORI #<data>,<ea> (table 8-5); the
// immediate operand's fetch is part of the base time.
func immediateCycles(mode, reg uint16, size Size) uint32 {
	switch {
	case mode == 0 && size == Long:
		return 16
	case mode == 0:
		return 8
	case size == Long:
		return 20 + eaAccessCycles(mode, reg, size)
	default:
		return 12 + eaAccessCycles(mode, reg, size)
	}
}

// controlCycles holds the time of JMP and LEA per control addressing mode
// (table 8-10): (An), (d16,An), (d8,An,Xn), (xxx).W, (xxx).L, (d16,PC),
// (d8,PC,Xn). JSR adds 8 to JMP and PEA adds 8 to LEA.
func controlCycles(mode, reg uint16, an, d16, index, absW, absL uint32) uint32 {
	switch mode {
	case 2:
		return an
	case 5:
		return d16
	case 6:
		return index
	case 7:
		switch reg {
		case 0:
			return absW
		case 1:
			return absL
		case 2:
			return d16
		case 3:
			return index
		}
	}
	return 0
}

func jmpCycles(mode, reg uint16) uint32 { return controlCycles(mode, reg, 8, 10, 14, 10, 12) }
func leaCycles(mode, reg uint16) uint32 { return controlCycles(mode, reg, 4, 8, 12, 8, 12) }

// movemBaseCycles is the fixed part of MOVEM (table 8-10); each register
// transferred adds 4 cycles for a word and 8 for a long.
func movemBaseCycles(mode, reg uint16, toRegisters bool) uint32 {
	if toRegisters {
		if mode == 3 {
			mode = 2
		}
		return controlCycles(mode, reg, 12, 16, 18, 16, 20)
	}
	if mode == 4 {
		mode = 2
	}
	return controlCycles(mode, reg, 8, 12, 14, 12, 16)
}

func movemRegisterCycles(size Size, count int) uint32 {
	if size == Long {
		return 8 * uint32(count)
	}
	return 4 * uint32(count)
}

// muluCycles is MULU's time without the source EA: 38 + 2 per set bit of the
// multiplier.
func muluCycles(multiplier uint16) uint32 {
	n := uint32(0)
	for v := multiplier; v != 0; v &= v - 1 {
		n++
	}
	return 38 + 2*n
}

// mulsCycles is MULS's time without the source EA: 38 + 2 per 01 or 10 bit
// pair in the multiplier with a zero appended on the right.
func mulsCycles(multiplier uint16) uint32 {
	v := uint32(multiplier) << 1
	n := uint32(0)
	for i := 0; i < 16; i++ {
		if (v>>i)&3 == 1 || (v>>i)&3 == 2 {
			n++
		}
	}
	return 38 + 2*n
}

// divuCycles is DIVU's time without the source EA, for a non-zero divisor.
// The manual only gives the maximum (140); this follows the exact
// per-quotient-bit algorithm worked out by Jorge Cwik ("68000 DIVU/DIVS
// timing"), which emulators such as MAME and Hatari use.
func divuCycles(dividend uint32, divisor uint16) uint32 {
	if dividend>>16 >= uint32(divisor) {
		return 10 // overflow, detected before the division starts
	}
	mcycles := uint32(38)
	hdivisor := uint32(divisor) << 16
	for range 15 {
		temp := dividend
		dividend <<= 1
		if int32(temp) < 0 {
			dividend -= hdivisor
		} else {
			mcycles += 2
			if dividend >= hdivisor {
				dividend -= hdivisor
				mcycles--
			}
		}
	}
	return mcycles * 2
}

// divsCycles is DIVS's time without the source EA, for a non-zero divisor
// (maximum 158 in the manual); same source as divuCycles.
func divsCycles(dividend int32, divisor int16) uint32 {
	mcycles := uint32(6)
	if dividend < 0 {
		mcycles++
	}
	absDividend := uint32(dividend)
	if dividend < 0 {
		absDividend = uint32(-dividend)
	}
	absDivisor := uint32(uint16(divisor))
	if divisor < 0 {
		absDivisor = uint32(uint16(-divisor))
	}
	if absDividend>>16 >= absDivisor {
		return (mcycles + 2) * 2 // overflow
	}
	quotient := absDividend / absDivisor
	mcycles += 55
	if divisor >= 0 {
		if dividend >= 0 {
			mcycles--
		} else {
			mcycles++
		}
	}
	for range 15 {
		if int16(quotient) >= 0 {
			mcycles++
		}
		quotient <<= 1
	}
	return mcycles * 2
}
