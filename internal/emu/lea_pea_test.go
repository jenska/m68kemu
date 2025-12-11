package emu

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
				if value != 0x2008 {
					t.Fatalf("expected pushed PC-relative address 0x2008, got %08x", value)
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
