package fpu

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/jenska/float"
)

func TestExceptionsAndConditionCodes(t *testing.T) {
	signaling := float.NewFromBits(0x7fff, 0xa000000000000000).Bytes96(binary.BigEndian)
	for _, tt := range []struct {
		name    string
		a       float64
		opmode  uint16
		src     []byte
		format  Format
		exc     uint8
		accrued uint32
		cc      uint32
	}{
		{"DivideByZero", 1, 0x20, doubleBytes(0), FormatDouble, DZ, accruedDZ, CCI},
		{"ZeroByZero", 0, 0x20, doubleBytes(0), FormatDouble, OPERR, accruedIOP, CCNaN}, // the default NaN is positive
		{"SqrtNegative", 0, 0x04, doubleBytes(-1), FormatDouble, OPERR, accruedIOP, CCNaN},
		{"SignalingNaN", 1, 0x22, signaling, FormatExtended, SNAN, accruedIOP, CCNaN},
		{"Overflow", math.MaxFloat64, 0x23, doubleBytes(math.MaxFloat64), FormatDouble, 0, 0, 0},
		{"Inexact", 1, 0x20, doubleBytes(3), FormatDouble, INEX2, accruedINEX, 0},
		{"NegativeZero", 0, 0x23, doubleBytes(-1), FormatDouble, 0, 0, CCZ | CCN},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := New(MC68881)
			loadDouble(t, f, 0, tt.a)
			run(t, f, command(classMemory, uint16(tt.format), 0, tt.opmode), 0, tt.src)
			if tt.name == "Overflow" {
				// Extended precision has room for MaxFloat64^2; store it as
				// a double to overflow.
				storeDouble(t, f, 0)
				tt.exc, tt.accrued = OVFL|INEX2, accruedOVFL|accruedINEX
			}
			if got := exceptionByte(f); got != tt.exc {
				t.Fatalf("exception byte %08b, want %08b", got, tt.exc)
			}
			if got := f.FPSR & 0xf8; got != tt.accrued {
				t.Fatalf("accrued byte %08b, want %08b", got, tt.accrued)
			}
			if tt.name != "Overflow" {
				if got := f.FPSR & 0x0f000000; got != tt.cc {
					t.Fatalf("condition codes %08x, want %08x", got, tt.cc)
				}
			}
		})
	}
}

func TestRoundingPrecisionAndMode(t *testing.T) {
	third := func(fpcr uint32) float.X80 {
		f := New(MC68881)
		f.FPCR = fpcr
		loadDouble(t, f, 0, 1)
		dyadic(t, f, 0x20, 0, 3)
		return f.FP[0]
	}
	single := third(1 << 6)
	if got := single.ToFloat64(); got != float64(float32(1.0/3.0)) {
		t.Fatalf("1/3 in single precision = %v, want %v", got, float64(float32(1.0/3.0)))
	}
	double := third(2 << 6)
	if got := double.ToFloat64(); got != 1.0/3.0 {
		t.Fatalf("1/3 in double precision = %v, want %v", got, 1.0/3.0)
	}
	nearest, down := third(0), third(2<<4)
	_, ln := nearest.Bits()
	_, ld := down.Bits()
	if ln != ld+1 {
		t.Fatalf("1/3 rounded to nearest %016x and down %016x should differ in the last bit", ln, ld)
	}
}

func TestFmovecr(t *testing.T) {
	for _, tt := range []struct {
		offset uint16
		want   float.X80
		exc    uint8
	}{
		{0x00, float.X80Pi, INEX2},
		{0x0f, float.X80Zero, 0},
		{0x32, float.X80One, 0},
		{0x34, float.Int32ToFloatX80(100), 0},
		{0x3f, float.Pow10(4096), INEX2},
		{0x10, float.X80Zero, 0}, // no documented constant
	} {
		f := New(MC68881)
		run(t, f, command(classMemory, 7, 2, tt.offset), 0, nil)
		if f.FP[2] != tt.want || exceptionByte(f) != tt.exc {
			t.Fatalf("FMOVECR #$%02x = %v with exceptions %08b, want %v and %08b", tt.offset, f.FP[2], exceptionByte(f), tt.want, tt.exc)
		}
	}
}

func TestQuotientByte(t *testing.T) {
	f := New(MC68881)
	loadDouble(t, f, 0, 7)
	dyadic(t, f, 0x25, 0, 2) // FREM: 7 = 4*2 - 1
	if q := f.FPSR >> 16 & 0xff; q != 4 {
		t.Fatalf("FREM 7,2 quotient byte %02x, want 04", q)
	}
	loadDouble(t, f, 0, -7)
	dyadic(t, f, 0x21, 0, 2) // FMOD: -7 = -3*2 - 1
	if q := f.FPSR >> 16 & 0xff; q != 0x83 {
		t.Fatalf("FMOD -7,2 quotient byte %02x, want 83", q)
	}
}

