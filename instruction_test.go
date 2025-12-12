package m68kemu

import (
	"testing"

	asm "github.com/jenska/m68kasm"
)

func newEnvironment(t *testing.T) (*cpu, *RAM) {
	t.Helper()

	memory := NewRAM(0, 1024*64)
	bus := NewBus(&memory)
	memory.Write(Long, 0, 0x1000)
	memory.Write(Long, 4, 0x2000)
	processor, err := NewCPU(bus)
	if err != nil {
		t.Fatalf("Failed to create CPU: %v", err)
	}
	impl, ok := processor.(*cpu)
	if !ok {
		t.Fatalf("CPU implementation has unexpected type %T", processor)
	}
	return impl, &memory
}

func assemble(t *testing.T, instruction string) []byte {
	t.Helper()

	code, err := asm.AssembleString(instruction)
	if err != nil {
		t.Fatalf("Assembler failed: %v", err)
	}
	return code
}

func TestInstructions(t *testing.T) {
	tests := []struct {
		name string
		src  string
		prec func(cpu *cpu) bool
	}{
		{"MoveAddressMnemonicWord", "MOVEA.W #1,A0\n",
			func(c *cpu) bool {
				return int16(c.Registers().A[0]) == 1
			}},
		{"MoveAddressAbsLong", "MOVEA.L $100,A0\n",
			func(c *cpu) bool {
				return int16(c.Registers().A[1]) == 0
			}},
		{"MoveAddressAbsWord", "MOVEA.L $100.w,A0\n",
			func(c *cpu) bool {
				return c.Registers().A[1] == uint32(0)
			}},
		{"MoveAddressAbsWord", "MOVEA.L $10(PC),A0\n",
			func(c *cpu) bool {
				return int16(c.Registers().A[1]) == 0
			}},

		{"MoveAddressMnemonicLong", "MOVEA.L #1,A1\n",
			func(c *cpu) bool {
				return int32(c.Registers().A[1]) == 1
			}},
		{"MoveAddressMnemonicLongFullWidth", "MOVEA.L #$12345678,A2\n",
			func(c *cpu) bool {
				return c.Registers().A[2] == 0x12345678
			}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)

			code := assemble(t, tt.src)

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
			if !tt.prec(cpu) {
				t.Fatalf("unexpected precondition after execuding '%s'\n%s", tt.src, cpu)
			}
		})
	}

}
