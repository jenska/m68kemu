package fpu

import (
	"encoding/binary"
	"errors"

	"github.com/jenska/float"
)

// ErrUnimplemented reports a command word or predicate the 68881/68882 does
// not define. The coprocessor reports it to the CPU as an F-line exception.
var ErrUnimplemented = errors.New("fpu: unimplemented command")

// Command classes, bits 15-13 of a command word.
const (
	classRegister       = 0 // FPm -> FPn
	classMemory         = 2 // <ea> -> FPn, FMOVECR
	classStore          = 3 // FMOVE FPn -> <ea>
	classLoadControl    = 4 // <ea> -> FPCR/FPSR/FPIAR
	classStoreControl   = 5 // FPCR/FPSR/FPIAR -> <ea>
	classLoadRegisters  = 6 // FMOVEM <ea> -> FPn list
	classStoreRegisters = 7 // FMOVEM FPn list -> <ea>
)

// Operation is a decoded command word: what the FPU reads from and writes to
// the CPU when it executes the command.
type Operation struct {
	Command uint16
	// Register is the data register (0-7) whose value the command needs, for
	// a dynamic k-factor or register list, or -1.
	Register int
	// In and Out are the numbers of bytes the command reads from and writes
	// to the CPU. For a dynamic register list they are only known after
	// SetDynamic.
	In, Out int
}

// Decode decodes the command word of a general instruction.
func Decode(cmd uint16) (Operation, error) {
	op := Operation{Command: cmd, Register: -1}
	src := Format((cmd >> 10) & 7)
	switch cmd >> 13 {
	case classRegister:
		if !validOpmode(cmd & 0x7f) {
			return op, ErrUnimplemented
		}
	case classMemory:
		if src == 7 { // FMOVECR
			return op, nil
		}
		if !validOpmode(cmd & 0x7f) {
			return op, ErrUnimplemented
		}
		op.In = src.Size()
	case classStore:
		op.Out = src.Size()
		if src == FormatPackedDynamic {
			op.Register = int(cmd>>4) & 7
		}
	case classLoadControl, classStoreControl:
		n := popcount(uint8(src))
		if n == 0 || cmd&0x3ff != 0 {
			return op, ErrUnimplemented
		}
		if cmd>>13 == classLoadControl {
			op.In = 4 * n
		} else {
			op.Out = 4 * n
		}
	case classLoadRegisters, classStoreRegisters:
		if cmd&0x0700 != 0 {
			return op, ErrUnimplemented
		}
		if cmd&0x0800 != 0 { // dynamic list
			op.Register = int(cmd>>4) & 7
		} else {
			op.SetDynamic(0)
		}
	default:
		return op, ErrUnimplemented
	}
	return op, nil
}

// SetDynamic completes a command that takes a register list from a data
// register: value is the content of op.Register.
func (op *Operation) SetDynamic(value uint32) {
	class := op.Command >> 13
	if class != classLoadRegisters && class != classStoreRegisters {
		return
	}
	n := 12 * popcount(op.registerList(value))
	if class == classLoadRegisters {
		op.In = n
	} else {
		op.Out = n
	}
}

// registerList returns the FMOVEM register list, from the command word or,
// for a dynamic list, from value.
func (op Operation) registerList(value uint32) uint8 {
	if op.Command&0x0800 != 0 {
		return uint8(value)
	}
	return uint8(op.Command)
}

// transferOrder returns the registers an FMOVEM moves, in transfer order. In
// the postincrement/control mode (bit 12 set) bit 7 of the list selects FP0
// and the registers go in ascending order; in the predecrement mode bit 7
// selects FP7 and they go in descending order.
func (op Operation) transferOrder(value uint32) []int {
	list := op.registerList(value)
	var regs []int
	for bit := 7; bit >= 0; bit-- {
		if list&(1<<bit) == 0 {
			continue
		}
		if op.Command&0x1000 != 0 {
			regs = append(regs, 7-bit)
		} else {
			regs = append(regs, bit)
		}
	}
	return regs
}

