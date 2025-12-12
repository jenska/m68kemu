package m68kemu

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

func TestAddqDataRegister(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 1

	code := assemble(t, "ADDQ.B #1,D0\n")
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

	if got := cpu.regs.D[0] & 0xff; got != 2 {
		t.Fatalf("expected 2 got %d", got)
	}
	if cpu.regs.SR&(srZero|srNegative|srOverflow|srCarry|srExtend) != 0 {
		t.Fatalf("unexpected flags %04x", cpu.regs.SR)
	}
}

func TestSubInstruction(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 5
	cpu.regs.D[1] = 3

	code := assemble(t, "SUB.L D1,D0\n")
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

	if got := cpu.regs.D[0]; got != 2 {
		t.Fatalf("expected 2 got %d", got)
	}
	if cpu.regs.SR&(srZero|srNegative|srOverflow|srCarry|srExtend) != 0 {
		t.Fatalf("unexpected flags %04x", cpu.regs.SR)
	}
}

func TestMulDivInstructions(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 10
	cpu.regs.D[1] = 2
	cpu.regs.D[2] = 4
	cpu.regs.D[3] = 3
	cpu.regs.D[4] = -2

	code := assemble(t, "DIVU D1,D0\nMULU D2,D0\nDIVS D3,D0\nMULS D4,D0\n")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	step := func(expect int32) {
		t.Helper()
		opcode, err := cpu.fetchOpcode()
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		if err := cpu.executeInstruction(opcode); err != nil {
			t.Fatalf("exec: %v", err)
		}
		if cpu.regs.D[0] != expect {
			t.Fatalf("expected %08x got %08x", expect, cpu.regs.D[0])
		}
	}

	step(0x00000005) // DIVU => quotient 5 remainder 0
	step(0x00000014) // MULU => 5*4
	step(0x00020006) // DIVS => quotient 6 remainder 2
	step(-12)        // MULS => 6 * -2 = -12
}
