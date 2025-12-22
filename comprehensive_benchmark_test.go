package m68kemu

import (
	"math/rand"
	"testing"
)

func BenchmarkBubbleSort(b *testing.B) {
	// Sort an array of 100 words (int16) located at 0x4000.
	const arraySize = 100
	const arrayBase = 0x4000
	const cycleBudget = 2_000_000

	// Bubble Sort Assembly
	// D0: Outer loop counter (n-2 down to 0)
	// D1: Inner loop counter (copy of D0)
	// A0: Array base pointer
	// A1: Current element pointer
	// D2: Current element value
	// D3: Next element value
	program := assemble(b, `
        MOVE.W  #98,D0          ; Outer loop counter (size-2)
outer:  MOVE.W  D0,D1           ; Inner loop counter copy
        LEA     $4000,A0        ; Reset pointer to start of array
        MOVEA.L A0,A1           ; A1 = current pointer
inner:  MOVE.W  (A1),D2         ; Load current
        CMP.W   2(A1),D2        ; Compare with next
        BLE.S   next            ; If current <= next, skip swap
        MOVE.W  2(A1),D3        ; Load next
        MOVE.W  D3,(A1)         ; Store next at current
        MOVE.W  D2,2(A1)        ; Store current at next
next:   ADDQ.L  #2,A1           ; Advance pointer
        DBRA    D1,inner        ; Inner loop
        DBRA    D0,outer        ; Outer loop
done:   BRA.S   done            ; Spin until budget exhausted
`)

	cpu, ram := newEnvironment(b)
	startPC := cpu.regs.PC

	// Load program
	for i, val := range program {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(val)); err != nil {
			b.Fatalf("failed to load program: %v", err)
		}
	}

	// Prepare random data
	data := make([]uint16, arraySize)
	rng := rand.New(rand.NewSource(42))
	for i := range data {
		data[i] = uint16(rng.Intn(65536))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset CPU state
		if err := cpu.Reset(); err != nil {
			b.Fatalf("cpu reset failed: %v", err)
		}
		cpu.regs.PC = startPC

		// Reload unsorted data into RAM
		for j, val := range data {
			if err := ram.Write(Word, arrayBase+uint32(j*2), uint32(val)); err != nil {
				b.Fatalf("failed to write data: %v", err)
			}
		}

		// Run
		if err := cpu.RunCycles(cycleBudget); err != nil {
			b.Fatalf("RunCycles failed: %v", err)
		}
	}
}

func BenchmarkPrimeSieve(b *testing.B) {
	// Sieve of Eratosthenes up to 1000
	// Array at 0x4000. 0 = prime, 1 = composite.
	const limit = 1000
	const arrayBase = 0x4000
	const cycleBudget = 2_000_000

	program := assemble(b, `
        LEA     $4000,A0        ; Array base
        MOVE.W  #999,D0         ; Clear loop counter (1000 bytes)
clear:  CLR.B   (A0)+           ; Clear byte
        DBRA    D0,clear        
        
        LEA     $4000,A0        ; Reset A0 to array base
        MOVEQ   #2,D0           ; Start with prime candidate 2
outer:  CMP.W   #1000,D0        ; Check limit
        BGE.S   done
        
        TST.B   0(A0,D0.W)      ; Is marked?
        BNE.S   next_iter       ; If marked (!=0), skip
        
        MOVE.W  D0,D1           ; D1 = D0
        ADD.W   D0,D1           ; D1 = 2*D0 (first multiple)
inner:  CMP.W   #1000,D1        ; Check limit for multiple
        BGE.S   next_iter
        MOVE.B  #1,0(A0,D1.W)   ; Mark as composite
        ADD.W   D0,D1           ; Next multiple
        BRA.S   inner
        
next_iter:
        ADDQ.W  #1,D0           ; Next candidate
        BRA.S   outer
done:   BRA.S   done            ; Spin
`)

	cpu, ram := newEnvironment(b)
	startPC := cpu.regs.PC

	for i, val := range program {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(val)); err != nil {
			b.Fatalf("failed to load program: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cpu.Reset(); err != nil {
			b.Fatalf("cpu reset failed: %v", err)
		}
		cpu.regs.PC = startPC

		if err := cpu.RunCycles(cycleBudget); err != nil {
			b.Fatalf("RunCycles failed: %v", err)
		}
	}
}
