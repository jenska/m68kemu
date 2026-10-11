package fpu

import "math/bits"

// Execution times in FPU clock cycles. They follow the typical times of the
// MC68881/MC68882 User's Manual, recalled from memory and not verified
// against it (see doc/fpu.md): register-to-register operations in extended
// precision, plus the cost of converting a memory operand. Real times vary
// with the operands, more so for the transcendental functions; these are the
// typical values. Both models use the same table: the MC68882's faster
// conversions and its overlap of consecutive FPU instructions are not
// modelled.

// opmodeCycles is the time of a register-to-register operation by opmode.
var opmodeCycles = [0x40]uint16{
	0x00: 33,                                   // FMOVE
	0x01: 55,                                   // FINT
	0x02: 687,                                  // FSINH
	0x03: 55,                                   // FINTRZ
	0x04: 107,                                  // FSQRT
	0x06: 571,                                  // FLOGNP1
	0x08: 545,                                  // FETOXM1
	0x09: 661,                                  // FTANH
	0x0a: 403,                                  // FATAN
	0x0c: 581,                                  // FASIN
	0x0d: 693,                                  // FATANH
	0x0e: 391,                                  // FSIN
	0x0f: 473,                                  // FTAN
	0x10: 497,                                  // FETOX
	0x11: 567,                                  // FTWOTOX
	0x12: 567,                                  // FTENTOX
	0x14: 525,                                  // FLOGN
	0x15: 581,                                  // FLOG10
	0x16: 581,                                  // FLOG2
	0x18: 35,                                   // FABS
	0x19: 607,                                  // FCOSH
	0x1a: 35,                                   // FNEG
	0x1c: 625,                                  // FACOS
	0x1d: 391,                                  // FCOS
	0x1e: 45,                                   // FGETEXP
	0x1f: 31,                                   // FGETMAN
	0x20: 103,                                  // FDIV
	0x21: 70,                                   // FMOD
	0x22: 51,                                   // FADD
	0x23: 71,                                   // FMUL
	0x24: 69,                                   // FSGLDIV
	0x25: 100,                                  // FREM
	0x26: 41,                                   // FSCALE
	0x27: 59,                                   // FSGLMUL
	0x28: 51,                                   // FSUB
	0x30: 451, 0x31: 451, 0x32: 451, 0x33: 451, // FSINCOS
	0x34: 451, 0x35: 451, 0x36: 451, 0x37: 451,
	0x38: 33, // FCMP
	0x3a: 33, // FTST
}

// loadCycles is the extra time to convert a source operand of each format
// to extended precision.
var loadCycles = [8]uint16{
	FormatLong:     18,
	FormatSingle:   12,
	FormatExtended: 6,
	FormatPacked:   842,
	FormatWord:     18,
	FormatDouble:   14,
	FormatByte:     18,
}

// storeCycles is the time of FMOVE FPn,<ea> to each format.
var storeCycles = [8]uint16{
	FormatLong:          75,
	FormatSingle:        65,
	FormatExtended:      47,
	FormatPacked:        1937,
	FormatWord:          75,
	FormatDouble:        77,
	FormatByte:          75,
	FormatPackedDynamic: 1937,
}

const (
	fmovecrCycles      = 29
	controlCycles      = 30 // FMOVE(M) of control registers, per register
	movemBaseCycles    = 33 // FMOVEM of data registers, plus per register:
	movemRegisterCycle = 25
)

// Cycles returns the time op takes on the FPU, in FPU clock cycles, after
// its source operand has arrived. dynamic is the value of op.Register when
// it is not -1. Cycles returns 0 for a command Decode rejects.
func (f *FPU) Cycles(op Operation, dynamic uint32) int {
	cmd := op.Command
	src := Format((cmd >> 10) & 7)
	opmode := cmd & 0x7f
	switch cmd >> 13 {
	case classRegister:
		if !validOpmode(opmode) {
			return 0
		}
		return int(opmodeCycles[opmode])
	case classMemory:
		if src == 7 {
			return fmovecrCycles
		}
		if !validOpmode(opmode) {
			return 0
		}
		return int(opmodeCycles[opmode] + loadCycles[src])
	case classStore:
		return int(storeCycles[src])
	case classLoadControl, classStoreControl:
		return controlCycles * bits.OnesCount8(uint8(src))
	case classLoadRegisters, classStoreRegisters:
		return movemBaseCycles + movemRegisterCycle*bits.OnesCount8(op.registerList(dynamic))
	}
	return 0
}
