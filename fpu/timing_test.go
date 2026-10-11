package fpu_test

import (
	"strings"
	"testing"

	"github.com/jenska/m68kemu"
	"github.com/jenska/m68kemu/fpu"
)

func TestCycles(t *testing.T) {
	f := fpu.New(fpu.MC68881)
	for _, tt := range []struct {
		name    string
		cmd     uint16
		dynamic uint32
		want    int
	}{
		{"FADD.X FP1,FP0", 0x0422, 0, 51},
		{"FSQRT.X FP0,FP0", 0x0004, 0, 107},
		{"FSINCOS.X FP0,FP1:FP2", 0x0131, 0, 451},
		{"FADD.D <ea>,FP0", 0x5422, 0, 51 + 14},
		{"FMOVE.L <ea>,FP0", 0x4000, 0, 33 + 18},
		{"FMOVE.D FP0,<ea>", 0x7400, 0, 77},
		{"FMOVECR #$32,FP0", 0x5c32, 0, 29},
		{"FMOVE.L <ea>,FPCR/FPSR", 0x9800, 0, 60},
		{"FMOVEM.X <ea>,FP0-FP2", 0xd0e0, 0, 33 + 3*25},
		{"FMOVEM.X <ea>,Dn list", 0xd810, 0x0f, 33 + 4*25},
	} {
		op, err := fpu.Decode(tt.cmd)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if got := f.Cycles(op, tt.dynamic); got != tt.want {
			t.Errorf("%s: %d cycles, want %d", tt.name, got, tt.want)
		}
	}
}

// TestCoprocessorConcurrency checks that integer instructions run while the
// FPU computes, and that the next FPU instruction waits for it.
func TestCoprocessorConcurrency(t *testing.T) {
	run := func(nops int) uint64 {
		cpu, _ := runFLine(t, fpu.New(fpu.MC68882), "\tFSQRT.X  FP0,FP1\n"+
			strings.Repeat("\tNOP\n", nops)+
			"\tFMOVE.X  FP1,FP2\ndone:\tBRA.S done\n", nil)
		return cpu.Cycles()
	}
	// FSQRT and FMOVE cost 4 CPU cycles each; FSQRT keeps the FPU busy for
	// 107 cycles, so FMOVE starts 111 cycles in.
	if got := run(0); got != 111 {
		t.Fatalf("FSQRT, FMOVE: %d cycles, want 111", got)
	}
	if got := run(10); got != 111 {
		t.Fatalf("FSQRT, 10 NOPs, FMOVE: %d cycles, want 111: the NOPs run while the FPU computes", got)
	}
	if got := run(40); got != 4+40*4+4 {
		t.Fatalf("FSQRT, 40 NOPs, FMOVE: %d cycles, want %d: no wait", got, 4+40*4+4)
	}
}

// TestCoprocessorStoreWaits checks that a command returning an operand keeps
// the CPU waiting for it.
func TestCoprocessorStoreWaits(t *testing.T) {
	cpu, _ := runFLine(t, fpu.New(fpu.MC68881), `
	LEA      $3000,A0
	FMOVE.D  FP0,(A0)
done:	BRA.S    done
`, nil)
	// LEA abs.L 12, FMOVE 4, then the CPU waits 77 cycles for the result.
	if got := cpu.Cycles(); got != 12+4+77 {
		t.Fatalf("LEA, FMOVE.D FP0,(A0): %d cycles, want %d", got, 12+4+77)
	}
}

func TestCIRBusy(t *testing.T) {
	var clock uint64
	c := fpu.NewCIR(fpu.New(fpu.MC68881), cirBase)
	c.Clock = func() uint64 { return clock }
	response := func() uint32 {
		v, _ := c.Read(m68kemu.Word, cirBase)
		return v
	}

	c.Write(m68kemu.Word, cirBase+0x0a, 0x0004) // FSQRT.X FP0,FP0
	c.Write(m68kemu.Word, cirBase+0x0a, 0x0422) // FADD.X FP1,FP0 queues behind it
	for _, tt := range []struct {
		clock uint64
		busy  bool
	}{{0, true}, {106, true}, {107, true}, {157, true}, {158, false}} {
		clock = tt.clock
		if got := response() == 0x8900; got != tt.busy {
			t.Fatalf("at clock %d: response %04x, busy %v; want busy %v", tt.clock, response(), got, tt.busy)
		}
	}

	clock = 200
	c.Write(m68kemu.Word, cirBase+0x0a, 0x0004)
	clock = 10 // a CPU reset restarts the clock
	if r := response(); r == 0x8900 {
		t.Fatalf("after the clock went backwards: response %04x, want idle", r)
	}
}
