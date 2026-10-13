package fpu_test

import (
	"errors"
	"math"
	"testing"

	"github.com/jenska/m68kemu"
	"github.com/jenska/m68kemu/fpu"

	asm "github.com/jenska/m68kasm"
)

// runFLine assembles source for an MC68020 with an FPU, runs it on an
// MC68020 with f as coprocessor 1 until the label "done", and returns the
// CPU and its RAM.
func runFLine(t *testing.T, f *fpu.FPU, source string, setup func(*m68kemu.RAM)) (*m68kemu.CPU, *m68kemu.RAM) {
	t.Helper()
	code, listing, err := asm.AssembleStringWithListingWithOptions(source,
		asm.ParseOptions{Target: asm.Target{CPU: asm.CPU68020, Features: asm.FeatFPU | asm.FeatFPUFull}})
	if err != nil {
		t.Fatalf("assembler: %v", err)
	}
	ram := m68kemu.NewRAM(0, 0x10000)
	ram.Write(m68kemu.Long, 0, 0x1000)
	ram.Write(m68kemu.Long, 4, 0x2000)
	for v := uint32(48); v <= 54; v++ {
		ram.Write(m68kemu.Long, v<<2, 0x6000+v)
	}
	for i, b := range code {
		ram.Write(m68kemu.Byte, 0x2000+uint32(i), uint32(b))
	}
	if setup != nil {
		setup(ram)
	}
	cpu, err := m68kemu.NewCPU(m68kemu.NewBus(ram), m68kemu.WithModel(m68kemu.M68020), m68kemu.WithCoprocessor(1, f.Coprocessor()))
	if err != nil {
		t.Fatal(err)
	}
	done := uint32(0)
	for _, e := range listing {
		if e.Line == countLines(source, "done:") {
			done = 0x2000 + e.PC
		}
	}
	stops := []uint32{done}
	for v := uint32(48); v <= 54; v++ {
		stops = append(stops, 0x6000+v) // exception handlers
	}
	if _, err := cpu.RunUntil(m68kemu.RunUntilOptions{MaxInstructions: 1000, StopAtPC: stops}); err != nil {
		t.Fatal(err)
	}
	return cpu, ram
}

func TestFLineArithmeticAndConditions(t *testing.T) {
	f := fpu.New(fpu.MC68882)
	cpu, ram := runFLine(t, f, `
	LEA      $3000,A0
	FMOVE.D  #1.5,FP0
	FADD.D   #2.25,FP0
	FMOVE.D  FP0,(A0)
	FMOVE.L  FP0,D1
	FCMP.D   #3.75,FP0
	FBEQ     equal
	MOVEQ    #-1,D2
equal:	FSEQ     D3
	FMOVE.L  FPSR,D4
	FMOVECR  #$32,FP1
	MOVEQ    #2,D5
loop:	FDBNE    D5,loop           ; equal: no loop
done:	BRA.S    done
`, nil)
	regs := cpu.Registers()
	hi, _ := ram.Read(m68kemu.Long, 0x3000)
	lo, _ := ram.Read(m68kemu.Long, 0x3004)
	if v := math.Float64frombits(uint64(hi)<<32 | uint64(lo)); v != 3.75 {
		t.Fatalf("FMOVE.D FP0,(A0) stored %v, want 3.75", v)
	}
	if regs.D[1] != 4 || regs.D[2] != 0 || regs.D[3]&0xff != 0xff || regs.D[5] != 2 {
		t.Fatalf("D1=%d D2=%d D3=%x D5=%d; want 4 (rounded), 0 (branch taken), ff (FSEQ), 2 (FDBNE fell through)", regs.D[1], regs.D[2], regs.D[3]&0xff, regs.D[5])
	}
	if regs.D[4]&fpu.CCZ == 0 {
		t.Fatalf("FPSR = %08x, want Z from FCMP", regs.D[4])
	}
	if f.FP[1].ToFloat64() != 1 {
		t.Fatalf("FMOVECR #$32 = %v, want 1", f.FP[1])
	}
	if f.FPIAR == 0 {
		t.Fatalf("FPIAR not set from the instruction address")
	}
}

