package emu

import "testing"

func TestCycleCounterBasicSequence(t *testing.T) {
	cpu, ram := newEnvironment(t)

	code := assemble(t, "MOVEQ #1,D0\nNOP")
	for i := range code {
		addr := cpu.regs.PC + uint32(i)
		if err := ram.Write(Byte, addr, uint32(code[i])); err != nil {
			t.Fatalf("failed to write byte to %04x: %v", addr, err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("first step failed: %v", err)
	}
	if cpu.Cycles() != 4 {
		t.Fatalf("unexpected cycles after MOVEQ: got %d want 4", cpu.Cycles())
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("second step failed: %v", err)
	}
	if cpu.Cycles() != 8 {
		t.Fatalf("unexpected cycles after NOP: got %d want 8", cpu.Cycles())
	}
}

func TestCycleCounterMemoryMove(t *testing.T) {
	cpu, ram := newEnvironment(t)

	cpu.regs.A[0] = 0x3000
	code := assemble(t, "MOVE.L D0,(A0)")
	for i := range code {
		addr := cpu.regs.PC + uint32(i)
		if err := ram.Write(Byte, addr, uint32(code[i])); err != nil {
			t.Fatalf("failed to write byte to %04x: %v", addr, err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("move failed: %v", err)
	}

	if cpu.Cycles() != 8 {
		t.Fatalf("unexpected cycles for MOVE.L D0,(A0): got %d want 8", cpu.Cycles())
	}
}

func TestCycleCounterWaitStates(t *testing.T) {
	ram := NewRAM(0, 0x1000)
	bus := NewBus(&ram)
	bus.SetWaitStates(2)
	if err := ram.Write(Long, 0, 0x100); err != nil {
		t.Fatalf("failed to seed SSP: %v", err)
	}
	if err := ram.Write(Long, 4, 0x200); err != nil {
		t.Fatalf("failed to seed PC: %v", err)
	}

	cpu, err := NewCPU(bus)
	if err != nil {
		t.Fatalf("failed to create CPU: %v", err)
	}

	if err := ram.Write(Word, 0x200, 0x4e71); err != nil { // NOP
		t.Fatalf("failed to write NOP: %v", err)
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("failed to execute NOP: %v", err)
	}

	expected := uint64(6) // 4 base cycles + 2 wait states for the opcode fetch
	if cpu.Cycles() != expected {
		t.Fatalf("unexpected cycles with wait states: got %d want %d", cpu.Cycles(), expected)
	}
}
