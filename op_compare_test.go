package m68kemu

import "testing"

func TestCmpDoesNotAffectExtend(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 3
	cpu.regs.D[1] = 5
	cpu.regs.SR |= srExtend

	code := assemble(t, "CMP.L D1,D0\n")
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

	if cpu.regs.D[0] != 3 {
		t.Fatalf("CMP modified destination register: %08x", cpu.regs.D[0])
	}
	if cpu.regs.SR&srExtend == 0 {
		t.Fatalf("CMP should leave extend bit unchanged")
	}
	expected := uint16(srNegative | srCarry)
	if got := cpu.regs.SR & (srNegative | srZero | srOverflow | srCarry); got != expected {
		t.Fatalf("unexpected condition codes %04x", got)
	}
}

func TestCmpiImmediate(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[2] = 0x12
	cpu.regs.SR |= srExtend

	code := assemble(t, "CMPI.B #$12,D2\n")
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

	if cpu.regs.D[2] != 0x12 {
		t.Fatalf("CMPI modified destination register: %08x", cpu.regs.D[2])
	}
	if cpu.regs.SR&srExtend == 0 {
		t.Fatalf("CMPI should leave extend bit unchanged")
	}
	expected := uint16(srZero)
	if got := cpu.regs.SR & (srNegative | srZero | srOverflow | srCarry); got != expected {
		t.Fatalf("unexpected condition codes %04x", got)
	}
}

func TestCmpmPostIncrement(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[0] = 0x3000
	cpu.regs.A[1] = 0x4000

	if err := ram.Write(Word, cpu.regs.A[0], 3); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := ram.Write(Word, cpu.regs.A[1], 5); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	code := assemble(t, "CMPM.W (A0)+,(A1)+\n")
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

	if cpu.regs.A[0] != 0x3000+uint32(Word) || cpu.regs.A[1] != 0x4000+uint32(Word) {
		t.Fatalf("address registers not incremented correctly: A0=%04x A1=%04x", cpu.regs.A[0], cpu.regs.A[1])
	}
	expected := uint16(0)
	if got := cpu.regs.SR & (srNegative | srZero | srOverflow | srCarry); got != expected {
		t.Fatalf("unexpected condition codes %04x", got)
	}
}
