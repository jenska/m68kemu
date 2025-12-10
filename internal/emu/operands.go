package emu

// Size contains properties of the M68K operad types
type Size struct {
	size  uint32
	align uint32
	mask  uint32
	bits  uint32
	msb   uint32
}

var (
	// Byte is an M68K operand
	Byte = &Size{
		size:  1,
		align: 2,
		mask:  0x000000ff,
		bits:  8,
		msb:   0x00000080,
	}

	// Word is an M68K operand type
	Word = &Size{
		size:  2,
		align: 2,
		mask:  0x0000ffff,
		bits:  16,
		msb:   0x00008000,
	}

	// Long is an M68K operand type
	Long = &Size{
		size:  4,
		align: 4,
		mask:  0xffffffff,
		bits:  32,
		msb:   0x80000000,
	}
)

// IsNegative tests an operand of a specifc size if it is negative
func (s *Size) IsNegative(value int32) bool {
	return s.msb&uint32(value) != 0
}

func (s *Size) uset(value uint32, target *uint32) {
	result := (uint32(*target) & ^s.mask) | (uint32(value) & s.mask)
	*target = result
}
