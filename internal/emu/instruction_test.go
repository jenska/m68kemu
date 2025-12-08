package emu

import (
	"testing"

	asm "github.com/jenska/m68kasm"
)

func newCPU(t *testing.T) (*CPU, *RAM) {
	t.Helper()

	memory := NewRAM(0, 1024*512)
	cpu, err := NewCPU(&memory, 0x1000, 0x2000)
	if err != nil {
		t.Fatalf("Failed to create CPU: %v", err)
	}
	return cpu, &memory
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
	cpu, ram := newCPU(t)

	tests := []struct {
		name string
		src  string
		prec func(cpu *CPU) bool
	}{
		{"MoveAddressMnemonicWord", "MOVEA.W #1,A0\n",
			func(c *CPU) bool {
				return int16(c.Registers().A[0]) == 1
			}},
		{"MoveAddressAbsLong", "MOVEA.L $100,A0\n",
			func(c *CPU) bool {
				return int16(c.Registers().A[1]) == 0
			}},
		{"MoveAddressAbsWord", "MOVEA.L $100.w,A0\n",
			func(c *CPU) bool {
				return c.Registers().A[1] == uint32(0)
			}},
		{"MoveAddressAbsWord", "MOVEA.L $10(PC),A0\n",
			func(c *CPU) bool {
				return int16(c.Registers().A[1]) == 0
			}},

		{"MoveAddressMnemonicLong", "MOVEA.L #1,A1\n",
			func(c *CPU) bool {
				return int32(c.Registers().A[1]) == 1
			}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := assemble(t, tt.src)

			for i := range code {
				ram.WriteByteTo(cpu.regs.PC+uint32(i), code[i])
			}

			cpu.Step()
			if !tt.prec(cpu) {
				t.Fatalf("unexpected precondition after execuding '%s'\n%s", tt.src, cpu)
			}
		})
	}

}
