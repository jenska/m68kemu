package m68kemu

import (
	"math/rand"
	"slices"
	"sync"
	"testing"
)

// concurrencyProgram sorts 64 words at $4000 with a bubble sort, then copies
// them to $5000 with post-increment and pre-decrement addressing, so it
// exercises most effective-address modes.
const concurrencyProgram = `
        MOVE.W  #62,D0
outer:  MOVE.W  D0,D1
        LEA     $4000,A1
inner:  MOVE.W  (A1),D2
        CMP.W   2(A1),D2
        BLE.S   next
        MOVE.W  2(A1),D3
        MOVE.W  D3,(A1)
        MOVE.W  D2,2(A1)
next:   ADDQ.L  #2,A1
        DBRA    D1,inner
        DBRA    D0,outer
        LEA     $4000,A0
        LEA     $5080,A2
        MOVE.W  #63,D0
copy:   MOVE.W  (A0)+,D4
        MOVE.W  D4,-(A2)
        DBRA    D0,copy
done:   BRA.S   done
`

// runConcurrencyProgram sorts data on a fresh CPU of model m and returns the
// words it left at $5000 and the CPU's registers and cycle count. With hooks,
// the CPU also runs with an instruction tracer (which disassembles every
// instruction), a bus tracer, a history and a scheduler.
func runConcurrencyProgram(t *testing.T, m Model, hooks bool, code []byte, data []uint16) ([]uint16, Registers, uint64) {
	ram := NewRAM(0, 0x10000)
	ram.Write(Long, 0, 0x1000)
	ram.Write(Long, 4, 0x2000)
	for i, b := range code {
		ram.Write(Byte, 0x2000+uint32(i), uint32(b))
	}
	for i, w := range data {
		ram.Write(Word, 0x4000+2*uint32(i), uint32(w))
	}
	c, err := NewCPU(NewBus(ram), WithModel(m))
	if err != nil {
		t.Error(err)
		return nil, Registers{}, 0
	}
	if hooks {
		var mnemonics, accesses int
		c.SetTracer(func(info TraceInfo) {
			if info.Mnemonic != "" {
				mnemonics++
			}
		})
		c.SetBusTracer(func(BusAccessInfo) { accesses++ })
		c.SetHistoryLimit(32)
		scheduler := NewCycleScheduler()
		var tick func(uint64)
		tick = func(now uint64) { scheduler.ScheduleAfter(1000, tick) }
		scheduler.ScheduleAfter(1000, tick)
		c.SetScheduler(scheduler)
		defer func() {
			if mnemonics == 0 || accesses == 0 {
				t.Errorf("hooks did not run: %d mnemonics, %d bus accesses", mnemonics, accesses)
			}
		}()
	}
	if err := c.RunCycles(400_000); err != nil {
		t.Error(err)
	}
	out := make([]uint16, len(data))
	for i := range out {
		w, _ := ram.Read(Word, 0x5000+2*uint32(i))
		out[i] = uint16(w)
	}
	return out, c.Registers(), c.Cycles()
}

// TestCPUsRunConcurrently runs CPUs of both models, with and without debug
// hooks, in parallel goroutines.
// Under -race it fails if they share mutable state; without it, the results
// must match those of the same programs run one after another.
func TestCPUsRunConcurrently(t *testing.T) {
	code := assemble(t, concurrencyProgram)
	const n = 8
	rng := rand.New(rand.NewSource(1))
	inputs := make([][]uint16, n)
	for i := range inputs {
		inputs[i] = make([]uint16, 64)
		for j := range inputs[i] {
			inputs[i][j] = uint16(rng.Intn(0x10000))
		}
	}
	model := func(i int) Model { return []Model{M68000, M68010}[i%2] }
	hooks := func(i int) bool { return i%4 >= 2 }

	type result struct {
		words  []uint16
		regs   Registers
		cycles uint64
	}
	want := make([]result, n)
	for i := range n {
		w, r, c := runConcurrencyProgram(t, model(i), hooks(i), code, inputs[i])
		want[i] = result{w, r, c}
	}

	got := make([]result, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			w, r, c := runConcurrencyProgram(t, model(i), hooks(i), code, inputs[i])
			got[i] = result{w, r, c}
		})
	}
	wg.Wait()

	for i := range n {
		sorted := slices.Clone(inputs[i])
		slices.SortFunc(sorted, func(a, b uint16) int { return int(int16(b)) - int(int16(a)) })
		if !slices.Equal(want[i].words, sorted) {
			t.Fatalf("CPU %d (sequential): copied words are not the sorted input in reverse", i)
		}
		if !slices.Equal(got[i].words, want[i].words) || got[i].regs != want[i].regs || got[i].cycles != want[i].cycles {
			t.Fatalf("CPU %d (%v) differs when run concurrently:\n got  %v cycles %d\n want %v cycles %d",
				i, model(i), got[i].regs, got[i].cycles, want[i].regs, want[i].cycles)
		}
	}
}
