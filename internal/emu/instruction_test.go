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
			cpu, ram := newCPU(t)

			code := assemble(t, tt.src)

			for i := range code {
				addr := cpu.regs.PC + uint32(i)
				if err := ram.WriteByteTo(addr, code[i]); err != nil {
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
