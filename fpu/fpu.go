// Package fpu emulates the MC68881 and MC68882 floating-point coprocessors.
//
// The FPU core executes the coprocessor's general instructions from their
// command words and moves operands as bytes in memory format, so it does not
// depend on the CPU. CIR wraps it in the memory-mapped coprocessor interface
// that 68000 machines such as the Atari Mega ST and Mega STE use.
//
// An FPU must not be used from several goroutines at once.
package fpu

import (
	"math/bits"

	"github.com/jenska/float"
)

// Model selects the coprocessor. The models differ only in the size of the
// FSAVE idle frame.
type Model int

const (
	MC68881 Model = iota
	MC68882
)

// Exception bits, as they appear in the FPCR exception enable byte and the
// FPSR exception status byte.
const (
	BSUN  uint8 = 1 << 7 // branch/set on unordered
	SNAN  uint8 = 1 << 6 // signaling not-a-number
	OPERR uint8 = 1 << 5 // operand error
	OVFL  uint8 = 1 << 4 // overflow
	UNFL  uint8 = 1 << 3 // underflow
	DZ    uint8 = 1 << 2 // divide by zero
	INEX2 uint8 = 1 << 1 // inexact operation
	INEX1 uint8 = 1 << 0 // inexact decimal input
)

// FPSR condition code bits.
const (
	CCN   uint32 = 1 << 27 // negative
	CCZ   uint32 = 1 << 26 // zero
	CCI   uint32 = 1 << 25 // infinity
	CCNaN uint32 = 1 << 24 // not a number
)

// FPSR accrued exception bits.
const (
	accruedIOP  uint32 = 1 << 7
	accruedOVFL uint32 = 1 << 6
	accruedUNFL uint32 = 1 << 5
	accruedDZ   uint32 = 1 << 4
	accruedINEX uint32 = 1 << 3
)

// Exception vectors.
const (
	VectorBSUN  = 48
	VectorINEX  = 49
	VectorDZ    = 50
	VectorUNFL  = 51
	VectorOPERR = 52
	VectorOVFL  = 53
	VectorSNAN  = 54
)

const (
	fpcrMask = 0x0000ffff
	fpsrMask = 0x0ffffff8
)

// nan is the non-signaling NaN the 68881 returns for invalid operations and
// loads into FP0-FP7 at reset.
var nan = float.NewFromBits(0x7fff, 0xffffffffffffffff)

// FPU is the state of one MC68881 or MC68882.
type FPU struct {
	FP    [8]float.X80
	FPCR  uint32
	FPSR  uint32
	FPIAR uint32

	model Model
	env   float.Env
	// instructionAddress is copied to FPIAR by the next arithmetic
	// instruction.
	instructionAddress uint32
	// pending is the vector of an enabled exception that has not been
	// taken yet, or 0.
	pending uint8
	// null is true from reset until the first instruction, while FSAVE
	// produces a null frame.
	null bool
}

// New returns an FPU of model m in its reset state.
func New(m Model) *FPU {
	f := &FPU{model: m}
	f.Reset()
	return f
}

// Model returns the coprocessor model.
func (f *FPU) Model() Model { return f.model }

// Reset puts the FPU into its reset state: FP0-FP7 hold non-signaling NaNs
// and FPCR, FPSR and FPIAR are zero.
func (f *FPU) Reset() {
	for i := range f.FP {
		f.FP[i] = nan
	}
	f.FPCR, f.FPSR, f.FPIAR = 0, 0, 0
	f.instructionAddress = 0
	f.pending = 0
	f.null = true
}

// SetInstructionAddress sets the address the next arithmetic instruction
// stores in FPIAR.
func (f *FPU) SetInstructionAddress(address uint32) {
	f.instructionAddress = address
}

// PendingException reports the vector of an enabled exception that an
// instruction raised and that has not been acknowledged.
func (f *FPU) PendingException() (vector int, ok bool) {
	return int(f.pending), f.pending != 0
}

// AcknowledgeException clears the pending exception.
func (f *FPU) AcknowledgeException() {
	f.pending = 0
}

// precision returns the rounding precision selected in the FPCR.
func (f *FPU) precision() int {
	switch (f.FPCR >> 6) & 3 {
	case 1:
		return 32
	case 2:
		return 64
	}
	return 80
}

// prepare returns the FPU's float environment with the FPCR's rounding mode
// and precision and no exceptions recorded.
func (f *FPU) prepare() *float.Env {
	f.env = float.Env{
		RoundingMode:      int((f.FPCR >> 4) & 3),
		RoundingPrecision: f.precision(),
		DefaultNaN:        nan,
	}
	return &f.env
}

// exceptions maps float exception flags onto the exception status bits.
// Invalid becomes SNAN when an operand was a signaling NaN.
func exceptions(flags int, signaling bool) uint8 {
	var exc uint8
	if flags&float.ExceptionInvalid != 0 {
		if signaling {
			exc |= SNAN
		} else {
			exc |= OPERR
		}
	}
	if flags&float.ExceptionDivbyzero != 0 {
		exc |= DZ
	}
	if flags&float.ExceptionOverflow != 0 {
		exc |= OVFL
	}
	if flags&float.ExceptionUnderflow != 0 {
		exc |= UNFL
	}
	if flags&float.ExceptionInexact != 0 {
		exc |= INEX2
	}
	return exc
}

