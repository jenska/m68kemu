package m68kemu

import "testing"

func prepareEnvironment(b *testing.B) CPU {
	b.Helper()
	cpu, ram := newEnvironment(b)
	code := assemble(b, "loop: ADDQ.L #1, D0\nMOVE.L D0, D1\nBRA.S loop")
	for offset, value := range code {
		addr := cpu.regs.PC + uint32(offset)
		if err := ram.Write(Byte, addr, uint32(value)); err != nil {
			b.Fatalf("failed to seed program byte at %04x: %v", addr, err)
		}
	}
	return cpu
}

func BenchmarkRunEightMillionCycles(b *testing.B) {
	const cycleBudget = 8_000_000
	cpu := prepareEnvironment(b)
	for i := 0; i < b.N; i++ {
		if err := cpu.RunCycles(cycleBudget); err != nil {
			b.Fatalf("RunCycles failed: %v", err)
		}
	}
}
