package emu

import "testing"

func runSingleInstruction(t *testing.T, cpu *CPU, ram *RAM, asmSrc string) {
	t.Helper()

	code := assemble(t, asmSrc)
	for i := range code {
		addr := cpu.regs.PC + uint32(i)
		if err := ram.Write(Byte, addr, uint32(code[i])); err != nil {
			t.Fatalf("failed to write byte to %04x: %v", addr, err)
		}
	}

	opcode, err := cpu.fetchOpcode()
	if err != nil {
		t.Fatalf("failed to fetch opcode: %v", err)
	}
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("failed to execute opcode %04x: %v", opcode, err)
	}
}

func TestABCDRegister(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.SR &^= srExtend | srCarry | srZero

	cpu.regs.D[0] = 0x09
	cpu.regs.D[1] = 0x01

	runSingleInstruction(t, cpu, ram, "ABCD D0,D1")

	if got := cpu.regs.D[1] & 0xff; got != 0x10 {
		t.Fatalf("expected 0x10 in D1 low byte, got %02x", got)
	}
	if cpu.regs.SR&(srCarry|srExtend) != 0 {
		t.Fatalf("carry/extend should be cleared, SR=%04x", cpu.regs.SR)
	}
	if cpu.regs.SR&srZero != 0 {
		t.Fatalf("zero flag should be cleared, SR=%04x", cpu.regs.SR)
	}
}

func TestABCDCarryAndZero(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 0x01
	cpu.regs.D[1] = 0x99

	runSingleInstruction(t, cpu, ram, "ABCD D0,D1")

	if got := cpu.regs.D[1] & 0xff; got != 0x00 {
		t.Fatalf("expected 0x00 in D1 low byte, got %02x", got)
	}
	if cpu.regs.SR&(srCarry|srExtend) != srCarry|srExtend {
		t.Fatalf("carry/extend should be set, SR=%04x", cpu.regs.SR)
	}
	if cpu.regs.SR&srZero == 0 {
		t.Fatalf("zero flag should be set, SR=%04x", cpu.regs.SR)
	}
}

func TestABCDMemory(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[0] = 0x0201
	cpu.regs.A[1] = 0x0101

	if err := ram.Write(Byte, 0x0200, 0x01); err != nil {
		t.Fatalf("failed to seed destination: %v", err)
	}
	if err := ram.Write(Byte, 0x0100, 0x09); err != nil {
		t.Fatalf("failed to seed source: %v", err)
	}

	runSingleInstruction(t, cpu, ram, "ABCD -(A1),-(A0)")

	if cpu.regs.A[0] != 0x0200 || cpu.regs.A[1] != 0x0100 {
		t.Fatalf("addresses not decremented correctly: A0=%04x A1=%04x", cpu.regs.A[0], cpu.regs.A[1])
	}
	if value, _ := ram.Read(Byte, 0x0200); value != 0x10 {
		t.Fatalf("expected destination memory to hold 0x10, got %02x", value)
	}
}

func TestSBCDBorrow(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 0x01
	cpu.regs.D[1] = 0x00

	runSingleInstruction(t, cpu, ram, "SBCD D0,D1")

	if got := cpu.regs.D[1] & 0xff; got != 0x99 {
		t.Fatalf("expected 0x99 in D1 low byte, got %02x", got)
	}
	if cpu.regs.SR&(srCarry|srExtend) != srCarry|srExtend {
		t.Fatalf("carry/extend should be set, SR=%04x", cpu.regs.SR)
	}
	if cpu.regs.SR&srZero != 0 {
		t.Fatalf("zero flag should be cleared, SR=%04x", cpu.regs.SR)
	}
}

func TestNBCDRegister(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.SR |= srExtend
	cpu.regs.D[3] = 0x01

	runSingleInstruction(t, cpu, ram, "NBCD D3")

	if got := cpu.regs.D[3] & 0xff; got != 0x98 {
		t.Fatalf("expected 0x98 in D3 low byte, got %02x", got)
	}
	if cpu.regs.SR&(srCarry|srExtend) != srCarry|srExtend {
		t.Fatalf("carry/extend should be set after borrow, SR=%04x", cpu.regs.SR)
	}
	if cpu.regs.SR&srZero != 0 {
		t.Fatalf("zero flag should be cleared, SR=%04x", cpu.regs.SR)
	}
}
