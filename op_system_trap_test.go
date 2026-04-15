package m68kemu

import "testing"

func TestTrapStacksExceptionFrameAndJumps(t *testing.T) {
	cpu, ram := newEnvironment(t)

	// Simulate user mode and preset a distinct handler address.
	cpu.regs.SR = 0x0700
	handler := uint32(0x123456)
	vector := uint32(1)
	vectorNumber := XTrap + vector
	vectorAddress := vectorNumber << 2

	if err := ram.Write(Long, vectorAddress, handler); err != nil {
		t.Fatalf("failed to write vector: %v", err)
	}

	code := assemble(t, "TRAP #1\n")
	for i := range code {
		addr := cpu.regs.PC + uint32(i)
		if err := ram.Write(Byte, addr, uint32(code[i])); err != nil {
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

	expectedSP := uint32(0x1000 - (Word + Long))
	if cpu.regs.A[7] != expectedSP {
		t.Fatalf("unexpected SP after trap: got %08x want %08x", cpu.regs.A[7], expectedSP)
	}

	stackedVector, err := ram.Read(Word, expectedSP)
	if err != nil {
		t.Fatalf("failed reading stacked vector: %v", err)
	}
	if stackedVector != 0x0700 {
		t.Fatalf("stacked SR mismatch: got %04x want %04x", stackedVector, 0x0700)
	}

	stackedPC, err := ram.Read(Long, expectedSP+uint32(Word))
	if err != nil {
		t.Fatalf("failed reading stacked PC: %v", err)
	}
	if stackedPC != returnPC {
		t.Fatalf("stacked PC mismatch: got %08x want %08x", stackedPC, returnPC)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("PC did not jump to handler: got %08x want %08x", cpu.regs.PC, handler)
	}
	if cpu.regs.SR&srSupervisor == 0 {
		t.Fatalf("trap did not set supervisor bit; SR=%04x", cpu.regs.SR)
	}
}

func TestExecuteInstructionTrapUsesSharedSynchronousExceptionFrame(t *testing.T) {
	cpu, ram := newEnvironment(t)

	cpu.regs.SR = 0x0700
	cpu.regs.USP = 0x4000
	cpu.regs.A[7] = cpu.regs.USP
	cpu.regs.SSP = 0x1800

	handler := uint32(0x654321)
	vectorNumber := uint32(XTrap + 5)
	if err := ram.Write(Long, vectorNumber<<2, handler); err != nil {
		t.Fatalf("failed to write vector: %v", err)
	}

	startPC := cpu.regs.PC
	if err := cpu.executeInstruction(0x4e45); err != nil {
		t.Fatalf("failed to execute TRAP directly: %v", err)
	}

	expectedSP := uint32(0x1800 - exceptionFrameSize)
	if cpu.regs.A[7] != expectedSP {
		t.Fatalf("unexpected SP after direct trap: got %08x want %08x", cpu.regs.A[7], expectedSP)
	}

	stackedSR, err := ram.Read(Word, expectedSP)
	if err != nil {
		t.Fatalf("failed reading stacked SR: %v", err)
	}
	if stackedSR != 0x0700 {
		t.Fatalf("stacked SR mismatch: got %04x want %04x", stackedSR, 0x0700)
	}

	stackedPC, err := ram.Read(Long, expectedSP+uint32(Word))
	if err != nil {
		t.Fatalf("failed reading stacked PC: %v", err)
	}
	if want := startPC + uint32(Word); stackedPC != want {
		t.Fatalf("stacked PC mismatch: got %08x want %08x", stackedPC, want)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("PC did not jump to handler: got %08x want %08x", cpu.regs.PC, handler)
	}
	if cpu.regs.SR&srSupervisor == 0 {
		t.Fatalf("trap did not set supervisor bit; SR=%04x", cpu.regs.SR)
	}
	if cpu.regs.USP != 0x4000 {
		t.Fatalf("USP changed after trap: got %08x want %08x", cpu.regs.USP, uint32(0x4000))
	}

	state := cpu.DebugState()
	if !state.HasException || state.LastException.Vector != vectorNumber {
		t.Fatalf("expected trap exception state, got %+v", state.LastException)
	}
	want := startPC + uint32(Word)
	if state.LastException.PC != want {
		t.Fatalf("exception PC mismatch: got %08x want %08x", state.LastException.PC, want)
	}
	if state.LastException.Frame.PC != want {
		t.Fatalf("frame PC mismatch: got %08x want %08x", state.LastException.Frame.PC, want)
	}
}
