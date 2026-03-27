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

func TestCmpiLongAbsoluteDestination(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.SR |= srExtend
	if err := ram.Write(Long, 0x3000, 3); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	code := assemble(t, "CMPI.L #2,$3000\n")
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

	if got, err := ram.Read(Long, 0x3000); err != nil || got != 3 {
		t.Fatalf("CMPI modified destination memory: %08x err=%v", got, err)
	}
	if cpu.regs.SR&srExtend == 0 {
		t.Fatalf("CMPI should leave extend bit unchanged")
	}
	if got := cpu.regs.SR & (srNegative | srZero | srOverflow | srCarry); got != 0 {
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

func TestCmpaWordSignExtendsSource(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[0] = 0xffffffff
	cpu.regs.SR |= srExtend

	code := assemble(t, "CMPA.W #-1,A0\n")
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

	if cpu.regs.A[0] != 0xffffffff {
		t.Fatalf("CMPA modified address register: %08x", cpu.regs.A[0])
	}
	if cpu.regs.SR&srExtend == 0 {
		t.Fatalf("CMPA should leave extend bit unchanged")
	}
	expected := uint16(srZero)
	if got := cpu.regs.SR & (srNegative | srZero | srOverflow | srCarry); got != expected {
		t.Fatalf("unexpected condition codes %04x", got)
	}
}

func TestCmpaLongUsesFullWidthSource(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[1] = 0x12345678

	code := assemble(t, "CMPA.L #$12345678,A1\n")
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

	if cpu.regs.A[1] != 0x12345678 {
		t.Fatalf("CMPA modified address register: %08x", cpu.regs.A[1])
	}
	expected := uint16(srZero)
	if got := cpu.regs.SR & (srNegative | srZero | srOverflow | srCarry); got != expected {
		t.Fatalf("unexpected condition codes %04x", got)
	}
}

func TestCmpmByteUsesWordStepForA7(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[7] = 0x3000
	cpu.regs.A[0] = 0x4000

	if err := ram.Write(Byte, 0x3000, 0x12); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := ram.Write(Byte, 0x4000, 0x12); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	code := assemble(t, "CMPM.B (A7)+,(A0)+\n")
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

	if cpu.regs.A[7] != 0x3002 {
		t.Fatalf("A7 should advance by 2 for byte CMPM, got %04x", cpu.regs.A[7])
	}
	if cpu.regs.A[0] != 0x4001 {
		t.Fatalf("A0 should advance by 1 for byte CMPM, got %04x", cpu.regs.A[0])
	}
}
