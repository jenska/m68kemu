package m68kemu

import (
	"encoding/binary"
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
		return uint32(binary.BigEndian.Uint16(ram.mem[address-ram.offset:])), nil
	case Long:
		return binary.BigEndian.Uint32(ram.mem[address-ram.offset:]), nil
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
		binary.BigEndian.PutUint16(ram.mem[address-ram.offset:], uint16(value))
	case Long:
		binary.BigEndian.PutUint32(ram.mem[address-ram.offset:], value)
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
