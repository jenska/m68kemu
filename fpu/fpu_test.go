package fpu

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/jenska/float"
)

// command builds a command word from its class, source field, destination
// register and opmode or extension field.
func command(class, src, dst, op uint16) uint16 {
	return class<<13 | src<<10 | dst<<7 | op
}

func doubleBytes(v float64) []byte {
	return binary.BigEndian.AppendUint64(nil, math.Float64bits(v))
}

// run decodes and executes cmd.
func run(t *testing.T, f *FPU, cmd uint16, dynamic uint32, in []byte) []byte {
	t.Helper()
	op, err := Decode(cmd)
	if err != nil {
		t.Fatalf("Decode(%04x): %v", cmd, err)
	}
	op.SetDynamic(dynamic)
	if len(in) != op.In {
		t.Fatalf("command %04x takes %d bytes, test passes %d", cmd, op.In, len(in))
	}
	out, err := f.Execute(op, dynamic, in)
	if err != nil {
		t.Fatalf("Execute(%04x): %v", cmd, err)
	}
	if len(out) != op.Out {
		t.Fatalf("command %04x returned %d bytes, want %d", cmd, len(out), op.Out)
	}
	return out
}

// loadDouble runs FMOVE.D #v,FPn.
func loadDouble(t *testing.T, f *FPU, n uint16, v float64) {
	t.Helper()
	run(t, f, command(classMemory, uint16(FormatDouble), n, 0x00), 0, doubleBytes(v))
}

// dyadic runs the opmode with FPn as destination and the double v as source.
func dyadic(t *testing.T, f *FPU, opmode, n uint16, v float64) {
	t.Helper()
	run(t, f, command(classMemory, uint16(FormatDouble), n, opmode), 0, doubleBytes(v))
}

// storeDouble runs FMOVE.D FPn,<ea> and returns the value.
func storeDouble(t *testing.T, f *FPU, n uint16) float64 {
	t.Helper()
	return math.Float64frombits(binary.BigEndian.Uint64(run(t, f, command(classStore, uint16(FormatDouble), n, 0), 0, nil)))
}

func exceptionByte(f *FPU) uint8 { return uint8(f.FPSR >> 8) }

func TestReset(t *testing.T) {
	f := New(MC68881)
	for i, x := range f.FP {
		if hi, lo := x.Bits(); hi != 0x7fff || lo != 0xffffffffffffffff {
			t.Fatalf("FP%d = %04x %016x, want the non-signaling NaN 7fff ffffffffffffffff", i, hi, lo)
		}
	}
	if f.FPCR != 0 || f.FPSR != 0 || f.FPIAR != 0 {
		t.Fatalf("FPCR=%x FPSR=%x FPIAR=%x, want zero", f.FPCR, f.FPSR, f.FPIAR)
	}
	if !bytes.Equal(f.Save(), []byte{0, 0, 0, 0}) {
		t.Fatalf("FSAVE after reset = % x, want a null frame", f.Save())
	}
}

func TestArithmetic(t *testing.T) {
	for _, tt := range []struct {
		name   string
		opmode uint16
		a, b   float64
		want   float64
	}{
		{"FADD", 0x22, 1.5, 2.25, 3.75},
		{"FSUB", 0x28, 1.5, 2.25, -0.75},
		{"FMUL", 0x23, 1.5, 2.25, 3.375},
		{"FDIV", 0x20, 3, 1.5, 2},
		{"FSGLMUL", 0x27, 1.5, 2.25, 3.375},
		{"FSGLDIV", 0x24, 3, 1.5, 2},
		{"FSCALE", 0x26, 3, 2.9, 12},
		{"FMOD", 0x21, 7, 2, 1},
		{"FREM", 0x25, 7, 2, -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := New(MC68881)
			loadDouble(t, f, 3, tt.a)
			dyadic(t, f, tt.opmode, 3, tt.b)
			if got := storeDouble(t, f, 3); got != tt.want {
				t.Fatalf("%v %s %v = %v, want %v", tt.a, tt.name, tt.b, got, tt.want)
			}
		})
	}
}

func TestMonadic(t *testing.T) {
	for _, tt := range []struct {
		name   string
		opmode uint16
		in     float64
		want   float64
	}{
		{"FMOVE", 0x00, -2.5, -2.5},
		{"FINT", 0x01, 2.5, 2},
		{"FINTRZ", 0x03, -2.75, -2},
		{"FSQRT", 0x04, 2.25, 1.5},
		{"FABS", 0x18, -2.5, 2.5},
		{"FNEG", 0x1a, 2.5, -2.5},
		{"FGETEXP", 0x1e, 12, 3},
		{"FGETMAN", 0x1f, 12, 1.5},
		{"FETOX", 0x10, 0, 1},
		{"FTWOTOX", 0x11, 10, 1024},
		{"FTENTOX", 0x12, 3, 1000},
		{"FLOG2", 0x16, 1024, 10},
		{"FLOG10", 0x15, 1000, 3},
		{"FLOGN", 0x14, 1, 0},
		{"FCOS", 0x1d, 0, 1},
		{"FSIN", 0x0e, 0, 0},
		{"FATAN", 0x0a, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := New(MC68881)
			run(t, f, command(classMemory, uint16(FormatDouble), 1, tt.opmode), 0, doubleBytes(tt.in))
			if got := storeDouble(t, f, 1); got != tt.want {
				t.Fatalf("%s(%v) = %v, want %v", tt.name, tt.in, got, tt.want)
			}
		})
	}
}

