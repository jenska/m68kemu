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
