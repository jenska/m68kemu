package fpu

import (
	"encoding/binary"
	"strconv"
	"strings"

	"github.com/jenska/float"
)

// Format is the data format of an operand, as encoded in bits 12-10 of a
// command word.
type Format uint16

const (
	FormatLong     Format = 0 // 32-bit integer
	FormatSingle   Format = 1 // IEEE single precision
	FormatExtended Format = 2 // 96-bit extended precision
	FormatPacked   Format = 3 // 96-bit packed decimal (static k-factor when stored)
	FormatWord     Format = 4 // 16-bit integer
	FormatDouble   Format = 5 // IEEE double precision
	FormatByte     Format = 6 // 8-bit integer
	// FormatPackedDynamic is packed decimal with the k-factor taken from a
	// data register; it only exists for FMOVE to memory.
	FormatPackedDynamic Format = 7
)

// Size returns the size of an operand of format f in bytes.
func (f Format) Size() int {
	switch f {
	case FormatLong, FormatSingle:
		return 4
	case FormatExtended, FormatPacked, FormatPackedDynamic:
		return 12
	case FormatWord:
		return 2
	case FormatDouble:
		return 8
	case FormatByte:
		return 1
	}
	return 0
}

// load converts the operand b of format f to an extended value and returns
// the exceptions of the conversion: SNAN for a single or double signaling
// NaN (which is quieted), INEX1 for an inexact packed decimal, and OVFL or
// UNFL for a packed decimal out of range.
func (f *FPU) load(format Format, b []byte) (float.X80, uint8) {
	var q float.Env
	switch format {
	case FormatByte:
		return float.Int32ToFloatX80(int32(int8(b[0]))), 0
	case FormatWord:
		return float.Int32ToFloatX80(int32(int16(binary.BigEndian.Uint16(b)))), 0
	case FormatLong:
		return float.Int32ToFloatX80(int32(binary.BigEndian.Uint32(b))), 0
	case FormatSingle:
		x := q.NewFromFloat32Bits(binary.BigEndian.Uint32(b))
		return x, exceptions(q.Exception, true)
	case FormatDouble:
		x := q.NewFromFloat64Bits(binary.BigEndian.Uint64(b))
		return x, exceptions(q.Exception, true)
	case FormatExtended:
		return float.NewFromBytes96(b, binary.BigEndian), 0
	case FormatPacked:
		return f.loadPacked(b)
	}
	return nan, OPERR
}

// store converts x to format with the k-factor k for packed decimal, and
// returns the bytes and the exceptions of the conversion.
func (f *FPU) store(format Format, x float.X80, k int) ([]byte, uint8) {
	e := f.prepare()
	signaling := x.IsSignalingNaN()
	b := make([]byte, format.Size())
	switch format {
	case FormatByte:
		b[0] = byte(e.ToInt8(x))
	case FormatWord:
		binary.BigEndian.PutUint16(b, uint16(e.ToInt16(x)))
	case FormatLong:
		binary.BigEndian.PutUint32(b, uint32(e.ToInt32(x)))
	case FormatSingle:
		binary.BigEndian.PutUint32(b, e.ToFloat32Bits(x))
	case FormatDouble:
		binary.BigEndian.PutUint64(b, e.ToFloat64Bits(x))
	case FormatExtended:
		return x.Bytes96(binary.BigEndian), 0
	case FormatPacked, FormatPackedDynamic:
		return storePacked(x, k)
	}
	return b, exceptions(e.Exception, signaling)
}

// Packed decimal: three longs. The first holds the sign of the mantissa
// (bit 31), the sign of the exponent (bit 30), three exponent digits (bits
// 27-16), a fourth exponent digit (bits 15-12) and the integer digit of the
// mantissa (bits 3-0); the other two hold 16 fraction digits. An exponent
// field of all ones encodes an infinity (zero mantissa) or a NaN.

const packedSpecial = 0x7fff0000

