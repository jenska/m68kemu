package m68kemu

import "testing"

func TestLEA(t *testing.T) {
	tests := []struct {
		name  string
		code  string
		setup func(cpu *CPU, ram *RAM)
		check func(t *testing.T, cpu *CPU, ram *RAM)
	}{
		{
			name: "PostIncrementAddressing",
			code: "LEA (A1)+,A0\n",
			setup: func(cpu *CPU, _ *RAM) {
				cpu.regs.A[1] = 0x3000
			},
			check: func(t *testing.T, cpu *CPU, _ *RAM) {
				if cpu.regs.A[0] != 0x3000 {
					t.Fatalf("expected A0 to capture original A1, got %04x", cpu.regs.A[0])
				}
				if cpu.regs.A[1] != 0x3004 {
					t.Fatalf("expected A1 to post-increment by 4, got %04x", cpu.regs.A[1])
				}
			},
		},
		{
			name: "PCRelativeDisplacement",
			code: "PEA 4(PC)\n LEA (A7),A1\n", // second instruction used to read back pushed address
			check: func(t *testing.T, cpu *CPU, ram *RAM) {
				if cpu.regs.A[7] != 0x0ffc {
					t.Fatalf("expected stack pointer to decrement by 4, got %04x", cpu.regs.A[7])
				}
				value, _ := ram.Read(Long, cpu.regs.A[7])
				if value != 0x2006 {
					t.Fatalf("expected pushed PC-relative address 0x2006, got %08x", value)
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

			code := assemble(t, tt.code)
			for i := range code {
				addr := cpu.regs.PC + uint32(i)
				if err := ram.Write(Byte, addr, uint32(code[i])); err != nil {
					t.Fatalf("failed to write byte to %08x: %v", addr, err)
				}
			}

			for {
				opcode, err := cpu.fetchOpcode()
				if err != nil {
					t.Fatalf("failed to fetch opcode: %v", err)
				}
				if err := cpu.executeInstruction(opcode); err != nil {
					t.Fatalf("failed to execute opcode %04x: %v", opcode, err)
				}

				// Stop once we've executed all assembled instructions.
				if cpu.regs.PC >= 0x2000+uint32(len(code)) {
					break
				}
			}

			tt.check(t, cpu, ram)
		})
	}
}

func TestLEAPCRelativeLabelTargetsLabelStart(t *testing.T) {
	helper := newStepTestHelper(t)
	helper.LoadProgram([]byte{
		0x41, 0xFA, 0x00, 0x04, // LEA 4(PC),A0
		0x4E, 0x71, // NOP
		0x4E, 0x75, // RTS
	})

	helper.RunInstructions(1)

	if helper.cpu.regs.A[0] != 0x2006 {
		t.Fatalf("A0 = %08x, want label start %08x", helper.cpu.regs.A[0], uint32(0x2006))
	}
}

func TestMoveInstruction(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		setup func(cpu *CPU, ram *RAM)
		check func(t *testing.T, cpu *CPU, ram *RAM)
	}{
		{
			name: "MoveqImmediateToDataRegisters",
			src:  "MOVEQ #-1,D1\nMOVEQ #1, D2\n",
			check: func(t *testing.T, cpu *CPU, _ *RAM) {
				if cpu.Registers().D[1] != -1 && cpu.Registers().D[2] == 1 {
					t.Fatalf("expected D1 to be -1, got %d", cpu.Registers().D[1])
				}
			},
		},
		{
			name: "MoveByteImmediateToDataRegister",
			src:  "MOVE.B #0,D0\n",
			check: func(t *testing.T, cpu *CPU, _ *RAM) {
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
			check: func(t *testing.T, cpu *CPU, _ *RAM) {
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
			setup: func(cpu *CPU, _ *RAM) {
				cpu.regs.D[0] = 0x12345678
				cpu.regs.A[1] = 0x2100
			},
			check: func(t *testing.T, cpu *CPU, ram *RAM) {
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
			setup: func(cpu *CPU, ram *RAM) {
				cpu.regs.A[0] = 0x3000
				ram.Write(Word, 0x3000, 0x0)
				cpu.regs.A[1] = 0x3102
			},
			check: func(t *testing.T, cpu *CPU, ram *RAM) {
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

func TestMovep(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		setup func(cpu *CPU, ram *RAM)
		check func(t *testing.T, cpu *CPU, ram *RAM)
	}{
		{
			name: "MovepWordRegisterToMemory",
			src:  "MOVEP.W D0,(16,A1)",
			setup: func(cpu *CPU, _ *RAM) {
				cpu.regs.D[0] = 0x1234
				cpu.regs.A[1] = 0x2000
			},
			check: func(t *testing.T, cpu *CPU, ram *RAM) {
				if got, _ := ram.Read(Byte, 0x2010); got != 0x12 {
					t.Fatalf("expected high byte at 0x2010 to be 0x12, got %02x", got)
				}
				if got, _ := ram.Read(Byte, 0x2012); got != 0x34 {
					t.Fatalf("expected low byte at 0x2012 to be 0x34, got %02x", got)
				}
				if cpu.regs.SR&(srNegative|srZero) != 0 {
					t.Fatalf("expected N and Z to be clear, got SR=%04x", cpu.regs.SR)
				}
			},
		},
		{
			name: "MovepWordMemoryToRegister",
			src:  "MOVEP.W (16,A2),D2",
			setup: func(cpu *CPU, ram *RAM) {
				cpu.regs.D[2] = 0x11110000
				cpu.regs.A[2] = 0x2100
				ram.Write(Byte, 0x2110, 0xab)
				ram.Write(Byte, 0x2112, 0xcd)
			},
			check: func(t *testing.T, cpu *CPU, _ *RAM) {
				if got := cpu.regs.D[2]; got != 0x1111abcd {
					t.Fatalf("expected D2 to be 0x1111abcd, got %08x", uint32(got))
				}
				if cpu.regs.SR&srNegative == 0 {
					t.Fatalf("expected negative flag set, SR=%04x", cpu.regs.SR)
				}
			},
		},
		{
			name: "MovepLongRegisterToMemory",
			src:  "MOVEP.L D3,(4,A0)",
			setup: func(cpu *CPU, _ *RAM) {
				cpu.regs.D[3] = -0x76543211
				cpu.regs.A[0] = 0x3000
			},
			check: func(t *testing.T, cpu *CPU, ram *RAM) {
				for i, b := range []byte{0x89, 0xab, 0xcd, 0xef} {
					addr := uint32(0x3004 + i*2)
					if got, _ := ram.Read(Byte, addr); got != uint32(b) {
						t.Fatalf("expected byte %02x at %04x, got %02x", b, addr, got)
					}
				}
				if cpu.regs.SR&srNegative == 0 {
					t.Fatalf("expected negative flag set after storing 0x89abcdef, SR=%04x", cpu.regs.SR)
				}
			},
		},
		{
			name: "MovepLongMemoryToRegister",
			src:  "MOVEP.L (8,A4),D4",
			setup: func(cpu *CPU, ram *RAM) {
				cpu.regs.A[4] = 0x4000
				ram.Write(Byte, 0x4008, 0xfe)
				ram.Write(Byte, 0x400a, 0xdc)
				ram.Write(Byte, 0x400c, 0xba)
				ram.Write(Byte, 0x400e, 0x98)
			},
			check: func(t *testing.T, cpu *CPU, _ *RAM) {
				if got := cpu.regs.D[4]; got != -0x01234568 {
					t.Fatalf("expected D4 to be 0xfedcba98, got %08x", uint32(got))
				}
				if cpu.regs.SR&srNegative == 0 {
					t.Fatalf("expected negative flag set for 0xfedcba98, SR=%04x", cpu.regs.SR)
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

func TestMovemStoreAndLoad(t *testing.T) {
	cpu, ram := newEnvironment(t)

	cpu.regs.D[0] = 0x11111111
	cpu.regs.D[1] = 0x22222222
	cpu.regs.A[1] = 0x33333333
	cpu.regs.A[2] = 0x3000

	code := assemble(t, `
                MOVEM.L D0-D1/A1,-(A2)
                MOVEM.L (A2)+,D2-D3/A3
        `)

	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program: %v", err)
		}
	}

	for i := range 2 {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d failed: %v", i, err)
		}
	}

	if cpu.regs.A[2] != 0x3000 {
		t.Fatalf("A2 should be restored by MOVEM pair, got %04x", cpu.regs.A[2])
	}
	if cpu.regs.D[2] != cpu.regs.D[0] || cpu.regs.D[3] != cpu.regs.D[1] || cpu.regs.A[3] != cpu.regs.A[1] {
		t.Fatalf("MOVEM load mismatch: D2=%08x D3=%08x A3=%08x", cpu.regs.D[2], cpu.regs.D[3], cpu.regs.A[3])
	}
}

func TestMovemWordLoadsSignExtendRegisters(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[2] = 0x3000

	if err := ram.Write(Word, 0x3000, 0x8001); err != nil {
		t.Fatalf("seed D0 source: %v", err)
	}
	if err := ram.Write(Word, 0x3002, 0xfffe); err != nil {
		t.Fatalf("seed A1 source: %v", err)
	}

	code := assemble(t, "MOVEM.W (A2)+,D0/A1")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program: %v", err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("step failed: %v", err)
	}

	if cpu.regs.D[0] != -32767 {
		t.Fatalf("D0 should be sign-extended, got %08x", uint32(cpu.regs.D[0]))
	}
	if cpu.regs.A[1] != 0xfffffffe {
		t.Fatalf("A1 should be sign-extended, got %08x", cpu.regs.A[1])
	}
	if cpu.regs.A[2] != 0x3004 {
		t.Fatalf("A2 should postincrement by 4, got %08x", cpu.regs.A[2])
	}
}

func TestMovemLongStoresSequentialWordsForControlMode(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[0] = 0x3000
	cpu.regs.A[1] = 0x4000

	values := []uint32{
		0x000000ff,
		0x000d000b,
		0x00080002,
		0x00020007,
		0x00080001,
		0x00070001,
		0x00015555,
		0x5555000d,
	}
	for i, value := range values {
		if err := ram.Write(Long, cpu.regs.A[0]+uint32(i*4), value); err != nil {
			t.Fatalf("seed memory %d: %v", i, err)
		}
	}

	code := assemble(t, `
		MOVEM.L (A0)+,D2-D7/A4-A5
		MOVEM.L D2-D7/A4-A5,(A1)
	`)
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program: %v", err)
		}
	}

	for step := range 2 {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d failed: %v", step, err)
		}
	}

	for i, want := range values {
		got, err := ram.Read(Long, cpu.regs.A[1]+uint32(i*4))
		if err != nil {
			t.Fatalf("read back %d: %v", i, err)
		}
		if got != want {
			t.Fatalf("destination long %d = %08x, want %08x", i, got, want)
		}
	}
	if cpu.regs.A[1] != 0x4000 {
		t.Fatalf("A1 should remain unchanged for MOVEM to (A1), got %08x", cpu.regs.A[1])
	}
}

func TestMovemLongLoadsSequentialWordsForControlMode(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[6] = 0x3050

	values := []uint32{
		0x11111111,
		0x22222222,
		0x33333333,
		0x44444444,
	}
	base := cpu.regs.A[6] - 56
	for i, value := range values {
		if err := ram.Write(Long, base+uint32(i*4), value); err != nil {
			t.Fatalf("seed memory %d: %v", i, err)
		}
	}

	code := assemble(t, "MOVEM.L -56(A6),D2/D3/A2/A3")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program: %v", err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("step failed: %v", err)
	}

	if uint32(cpu.regs.D[2]) != values[0] {
		t.Fatalf("D2 = %08x, want %08x", uint32(cpu.regs.D[2]), values[0])
	}
	if uint32(cpu.regs.D[3]) != values[1] {
		t.Fatalf("D3 = %08x, want %08x", uint32(cpu.regs.D[3]), values[1])
	}
	if cpu.regs.A[2] != values[2] {
		t.Fatalf("A2 = %08x, want %08x", cpu.regs.A[2], values[2])
	}
	if cpu.regs.A[3] != values[3] {
		t.Fatalf("A3 = %08x, want %08x", cpu.regs.A[3], values[3])
	}
	if cpu.regs.A[6] != 0x3050 {
		t.Fatalf("A6 should remain unchanged for control-mode MOVEM load, got %08x", cpu.regs.A[6])
	}
}
