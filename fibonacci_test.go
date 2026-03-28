package m68kemu

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

func TestRecursiveFibonacciProgram(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC

	program := assemble(t, `
main:   LEA $4000,A0
        MOVEQ #5,D0
        BSR fibonacci
        MOVE.L D0,(A0)
        NOP
fibonacci:    MOVE.L D0,D1
        SUBQ.L #1,D1
        BLE.S return
        MOVE.L D0,-(A7)
        SUBQ.L #1,D0
        BSR.S fibonacci
        MOVE.L D0,-(A7)
        MOVE.L 4(A7),D0
        SUBQ.L #2,D0
        BSR.S fibonacci
        MOVE.L (A7)+,D2
        MOVE.L (A7)+,D1
        ADD.L D2,D0
return: RTS
`)

	for i, b := range program {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	endPC := startPC + uint32(len(program))
	for steps := 0; steps < 500; steps++ {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d: %v", steps, err)
		}
		if cpu.regs.PC >= endPC {
			break
		}
	}

	result, err := ram.Read(Long, 0x4000)
	if err != nil {
		t.Fatalf("read fib result: %v", err)
	}
	if result != 5 {
		t.Fatalf("fib(5) = %d, want 5 (pc=%04x sp=%04x d0=%08x)", result, cpu.regs.PC, cpu.regs.A[7], uint32(cpu.regs.D[0]))
	}
}
