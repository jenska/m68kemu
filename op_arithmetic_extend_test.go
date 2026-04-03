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

func TestAddxByteUsesWordStepForA7(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[7] = 0x3002
	cpu.regs.A[0] = 0x4001

	if err := ram.Write(Byte, 0x3000, 0x01); err != nil {
		t.Fatalf("seed src: %v", err)
	}
	if err := ram.Write(Byte, 0x4000, 0x02); err != nil {
		t.Fatalf("seed dst: %v", err)
	}

	code := assemble(t, "ADDX.B -(A7),-(A0)\n")
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

	if cpu.regs.A[7] != 0x3000 {
		t.Fatalf("A7 should predecrement by 2 for byte ADDX, got %04x", cpu.regs.A[7])
	}
	if cpu.regs.A[0] != 0x4000 {
		t.Fatalf("A0 should predecrement by 1 for byte ADDX, got %04x", cpu.regs.A[0])
	}
	if got, _ := ram.Read(Byte, 0x4000); got != 0x03 {
		t.Fatalf("unexpected result byte %02x", got)
	}
}
