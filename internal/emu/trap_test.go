package emu

import "testing"

func TestTrapStacksExceptionFrameAndJumps(t *testing.T) {
	cpu, ram := newEnvironment(t)

	// Simulate user mode and preset a distinct handler address.
	cpu.regs.SR = 0x0700
	handler := uint32(0x123456)
	vector := uint32(1)
	vectorNumber := XTrap + vector
	vectorAddress := vectorNumber << 2

	if err := ram.WriteLongTo(vectorAddress, handler); err != nil {
		t.Fatalf("failed to write vector: %v", err)
	}

	code := assemble(t, "TRAP #1\n")
	for i := range code {
		addr := cpu.regs.PC + uint32(i)
		if err := ram.WriteByteTo(addr, code[i]); err != nil {
			t.Fatalf("failed to write byte to %04x: %v", addr, err)
		}
	}

	opcode, err := cpu.fetchOpcode()
	if err != nil {
		t.Fatalf("failed to fetch opcode: %v", err)
	}
	returnPC := cpu.regs.PC

	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("failed to execute TRAP: %v", err)
	}

	expectedSP := uint32(0x1000 - (Word.size + Long.size + Word.size))
	if cpu.regs.A[7] != expectedSP {
		t.Fatalf("unexpected SP after trap: got %08x want %08x", cpu.regs.A[7], expectedSP)
	}

	stackedVector, err := ram.ReadWordFrom(expectedSP)
	if err != nil {
		t.Fatalf("failed reading stacked vector: %v", err)
	}
	if stackedVector != uint16(vectorAddress) {
		t.Fatalf("stacked vector mismatch: got %04x want %04x", stackedVector, uint16(vectorAddress))
	}

	stackedPC, err := ram.ReadLongFrom(expectedSP + Word.size)
	if err != nil {
		t.Fatalf("failed reading stacked PC: %v", err)
	}
	if stackedPC != returnPC {
		t.Fatalf("stacked PC mismatch: got %08x want %08x", stackedPC, returnPC)
	}

	stackedSR, err := ram.ReadWordFrom(expectedSP + Word.size + Long.size)
	if err != nil {
		t.Fatalf("failed reading stacked SR: %v", err)
	}
	if stackedSR != 0x0700 {
		t.Fatalf("stacked SR mismatch: got %04x want %04x", stackedSR, 0x0700)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("PC did not jump to handler: got %08x want %08x", cpu.regs.PC, handler)
	}
	if cpu.regs.SR&srSupervisor == 0 {
		t.Fatalf("trap did not set supervisor bit; SR=%04x", cpu.regs.SR)
	}
}