func TestCompareAndTest(t *testing.T) {
	for _, tt := range []struct {
		a, b float64
		cc   uint32
	}{
		{1, 2, CCN},
		{2, 2, CCZ},
		{2, 1, 0},
		{math.Inf(1), math.Inf(1), CCZ},
		{math.Inf(-1), math.Inf(-1), CCZ | CCN},
		{math.NaN(), 1, CCNaN},
	} {
		f := New(MC68881)
		loadDouble(t, f, 0, tt.a)
		dyadic(t, f, 0x38, 0, tt.b) // FCMP
		if got := f.FPSR & 0x0f000000 &^ CCN; tt.cc&CCNaN != 0 && got != CCNaN {
			t.Fatalf("FCMP %v,%v condition codes %08x, want NAN", tt.b, tt.a, f.FPSR&0x0f000000)
		} else if tt.cc&CCNaN == 0 && f.FPSR&0x0f000000 != tt.cc {
			t.Fatalf("FCMP %v,%v condition codes %08x, want %08x", tt.b, tt.a, f.FPSR&0x0f000000, tt.cc)
		}
		if f.FP[0].ToFloat64() != tt.a && !math.IsNaN(tt.a) {
			t.Fatalf("FCMP changed FP0")
		}
	}

	f := New(MC68881)
	run(t, f, command(classMemory, uint16(FormatDouble), 0, 0x3a), 0, doubleBytes(-3)) // FTST
	if f.FPSR&0x0f000000 != CCN {
		t.Fatalf("FTST -3 condition codes %08x, want N", f.FPSR&0x0f000000)
	}
}

func TestConditionPredicates(t *testing.T) {
	type cc struct{ n, z, nan bool }
	set := func(f *FPU, c cc) {
		f.FPSR = 0
		if c.n {
			f.FPSR |= CCN
		}
		if c.z {
			f.FPSR |= CCZ
		}
		if c.nan {
			f.FPSR |= CCNaN
		}
	}
	less, equal, greater, unordered := cc{n: true}, cc{z: true}, cc{}, cc{nan: true}
	// want[pred] = results for less, equal, greater, unordered
	want := map[uint16][4]bool{
		0x00: {false, false, false, false}, // F
		0x01: {false, true, false, false},  // EQ
		0x02: {false, false, true, false},  // OGT
		0x03: {false, true, true, false},   // OGE
		0x04: {true, false, false, false},  // OLT
		0x05: {true, true, false, false},   // OLE
		0x06: {true, false, true, false},   // OGL
		0x07: {true, true, true, false},    // OR
		0x08: {false, false, false, true},  // UN
		0x09: {false, true, false, true},   // UEQ
		0x0a: {false, false, true, true},   // UGT
		0x0b: {false, true, true, true},    // UGE
		0x0c: {true, false, false, true},   // ULT
		0x0d: {true, true, false, true},    // ULE
		0x0e: {true, false, true, true},    // NE
		0x0f: {true, true, true, true},     // T
	}
	for pred, results := range want {
		for _, signaling := range []uint16{0, 0x10} {
			for i, c := range []cc{less, equal, greater, unordered} {
				f := New(MC68881)
				set(f, c)
				got, err := f.Condition(pred | signaling)
				if err != nil || got != results[i] {
					t.Fatalf("predicate %02x with %+v = %v (%v), want %v", pred|signaling, c, got, err, results[i])
				}
				bsun := exceptionByte(f)&BSUN != 0
				if wantBSUN := signaling != 0 && c.nan; bsun != wantBSUN {
					t.Fatalf("predicate %02x with %+v: BSUN = %v, want %v", pred|signaling, c, bsun, wantBSUN)
				}
			}
		}
	}
	if _, err := New(MC68881).Condition(0x20); !errors.Is(err, ErrUnimplemented) {
		t.Fatalf("predicate 20: error %v, want ErrUnimplemented", err)
	}
}

