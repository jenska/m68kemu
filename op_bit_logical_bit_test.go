package m68kemu

import "testing"

func TestBitOpcodeRegistrationPCRelative(t *testing.T) {
	t.Run("all bit ops are registered for PC-relative operands", func(t *testing.T) {
		for _, opcode := range []uint16{
			0x013a, 0x013b, 0x017a, 0x017b, 0x01ba, 0x01bb, 0x01fa, 0x01fb,
			0x083a, 0x083b, 0x087a, 0x087b, 0x08ba, 0x08bb, 0x08fa, 0x08fb,
		} {
			if opcodeTable[opcode] == nil {
				t.Fatalf("expected opcode %04x to be registered", opcode)
			}
		}
	})
}

func TestBitOperationsDataRegister(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[1] = 0b11
	cpu.regs.D[0] = 0 // bit number 0
	cpu.regs.SR |= srExtend

	code := assemble(t, "BTST #1,D1\nBCLR D0,D1\nBCHG #1,D1\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BTST failed: %v", err)
	}
	if cpu.regs.SR&srZero != 0 || cpu.regs.SR&srExtend == 0 {
		t.Fatalf("expected zero clear and extend preserved after BTST, SR=%04x", cpu.regs.SR)
	}

	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BCLR failed: %v", err)
	}
	if cpu.regs.D[1] != 0b10 || cpu.regs.SR&srZero != 0 {
		t.Fatalf("expected bit 0 cleared with zero clear, D1=%b SR=%04x", cpu.regs.D[1], cpu.regs.SR)
	}

	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BCHG failed: %v", err)
	}
	if cpu.regs.D[1] != 0 || cpu.regs.SR&srZero != 0 {
		t.Fatalf("expected bit 1 toggled to 0 with zero clear, D1=%b SR=%04x", cpu.regs.D[1], cpu.regs.SR)
	}
}

func TestBitSetMemory(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 9 // wraps to bit 1 for memory targets

	if err := ram.Write(Byte, 0x3000, 0x00); err != nil {
		t.Fatalf("failed to seed memory: %v", err)
	}

	code := assemble(t, "BSET D0,$3000\nBTST #1,$3000\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BSET failed: %v", err)
	}
	val, _ := ram.Read(Byte, 0x3000)
	if val != 0x02 || cpu.regs.SR&srZero == 0 {
		t.Fatalf("expected bit 1 set with zero flag set, val=%02x SR=%04x", val, cpu.regs.SR)
	}

	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BTST failed: %v", err)
	}
	if cpu.regs.SR&srZero != 0 {
		t.Fatalf("expected zero clear when testing set bit, SR=%04x", cpu.regs.SR)
	}
}

func TestBitImmediateWithDisplacementUsesImmediateBeforeEAExtensions(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[0] = 0x3000

	if err := ram.Write(Byte, 0x3008, 0x00); err != nil {
		t.Fatalf("failed to seed memory: %v", err)
	}

	code := assemble(t, "BTST #0,(8,A0)\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BTST failed: %v", err)
	}
	if cpu.regs.SR&srZero == 0 {
		t.Fatalf("expected zero flag set when testing clear bit, SR=%04x", cpu.regs.SR)
	}
}

func TestBTSTAllowsPCRelativeOperands(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 0
	start := cpu.regs.PC

	code := assemble(t, "BTST D0,target(PC)\n.WORD 0\ntarget:\n.BYTE 1\n")
	for i, b := range code {
		if err := ram.Write(Byte, start+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write opcode byte: %v", err)
		}
	}

	opcode, err := cpu.fetchOpcode()
	if err != nil {
		t.Fatalf("fetch opcode: %v", err)
	}
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BTST failed: %v", err)
	}
	if cpu.regs.SR&srZero != 0 {
		t.Fatalf("expected zero clear when testing set PC-relative bit, SR=%04x", cpu.regs.SR)
	}
	if cpu.regs.PC != start+4 {
		t.Fatalf("PC after BTST = %08x, want %08x", cpu.regs.PC, start+4)
	}
}

func TestBitModifyAllowsPCRelativeOperands(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		initial  byte
		want     byte
		wantZero bool
	}{
		{name: "BCHG", src: "BCHG D0,target(PC)\n", initial: 0x01, want: 0x00, wantZero: false},
		{name: "BCLR", src: "BCLR D0,target(PC)\n", initial: 0x01, want: 0x00, wantZero: false},
		{name: "BSET", src: "BSET D0,target(PC)\n", initial: 0x00, want: 0x01, wantZero: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			cpu.regs.D[0] = 0
			start := cpu.regs.PC

			code := assemble(t, tc.src+".WORD 0\ntarget:\n.BYTE "+byteLiteral(tc.initial)+"\n")
			for i, b := range code {
				if err := ram.Write(Byte, start+uint32(i), uint32(b)); err != nil {
					t.Fatalf("failed to write opcode byte: %v", err)
				}
			}

			opcode, err := cpu.fetchOpcode()
			if err != nil {
				t.Fatalf("fetch opcode: %v", err)
			}
			if err := cpu.executeInstruction(opcode); err != nil {
				t.Fatalf("%s failed: %v", tc.name, err)
			}

			got, err := ram.Read(Byte, start+6)
			if err != nil {
				t.Fatalf("read modified byte: %v", err)
			}
			if got != uint32(tc.want) {
				t.Fatalf("modified byte = %02x, want %02x", got, tc.want)
			}

			gotZero := cpu.regs.SR&srZero != 0
			if gotZero != tc.wantZero {
				t.Fatalf("zero flag = %v, want %v (SR=%04x)", gotZero, tc.wantZero, cpu.regs.SR)
			}
			if cpu.regs.PC != start+4 {
				t.Fatalf("PC after %s = %08x, want %08x", tc.name, cpu.regs.PC, start+4)
			}
		})
	}
}

func byteLiteral(value byte) string {
	return "$" + string([]byte{
		"0123456789ABCDEF"[value>>4],
		"0123456789ABCDEF"[value&0x0f],
	})
}
