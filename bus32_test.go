package m68kemu

import (
	"errors"
	"testing"
)

func TestBus32DecodesFullAddresses(t *testing.T) {
	ram := NewRAM(0, 0x10000)
	io := newStubMappedDevice(0xffff8000, 0xffff80ff)
	io.data[0xffff8000] = 0x1234
	bus := NewBus(ram, io)
	bus.SetAddressWidth(32)
	if bus.AddressMask() != 0xffffffff {
		t.Fatalf("AddressMask = %x, want ffffffff", bus.AddressMask())
	}

	if v, err := bus.Read(Word, 0xffff8000); err != nil || v != 0x1234 {
		t.Fatalf("read $FFFF8000 = %x, %v; want 1234", v, err)
	}
	// No 24-bit aliasing: $01000000 is not RAM at 0.
	if _, err := bus.Read(Word, 0x01000000); !errors.As(err, new(BusError)) {
		t.Fatalf("read $01000000 on a 32-bit bus: error %v, want a bus error", err)
	}
	if _, err := bus.Read(Word, 0x00ff8000); !errors.As(err, new(BusError)) {
		t.Fatalf("read $00FF8000 on a 32-bit bus: error %v, want a bus error", err)
	}
}

func TestBus24KeepsAliasing(t *testing.T) {
	ram := NewRAM(0, 0x10000)
	ram.Write(Word, 0x10, 0xbeef)
	io := MapDevice(0xffff8000, 0xffff80ff, newStubMappedDevice(0xff8000, 0xff80ff))
	bus := NewBus(ram, io)
	if v, err := bus.Read(Word, 0x01000010); err != nil || v != 0xbeef {
		t.Fatalf("read $01000010 on a 24-bit bus = %x, %v; want RAM at $10", v, err)
	}
	if _, err := bus.Read(Word, 0xffff8000); err != nil {
		t.Fatalf("MapDevice with a 32-bit address on a 24-bit bus: %v", err)
	}
}

func TestBus32MapDevice(t *testing.T) {
	inner := newStubMappedDevice(0xffff8000, 0xffff80ff)
	inner.data[0xffff8002] = 0x55
	bus := NewBus(NewRAM(0, 0x100), MapDevice(0xffff8000, 0xffff80ff, inner))
	bus.SetAddressWidth(32)
	if v, err := bus.Read(Word, 0xffff8002); err != nil || v != 0x55 {
		t.Fatalf("read through MapDevice = %x, %v; want 55", v, err)
	}
	if _, err := bus.Read(Word, 0x00ff8002); err == nil {
		t.Fatalf("a mapped device at $FFFF8000 should not answer for $00FF8002 on a 32-bit bus")
	}
}

func TestBus32DeviceAcrossTopLevelTables(t *testing.T) {
	ram := NewRAM(0x00ff0000, 0x20000) // $00FF0000-$0100FFFF
	other := newStubMappedDevice(0x40000000, 0x4000ffff)
	bus := NewBus(ram, other)
	bus.SetAddressWidth(32)
	for _, a := range []uint32{0x00ff0000, 0x00fffffe, 0x01000000, 0x0100fffe} {
		if err := bus.Write(Word, a, a>>8&0xffff); err != nil {
			t.Fatalf("write %08x: %v", a, err)
		}
		if v, err := bus.Read(Word, a); err != nil || v != a>>8&0xffff {
			t.Fatalf("read %08x = %x, %v", a, v, err)
		}
	}
	if _, err := bus.Read(Word, 0x01010000); err == nil {
		t.Fatalf("read past the RAM should fail")
	}
}

func TestBusSetAddressWidthRejectsOtherWidths(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("SetAddressWidth(16) should panic")
		}
	}()
	NewBus(NewRAM(0, 0x100)).SetAddressWidth(16)
}
