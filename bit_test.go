package m68kemu

import "testing"

func TestBitOperationsDataRegister(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[1] = 0b11
	cpu.regs.D[0] = 0 // bit number 0
	cpu.regs.SR |= srExtend

	code := assemble(t, "BTST #1,D1\nBCLR D0,D1\nBCHG #1,D1\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BTST failed: %v", err)
	}
	if cpu.regs.SR&srZero != 0 || cpu.regs.SR&srExtend == 0 {
		t.Fatalf("expected zero clear and extend preserved after BTST, SR=%04x", cpu.regs.SR)
	}

	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BCLR failed: %v", err)
	}
	if cpu.regs.D[1] != 0b10 || cpu.regs.SR&srZero != 0 {
		t.Fatalf("expected bit 0 cleared with zero clear, D1=%b SR=%04x", cpu.regs.D[1], cpu.regs.SR)
	}

	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BCHG failed: %v", err)
	}
	if cpu.regs.D[1] != 0 || cpu.regs.SR&srZero != 0 {
		t.Fatalf("expected bit 1 toggled to 0 with zero clear, D1=%b SR=%04x", cpu.regs.D[1], cpu.regs.SR)
	}
}

func TestBitSetMemory(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 9 // wraps to bit 1 for memory targets

	if err := ram.Write(Byte, 0x3000, 0x00); err != nil {
		t.Fatalf("failed to seed memory: %v", err)
	}

	code := assemble(t, "BSET D0,$3000\nBTST #1,$3000\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BSET failed: %v", err)
	}
	val, _ := ram.Read(Byte, 0x3000)
	if val != 0x02 || cpu.regs.SR&srZero == 0 {
		t.Fatalf("expected bit 1 set with zero flag set, val=%02x SR=%04x", val, cpu.regs.SR)
	}

	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BTST failed: %v", err)
	}
	if cpu.regs.SR&srZero != 0 {
		t.Fatalf("expected zero clear when testing set bit, SR=%04x", cpu.regs.SR)
	}
}