func validOpmode(opmode uint16) bool {
	switch {
	case opmode <= 0x04, opmode == 0x06, opmode >= 0x08 && opmode <= 0x0a,
		opmode >= 0x0c && opmode <= 0x12, opmode >= 0x14 && opmode <= 0x16,
		opmode >= 0x18 && opmode <= 0x1a, opmode >= 0x1c && opmode <= 0x28,
		opmode >= 0x30 && opmode <= 0x38, opmode == 0x3a:
		return true
	}
	return false
}

// Execute runs op. in holds op.In bytes of source operand; dynamic is the
// value of op.Register when it is not -1. Execute returns op.Out bytes.
func (f *FPU) Execute(op Operation, dynamic uint32, in []byte) ([]byte, error) {
	if len(in) < op.In {
		return nil, errors.New("fpu: operand too short")
	}
	f.null = false
	cmd := op.Command
	src := Format((cmd >> 10) & 7)
	dst := int(cmd>>7) & 7

	switch cmd >> 13 {
	case classRegister:
		f.FPIAR = f.instructionAddress
		f.arithmetic(cmd&0x7f, dst, f.FP[src], 0)
	case classMemory:
		f.FPIAR = f.instructionAddress
		if src == 7 {
			e := f.prepare()
			x := constant(e, cmd&0x7f)
			f.FP[dst] = x
			f.setConditionCodes(x)
			f.report(exceptions(e.Exception, false))
			return nil, nil
		}
		x, exc := f.load(src, in)
		f.arithmetic(cmd&0x7f, dst, x, exc)
	case classStore:
		f.FPIAR = f.instructionAddress
		k := int(int8(cmd<<1)) >> 1 // static k-factor, -64..63
		if src == FormatPackedDynamic {
			k = int(int8(dynamic<<1)) >> 1
		}
		out, exc := f.store(src, f.FP[dst], k)
		f.report(exc)
		return out, nil
	case classLoadControl:
		for _, reg := range controlRegisters(src) {
			v := binary.BigEndian.Uint32(in)
			in = in[4:]
			switch reg {
			case 4:
				f.FPCR = v & fpcrMask
			case 2:
				f.FPSR = v & fpsrMask
			case 1:
				f.FPIAR = v
			}
		}
	case classStoreControl:
		var out []byte
		for _, reg := range controlRegisters(src) {
			v := f.FPIAR
			switch reg {
			case 4:
				v = f.FPCR
			case 2:
				v = f.FPSR
			}
			out = binary.BigEndian.AppendUint32(out, v)
		}
		return out, nil
	case classLoadRegisters:
		for _, reg := range op.transferOrder(dynamic) {
			f.FP[reg] = float.NewFromBytes96(in, binary.BigEndian)
			in = in[12:]
		}
	case classStoreRegisters:
		var out []byte
		for _, reg := range op.transferOrder(dynamic) {
			out = append(out, f.FP[reg].Bytes96(binary.BigEndian)...)
		}
		return out, nil
	}
	return nil, nil
}

// controlRegisters returns the control registers selected by list (FPCR 4,
// FPSR 2, FPIAR 1) in transfer order.
func controlRegisters(list Format) []Format {
	var regs []Format
	for _, r := range []Format{4, 2, 1} {
		if list&r != 0 {
			regs = append(regs, r)
		}
	}
	return regs
}

