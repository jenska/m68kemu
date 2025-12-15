package m68kemu

import "testing"

func TestTrapvResetAndStop(t *testing.T) {
	cpu, ram := newEnvironment(t)

	ram.Write(Long, 7<<2, 0x2222)
	ram.Write(Long, (autoVectorBase+2)<<2, 0x2008)

	code := assemble(t, "TRAPV\nRESET\nSTOP #$2000\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	cpu.regs.SR |= srOverflow
	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("TRAPV failed: %v", err)
	}
	if cpu.regs.PC != 0x2222 {
		t.Fatalf("expected TRAPV to vector to 0x2222, PC=%04x", cpu.regs.PC)
	}

	// Execute RESET
	cpu.regs.PC = 0x2002
	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("RESET failed: %v", err)
	}
	if cpu.regs.SR != 0x2702 {
		t.Fatalf("RESET should preserve SR bits, got %04x", cpu.regs.SR)
	}
	if val, _ := ram.Read(Long, 0x3000); val != 0 {
		t.Fatalf("expected memory reset to clear RAM, got %08x", val)
	}

	// Reinstall STOP instruction and autovector handler after reset cleared RAM.
	ram.Write(Long, (autoVectorBase+2)<<2, 0x2008)
	stopCode := assemble(t, "STOP #$2000\nNOP")
	for i, b := range stopCode {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	// Execute STOP and resume via interrupt
	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("STOP failed: %v", err)
	}
	if !cpu.stopped {
		t.Fatalf("CPU should be stopped")
	}

	if err := cpu.RequestInterrupt(2, nil); err != nil {
		t.Fatalf("failed to request interrupt: %v", err)
	}
	if err := cpu.Step(); err != nil {
		t.Fatalf("failed to service interrupt: %v", err)
	}
	if cpu.stopped {
		t.Fatalf("CPU should resume after interrupt")
	}
	if cpu.regs.PC != 0x2008 {
		t.Fatalf("expected autovector handler at 0x2008, PC=%04x", cpu.regs.PC)
	}
}

func TestRtrRestoresPcAndCcr(t *testing.T) {
	cpu, ram := newEnvironment(t)

	cpu.regs.SR = srSupervisor | 0x0700
	stackBase := cpu.regs.A[7]

	returnCCR := uint16(0x0033)
	returnPC := uint32(0x004000)

	ram.Write(Word, stackBase, uint32(returnCCR))
	ram.Write(Long, stackBase+uint32(Word), returnPC)

	code := assemble(t, "RTR")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("RTR failed: %v", err)
	}
	if cpu.regs.PC != returnPC {
		t.Fatalf("RTR should restore PC, got %08x want %08x", cpu.regs.PC, returnPC)
	}
	expectedSR := uint16(0x2733)
	if cpu.regs.SR != expectedSR {
		t.Fatalf("RTR should restore CCR and preserve upper SR bits, got %04x want %04x", cpu.regs.SR, expectedSR)
	}
	if cpu.regs.A[7] != stackBase+uint32(Word+Long) {
		t.Fatalf("RTR should advance SP, got %04x want %04x", cpu.regs.A[7], stackBase+uint32(Word+Long))
	}
}