func (f *FPU) loadPacked(b []byte) (float.X80, uint8) {
	w0 := binary.BigEndian.Uint32(b)
	frac := binary.BigEndian.Uint64(b[4:])
	sign := uint16(w0>>31) << 15
	if w0&packedSpecial == packedSpecial {
		if frac == 0 && w0&0xf == 0 {
			return float.NewFromBits(sign|0x7fff, 1<<63), 0
		}
		return float.NewFromBits(sign|0x7fff, frac|1<<63), 0
	}

	digit := func(v uint32) byte {
		return '0' + byte(min(v&0xf, 9)) // non-decimal digits are undefined
	}
	var s strings.Builder
	if w0>>31 != 0 {
		s.WriteByte('-')
	}
	s.WriteByte(digit(w0))
	s.WriteByte('.')
	for i := 60; i >= 0; i -= 4 {
		s.WriteByte(digit(uint32(frac >> i)))
	}
	exp := int(min((w0>>12)&0xf, 9))*1000 + int(min((w0>>24)&0xf, 9))*100 +
		int(min((w0>>20)&0xf, 9))*10 + int(min((w0>>16)&0xf, 9))
	if w0&(1<<30) != 0 {
		exp = -exp
	}
	s.WriteString("e" + strconv.Itoa(exp))

	// The decimal input is rounded to extended precision in the FPCR's
	// rounding mode; inexactness is INEX1.
	q := float.Env{RoundingMode: int((f.FPCR >> 4) & 3), DefaultNaN: nan}
	x, err := q.Parse(s.String())
	if err != nil {
		return nan, OPERR
	}
	exc := exceptions(q.Exception&^float.ExceptionInexact, false)
	if q.Exception&float.ExceptionInexact != 0 {
		exc |= INEX1
	}
	return x, exc
}

// storePacked converts x to packed decimal. A k-factor of 1 to 17 gives the
// number of significant digits; zero or less gives the number of digits
// right of the decimal point. A k-factor above 17 or an exponent that needs
// four digits sets OPERR. The digits are rounded to nearest.
func storePacked(x float.X80, k int) ([]byte, uint8) {
	b := make([]byte, 12)
	var exc uint8
	if k > 17 {
		k, exc = 17, OPERR
	}
	var w0 uint32
	if x.Signbit() {
		w0 = 1 << 31
	}
	switch {
	case x.IsNaN():
		_, low := x.Bits()
		if x.IsSignalingNaN() {
			exc |= SNAN
			low |= 1 << 62
		}
		binary.BigEndian.PutUint32(b, w0|packedSpecial)
		binary.BigEndian.PutUint64(b[4:], low&^(1<<63))
		return b, exc
	case x.IsInf():
		binary.BigEndian.PutUint32(b, w0|packedSpecial)
		return b, exc
	case x.IsZero():
		binary.BigEndian.PutUint32(b, w0)
		return b, exc
	}

	a := x.Abs()
	digits := k
	if k <= 0 {
		_, e10 := decimalDigits(a.Format('e', 24))
		digits = e10 + 1 - k
	}
	digits = min(max(digits, 1), 17)
	mant, exp := decimalDigits(a.Format('e', digits-1))
	if tail, _ := decimalDigits(a.Format('e', digits-1+25)); strings.TrimRight(tail[digits:], "0") != "" {
		exc |= INEX2
	}

	if exp < 0 {
		w0 |= 1 << 30
		exp = -exp
	}
	if exp > 999 {
		exc |= OPERR
		w0 |= uint32(exp/1000%10) << 12
	}
	w0 |= uint32(exp/100%10)<<24 | uint32(exp/10%10)<<20 | uint32(exp%10)<<16
	w0 |= uint32(mant[0] - '0')
	var frac uint64
	for i := 1; i <= 16; i++ {
		frac <<= 4
		if i < len(mant) {
			frac |= uint64(mant[i] - '0')
		}
	}
	binary.BigEndian.PutUint32(b, w0)
	binary.BigEndian.PutUint64(b[4:], frac)
	return b, exc
}

// decimalDigits splits the 'e' format output "d.ddde±xx" into its digits and
// its decimal exponent.
func decimalDigits(s string) (string, int) {
	mant, exp, _ := strings.Cut(s, "e")
	e, _ := strconv.Atoi(exp)
	return strings.Replace(mant, ".", "", 1), e
}