// report replaces the exception status byte with exc, updates the accrued
// byte and latches the highest-priority enabled exception.
func (f *FPU) report(exc uint8) {
	f.FPSR = f.FPSR&^0xff00 | uint32(exc)<<8
	f.accrue(exc)
}

// accrue adds exc to the accrued exception byte and latches the
// highest-priority enabled exception.
func (f *FPU) accrue(exc uint8) {
	if exc&(BSUN|SNAN|OPERR) != 0 {
		f.FPSR |= accruedIOP
	}
	if exc&OVFL != 0 {
		f.FPSR |= accruedOVFL
	}
	if exc&UNFL != 0 && exc&INEX2 != 0 {
		f.FPSR |= accruedUNFL
	}
	if exc&DZ != 0 {
		f.FPSR |= accruedDZ
	}
	if exc&(INEX1|INEX2|OVFL) != 0 {
		f.FPSR |= accruedINEX
	}
	if enabled := exc & uint8(f.FPCR>>8); enabled != 0 && f.pending == 0 {
		f.pending = vectorFor(enabled)
	}
}

// vectorFor returns the vector of the highest-priority exception in exc.
func vectorFor(exc uint8) uint8 {
	switch {
	case exc&BSUN != 0:
		return VectorBSUN
	case exc&SNAN != 0:
		return VectorSNAN
	case exc&OPERR != 0:
		return VectorOPERR
	case exc&OVFL != 0:
		return VectorOVFL
	case exc&UNFL != 0:
		return VectorUNFL
	case exc&DZ != 0:
		return VectorDZ
	}
	return VectorINEX
}

// setConditionCodes sets N, Z, I and NAN from x.
func (f *FPU) setConditionCodes(x float.X80) {
	cc := uint32(0)
	if x.Signbit() {
		cc |= CCN
	}
	switch {
	case x.IsNaN():
		cc |= CCNaN
	case x.IsInf():
		cc |= CCI
	case x.IsZero():
		cc |= CCZ
	}
	f.FPSR = f.FPSR&^0x0f000000 | cc
}

// setQuotient sets the FPSR quotient byte from the quotient bits of FMOD or
// FREM.
func (f *FPU) setQuotient(quo int) {
	q := uint32(0)
	if quo < 0 {
		q, quo = 0x80, -quo
	}
	q |= uint32(quo) & 0x7f
	f.FPSR = f.FPSR&^0x00ff0000 | q<<16
}

// Condition evaluates the conditional predicate pred (0-$1F) of FBcc, FScc,
// FDBcc or FTRAPcc. Predicates $10-$1F set BSUN when the condition codes
// report a NaN.
func (f *FPU) Condition(pred uint16) (bool, error) {
	if pred > 0x1f {
		return false, ErrUnimplemented
	}
	f.null = false
	n, z, nan := f.FPSR&CCN != 0, f.FPSR&CCZ != 0, f.FPSR&CCNaN != 0
	if pred&0x10 != 0 && nan {
		f.FPSR |= uint32(BSUN) << 8
		f.accrue(BSUN)
	}
	switch pred & 0x0f {
	case 0x0: // F, SF
		return false, nil
	case 0x1: // EQ, SEQ
		return z, nil
	case 0x2: // OGT, GT
		return !(nan || z || n), nil
	case 0x3: // OGE, GE
		return z || !(nan || n), nil
	case 0x4: // OLT, LT
		return n && !(nan || z), nil
	case 0x5: // OLE, LE
		return z || (n && !nan), nil
	case 0x6: // OGL, GL
		return !(nan || z), nil
	case 0x7: // OR, GLE
		return !nan, nil
	case 0x8: // UN, NGLE
		return nan, nil
	case 0x9: // UEQ, NGL
		return nan || z, nil
	case 0xa: // UGT, NLE
		return nan || !(n || z), nil
	case 0xb: // UGE, NLT
		return nan || z || !n, nil
	case 0xc: // ULT, NGE
		return nan || (n && !z), nil
	case 0xd: // ULE, NGT
		return nan || z || n, nil
	case 0xe: // NE, SNE
		return !z, nil
	}
	return true, nil // T, ST
}

// constant returns the value at offset of the constant ROM that FMOVECR
// reads, rounded in e. Offsets without a documented constant read as zero.
func constant(e *float.Env, offset uint16) float.X80 {
	switch offset {
	case 0x00:
		return e.Constant(float.ConstPi)
	case 0x0b:
		return e.Constant(float.ConstLog10Of2)
	case 0x0c:
		return e.Constant(float.ConstE)
	case 0x0d:
		return e.Constant(float.ConstLog2E)
	case 0x0e:
		return e.Constant(float.ConstLog10E)
	case 0x30:
		return e.Constant(float.ConstLn2)
	case 0x31:
		return e.Constant(float.ConstLn10)
	}
	if offset >= 0x32 && offset <= 0x3f {
		// 10^0, 10^1, 10^2, 10^4, ..., 10^4096
		n := 0
		if offset > 0x32 {
			n = 1 << (offset - 0x33)
		}
		return e.Pow10(n)
	}
	return float.X80Zero
}

func popcount(list uint8) int { return bits.OnesCount8(list) }
