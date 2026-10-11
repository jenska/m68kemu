package m68kemu

import (
	"fmt"
)

// RAM is flat read/write memory at a fixed address range. It is a
// MemoryDevice, so the CPU accesses it directly.
type RAM struct {
	offset uint32
	mem    []byte
}

func (ram *RAM) Contains(address uint32) bool {
	return address >= ram.offset && address < ram.offset+uint32(len(ram.mem))
}

func (ram *RAM) AddressRange() (uint32, uint32) {
	if len(ram.mem) == 0 {
		return ram.offset, ram.offset
	}
	return ram.offset, ram.offset + uint32(len(ram.mem)) - 1
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

func (ram *RAM) Peek(s Size, address uint32) (uint32, error) {
	return ram.Read(s, address)
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
	// CPU RESET does not erase system RAM on a 68000-based machine such as the Atari ST.
}

// NewRAM returns size bytes of zeroed RAM starting at address offset.
func NewRAM(offset, size uint32) *RAM {
	return &RAM{offset: offset, mem: make([]byte, size)}
}

// Memory implements MemoryDevice.
func (ram *RAM) Memory() ([]byte, bool) {
	return ram.mem, false
}

// Bytes returns the RAM's contents; index 0 is the byte at its start address.
// The slice is the live backing store.
func (ram *RAM) Bytes() []byte {
	return ram.mem
}

// Load copies data into the RAM at address. It returns a BusError, and
// copies nothing, when data does not fit.
func (ram *RAM) Load(address uint32, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if address < ram.offset || uint64(address-ram.offset)+uint64(len(data)) > uint64(len(ram.mem)) {
		return BusError(address)
	}
	copy(ram.mem[address-ram.offset:], data)
	return nil
}

// ROM is read-only memory at a fixed address range. Writes to it are bus
// errors. It is a MemoryDevice, so the CPU reads it directly.
type ROM RAM

// NewROM returns a ROM at address offset holding a copy of data.
func NewROM(offset uint32, data []byte) *ROM {
	return &ROM{offset: offset, mem: append([]byte(nil), data...)}
}

func (rom *ROM) AddressRange() (uint32, uint32) { return (*RAM)(rom).AddressRange() }

func (rom *ROM) Read(s Size, address uint32) (uint32, error) {
	return (*RAM)(rom).Read(s, address)
}

func (rom *ROM) Peek(s Size, address uint32) (uint32, error) {
	return (*RAM)(rom).Read(s, address)
}

// Write fails with a BusError.
func (rom *ROM) Write(s Size, address uint32, value uint32) error {
	return BusError(address)
}

func (rom *ROM) Reset() {}

// Memory implements MemoryDevice.
func (rom *ROM) Memory() ([]byte, bool) {
	return rom.mem, true
}

// Bytes returns the ROM's contents; index 0 is the byte at its start address.
func (rom *ROM) Bytes() []byte {
	return rom.mem
}
