package m68kemu

import "testing"

func TestClrAndTst(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.SR |= srExtend

	ram.Write(Long, 0x3000, 0xdeadbeef)
	code := assemble(t, "CLR.L $3000\nTST.L $3000\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("CLR failed: %v", err)
	}
	if got, _ := ram.Read(Long, 0x3000); got != 0 {
		t.Fatalf("expected memory to be cleared, got %08x", got)
	}
	if cpu.regs.SR&srZero == 0 || cpu.regs.SR&srExtend == 0 {
		t.Fatalf("expected zero set and extend preserved, SR=%04x", cpu.regs.SR)
	}

	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("TST failed: %v", err)
	}
	if cpu.regs.SR&srZero == 0 {
		t.Fatalf("expected zero flag after TST on cleared memory")
	}
}

func TestAddaSuba(t *testing.T) {
	cpu, ram := newEnvironment(t)
	code := assemble(t, "ADDA.W #-1,A0\nSUBA.L #1,A0\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("ADDA failed: %v", err)
	}
	if cpu.regs.A[0] != 0xffffffff {
		t.Fatalf("expected sign-extended word add, got %08x", cpu.regs.A[0])
	}

	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("SUBA failed: %v", err)
	}
	if cpu.regs.A[0] != 0xfffffffe {
		t.Fatalf("expected subtraction result 0xfffffffe, got %08x", cpu.regs.A[0])
	}
	if cpu.regs.SR != 0x2700 {
		t.Fatalf("expected condition codes untouched, SR=%04x", cpu.regs.SR)
	}
}

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
