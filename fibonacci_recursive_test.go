package m68kemu

import "testing"

func TestRecursiveFibonacciProgram(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC

	program := assemble(t, `
        BRA main
fib:    MOVE.L D0,D1
        SUBQ.L #1,D1
        BLE.S base
        MOVE.L D0,-(A7)
        SUBQ.L #1,D0
        BSR fib
        MOVE.L D0,-(A7)
        MOVE.L 4(A7),D0
        SUBQ.L #2,D0
        BSR fib
        MOVE.L (A7)+,D2
        MOVE.L (A7)+,D1
        ADD.L D2,D0
        RTS
base:   RTS
main:   LEA $4000,A0
        MOVEQ #7,D0
        BSR fib
        MOVE.L D0,(A0)
        NOP
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
	if result != 13 {
		t.Fatalf("fib(7) = %d, want 13", result)
	}
}
