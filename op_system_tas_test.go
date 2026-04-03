package m68kemu

import "testing"

func TestTas(t *testing.T) {
	cpu, ram := newEnvironment(t)

	cpu.regs.A[0] = 0x4000
	if err := ram.Write(Byte, cpu.regs.A[0], 0x00); err != nil {
		t.Fatalf("failed to seed memory: %v", err)
	}

	cpu.regs.SR = srExtend

	code := assemble(t, "TAS (A0)")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program: %v", err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("TAS execution failed: %v", err)
	}

	value, err := ram.Read(Byte, cpu.regs.A[0])
	if err != nil {
		t.Fatalf("failed to read TAS result: %v", err)
	}

	if value != 0x80 {
		t.Fatalf("TAS did not set high bit, got %02x", value)
	}
	if cpu.regs.SR&srZero == 0 || cpu.regs.SR&srNegative != 0 || cpu.regs.SR&srExtend == 0 {
		t.Fatalf("TAS flags incorrect: SR=%04x", cpu.regs.SR)
	}
}
