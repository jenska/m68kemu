package m68kemu

import (
	"errors"
	"testing"
)

func TestNewCPUDefaultsToM68000(t *testing.T) {
	cpu, _ := newEnvironment(t)
	if got := cpu.Model(); got != M68000 {
		t.Fatalf("Model() = %v, want %v", got, M68000)
	}
}

func TestWithModel(t *testing.T) {
	for _, tt := range []struct {
		model Model
		err   error
	}{
		{M68000, nil},
		{M68010, nil},
		{M68020, nil},
		{M68030, nil},
		{M68040, ErrModelUnsupported},
		{Model(-1), ErrModelUnsupported},
		{modelCount, ErrModelUnsupported},
	} {
		t.Run(tt.model.String(), func(t *testing.T) {
			ram := NewRAM(0, 0x10000)
			cpu, err := NewCPU(NewBus(ram), WithModel(tt.model), WithDeferredReset())
			if !errors.Is(err, tt.err) {
				t.Fatalf("NewCPU error = %v, want %v", err, tt.err)
			}
			if err == nil && cpu.Model() != tt.model {
				t.Fatalf("Model() = %v, want %v", cpu.Model(), tt.model)
			}
		})
	}
}

func TestModelString(t *testing.T) {
	if got := M68030.String(); got != "MC68030" {
		t.Fatalf("M68030.String() = %q", got)
	}
	if got := Model(42).String(); got != "Model(42)" {
		t.Fatalf("Model(42).String() = %q", got)
	}
}

func TestNewCPUSetsBusAddressWidth(t *testing.T) {
	for _, tt := range []struct {
		model Model
		mask  uint32
	}{{M68000, 0xffffff}, {M68010, 0xffffff}, {M68020, 0xffffffff}, {M68030, 0xffffffff}} {
		bus := NewBus(NewRAM(0, 0x100))
		if tt.mask == 0xffffff {
			bus.setAddressMask(0xffffffff) // NewCPU narrows it again
		}
		if _, err := NewCPU(bus, WithModel(tt.model), WithDeferredReset()); err != nil {
			t.Fatalf("%v: %v", tt.model, err)
		}
		if got := bus.AddressMask(); got != tt.mask {
			t.Fatalf("%v: bus address mask = %08x, want %08x", tt.model, got, tt.mask)
		}
	}
}

func TestNewCPURejectsBusInUse(t *testing.T) {
	bus := NewBus(NewRAM(0, 0x100))
	if _, err := NewCPU(bus, WithDeferredReset()); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCPU(bus, WithDeferredReset()); !errors.Is(err, ErrBusInUse) {
		t.Fatalf("second NewCPU on one bus: err = %v, want ErrBusInUse", err)
	}

	// A CPU that failed to reset does not keep the bus.
	empty := NewBus()
	if _, err := NewCPU(empty); err == nil {
		t.Fatal("NewCPU reset from an empty bus")
	}
	empty.AddDevice(NewRAM(0, 0x100))
	if _, err := NewCPU(empty); err != nil {
		t.Fatalf("NewCPU after a failed one: %v", err)
	}
}