func TestRegisterToRegisterAndSincos(t *testing.T) {
	f := New(MC68881)
	loadDouble(t, f, 1, 0.5)
	loadDouble(t, f, 2, 0.25)
	run(t, f, command(classRegister, 2, 1, 0x22), 0, nil) // FADD FP2,FP1
	if got := storeDouble(t, f, 1); got != 0.75 {
		t.Fatalf("FADD FP2,FP1 = %v, want 0.75", got)
	}
	run(t, f, command(classRegister, 1, 4, 0x30|5), 0, nil) // FSINCOS FP1,FP5:FP4
	e := float.Env{}
	s, c := e.Sincos(float.Float64ToFloatX80(0.75))
	if f.FP[4] != s || f.FP[5] != c {
		t.Fatalf("FSINCOS: sin %v cos %v, want %v %v", f.FP[4], f.FP[5], s, c)
	}
}

func TestLoadFormats(t *testing.T) {
	x := float.Float64ToFloatX80(-1.25)
	for _, tt := range []struct {
		name   string
		format Format
		in     []byte
		want   float64
	}{
		{"Byte", FormatByte, []byte{0xfe}, -2},
		{"Word", FormatWord, []byte{0xfe, 0xd4}, -300},
		{"Long", FormatLong, binary.BigEndian.AppendUint32(nil, 100000), 100000},
		{"Single", FormatSingle, binary.BigEndian.AppendUint32(nil, math.Float32bits(0.5)), 0.5},
		{"Double", FormatDouble, doubleBytes(1e300), 1e300},
		{"Extended", FormatExtended, x.Bytes96(binary.BigEndian), -1.25},
		{"Packed", FormatPacked, []byte{0x80, 0x01, 0, 0x01, 0x25, 0, 0, 0, 0, 0, 0, 0}, -12.5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := New(MC68881)
			run(t, f, command(classMemory, uint16(tt.format), 0, 0x00), 0, tt.in)
			if got := storeDouble(t, f, 0); got != tt.want {
				t.Fatalf("loaded %v, want %v", got, tt.want)
			}
			if exceptionByte(f) != 0 {
				t.Fatalf("exceptions %08b, want none", exceptionByte(f))
			}
		})
	}
}

func TestStoreIntegerRounding(t *testing.T) {
	for _, tt := range []struct {
		mode uint32
		in   float64
		want int32
	}{
		{0, 2.5, 2}, {0, 3.5, 4}, {1, -2.75, -2}, {2, -2.25, -3}, {3, 2.25, 3},
	} {
		f := New(MC68881)
		f.FPCR = tt.mode << 4
		loadDouble(t, f, 0, tt.in)
		out := run(t, f, command(classStore, uint16(FormatLong), 0, 0), 0, nil)
		if got := int32(binary.BigEndian.Uint32(out)); got != tt.want {
			t.Fatalf("FMOVE.L %v in mode %d = %d, want %d", tt.in, tt.mode, got, tt.want)
		}
		if exceptionByte(f) != INEX2 {
			t.Fatalf("FMOVE.L %v: exceptions %08b, want INEX2", tt.in, exceptionByte(f))
		}
	}

	f := New(MC68881)
	loadDouble(t, f, 0, 1e10)
	out := run(t, f, command(classStore, uint16(FormatWord), 0, 0), 0, nil)
	if got := int16(binary.BigEndian.Uint16(out)); got != math.MaxInt16 || exceptionByte(f)&OPERR == 0 {
		t.Fatalf("FMOVE.W 1e10 = %d with exceptions %08b, want %d and OPERR", got, exceptionByte(f), math.MaxInt16)
	}
}

func TestStoreDoesNotChangeConditionCodes(t *testing.T) {
	f := New(MC68881)
	loadDouble(t, f, 0, -1)
	loadDouble(t, f, 1, 0)
	cc := f.FPSR & 0x0f000000
	storeDouble(t, f, 0)
	if f.FPSR&0x0f000000 != cc {
		t.Fatalf("FMOVE to memory changed the condition codes from %08x to %08x", cc, f.FPSR&0x0f000000)
	}
}