func TestPendingException(t *testing.T) {
	f := New(MC68881)
	f.FPCR = uint32(DZ|INEX2) << 8
	loadDouble(t, f, 0, 1)
	if _, ok := f.PendingException(); ok {
		t.Fatalf("exception pending before any enabled exception")
	}
	dyadic(t, f, 0x20, 0, 0) // 1/0: DZ
	if v, ok := f.PendingException(); !ok || v != VectorDZ {
		t.Fatalf("PendingException = %d, %v; want %d", v, ok, VectorDZ)
	}
	if !f.FP[0].IsInf() {
		t.Fatalf("result %v, want the default result +Inf", f.FP[0])
	}
	f.AcknowledgeException()
	if _, ok := f.PendingException(); ok {
		t.Fatalf("exception still pending after AcknowledgeException")
	}

	f.FPCR = uint32(OPERR|INEX2) << 8
	loadDouble(t, f, 0, 0)
	dyadic(t, f, 0x20, 0, 0) // 0/0: OPERR
	if v, _ := f.PendingException(); v != VectorOPERR {
		t.Fatalf("PendingException = %d, want %d", v, VectorOPERR)
	}
}

func TestPacked(t *testing.T) {
	f := New(MC68881)
	load := func(b []byte) float.X80 {
		run(t, f, command(classMemory, uint16(FormatPacked), 0, 0), 0, b)
		return f.FP[0]
	}
	store := func(x float.X80, k int) []byte {
		f.FP[1] = x
		return run(t, f, command(classStore, uint16(FormatPacked), 1, uint16(k)&0x7f), 0, nil)
	}
	words := func(w0, w1, w2 uint32) []byte {
		return binary.BigEndian.AppendUint32(binary.BigEndian.AppendUint32(binary.BigEndian.AppendUint32(nil, w0), w1), w2)
	}

	if got := load(words(0x40010001, 0, 0)); got.ToFloat64() != 0.1 {
		t.Fatalf("packed 1e-1 = %v, want 0.1", got)
	}
	if exceptionByte(f)&INEX1 == 0 {
		t.Fatalf("loading 0.1 should set INEX1, exceptions %08b", exceptionByte(f))
	}

	minus12 := float.Float64ToFloatX80(-12.5)
	if got, want := store(minus12, 3), words(0x80010001, 0x25000000, 0); !bytes.Equal(got, want) {
		t.Fatalf("-12.5 with k=3 = % x, want % x", got, want)
	}
	if exceptionByte(f) != 0 {
		t.Fatalf("-12.5 is exact, exceptions %08b", exceptionByte(f))
	}
	if got, want := store(float.Float64ToFloatX80(12.25), 0), words(0x00010001, 0x20000000, 0); !bytes.Equal(got, want) {
		t.Fatalf("12.25 with k=0 = % x, want % x (no digits right of the point)", got, want)
	}
	if exceptionByte(f) != INEX2 {
		t.Fatalf("12.25 with k=0 drops digits, exceptions %08b, want INEX2", exceptionByte(f))
	}
	if store(minus12, 18); exceptionByte(f)&OPERR == 0 {
		t.Fatalf("k=18 should set OPERR")
	}
	if got := store(float.Pow10(1000), 1); binary.BigEndian.Uint32(got)&0x0fff0000 != 0 || got[2]>>4 != 1 || exceptionByte(f)&OPERR == 0 {
		t.Fatalf("1e1000 = % x with exceptions %08b, want exponent 000 with EXP3 = 1 and OPERR", got, exceptionByte(f))
	}
	if got, want := store(float.X80InfNeg, 5), words(0xffff0000, 0, 0); !bytes.Equal(got, want) {
		t.Fatalf("-Inf = % x, want % x", got, want)
	}
	if got := load(words(0x7fff0000, 0, 0)); got != float.X80InfPos {
		t.Fatalf("packed +Inf loads as %v", got)
	}

	// Dynamic k-factor from a data register.
	f.FP[1] = float.Float64ToFloatX80(3.3)
	got := run(t, f, command(classStore, uint16(FormatPackedDynamic), 1, 2<<4), 2, nil)
	if want := words(0x00000003, 0x30000000, 0); !bytes.Equal(got, want) {
		t.Fatalf("3.3 with dynamic k=2 = % x, want % x", got, want)
	}
}

func TestControlRegisters(t *testing.T) {
	f := New(MC68881)
	in := binary.BigEndian.AppendUint32(nil, 0xffff_ffff) // FPCR
	in = binary.BigEndian.AppendUint32(in, 0xffff_ffff)   // FPSR
	in = binary.BigEndian.AppendUint32(in, 0x1234_5678)   // FPIAR
	run(t, f, command(classLoadControl, 7, 0, 0), 0, in)
	if f.FPCR != 0xffff || f.FPSR != 0x0ffffff8 || f.FPIAR != 0x12345678 {
		t.Fatalf("FPCR=%08x FPSR=%08x FPIAR=%08x, want ffff, 0ffffff8 and 12345678", f.FPCR, f.FPSR, f.FPIAR)
	}
	out := run(t, f, command(classStoreControl, 2|1, 0, 0), 0, nil) // FPSR/FPIAR
	if want := []byte{0x0f, 0xff, 0xff, 0xf8, 0x12, 0x34, 0x56, 0x78}; !bytes.Equal(out, want) {
		t.Fatalf("FMOVEM FPSR/FPIAR = % x, want % x", out, want)
	}
}