func TestFLineFmovemAndSaveRestore(t *testing.T) {
	f := fpu.New(fpu.MC68881)
	cpu, ram := runFLine(t, f, `
	FMOVE.L  #10,FP0
	FMOVE.L  #20,FP1
	FMOVEM.X FP0-FP1,-(A7)
	MOVE.L   A7,A2
	MOVE.L   (A7),D6           ; first long of the lowest register
	FMOVEM.X (A7)+,FP2-FP3
	FSAVE    -(A7)
	MOVE.L   A7,A3
	FRESTORE (A7)+
done:	BRA.S    done
`, nil)
	regs := cpu.Registers()
	if regs.A[2] != 0x1000-24 || regs.A[7] != 0x1000 {
		t.Fatalf("FMOVEM moved A7 to %x and back to %x, want %x and 1000", regs.A[2], regs.A[7], 0x1000-24)
	}
	if regs.D[6] != 0x40020000 { // 10 = $4002 0000 A0000000 00000000
		t.Fatalf("lowest register in memory starts %08x, want FP0 (10 = 40020000)", regs.D[6])
	}
	if f.FP[2].ToInt64() != 10 || f.FP[3].ToInt64() != 20 {
		t.Fatalf("FP2=%v FP3=%v, want 10 and 20", f.FP[2], f.FP[3])
	}
	if hdr, _ := ram.Read(m68kemu.Long, regs.A[3]); hdr != 0x1f180000 {
		t.Fatalf("FSAVE frame header %08x, want the MC68881 idle frame 1f180000", hdr)
	}
}

func TestFLinePendingExceptionAndTrap(t *testing.T) {
	f := fpu.New(fpu.MC68881)
	cpu, _ := runFLine(t, f, `
	FMOVE.L  #$400,FPCR          ; enable DZ
	FMOVE.L  #1,FP0
	FDIV.L   #0,FP0
fnop:	FNOP                         ; takes the pending DZ exception
done:	BRA.S    done
`, nil)
	regs := cpu.Registers()
	if regs.PC != 0x6000+fpu.VectorDZ {
		t.Fatalf("PC = %x, want the DZ handler %x", regs.PC, 0x6000+fpu.VectorDZ)
	}
	frame, _, err := cpu.CurrentExceptionFrame()
	if err != nil || frame.VectorOffset != fpu.VectorDZ<<2 {
		t.Fatalf("frame %+v (%v), want vector %d", frame, err, fpu.VectorDZ)
	}
	if _, ok := f.PendingException(); ok {
		t.Fatalf("exception still pending after the CPU took it")
	}
}

func TestFLineWithoutCoprocessor(t *testing.T) {
	ram := m68kemu.NewRAM(0, 0x10000)
	ram.Write(m68kemu.Long, 0, 0x1000)
	ram.Write(m68kemu.Long, 4, 0x2000)
	ram.Write(m68kemu.Long, 11<<2, 0x5000)
	ram.Write(m68kemu.Word, 0x2000, 0xf280) // FNOP
	ram.Write(m68kemu.Word, 0x2002, 0x0000)
	cpu, err := m68kemu.NewCPU(m68kemu.NewBus(ram), m68kemu.WithModel(m68kemu.M68020))
	if err != nil {
		t.Fatal(err)
	}
	if err := cpu.Step(); err != nil {
		t.Fatal(err)
	}
	if cpu.Registers().PC != 0x5000 {
		t.Fatalf("FNOP without an FPU: PC = %x, want the line-F handler", cpu.Registers().PC)
	}

	_, err = m68kemu.NewCPU(m68kemu.NewBus(m68kemu.NewRAM(0, 0x100)), m68kemu.WithDeferredReset(),
		m68kemu.WithCoprocessor(1, fpu.New(fpu.MC68881).Coprocessor()))
	if !errors.Is(err, m68kemu.ErrCoprocessorUnsupported) {
		t.Fatalf("coprocessor on an MC68000: error %v, want ErrCoprocessorUnsupported", err)
	}
}

func TestFDBccLoops(t *testing.T) {
	// FDBF never ends on its condition: it counts D0 down to -1, so the
	// loop body runs four times.
	cpu, _ := runFLine(t, fpu.New(fpu.MC68882), `
	MOVEQ    #3,D0
	MOVEQ    #0,D1
loop:	ADDQ.L   #1,D1
	FDBF     D0,loop
done:	NOP
`, nil)
	if regs := cpu.Registers(); regs.D[1] != 4 || regs.D[0]&0xffff != 0xffff {
		t.Fatalf("D1 = %d, D0 = %x; want 4 passes and ffff", regs.D[1], regs.D[0]&0xffff)
	}
}
