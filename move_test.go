package m68kemu

import "testing"

func TestMoveInstruction(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		setup func(cpu *cpu, ram *RAM)
		check func(t *testing.T, cpu *cpu, ram *RAM)
	}{
		{
			name: "MoveqImmediateToDataRegisters",
			src:  "MOVEQ #-1,D1\nMOVEQ #1, D2\n",
			check: func(t *testing.T, cpu *cpu, _ *RAM) {
				if cpu.Registers().D[1] != -1 && cpu.Registers().D[2] == 1 {
					t.Fatalf("expected D1 to be -1, got %d", cpu.Registers().D[1])
				}
			},
		},
		{
			name: "MoveByteImmediateToDataRegister",
			src:  "MOVE.B #0,D0\n",
			check: func(t *testing.T, cpu *cpu, _ *RAM) {
				if got := cpu.Registers().D[0] & 0xff; got != 0 {
					t.Fatalf("expected D0 low byte to be 0, got %02x", got)
				}
				if cpu.regs.SR&srZero == 0 {
					t.Fatalf("expected zero flag to be set")
				}
				if cpu.regs.SR&(srNegative|srOverflow|srCarry) != 0 {
					t.Fatalf("expected N, V and C flags to be cleared, got SR=%04x", cpu.regs.SR)
				}
			},
		},
		{
			name: "MoveWordImmediateSetsNegative",
			src:  "MOVE.W #$ffff,D1\n",
			check: func(t *testing.T, cpu *cpu, _ *RAM) {
				if got := cpu.Registers().D[1] & 0xffff; got != 0xffff {
					t.Fatalf("expected D1 low word to be ffff, got %04x", got)
				}
				if cpu.regs.SR&srNegative == 0 {
					t.Fatalf("expected negative flag to be set")
				}
				if cpu.regs.SR&srZero != 0 {
					t.Fatalf("expected zero flag to be cleared")
				}
			},
		},
		{
			name: "MoveLongRegisterToPostIncrement",
			src:  "MOVE.L D0,(A1)+\n",
			setup: func(cpu *cpu, _ *RAM) {
				cpu.regs.D[0] = 0x12345678
				cpu.regs.A[1] = 0x2100
			},
			check: func(t *testing.T, cpu *cpu, ram *RAM) {
				for i, b := range []byte{0x12, 0x34, 0x56, 0x78} {
					addr := uint32(0x2100 + i)
					if got, _ := ram.Read(Byte, addr); got != uint32(b) {
						t.Fatalf("expected memory %04x to be %02x, got %02x (A1=%04x, D0=%08x)", addr, b, got, cpu.regs.A[1], uint32(cpu.regs.D[0]))
					}
				}
				if cpu.regs.A[1] != 0x2104 {
					t.Fatalf("expected A1 to increment to 0x2104, got %04x", cpu.regs.A[1])
				}
			},
		},
		{
			name: "MoveWordPostIncrementToPreDecrement",
			src:  "MOVE.W (A0)+,-(A1)\n",
			setup: func(cpu *cpu, ram *RAM) {
				cpu.regs.A[0] = 0x3000
				ram.Write(Word, 0x3000, 0x0)
				cpu.regs.A[1] = 0x3102
			},
			check: func(t *testing.T, cpu *cpu, ram *RAM) {
				if cpu.regs.A[0] != 0x3002 {
					t.Fatalf("expected A0 to post-increment to 0x3002, got %04x", cpu.regs.A[0])
				}
				if cpu.regs.A[1] != 0x3100 {
					t.Fatalf("expected A1 to pre-decrement to 0x3100, got %04x", cpu.regs.A[1])
				}
				if got, _ := ram.Read(Word, 0x3100); got != 0x0000 {
					t.Fatalf("expected memory at 0x3100 to be zero, got %04x", got)
				}
				if cpu.regs.SR&srZero == 0 {
					t.Fatalf("expected zero flag to be set after moving zero")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			if tt.setup != nil {
				tt.setup(cpu, ram)
			}

			code := assemble(t, tt.src)
			for i := range code {
				addr := cpu.regs.PC + uint32(i)
				if err := ram.Write(Byte, addr, uint32(code[i])); err != nil {
					t.Fatalf("failed to write byte to %08x: %v", addr, err)
				}
			}

			opcode, err := cpu.fetchOpcode()
			if err != nil {
				t.Fatalf("failed to fetch opcode: %v", err)
			}
			if err := cpu.executeInstruction(opcode); err != nil {
				t.Fatalf("failed to execute opcode %04x: %v", opcode, err)
			}
			tt.check(t, cpu, ram)
		})
	}
}
