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
		{M68020, ErrModelUnsupported},
		{M68030, ErrModelUnsupported},
		{M68040, ErrModelUnsupported},
		{M68060, ErrModelUnsupported},
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
