package emu

import "testing"

func TestNopAdvancesPCAndLeavesState(t *testing.T) {
	cpu, ram := newEnvironment(t)

	opcode := uint16(0x4e71)
	if err := ram.Write(Word, cpu.regs.PC, uint32(opcode)); err != nil {
		t.Fatalf("failed to write NOP opcode: %v", err)
	}

	initialPC := cpu.regs.PC
	initialSR := cpu.regs.SR
	initialD := cpu.regs.D
	initialA := cpu.regs.A

	if err := cpu.Step(); err != nil {
		t.Fatalf("failed to execute NOP: %v", err)
	}

	if cpu.regs.PC != initialPC+uint32(Word) {
		t.Fatalf("PC not advanced after NOP: got %08x want %08x", cpu.regs.PC, initialPC+uint32(Word))
	}

	if cpu.regs.SR != initialSR {
		t.Fatalf("SR changed after NOP: got %04x want %04x", cpu.regs.SR, initialSR)
	}

	if cpu.regs.D != initialD {
		t.Fatalf("data registers changed after NOP: got %+v want %+v", cpu.regs.D, initialD)
	}

	if cpu.regs.A != initialA {
		t.Fatalf("address registers changed after NOP: got %+v want %+v", cpu.regs.A, initialA)
	}
}
