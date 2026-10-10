package m68kemu

// This file decides which opcode words the MC68000 has, following the
// operation code map and the effective-address categories of the M68000
// Family Programmer's Reference Manual. opcodesFor drops every handler the
// instruction registrations attached to a word outside that map, so such a
// word raises the illegal-instruction exception (or line A/F) like on the
// real CPU instead of running with an operand the instruction cannot take.

// validFor reports whether op is a valid opcode word for model m.
func validFor(m Model, op uint16) bool {
	if m >= M68010 && valid68010Addition(op) {
		return true
	}
	return valid68000(op)
}

// valid68010Addition reports whether op is one of the opcode words the
// MC68010 adds to the MC68000 map. BKPT ($4848-$484F) is left out: without
// hardware that answers its breakpoint acknowledge cycle it takes the
// illegal-instruction exception, like an unassigned word.
func valid68010Addition(op uint16) bool {
	switch {
	case op == 0x4E7A || op == 0x4E7B: // MOVEC
		return true
	case op == 0x4E74: // RTD
		return true
	case op&0xFFC0 == 0x42C0: // MOVE CCR,<ea>
		return specIn(op, specDataAlterable)
	case op&0xFF00 == 0x0E00: // MOVES
		return (op>>6)&3 != 3 && specIn(op, specMemoryAlterable)
	}
	return false
}

// valid68000 reports whether op is a valid MC68000 opcode word, following
// the operation code map and addressing-mode categories of the M68000
// Family Programmer's Reference Manual.

type specEAClass uint16

const (
	specDn specEAClass = 1 << iota
	specAn
	specInd
	specPostInc
	specPreDec
	specDisp
	specIndex
	specAbsW
	specAbsL
	specPCDisp
	specPCIndex
	specImm
)

const (
	specAll             = specDn | specAn | specInd | specPostInc | specPreDec | specDisp | specIndex | specAbsW | specAbsL | specPCDisp | specPCIndex | specImm
	specData            = specAll &^ specAn
	specMemory          = specAll &^ (specDn | specAn)
	specControl         = specInd | specDisp | specIndex | specAbsW | specAbsL | specPCDisp | specPCIndex
	specAlterable       = specDn | specAn | specInd | specPostInc | specPreDec | specDisp | specIndex | specAbsW | specAbsL
	specDataAlterable   = specAlterable &^ specAn
	specMemoryAlterable = specAlterable &^ (specDn | specAn)
)

func specEAOf(mode, reg uint16) specEAClass {
	if mode < 7 {
		return specEAClass(1) << mode
	}
	switch reg {
	case 0:
		return specAbsW
	case 1:
		return specAbsL
	case 2:
		return specPCDisp
	case 3:
		return specPCIndex
	case 4:
		return specImm
	}
	return 0
}

func specIn(op uint16, set specEAClass) bool {
	c := specEAOf((op>>3)&7, op&7)
	return c != 0 && c&set != 0
}

