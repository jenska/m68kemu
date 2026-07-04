package m68kemu

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
	if cpu.regs.SR&srZero != 0 {
		t.Fatalf("zero flag should remain clear without prior zero, SR=%04x", cpu.regs.SR)
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

func TestBCDZeroPropagation(t *testing.T) {
	tests := []struct {
		name    string
		setupSR uint16
		src     uint8
		dst     uint8
		asm     string
		wantSRZ bool
		wantDst uint8
	}{
		{"ABCDZeroStaysSet", srZero, 0x00, 0x00, "ABCD D0,D1", true, 0x00},
		{"ABCDZeroClears", srZero, 0x01, 0x00, "ABCD D0,D1", false, 0x01},
		{"ABCDZeroNeedsPriorZ", 0, 0x00, 0x00, "ABCD D0,D1", false, 0x00},
		{"SBCDZeroClears", srZero, 0x00, 0x01, "SBCD D0,D1", false, 0x01},
		{"SBCDZeroNeedsPriorZ", 0, 0x00, 0x00, "SBCD D0,D1", false, 0x00},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			cpu.regs.SR &^= srCarry | srExtend | srZero
			cpu.regs.SR |= tt.setupSR
			cpu.regs.D[0] = int32(tt.src)
			cpu.regs.D[1] = int32(tt.dst)

			runSingleInstruction(t, cpu, ram, tt.asm)

			if got := cpu.regs.D[1] & 0xff; got != int32(tt.wantDst) {
				t.Fatalf("expected %02x in destination, got %02x", tt.wantDst, got)
			}
			if zSet := cpu.regs.SR&srZero != 0; zSet != tt.wantSRZ {
				t.Fatalf("zero flag mismatch: want %v got %v (SR=%04x)", tt.wantSRZ, zSet, cpu.regs.SR)
			}
		})
	}
}

func TestNBCDZeroAndExtend(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.SR &^= srExtend | srZero
	cpu.regs.D[2] = 0x00

	runSingleInstruction(t, cpu, ram, "NBCD D2")

	if got := cpu.regs.D[2] & 0xff; got != 0x00 {
		t.Fatalf("expected NBCD of zero to stay zero, got %02x", got)
	}
	if cpu.regs.SR&srZero == 0 {
		t.Fatalf("zero flag should be set for NBCD of zero, SR=%04x", cpu.regs.SR)
	}
	if cpu.regs.SR&(srCarry|srExtend) != 0 {
		t.Fatalf("carry/extend should remain clear without borrow, SR=%04x", cpu.regs.SR)
	}
}

func TestNbcdByteUsesWordStepForA7(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[7] = 0x3002

	if err := ram.Write(Byte, 0x3000, 0x01); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	runSingleInstruction(t, cpu, ram, "NBCD -(A7)")

	if cpu.regs.A[7] != 0x3000 {
		t.Fatalf("A7 should predecrement by 2 for byte NBCD, got %04x", cpu.regs.A[7])
	}
	if got, _ := ram.Read(Byte, 0x3000); got != 0x99 {
		t.Fatalf("unexpected NBCD result %02x", got)
	}
}

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

func TestAddiByteToDataRegister(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 0x1234ff

	code := assemble(t, "ADDI.B #1,D0\n")
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

	if got := uint32(cpu.regs.D[0]); got != 0x00123400 {
		t.Fatalf("expected D0=0x00123400 got %08x", got)
	}
	expected := uint16(srZero | srCarry | srExtend)
	if got := cpu.regs.SR & (srZero | srNegative | srOverflow | srCarry | srExtend); got != expected {
		t.Fatalf("unexpected flags %04x", got)
	}
}

func TestSubiWordToDataRegister(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[1] = 0x12340000

	code := assemble(t, "SUBI.W #1,D1\n")
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

	if got := uint32(cpu.regs.D[1]); got != 0x1234ffff {
		t.Fatalf("expected D1=0x1234ffff got %08x", got)
	}
	expected := uint16(srNegative | srCarry | srExtend)
	if got := cpu.regs.SR & (srZero | srNegative | srOverflow | srCarry | srExtend); got != expected {
		t.Fatalf("unexpected flags %04x", got)
	}
}

