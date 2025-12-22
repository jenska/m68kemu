package m68kemu

import (
	"fmt"
)

// simple flat memory structure
type RAM struct {
	offset uint32
	mem    []byte
}

// WaitStates allows RAM to satisfy WaitStateDevice while imposing no additional
// delay.
func (ram *RAM) WaitStates(Size, uint32) uint32 { return 0 }

func (ram *RAM) Contains(address uint32) bool {
	return address >= ram.offset && address < ram.offset+uint32(len(ram.mem))
}

func (ram *RAM) rangeCheck(address uint32, s Size) bool {
	end := address + uint32(s) - 1
	return address >= ram.offset && end < ram.offset+uint32(len(ram.mem))
}

func (ram *RAM) Read(s Size, address uint32) (uint32, error) {
	if !ram.rangeCheck(address, s) {
		return 0, BusError(address)
	}
	switch s {
	case Byte:
		return uint32(ram.mem[address-ram.offset]), nil
	case Word:
		idx := address - ram.offset
		return uint32(ram.mem[idx])<<8 | uint32(ram.mem[idx+1]), nil
	case Long:
		idx := address - ram.offset
		return uint32(ram.mem[idx])<<24 | uint32(ram.mem[idx+1])<<16 | uint32(ram.mem[idx+2])<<8 | uint32(ram.mem[idx+3]), nil
	}
	return 0, fmt.Errorf("unknown size %d", s)
}

func (ram *RAM) Write(s Size, address uint32, value uint32) error {
	if !ram.rangeCheck(address, s) {
		return BusError(address)
	}
	switch s {
	case Byte:
		ram.mem[address-ram.offset] = uint8(value)
	case Word:
		idx := address - ram.offset
		ram.mem[idx] = uint8(value >> 8)
		ram.mem[idx+1] = uint8(value)
	case Long:
		idx := address - ram.offset
		ram.mem[idx] = uint8(value >> 24)
		ram.mem[idx+1] = uint8(value >> 16)
		ram.mem[idx+2] = uint8(value >> 8)
		ram.mem[idx+3] = uint8(value)
	}
	return nil
}

func (ram *RAM) Reset() {
	for i := range ram.mem {
		ram.mem[i] = 0
	}
}

func NewRAM(offset, size uint32) *RAM {
	return &RAM{offset: offset, mem: make([]byte, size)}
}