func TestFmovemRegisters(t *testing.T) {
	f := New(MC68881)
	for i := range f.FP {
		f.FP[i] = float.Int32ToFloatX80(int32(i))
	}
	value := func(b []byte, i int) int64 { return float.NewFromBytes96(b[12*i:], binary.BigEndian).ToInt64() }

	// Postincrement/control mode: bit 7 is FP0.
	out := run(t, f, command(classStoreRegisters, 4, 0, 0b1010_0000), 0, nil)
	if value(out, 0) != 0 || value(out, 1) != 2 {
		t.Fatalf("FMOVEM FP0/FP2 to (An) moved %d, %d; want 0, 2", value(out, 0), value(out, 1))
	}
	// Predecrement mode: bit 0 is FP0, registers go from FP7 down.
	out = run(t, f, command(classStoreRegisters, 0, 0, 0b0000_0101), 0, nil)
	if value(out, 0) != 2 || value(out, 1) != 0 {
		t.Fatalf("FMOVEM FP0/FP2 to -(An) moved %d, %d; want 2, 0", value(out, 0), value(out, 1))
	}
	// Dynamic list in D3, loading FP6 and FP7.
	in := append(float.Int32ToFloatX80(60).Bytes96(binary.BigEndian), float.Int32ToFloatX80(70).Bytes96(binary.BigEndian)...)
	run(t, f, command(classLoadRegisters, 4|2, 0, 3<<4), 0b0000_0011, in)
	if f.FP[6].ToInt64() != 60 || f.FP[7].ToInt64() != 70 {
		t.Fatalf("FMOVEM to FP6/FP7 loaded %v, %v; want 60, 70", f.FP[6], f.FP[7])
	}
}

func TestDecodeRejectsUndefinedCommands(t *testing.T) {
	for _, cmd := range []uint16{
		command(1, 0, 0, 0),                // class 001
		command(classRegister, 0, 0, 0x05), // undefined opmode
		command(classRegister, 0, 0, 0x40), // 68040 opmode
		command(classLoadControl, 0, 0, 0), // empty control register list
	} {
		if _, err := Decode(cmd); !errors.Is(err, ErrUnimplemented) {
			t.Fatalf("Decode(%04x) error %v, want ErrUnimplemented", cmd, err)
		}
	}
}

func TestInstructionAddress(t *testing.T) {
	f := New(MC68881)
	f.SetInstructionAddress(0x1000)
	loadDouble(t, f, 0, 1)
	if f.FPIAR != 0x1000 {
		t.Fatalf("FPIAR = %x after an arithmetic instruction, want 1000", f.FPIAR)
	}
	f.SetInstructionAddress(0x2000)
	run(t, f, command(classStoreRegisters, 4, 0, 0x80), 0, nil)
	if f.FPIAR != 0x1000 {
		t.Fatalf("FMOVEM changed FPIAR to %x", f.FPIAR)
	}
}

func TestSaveRestore(t *testing.T) {
	for _, tt := range []struct {
		model Model
		size  int
	}{{MC68881, 0x18}, {MC68882, 0x38}} {
		f := New(tt.model)
		f.FPCR = uint32(DZ) << 8
		loadDouble(t, f, 0, 1)
		dyadic(t, f, 0x20, 0, 0) // pending DZ
		frame := f.Save()
		if len(frame) != 4+tt.size || binary.BigEndian.Uint32(frame) != 0x1f000000|uint32(tt.size)<<16 {
			t.Fatalf("idle frame header %08x, length %d", binary.BigEndian.Uint32(frame), len(frame))
		}

		g := New(tt.model)
		if err := g.Restore(frame); err != nil {
			t.Fatal(err)
		}
		if v, ok := g.PendingException(); !ok || v != VectorDZ {
			t.Fatalf("pending exception after FRESTORE = %d, %v; want %d", v, ok, VectorDZ)
		}
		if err := g.Restore([]byte{0, 0, 0, 0}); err != nil || g.FP[0] != nan {
			t.Fatalf("null frame should reset the FPU (err %v)", err)
		}
		if err := g.Restore([]byte{0x1f, 0x99, 0, 0}); !errors.Is(err, ErrFrameFormat) {
			t.Fatalf("bad frame error %v, want ErrFrameFormat", err)
		}
	}
}