func TestAddiLongToMemory(t *testing.T) {
	cpu, ram := newEnvironment(t)
	if err := ram.Write(Long, 0x3000, 1); err != nil {
		t.Fatalf("write data: %v", err)
	}

	code := assemble(t, "ADDI.L #2,$3000\n")
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
		t.Fatalf("expected memory=3 got %08x err=%v", got, err)
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

func TestDivByZeroTriggersExceptionVector(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 10
	cpu.regs.D[1] = 0

	handler := uint32(0x3456)
	if err := ram.Write(Long, uint32(XDivByZero<<2), handler); err != nil {
		t.Fatalf("failed to install divide-by-zero vector: %v", err)
	}

	originalPC := cpu.regs.PC
	code := assemble(t, "DIVU D1,D0\n")
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

	expectedSP := cpu.regs.SSP - exceptionFrameSize
	if cpu.regs.A[7] != expectedSP {
		t.Fatalf("SP after divide-by-zero = %08x, want %08x", cpu.regs.A[7], expectedSP)
	}

	stackedPC, err := ram.Read(Long, expectedSP+uint32(Word))
	if err != nil {
		t.Fatalf("read stacked PC: %v", err)
	}
	if want := originalPC + uint32(Word); stackedPC != want {
		t.Fatalf("stacked PC = %08x, want %08x", stackedPC, want)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("PC after divide-by-zero = %08x, want %08x", cpu.regs.PC, handler)
	}

	state := cpu.DebugState()
	if !state.HasException || state.LastException.Vector != XDivByZero {
		t.Fatalf("expected divide-by-zero exception state, got %+v", state.LastException)
	}
	if want := originalPC + uint32(Word); state.LastException.PC != want {
		t.Fatalf("exception PC = %08x, want %08x", state.LastException.PC, want)
	}
}

func TestMulDivImmediateInstructions(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 3
	cpu.regs.D[1] = -7

	code := []byte{
		0xC0, 0xFC, 0x00, 0x05, // MULU #5,D0
		0xC3, 0xFC, 0xFF, 0xFE, // MULS #-2,D1
	}
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	step := func(reg int, expect int32) {
		t.Helper()
		opcode, err := cpu.fetchOpcode()
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		if err := cpu.executeInstruction(opcode); err != nil {
			t.Fatalf("exec: %v", err)
		}
		if cpu.regs.D[reg] != expect {
			t.Fatalf("expected D%d=%08x got %08x", reg, expect, cpu.regs.D[reg])
		}
	}

	step(0, 15)
	step(1, 14)
}

func TestAddaSuba(t *testing.T) {
	cpu, ram := newEnvironment(t)
	code := assemble(t, "ADDA.W #-1,A0\nSUBA.L #1,A0\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("ADDA failed: %v", err)
	}
	if cpu.regs.A[0] != 0xffffffff {
		t.Fatalf("expected sign-extended word add, got %08x", cpu.regs.A[0])
	}

	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("SUBA failed: %v", err)
	}
	if cpu.regs.A[0] != 0xfffffffe {
		t.Fatalf("expected subtraction result 0xfffffffe, got %08x", cpu.regs.A[0])
	}
	if cpu.regs.SR != 0x2700 {
		t.Fatalf("expected condition codes untouched, SR=%04x", cpu.regs.SR)
	}
}

func TestAddaSubaAddressRegisterSource(t *testing.T) {
	tests := []struct {
		name   string
		asm    string
		a0     uint32
		a1     uint32
		sr     uint16
		wantA1 uint32
	}{
		{
			name:   "SUBA.L A0,A1",
			asm:    "SUBA.L A0,A1\n",
			a0:     0x00000003,
			a1:     0x00000010,
			sr:     0x271b,
			wantA1: 0x0000000d,
		},
		{
			name:   "SUBA.W A0,A1",
			asm:    "SUBA.W A0,A1\n",
			a0:     0x1234ffff,
			a1:     0x00000005,
			sr:     0x271b,
			wantA1: 0x00000006,
		},
		{
			name:   "ADDA.L A0,A1",
			asm:    "ADDA.L A0,A1\n",
			a0:     0x00000003,
			a1:     0x00000005,
			sr:     0x271b,
			wantA1: 0x00000008,
		},
		{
			name:   "ADDA.W A0,A1",
			asm:    "ADDA.W A0,A1\n",
			a0:     0x1234ffff,
			a1:     0x00000005,
			sr:     0x271b,
			wantA1: 0x00000004,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			cpu.regs.A[0] = tt.a0
			cpu.regs.A[1] = tt.a1
			cpu.regs.SR = tt.sr

			code := assemble(t, tt.asm)
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
				t.Fatalf("execute %s: %v", tt.name, err)
			}

			if cpu.regs.A[1] != tt.wantA1 {
				t.Fatalf("A1 = %08x, want %08x", cpu.regs.A[1], tt.wantA1)
			}
			if cpu.regs.SR != tt.sr {
				t.Fatalf("expected SR unchanged, got %04x want %04x", cpu.regs.SR, tt.sr)
			}
		})
	}
}

func TestAddSubAddressRegisterSource(t *testing.T) {
	tests := []struct {
		name   string
		asm    string
		a3     uint32
		d0     int32
		sr     uint16
		wantD0 int32
	}{
		{
			name:   "ADD.L A3,D0",
			asm:    "ADD.L A3,D0\n",
			a3:     0xfffffff8,
			d0:     0x0000000e,
			sr:     0x2700,
			wantD0: 0x00000006,
		},
		{
			name:   "ADD.W A3,D0",
			asm:    "ADD.W A3,D0\n",
			a3:     0x1234ffff,
			d0:     0x00000005,
			sr:     0x2700,
			wantD0: 0x00000004,
		},
		{
			name:   "SUB.L A3,D0",
			asm:    "SUB.L A3,D0\n",
			a3:     0x00000003,
			d0:     0x00000010,
			sr:     0x2700,
			wantD0: 0x0000000d,
		},
		{
			name:   "SUB.W A3,D0",
			asm:    "SUB.W A3,D0\n",
			a3:     0x1234ffff,
			d0:     0x00000005,
			sr:     0x2700,
			wantD0: 0x00000006,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			cpu.regs.A[3] = tt.a3
			cpu.regs.D[0] = tt.d0
			cpu.regs.SR = tt.sr

			code := assemble(t, tt.asm)
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
				t.Fatalf("execute %s: %v", tt.name, err)
			}

			if cpu.regs.D[0] != tt.wantD0 {
				t.Fatalf("D0 = %08x, want %08x", uint32(cpu.regs.D[0]), uint32(tt.wantD0))
			}
		})
	}
}
