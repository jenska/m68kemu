package mmu_test

import (
	"strings"
	"testing"

	"github.com/jenska/m68kemu"

	asm "github.com/jenska/m68kasm"
)

// BenchmarkTranslation runs a loop that sums 64 KB of memory, once with
// physical addresses and once through the tables of the tests, which map it
// one-to-one in 4 KB pages.
func BenchmarkTranslation(b *testing.B) {
	for _, tt := range []struct {
		name  string
		setup string
	}{
		{"off", ""},
		{"on", "\tPMOVE\tcrp,CRP\n\tPMOVE\ttc,TC\n"},
	} {
		b.Run(tt.name, func(b *testing.B) {
			result, err := asm.AssembleDetailedWithOptions(strings.NewReader("\tORG\t$2000\n"+tt.setup+`
loop:	LEA	$10000,A0
	MOVE.W	#$3FFF,D1
sum:	ADD.L	(A0)+,D0
	DBRA	D1,sum
	BRA	loop
	ALIGN	4
crp:	DC.L	$7FFF0002,$20000
tc:	DC.L	$80C86600
`), asm.ParseOptions{Target: asm.Target{CPU: asm.CPU68030, Features: asm.FeatPMMU}})
			if err != nil {
				b.Fatal(err)
			}
			ram := m68kemu.NewRAM(0, 0x40000)
			ram.Write(m68kemu.Long, 0, 0x8000)
			ram.Write(m68kemu.Long, 4, 0x2000)
			for i, v := range result.Bytes {
				ram.Write(m68kemu.Byte, result.Origin+uint32(i), uint32(v))
			}
			setupTables(ram)
			for _, page := range []uint32{remapped, missing, protected} {
				ram.Write(m68kemu.Long, pageDescriptor(page>>12), page|1)
			}
			cpu, err := m68kemu.NewCPU(m68kemu.NewBus(ram), m68kemu.WithModel(m68kemu.M68030))
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for range b.N {
				if err := cpu.RunCycles(1_000_000); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
