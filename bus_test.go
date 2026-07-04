package m68kemu

import (
	"errors"
	"testing"
)

type stubDevice struct {
	start uint32
	end   uint32
	data  map[uint32]uint32
}

func newStubDevice(start, end uint32) *stubDevice {
	return &stubDevice{
		start: start,
		end:   end,
		data:  make(map[uint32]uint32),
	}
}

func (d *stubDevice) Contains(address uint32) bool {
	return address >= d.start && address <= d.end
}

func (d *stubDevice) Read(_ Size, address uint32) (uint32, error) {
	if address < d.start || address > d.end {
		return 0, BusError(address)
	}
	return d.data[address], nil
}

func (d *stubDevice) Write(_ Size, address uint32, value uint32) error {
	if address < d.start || address > d.end {
		return BusError(address)
	}
	d.data[address] = value
	return nil
}

func (d *stubDevice) Reset() {}

func TestBusAlignmentErrors(t *testing.T) {
	ram := NewRAM(0x0000, 0x0010)
	bus := NewBus(ram)

	tests := []struct {
		name    string
		size    Size
		address uint32
	}{
		{"word on odd", Word, 0x0003},
		{"long on odd", Long, 0x0005},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := bus.Read(tt.size, tt.address)
			expectAddressError(t, err)

			err = bus.Write(tt.size, tt.address, 0xFF)
			expectAddressError(t, err)
		})
	}

	t.Run("byte on odd", func(t *testing.T) {
		if err := bus.Write(Byte, 0x0001, 0xAA); err != nil {
			t.Fatalf("byte write on odd failed: %v", err)
		}
	})
}

func TestBusUnmappedAddress(t *testing.T) {
	bus := NewBus()

	_, err := bus.Read(Byte, 0x1000)
	expectBusError(t, err)

	err = bus.Write(Long, 0x1000, 0xFFFFFFFF)
	expectBusError(t, err)
}

func TestBusWaitHook(t *testing.T) {
	ram := NewRAM(0x0000, 0x0004)
	bus := NewBus(ram)
	bus.SetWaitStates(3)
	var waited uint32
	bus.SetWaitHook(func(states uint32) { waited += states })

	if err := bus.Write(Byte, 0x0000, 0xAA); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if waited != 3 {
		t.Fatalf("wait hook not called with expected states: got %d", waited)
	}
}

func TestBusDeviceRangeLookup(t *testing.T) {
	low := NewRAM(0x0000, 0x0010)
	high := NewRAM(0xFC0000, 0x0010)
	bus := NewBus(low, high)

	if err := bus.Write(Byte, 0x0002, 0x11); err != nil {
		t.Fatalf("write low region failed: %v", err)
	}
	if err := bus.Write(Byte, 0xFC0002, 0x22); err != nil {
		t.Fatalf("write high region failed: %v", err)
	}

	if got, err := bus.Read(Byte, 0x0002); err != nil || got != 0x11 {
		t.Fatalf("read low region = (%02x, %v), want (11, <nil>)", got, err)
	}
	if got, err := bus.Read(Byte, 0xFC0002); err != nil || got != 0x22 {
		t.Fatalf("read high region = (%02x, %v), want (22, <nil>)", got, err)
	}
}

func TestBusPrefersEarlierOverlappingDeviceAfterROMHit(t *testing.T) {
	overlay := newStubDevice(0xFF8000, 0xFF8007)
	rom := newStubDevice(0xFC0000, 0xFFFFFF)
	overlay.data[0xFF8006] = 0x12
	rom.data[0xFF8006] = 0x34
	rom.data[0xFC0000] = 0x56

	bus := NewBus(overlay, rom)

	if got, err := bus.Read(Byte, 0xFC0000); err != nil || got != 0x56 {
		t.Fatalf("prime ROM read = (%02x, %v), want (56, <nil>)", got, err)
	}

	if got, err := bus.Read(Byte, 0xFF8006); err != nil || got != 0x12 {
		t.Fatalf("overlapping device read = (%02x, %v), want (12, <nil>)", got, err)
	}
}

func TestBusSingleDeviceRejectsUnmappedAddresses(t *testing.T) {
	ram := NewRAM(0x2000, 0x0010)
	bus := NewBus(ram)

	if _, err := bus.Read(Byte, 0x1fff); err == nil {
		t.Fatalf("read below device range unexpectedly succeeded")
	} else {
		expectBusError(t, err)
	}

	if err := bus.Write(Byte, 0x2010, 0x55); err == nil {
		t.Fatalf("write above device range unexpectedly succeeded")
	} else {
		expectBusError(t, err)
	}
}

func TestBusLongReadSpansAdjacentDevices(t *testing.T) {
	low := NewRAM(0x0000, 0x0004)
	high := NewRAM(0x0004, 0x0004)
	bus := NewBus(low, high)

	if err := low.Write(Word, 0x0002, 0xaabb); err != nil {
		t.Fatalf("seed low word: %v", err)
	}
	if err := high.Write(Word, 0x0004, 0xccdd); err != nil {
		t.Fatalf("seed high word: %v", err)
	}

	got, err := bus.Read(Long, 0x0002)
	if err != nil {
		t.Fatalf("long read across boundary failed: %v", err)
	}
	if got != 0xaabbccdd {
		t.Fatalf("long read across boundary = %08x, want aabbccdd", got)
	}
}

func TestBusLongWriteReportsSecondCycleBusError(t *testing.T) {
	ram := NewRAM(0x0000, 0x0004)
	bus := NewBus(ram)

	err := bus.Write(Long, 0x0002, 0xaabbccdd)
	expectBusError(t, err)

	var be BusError
	if !errors.As(err, &be) {
		t.Fatalf("expected BusError, got %v", err)
	}
	if uint32(be) != 0x0004 {
		t.Fatalf("bus error address = %08x, want 00000004", uint32(be))
	}

	got, err := ram.Read(Word, 0x0002)
	if err != nil {
		t.Fatalf("read partially written word: %v", err)
	}
	if got != 0xaabb {
		t.Fatalf("first word after partial long write = %04x, want aabb", got)
	}
}
