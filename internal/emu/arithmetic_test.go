package emu

import "testing"

func TestAddDataRegisterToDataRegister(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 2
	cpu.regs.D[1] = 1

	code := assemble(t, "ADD.L D1,D0\n")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	opcode, err := cpu.fetchOpcode()
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("exec: %v", err)
	}

	if got := cpu.regs.D[0]; got != 3 {
		t.Fatalf("expected D0=3 got %d", got)
	}
	if cpu.regs.SR&(srZero|srNegative|srCarry|srOverflow) != 0 {
		t.Fatalf("unexpected flags %04x", cpu.regs.SR)
	}
}

func TestSubqUpdatesFlags(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[2] = 1

	code := assemble(t, "SUBQ.W #1,D2\n")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	opcode, err := cpu.fetchOpcode()
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("exec: %v", err)
	}

	if got := cpu.regs.D[2] & 0xffff; got != 0 {
		t.Fatalf("expected zero, got %04x", got)
	}
	if cpu.regs.SR&srZero == 0 {
		t.Fatalf("expected zero flag set, got %04x", cpu.regs.SR)
	}
}
