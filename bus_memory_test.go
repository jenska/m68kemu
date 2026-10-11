package m68kemu

import (
	"testing"
)

func TestRAMAccessWithOffset(t *testing.T) {
	ram := NewRAM(0x1000, 0x10)

	if err := ram.Write(Byte, 0x1000, 0xAB); err != nil {
		t.Fatalf("write at base offset failed: %v", err)
	}
	if val, err := ram.Read(Byte, 0x1000); err != nil || val != 0xAB {
		t.Fatalf("read at base offset = (%x, %v), want (0xAB, <nil>)", val, err)
	}

	if err := ram.Write(Long, 0x100c, 0xAABBCCDD); err != nil {
		t.Fatalf("long write near end failed: %v", err)
	}
	if val, err := ram.Read(Long, 0x100c); err != nil || val != 0xAABBCCDD {
		t.Fatalf("read long near end = (%x, %v), want (0xAABBCCDD, <nil>)", val, err)
	}

	if err := ram.Write(Word, 0x100e, 0x1122); err != nil {
		t.Fatalf("word write at final boundary failed: %v", err)
	}
	if val, err := ram.Read(Word, 0x100e); err != nil || val != 0x1122 {
		t.Fatalf("read word at final boundary = (%x, %v), want (0x1122, <nil>)", val, err)
	}
}

func TestRAMAccessOutOfRange(t *testing.T) {
	ram := NewRAM(0x2000, 0x08)

	expectBusError(t, ram.Write(Byte, 0x1FFF, 0x00))
	_, err := ram.Read(Byte, 0x1FFF)
	expectBusError(t, err)

	expectBusError(t, ram.Write(Byte, 0x2008, 0x00))
	_, err = ram.Read(Byte, 0x2008)
	expectBusError(t, err)

	expectBusError(t, ram.Write(Word, 0x2007, 0xFFFF))
	expectBusError(t, ram.Write(Long, 0x2005, 0xFFFFFFFF))
}
