package m68kemu

import (
	"testing"

	asm "github.com/jenska/m68kasm"
)

func newEnvironment010(tb testing.TB) (*cpu, *RAM) {
	tb.Helper()

	memory := NewRAM(0, 1024*64)
	bus := NewBus(memory)
	memory.Write(Long, 0, 0x1000)
	memory.Write(Long, 4, 0x2000)
	processor, err := NewCPU(bus, WithModel(M68010))
	if err != nil {
		tb.Fatalf("Failed to create CPU: %v", err)
	}
	return processor.(*cpu), memory
}

// load010 assembles source for the 68010 and stores it at the current PC.
func load010(tb testing.TB, cpu *cpu, ram *RAM, source string) {
	tb.Helper()
	code, err := asm.AssembleStringWithOptions(source, asm.ParseOptions{Target: asm.Target{CPU: asm.CPU68010}})
	if err != nil {
		tb.Fatalf("Assembler failed: %v", err)
	}
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			tb.Fatalf("failed to store program byte: %v", err)
		}
	}
}

func step(tb testing.TB, cpu *cpu) {
	tb.Helper()
	if err := cpu.Step(); err != nil {
		tb.Fatalf("Step failed: %v", err)
	}
}

func TestVectorsRelativeToVBR(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	cpu.regs.VBR = 0x8000
	ram.Write(Long, 0x8000+XTrap<<2, 0x5000)
	ram.Write(Long, XTrap<<2, 0x6000) // must not be used
	load010(t, cpu, ram, "TRAP #0")

	step(t, cpu)
	if cpu.regs.PC != 0x5000 {
		t.Fatalf("PC = %04x, want the TRAP #0 vector from the VBR table at 5000", cpu.regs.PC)
	}
}

func TestResetClearsControlRegisters(t *testing.T) {
	cpu, _ := newEnvironment010(t)
	cpu.regs.VBR, cpu.regs.SFC, cpu.regs.DFC = 0x8000, 5, 6
	if err := cpu.Reset(); err != nil {
		t.Fatal(err)
	}
	if cpu.regs.VBR != 0 || cpu.regs.SFC != 0 || cpu.regs.DFC != 0 {
		t.Fatalf("after reset VBR=%x SFC=%d DFC=%d, want all zero", cpu.regs.VBR, cpu.regs.SFC, cpu.regs.DFC)
	}
}
