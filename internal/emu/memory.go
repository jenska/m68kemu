package emu

import "encoding/binary"

// simple flat memory structure
type RAM struct {
	offset uint32
	mem    []byte
}

func (ram *RAM) rangeCheck(address uint32, s *Size) bool {
	return address >= ram.offset && address < ram.offset+uint32(len(ram.mem)-int(s.size))
}

func (ram *RAM) ReadLongFrom(address uint32) (uint32, error) {
	if !ram.rangeCheck(address, Long) {
		return 0, BusError(address)
	}
	return binary.BigEndian.Uint32(ram.mem[address-ram.offset:]), nil
}

func (ram *RAM) WriteLongTo(address uint32, value uint32) error {
	if !ram.rangeCheck(address, Long) {
		return BusError(address)
	}
	binary.BigEndian.PutUint32(ram.mem[address-ram.offset:], value)
	return nil
}

func (ram *RAM) ReadWordFrom(address uint32) (uint16, error) {
	if !ram.rangeCheck(address, Word) {
		return 0, BusError(address)
	}
	return binary.BigEndian.Uint16(ram.mem[address-ram.offset:]), nil
}

func (ram *RAM) WriteWordTo(address uint32, value uint16) error {
	if !ram.rangeCheck(address, Word) {
		return BusError(address)
	}
	binary.BigEndian.PutUint16(ram.mem[address-ram.offset:], value)
	return nil
}

func (ram *RAM) ReadByteFrom(address uint32) (uint8, error) {
	if !ram.rangeCheck(address, Word) {
		return 0, BusError(address)
	}
	return ram.mem[address-ram.offset], nil
}

func (ram *RAM) WriteByteTo(address uint32, value uint8) error {
	if !ram.rangeCheck(address, Word) {
		return BusError(address)
	}
	ram.mem[address-ram.offset] = value
	return nil
}

func (ram *RAM) Reset() {
	for i := range ram.mem {
		ram.mem[i] = 0
	}
}

func NewRAM(offset, size uint32) RAM {
	return RAM{offset: 0, mem: make([]byte, size)}
}
