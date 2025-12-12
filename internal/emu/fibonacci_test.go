package emu

import "testing"

func TestFibonacciProgram(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC

	program := assemble(t, `
        LEA $3000,A0
        MOVEQ #0,D0
        MOVEQ #1,D1
        MOVEQ #8,D2
        MOVE.L D0,(A0)+
        MOVE.L D1,(A0)+
loop:   MOVE.L D1,D3
        ADD.L D0,D1
        MOVE.L D1,(A0)+
        MOVE.L D3,D0
        SUBQ.W #1,D2
        BNE.S loop
        NOP
`)

	for i, b := range program {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	endPC := startPC + uint32(len(program))
	for steps := 0; steps < 200; steps++ {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d: %v", steps, err)
		}
		if cpu.regs.PC >= endPC {
			break
		}
	}

	expected := []uint32{0, 1, 1, 2, 3, 5, 8, 13, 21, 34}
	for i, want := range expected {
		addr := uint32(0x3000 + i*4)
		got, err := ram.Read(Long, addr)
		if err != nil {
			t.Fatalf("read fib @%04x: %v", addr, err)
		}
		if got != want {
			t.Fatalf("fib(%d) = %d, want %d", i, got, want)
		}
	}
}
