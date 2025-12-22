package m68kemu

import "testing"

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
			expectBusError(t, err)

			err = bus.Write(tt.size, tt.address, 0xFF)
			expectBusError(t, err)
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