// arithmetic performs opmode with source operand src and destination FPn
// (dst). exc holds the exceptions of the source conversion.
func (f *FPU) arithmetic(opmode uint16, dst int, src float.X80, exc uint8) {
	e := f.prepare()
	d := f.FP[dst]
	signaling := src.IsSignalingNaN()
	dyadic := opmode >= 0x20 && opmode <= 0x28 || opmode == 0x38
	if dyadic {
		signaling = signaling || d.IsSignalingNaN()
	}

	var z float.X80
	store := true
	switch opmode {
	case 0x00:
		z = e.RoundToPrecision(src, e.RoundingPrecision)
	case 0x01:
		z = e.RoundToInt(src)
	case 0x02:
		z = e.Sinh(src)
	case 0x03:
		z = e.Trunc(src)
	case 0x04:
		z = e.Sqrt(src)
	case 0x06:
		z = e.Log1p(src)
	case 0x08:
		z = e.Expm1(src)
	case 0x09:
		z = e.Tanh(src)
	case 0x0a:
		z = e.Atan(src)
	case 0x0c:
		z = e.Asin(src)
	case 0x0d:
		z = e.Atanh(src)
	case 0x0e:
		z = e.Sin(src)
	case 0x0f:
		z = e.Tan(src)
	case 0x10:
		z = e.Exp(src)
	case 0x11:
		z = e.Exp2(src)
	case 0x12:
		z = e.Exp10(src)
	case 0x14:
		z = e.Ln(src)
	case 0x15:
		z = e.Log10(src)
	case 0x16:
		z = e.Log2(src)
	case 0x18:
		z = e.RoundToPrecision(src.Abs(), e.RoundingPrecision)
	case 0x19:
		z = e.Cosh(src)
	case 0x1a:
		z = e.RoundToPrecision(src.Neg(), e.RoundingPrecision)
	case 0x1c:
		z = e.Acos(src)
	case 0x1d:
		z = e.Cos(src)
	case 0x1e:
		z = e.GetExp(src)
	case 0x1f:
		z = e.GetMan(src)
	case 0x20:
		z = e.Div(d, src)
	case 0x21:
		var quo int
		z, quo = e.ModQuo(d, src)
		f.setQuotient(quo)
	case 0x22:
		z = e.Add(d, src)
	case 0x23:
		z = e.Mul(d, src)
	case 0x24:
		z = e.SglDiv(d, src)
	case 0x25:
		var quo int
		z, quo = e.RemQuo(d, src)
		f.setQuotient(quo)
	case 0x26:
		z = f.scale(e, d, src)
	case 0x27:
		z = e.SglMul(d, src)
	case 0x28:
		z = e.Sub(d, src)
	case 0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37:
		var c float.X80
		z, c = e.Sincos(src)
		f.FP[opmode&7] = e.RoundToPrecision(c, e.RoundingPrecision)
	case 0x38:
		z, store = compare(d, src), false
	case 0x3a:
		z, store = src, false
	}
	if store {
		// Results the operation rounded to extended precision are rounded
		// to the FPCR precision; for the others this changes nothing.
		z = e.RoundToPrecision(z, e.RoundingPrecision)
		f.FP[dst] = z
	}
	if !store && signaling {
		e.Raise(float.ExceptionInvalid)
	}
	f.setConditionCodes(z)
	f.report(exc | exceptions(e.Exception, signaling))
}

// scale is FSCALE: d * 2^n with n the integer part of src, toward zero.
func (f *FPU) scale(e *float.Env, d, src float.X80) float.X80 {
	switch {
	case d.IsNaN() || src.IsNaN():
		return e.Mul(d, src) // propagates the NaN
	case src.IsInf():
		e.Raise(float.ExceptionInvalid)
		return nan
	}
	var q float.Env
	return e.Scale(d, int(q.ToInt32RoundZero(src)))
}

// compare returns a value whose condition codes are those of FCMP: the
// difference d - src, exact and without exceptions, with infinities of the
// same sign comparing equal.
func compare(d, src float.X80) float.X80 {
	if d.IsInf() && src.IsInf() && d.Signbit() == src.Signbit() {
		if d.Signbit() {
			return float.X80Zero.Neg()
		}
		return float.X80Zero
	}
	var q float.Env
	return q.Sub(d, src)
}
