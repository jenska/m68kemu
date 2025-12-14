package m68kemu

import "testing"

func TestLogicalInstructions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*cpu, *RAM)
		src   string
		code  []byte
		check func(*cpu, *RAM)
	}{
		{
			name: "ANDIDestinationDataRegister",
			setup: func(c *cpu, _ *RAM) {
				c.regs.D[0] = 0xf0f0
				c.regs.SR = srExtend | srCarry
			},
			code: []byte{0x02, 0x40, 0x0f, 0x0f}, // ANDI.W #$0f0f,D0
			check: func(c *cpu, _ *RAM) {
				if got := c.regs.D[0] & 0xffff; got != 0x0000 {
					t.Fatalf("unexpected D0 after ANDI: %04x", got)
				}
				if c.regs.SR&srZero == 0 {
					t.Fatalf("zero flag not set after ANDI: SR=%04x", c.regs.SR)
				}
				if c.regs.SR&(srCarry|srOverflow) != 0 {
					t.Fatalf("carry/overflow not cleared: SR=%04x", c.regs.SR)
				}
				if c.regs.SR&srExtend == 0 {
					t.Fatalf("extend flag should be preserved: SR=%04x", c.regs.SR)
				}
			},
		},
		{
			name: "ORDestinationMemory",
			setup: func(c *cpu, ram *RAM) {
				c.regs.D[0] = 0x0f
				c.regs.A[0] = 0x1000
				_ = ram.Write(Byte, 0x1000, 0xf0)
			},
			src: "OR.B D0,(A0)\n",
			check: func(c *cpu, ram *RAM) {
				value, _ := ram.Read(Byte, 0x1000)
				if value != 0xff {
					t.Fatalf("unexpected OR result in memory: %02x", value)
				}
				if c.regs.SR&srNegative == 0 {
					t.Fatalf("negative flag not set after OR: SR=%04x", c.regs.SR)
				}
				if c.regs.SR&srZero != 0 {
					t.Fatalf("zero flag incorrectly set after OR: SR=%04x", c.regs.SR)
				}
			},
		},
		{
			name: "EORDestinationData",
			setup: func(c *cpu, _ *RAM) {
				c.regs.D[0] = 0x55
				c.regs.D[1] = 0xaa
			},
			src: "EOR.B D0,D1\n",
			check: func(c *cpu, _ *RAM) {
				if got := c.regs.D[1] & 0xff; got != 0xff {
					t.Fatalf("unexpected EOR result: %02x", got)
				}
				if c.regs.SR&srZero != 0 {
					t.Fatalf("zero flag incorrectly set after EOR: SR=%04x", c.regs.SR)
				}
			},
		},
		{
			name: "NOTMemory",
			setup: func(c *cpu, ram *RAM) {
				c.regs.A[0] = 0x1000
				_ = ram.Write(Byte, 0x1000, 0x00)
			},
			src: "NOT.B (A0)\n",
			check: func(c *cpu, ram *RAM) {
				value, _ := ram.Read(Byte, 0x1000)
				if value != 0xff {
					t.Fatalf("unexpected NOT result: %02x", value)
				}
				if c.regs.SR&srNegative == 0 {
					t.Fatalf("negative flag not set after NOT: SR=%04x", c.regs.SR)
				}
			},
		},
		{
			name: "EORIImmediateMemory",
			setup: func(c *cpu, ram *RAM) {
				c.regs.A[0] = 0x1000
				_ = ram.Write(Long, 0x1000, 0xaaaa5555)
			},
			code: []byte{0x0a, 0x90, 0xff, 0xff, 0x00, 0x00}, // EORI.L #$ffff0000,(A0)
			check: func(c *cpu, ram *RAM) {
				value, _ := ram.Read(Long, 0x1000)
				if value != 0x55555555 {
					t.Fatalf("unexpected EORI result: %08x", value)
				}
				if c.regs.SR&srZero != 0 {
					t.Fatalf("zero flag incorrectly set after EORI: SR=%04x", c.regs.SR)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			tt.setup(cpu, ram)

			code := tt.code
			if code == nil {
				code = assemble(t, tt.src)
			}
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

			tt.check(cpu, ram)
		})
	}
}
