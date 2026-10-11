package m68kemu

import "testing"

func BenchmarkRecursiveFibonacci(b *testing.B) {
	const cycleBudget = 8_000_000

	program := assemble(b, `
BRA main
fib:    MOVE.L D0,D1
        SUBQ.L #1,D1
        BLE.S return
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
return: RTS
main:   LEA $4000,A0
        MOVEQ #7,D0	
        BSR fib
		MOVE.L D0,(A0)
		BRA main
`)

	cpu, ram := newEnvironment(b)
	startPC := cpu.regs.PC
	for i, value := range program {
		addr := startPC + uint32(i)
		if err := ram.Write(Byte, addr, uint32(value)); err != nil {
			b.Fatalf("write program: %v", err)
		}
	}

	b.ResetTimer()
	for b.Loop() {
		if err := cpu.Reset(); err != nil {
			b.Fatalf("cpu reset: %v", err)
		}

		if err := cpu.RunCycles(cycleBudget); err != nil {
			b.Fatalf("RunCycles failed: %v", err)
		}
	}
}
