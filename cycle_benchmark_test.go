package m68kemu

import "testing"

func BenchmarkRunEightMillionCycles(b *testing.B) {
	const cycleBudget = 8_000_000

	code := assemble(b, "loop: NOP\nBRA loop")

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cpu, ram := newEnvironment(b)

		for offset, value := range code {
			addr := cpu.regs.PC + uint32(offset)
			if err := ram.Write(Byte, addr, uint32(value)); err != nil {
				b.Fatalf("failed to seed program byte at %04x: %v", addr, err)
			}
		}

		b.ResetTimer()
		if err := cpu.RunCycles(cycleBudget); err != nil {
			b.Fatalf("RunCycles failed: %v", err)
		}
		b.StopTimer()
	}
}
