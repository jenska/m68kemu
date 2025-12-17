package m68kemu

import "testing"

func TestMovep(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		setup func(cpu *cpu, ram *RAM)
		check func(t *testing.T, cpu *cpu, ram *RAM)
	}{
		{
			name: "MovepWordRegisterToMemory",
			src:  "MOVEP.W D0,(16,A1)",
			setup: func(cpu *cpu, _ *RAM) {
				cpu.regs.D[0] = 0x1234
				cpu.regs.A[1] = 0x2000
			},
			check: func(t *testing.T, cpu *cpu, ram *RAM) {
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
			setup: func(cpu *cpu, ram *RAM) {
				cpu.regs.D[2] = 0x11110000
				cpu.regs.A[2] = 0x2100
				ram.Write(Byte, 0x2110, 0xab)
				ram.Write(Byte, 0x2112, 0xcd)
			},
			check: func(t *testing.T, cpu *cpu, _ *RAM) {
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
			setup: func(cpu *cpu, _ *RAM) {
				cpu.regs.D[3] = -0x76543211
				cpu.regs.A[0] = 0x3000
			},
			check: func(t *testing.T, cpu *cpu, ram *RAM) {
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
			setup: func(cpu *cpu, ram *RAM) {
				cpu.regs.A[4] = 0x4000
				ram.Write(Byte, 0x4008, 0xfe)
				ram.Write(Byte, 0x400a, 0xdc)
				ram.Write(Byte, 0x400c, 0xba)
				ram.Write(Byte, 0x400e, 0x98)
			},
			check: func(t *testing.T, cpu *cpu, _ *RAM) {
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
