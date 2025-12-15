package m68kemu

import "testing"

func TestAddxDataRegistersWithExtend(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 1
	cpu.regs.D[1] = 1
	cpu.regs.SR = srExtend | srZero

	code := assemble(t, "ADDX.L D0,D1\n")
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

	if got := cpu.regs.D[1]; got != 3 {
		t.Fatalf("expected D1=3 got %d", got)
	}
	if cpu.regs.SR&(srZero|srCarry|srExtend|srOverflow|srNegative) != 0 {
		t.Fatalf("unexpected flags %04x", cpu.regs.SR)
	}
}

func TestSubxPreservesZeroFlagAcrossOperations(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 1
	cpu.regs.D[1] = 1
	cpu.regs.SR = srZero | srExtend

	code := assemble(t, "SUBX.W D0,D1\n")
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

	if got := cpu.regs.D[1] & 0xffff; got != 0xffff {
		t.Fatalf("unexpected result %04x", got)
	}
	if cpu.regs.SR&srZero != 0 {
		t.Fatalf("expected zero flag cleared, got %04x", cpu.regs.SR)
	}
	if cpu.regs.SR&(srCarry|srExtend) == 0 {
		t.Fatalf("expected borrow/extend to be set, got %04x", cpu.regs.SR)
	}
}

func TestNegxClearsZeroWhenResultNonZero(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[2] = 1
	cpu.regs.SR = srZero | srExtend

	code := assemble(t, "NEGX.B D2\n")
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

	if got := cpu.regs.D[2] & 0xff; got != 0xfe {
		t.Fatalf("unexpected result %02x", got)
	}
	if cpu.regs.SR&srZero != 0 {
		t.Fatalf("expected zero flag cleared, got %04x", cpu.regs.SR)
	}
	if cpu.regs.SR&(srCarry|srExtend) == 0 {
		t.Fatalf("expected borrow to set carry/extend, got %04x", cpu.regs.SR)
	}
}

func TestSccSetsByteOnCondition(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.SR = srZero

	code := assemble(t, "SEQ D0\n")
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

	if got := cpu.regs.D[0] & 0xff; got != 0xff {
		t.Fatalf("expected 0xff got %02x", got)
	}

	cpu.regs.SR &^= srZero
	cpu.regs.PC = 0x2000
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	opcode, err = cpu.fetchOpcode()
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("exec: %v", err)
	}

	if got := cpu.regs.D[0] & 0xff; got != 0x00 {
		t.Fatalf("expected 0x00 got %02x", got)
	}
}

func TestIllegalInstructionTriggersException(t *testing.T) {
	cpu, ram := newEnvironment(t)

	handler := uint32(0xbeef)
	if err := ram.Write(Long, uint32(XIllegal<<2), handler); err != nil {
		t.Fatalf("vector write: %v", err)
	}

	code := assemble(t, "ILLEGAL\n")
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

	if cpu.regs.PC != handler {
		t.Fatalf("expected PC to jump to handler %08x got %08x", handler, cpu.regs.PC)
	}
}
