package m68kemu

type Size uint32

func (s Size) mask() uint32 {
	switch s {
	case Byte:
		return 0xff
	case Word:
		return 0xffff
	default:
		return 0xffffffff
	}
}

func (s Size) signBit() uint32 {
	switch s {
	case Byte:
		return 0x80
	case Word:
		return 0x8000
	default:
		return 0x80000000
	}
}

func (s Size) isZero(value uint32) bool {
	return value&s.mask() == 0
}

func (s Size) isNegative(value uint32) bool {
	return value&s.signBit() != 0
}

var opSizes = []Size{Byte, Word, Long, Byte, Byte, Word, Long}

func operandSizeFromOpmode(opmode uint16) Size {
	return opSizes[opmode&0x7]
}