func valid68000(op uint16) bool {
	line := op >> 12
	size := (op >> 6) & 3
	mode := (op >> 3) & 7
	switch line {
	case 0x0:
		switch op {
		case 0x003C, 0x007C, 0x023C, 0x027C, 0x0A3C, 0x0A7C: // ORI/ANDI/EORI to CCR/SR
			return true
		}
		if op&0x0100 != 0 {
			if mode == 1 {
				return true // MOVEP
			}
			if size == 0 { // BTST Dn,<ea>
				return specIn(op, specData)
			}
			return specIn(op, specDataAlterable)
		}
		switch (op >> 9) & 7 {
		case 0, 1, 2, 3, 5, 6: // ORI ANDI SUBI ADDI EORI CMPI
			return size != 3 && specIn(op, specDataAlterable)
		case 4: // static bit
			if size == 0 {
				return specIn(op, specData&^specImm)
			}
			return specIn(op, specDataAlterable)
		}
		return false
	case 0x1, 0x2, 0x3:
		dstMode := (op >> 6) & 7
		dstReg := (op >> 9) & 7
		if !specIn(op, specAll) {
			return false
		}
		if line == 1 && specIn(op, specAn) {
			return false // no byte reads from An
		}
		if dstMode == 1 {
			return line != 1 // MOVEA.W/L
		}
		c := specEAOf(dstMode, dstReg)
		return c != 0 && c&specDataAlterable != 0
	case 0x4:
		return valid68000Line4(op)
	case 0x5:
		if size == 3 {
			if mode == 1 {
				return true // DBcc
			}
			return specIn(op, specDataAlterable) // Scc
		}
		if mode == 1 {
			return size != 0 // ADDQ/SUBQ to An: word or long
		}
		return specIn(op, specAlterable)
	case 0x6:
		return true
	case 0x7:
		return op&0x0100 == 0
	case 0x8, 0xC:
		opmode := (op >> 6) & 7
		if opmode == 3 || opmode == 7 { // DIVU/DIVS, MULU/MULS
			return specIn(op, specData)
		}
		if opmode >= 4 && mode <= 1 {
			if opmode == 4 {
				return true // SBCD / ABCD
			}
			if line == 0xC {
				// EXG Dx,Dy (01000), Ax,Ay (01001), Dx,Ay (10001)
				return (opmode == 5 && (mode == 0 || mode == 1)) || (opmode == 6 && mode == 1)
			}
			return false
		}
		if opmode >= 4 {
			return specIn(op, specMemoryAlterable) // OR/AND Dn,<ea>
		}
		return specIn(op, specData) // OR/AND <ea>,Dn
	case 0x9, 0xD, 0xB:
		opmode := (op >> 6) & 7
		if opmode == 3 || opmode == 7 { // SUBA/ADDA/CMPA
			return specIn(op, specAll)
		}
		if opmode < 3 { // SUB/ADD/CMP <ea>,Dn
			if opmode == 0 && specIn(op, specAn) {
				return false
			}
			return specIn(op, specAll)
		}
		if line == 0xB {
			if mode == 1 {
				return true // CMPM
			}
			return specIn(op, specDataAlterable) // EOR
		}
		if mode <= 1 {
			return true // SUBX/ADDX
		}
		return specIn(op, specMemoryAlterable)
	case 0xE:
		if size == 3 {
			return op&0x0800 == 0 && specIn(op, specMemoryAlterable)
		}
		return true
	}
	return false // line A, line F
}

func valid68000Line4(op uint16) bool {
	size := (op >> 6) & 3
	mode := (op >> 3) & 7
	if op&0x0100 != 0 {
		switch size {
		case 3:
			return specIn(op, specControl) // LEA
		case 2:
			return specIn(op, specData) // CHK.W
		}
		return false
	}
	switch (op >> 8) & 0xF {
	case 0x0: // NEGX / MOVE from SR
		if size == 3 {
			return specIn(op, specDataAlterable)
		}
		return specIn(op, specDataAlterable)
	case 0x2: // CLR (MOVE from CCR is 68010+)
		return size != 3 && specIn(op, specDataAlterable)
	case 0x4: // NEG / MOVE to CCR
		if size == 3 {
			return specIn(op, specData)
		}
		return specIn(op, specDataAlterable)
	case 0x6: // NOT / MOVE to SR
		if size == 3 {
			return specIn(op, specData)
		}
		return specIn(op, specDataAlterable)
	case 0x8:
		switch size {
		case 0:
			return specIn(op, specDataAlterable) // NBCD
		case 1:
			if mode == 0 {
				return true // SWAP
			}
			return specIn(op, specControl) // PEA
		default:
			if mode == 0 {
				return true // EXT.W/EXT.L
			}
			return specIn(op, specControl&^(specPCDisp|specPCIndex)|specPreDec) // MOVEM regs to memory
		}
	case 0xA:
		if op == 0x4AFC {
			return true // ILLEGAL
		}
		if size == 3 {
			return specIn(op, specDataAlterable) // TAS
		}
		return specIn(op, specDataAlterable) // TST
	case 0xC:
		if size >= 2 {
			return specIn(op, specControl|specPostInc) // MOVEM memory to regs
		}
		return false
	case 0xE:
		switch {
		case op >= 0x4E40 && op <= 0x4E6F: // TRAP, LINK, UNLK, MOVE USP
			return true
		case op == 0x4E70, op == 0x4E71, op == 0x4E72, op == 0x4E73, op == 0x4E75, op == 0x4E76, op == 0x4E77:
			return true
		case size == 2:
			return specIn(op, specControl) // JSR
		case size == 3:
			return specIn(op, specControl) // JMP
		}
	}
	return false
}
