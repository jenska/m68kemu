package emu

import "testing"

func TestJsrRts(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC

	program := assemble(t, `
        BRA main
inc:    ADD.L D1,D0
        RTS
main:   MOVEQ #1,D0
        MOVEQ #1,D1
        BSR inc
        NOP
`)

	for i, b := range program {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	endPC := startPC + uint32(len(program))
	for cpu.regs.PC < endPC {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step: %v", err)
		}
	}

	if cpu.regs.D[0] != 2 {
		t.Fatalf("D0=%d, want 2", cpu.regs.D[0])
	}
}
