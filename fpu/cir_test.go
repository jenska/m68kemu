package fpu_test

import (
	"math"
	"testing"

	"github.com/jenska/m68kemu"
	"github.com/jenska/m68kemu/fpu"

	asm "github.com/jenska/m68kasm"
)

// The programs below drive the FPU the way the GNU C library for the Atari
// SFP004 does: A0 points to the operand CIR at $FFFA50, so the command CIR is
// at -6(A0) and the response CIR at -16(A0). After a command that transfers
// an operand, the code waits while the response is $8900; after a
// register-to-register command it waits while bit 15 is set. Like the
// library, the tests encode that loop as words: m68kasm v1.6.1 lays out TST
// with a displacement as two bytes, so a label after it would be wrong.

const cirBase = 0xfffa40

// runProgram assembles source at $2000 and runs it on a 68000 with the
// memory-mapped FPU until it reaches the label "done". The program reads its
// operands from $3000 and leaves results in the data registers.
func runProgram(t *testing.T, f *fpu.FPU, source string, data map[uint32]uint32) m68kemu.Registers {
	t.Helper()
	code, listing, err := asm.AssembleStringWithListing(source)
	if err != nil {
		t.Fatalf("assembler: %v", err)
	}
	ram := m68kemu.NewRAM(0, 0x10000)
	ram.Write(m68kemu.Long, 0, 0x1000)
	ram.Write(m68kemu.Long, 4, 0x2000)
	for i, b := range code {
		ram.Write(m68kemu.Byte, 0x2000+uint32(i), uint32(b))
	}
	for addr, v := range data {
		ram.Write(m68kemu.Long, addr, v)
	}
	cpu, err := m68kemu.NewCPU(m68kemu.NewBus(ram, fpu.NewCIR(f, cirBase)))
	if err != nil {
		t.Fatal(err)
	}
	done := uint32(0)
	for _, entry := range listing {
		if entry.Line == countLines(source, "done:") {
			done = 0x2000 + entry.PC
		}
	}
	if done == 0 {
		t.Fatalf("program has no done: label")
	}
	result, err := cpu.RunUntil(m68kemu.RunUntilOptions{MaxInstructions: 10_000, StopAtPC: []uint32{done}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != m68kemu.RunStopPC {
		t.Fatalf("program stopped with %v at PC %x, want done", result.Reason, cpu.Registers().PC)
	}
	return cpu.Registers()
}

// countLines returns the 1-based line of the first line containing label.
func countLines(source, label string) int {
	line := 1
	for i := 0; i < len(source); i++ {
		if source[i] == '\n' {
			line++
			continue
		}
		if i+len(label) <= len(source) && source[i:i+len(label)] == label && (i == 0 || source[i-1] == '\n') {
			return line
		}
	}
	return -1
}

func doubleWords(v float64) (uint32, uint32) {
	b := math.Float64bits(v)
	return uint32(b >> 32), uint32(b)
}

func TestCIRDoubleArithmetic(t *testing.T) {
	ahi, alo := doubleWords(1.5)
	bhi, blo := doubleWords(2.25)
	regs := runProgram(t, fpu.New(fpu.MC68881), `
	LEA     $FFFA50,A0
	MOVE.W  #$5400,-6(A0)     ; FMOVE.D <ea>,FP0
	CMPI.W  #$8900,-16(A0)
	MOVE.L  $3000,(A0)
	MOVE.L  $3004,(A0)
	MOVE.W  #$5422,-6(A0)     ; FADD.D <ea>,FP0
w1:     CMPI.W  #$8900,-16(A0)
	BEQ.S   w1
	MOVE.L  $3008,(A0)
	MOVE.L  $300C,(A0)
	MOVE.W  #$7400,-6(A0)     ; FMOVE.D FP0,<ea>
w2:     CMPI.W  #$8900,-16(A0)
	BEQ.S   w2
	MOVE.L  (A0),D0
	MOVE.L  (A0),D1
	MOVE.W  #$009E,-6(A0)     ; FGETEXP FP0,FP1
	DC.W    $4A68,$FFF0,$6BFA ; w3: TST.W -16(A0); BMI.S w3
	MOVE.W  #$7080,-6(A0)     ; FMOVE.W FP1,<ea>
w4:     CMPI.W  #$8900,-16(A0)
	BEQ.S   w4
	MOVE.W  (A0),D2
	MOVE.W  -16(A0),D3        ; response once idle
done:   BRA.S   done
`, map[uint32]uint32{0x3000: ahi, 0x3004: alo, 0x3008: bhi, 0x300c: blo})

	sum := math.Float64frombits(uint64(uint32(regs.D[0]))<<32 | uint64(uint32(regs.D[1])))
	if sum != 3.75 {
		t.Fatalf("1.5 + 2.25 = %v, want 3.75", sum)
	}
	if regs.D[2]&0xffff != 1 {
		t.Fatalf("FGETEXP(3.75) = %d, want 1", regs.D[2]&0xffff)
	}
	if regs.D[3]&0x8000 != 0 || regs.D[3]&0xffff == 0x8900 {
		t.Fatalf("idle response %04x has bit 15 set", regs.D[3]&0xffff)
	}
}

func TestCIRSingleAndInteger(t *testing.T) {
	regs := runProgram(t, fpu.New(fpu.MC68881), `
	LEA     $FFFA50,A0
	MOVE.W  #$4400,-6(A0)     ; FMOVE.S <ea>,FP0
	CMPI.W  #$8900,-16(A0)
	MOVE.L  $3000,(A0)
	MOVE.W  #$4404,-6(A0)     ; FSQRT.S <ea>,FP0
w1:     CMPI.W  #$8900,-16(A0)
	BEQ.S   w1
	MOVE.L  $3004,(A0)
	MOVE.W  #$6400,-6(A0)     ; FMOVE.S FP0,<ea>
w2:     CMPI.W  #$8900,-16(A0)
	BEQ.S   w2
	MOVE.L  (A0),D0
	MOVE.W  #$5403,-6(A0)     ; FINTRZ.D <ea>,FP0
	CMPI.W  #$8900,-16(A0)
	MOVE.L  $3008,(A0)
	MOVE.L  $300C,(A0)
	MOVE.W  #$6000,-6(A0)     ; FMOVE.L FP0,<ea>
w3:     CMPI.W  #$8900,-16(A0)
	BEQ.S   w3
	MOVE.L  (A0),D1
done:   BRA.S   done
`, func() map[uint32]uint32 {
		hi, lo := doubleWords(-7.9)
		return map[uint32]uint32{0x3000: math.Float32bits(2), 0x3004: math.Float32bits(16), 0x3008: hi, 0x300c: lo}
	}())

	if got := math.Float32frombits(uint32(regs.D[0])); got != 4 {
		t.Fatalf("sqrt(16) = %v, want 4", got)
	}
	if regs.D[1] != -7 {
		t.Fatalf("FINTRZ(-7.9) as long = %d, want -7", regs.D[1])
	}
}

func TestCIRControlRegistersAndExceptions(t *testing.T) {
	f := fpu.New(fpu.MC68881)
	regs := runProgram(t, f, `
	LEA     $FFFA50,A0
	MOVE.W  #$9000,-6(A0)     ; FMOVE.L <ea>,FPCR
	CMPI.W  #$8900,-16(A0)
	MOVE.L  #$0400,(A0)       ; enable DZ
	MOVE.W  #$4000,-6(A0)     ; FMOVE.L <ea>,FP0
	CMPI.W  #$8900,-16(A0)
	MOVE.L  #1,(A0)
	MOVE.W  #$4020,-6(A0)     ; FDIV.L <ea>,FP0
	CMPI.W  #$8900,-16(A0)
	MOVE.L  #0,(A0)           ; 1/0
	MOVE.W  #$A800,-6(A0)     ; FMOVE.L FPSR,<ea>: refused, DZ is pending
	MOVE.W  -16(A0),D0
	MOVE.W  #2,-14(A0)        ; control CIR: acknowledge the exception
	MOVE.W  #$A800,-6(A0)     ; FMOVE.L FPSR,<ea>
w1:     CMPI.W  #$8900,-16(A0)
	BEQ.S   w1
	MOVE.L  (A0),D1
	MOVE.W  #$0001,-2(A0)     ; condition CIR: EQ
	MOVE.W  -16(A0),D2
	MOVE.W  #$000E,-2(A0)     ; condition CIR: NE
	MOVE.W  -16(A0),D3
done:   BRA.S   done
`, nil)

	if got := regs.D[0] & 0xffff; got != 0x1c00|fpu.VectorDZ {
		t.Fatalf("response to a command with DZ pending = %04x, want take pre-instruction exception %04x", got, 0x1c00|fpu.VectorDZ)
	}
	if fpsr := uint32(regs.D[1]); fpsr&fpu.CCI == 0 || fpsr>>8&0xff != uint32(fpu.DZ) || fpsr&0x10 == 0 {
		t.Fatalf("FPSR = %08x, want I, the DZ exception and accrued DZ", fpsr)
	}
	if regs.D[2]&1 != 0 || regs.D[3]&1 != 1 {
		t.Fatalf("condition responses EQ %04x, NE %04x; want false, true for +Inf", regs.D[2]&0xffff, regs.D[3]&0xffff)
	}
}

func TestCIRFmovemAndSave(t *testing.T) {
	f := fpu.New(fpu.MC68882)
	regs := runProgram(t, f, `
	LEA     $FFFA50,A0
	MOVE.W  -12(A0),D0        ; save CIR in the reset state: null frame
	MOVE.W  #$4080,-6(A0)     ; FMOVE.L <ea>,FP1
	CMPI.W  #$8900,-16(A0)
	MOVE.L  #42,(A0)
	MOVE.W  #$F020,-6(A0)     ; FMOVEM FP2 (list bit 5 = FP2),<ea>
w1:     CMPI.W  #$8900,-16(A0)
	BEQ.S   w1
	MOVE.W  #$F040,-6(A0)     ; FMOVEM FP1,<ea>
w2:     CMPI.W  #$8900,-16(A0)
	BEQ.S   w2
	MOVE.W  4(A0),D1          ; register select CIR
	MOVE.L  (A0),D2
	MOVE.L  (A0),D3
	MOVE.L  (A0),D4
	MOVE.W  -12(A0),D5        ; save CIR: idle frame
	MOVEQ   #13,D6            ; 0x38 bytes = 14 longs
w3:     MOVE.L  (A0),D7
	DBRA    D6,w3
	MOVE.W  -16(A0),D7
done:   BRA.S   done
`, nil)

	if regs.D[0]&0xffff != 0 {
		t.Fatalf("save CIR after reset = %04x, want a null frame", regs.D[0]&0xffff)
	}
	if regs.D[1]&0xffff != 0x4000 {
		t.Fatalf("register select = %04x, want 4000 (FP1)", regs.D[1]&0xffff)
	}
	// 42 = $4004 0000 A8000000 00000000 in the 96-bit extended format.
	if uint32(regs.D[2]) != 0x40040000 || uint32(regs.D[3]) != 0xa8000000 || regs.D[4] != 0 {
		t.Fatalf("FMOVEM FP1 = %08x %08x %08x, want 40040000 a8000000 00000000", uint32(regs.D[2]), uint32(regs.D[3]), uint32(regs.D[4]))
	}
	if regs.D[5]&0xffff != 0x1f38 {
		t.Fatalf("save CIR = %04x, want the MC68882 idle frame 1f38", regs.D[5]&0xffff)
	}
	if regs.D[7]&0xffff != 0x0802 {
		t.Fatalf("response after reading the whole frame = %04x, want idle", regs.D[7]&0xffff)
	}
}

func TestCIRUnknownCommandAndReset(t *testing.T) {
	f := fpu.New(fpu.MC68881)
	c := fpu.NewCIR(f, cirBase)
	c.Write(m68kemu.Word, cirBase+0x0a, 0x0005) // undefined opmode
	if r, _ := c.Read(m68kemu.Word, cirBase); r != 0x1c00|11 {
		t.Fatalf("response to an undefined command = %04x, want the F-line vector %04x", r, 0x1c00|11)
	}
	f.FPCR = 0x30
	c.Reset()
	if f.FPCR != 0 {
		t.Fatalf("bus reset should reset the FPU")
	}
	if r, _ := c.Read(m68kemu.Word, cirBase); r != 0x0802 {
		t.Fatalf("response after reset = %04x, want idle", r)
	}
}

// cirIO drives a CIR directly, without a CPU.
type cirIO struct {
	t *testing.T
	c *fpu.CIR
}

func (io cirIO) write(s m68kemu.Size, offset, v uint32) {
	if err := io.c.Write(s, cirBase+offset, v); err != nil {
		io.t.Fatal(err)
	}
}

func (io cirIO) read(s m68kemu.Size, offset uint32) uint32 {
	v, err := io.c.Read(s, cirBase+offset)
	if err != nil {
		io.t.Fatal(err)
	}
	return v
}

func (io cirIO) writeLong(offset, v uint32) {
	io.write(m68kemu.Word, offset, v>>16)
	io.write(m68kemu.Word, offset+2, v&0xffff)
}

func TestCIRDynamicRegisterAndInstructionAddress(t *testing.T) {
	f := fpu.New(fpu.MC68881)
	io := cirIO{t, fpu.NewCIR(f, cirBase)}
	io.writeLong(0x18, 0x00012344) // instruction address
	io.write(m68kemu.Word, 0x0a, 0x4000)
	io.writeLong(0x10, 3) // FMOVE.L #3,FP0
	if f.FPIAR != 0x12344 {
		t.Fatalf("FPIAR = %x, want the instruction address 12344", f.FPIAR)
	}

	io.write(m68kemu.Word, 0x0a, 0x7c00|5<<4) // FMOVE.P FP0,<ea>{D5}
	if r := io.read(m68kemu.Word, 0); r != 0x8c05 {
		t.Fatalf("response = %04x, want transfer main processor register D5 (8c05)", r)
	}
	io.writeLong(0x10, 1) // D5 = 1: one significant digit
	if r := io.read(m68kemu.Word, 0); r != 0xb00c {
		t.Fatalf("response = %04x, want transfer 12 bytes to the CPU (b00c)", r)
	}
	got := []uint32{io.read(m68kemu.Long, 0x10), io.read(m68kemu.Long, 0x10), io.read(m68kemu.Long, 0x10)}
	if got[0] != 0x00000003 || got[1] != 0 || got[2] != 0 {
		t.Fatalf("FMOVE.P 3 = %08x, want 00000003 00000000 00000000", got)
	}
	if r := io.read(m68kemu.Word, 0); r != 0x0802 {
		t.Fatalf("response after the transfer = %04x, want idle", r)
	}
}

func TestCIRRestore(t *testing.T) {
	f := fpu.New(fpu.MC68881)
	io := cirIO{t, fpu.NewCIR(f, cirBase)}
	io.write(m68kemu.Word, 0x06, 0x1f18) // idle frame: 24 bytes follow
	for range 6 {
		io.writeLong(0x10, 0)
	}
	if r := io.read(m68kemu.Word, 0x06); r != 0x1f18 {
		t.Fatalf("restore CIR = %04x, want the accepted format 1f18", r)
	}
	if hdr := f.FrameHeader(); hdr != 0x1f180000 {
		t.Fatalf("FPU frame header after restoring an idle frame = %08x", hdr)
	}

	io.write(m68kemu.Word, 0x06, 0x1f30) // wrong size for an MC68881
	for range 12 {
		io.writeLong(0x10, 0)
	}
	if r := io.read(m68kemu.Word, 0x06); r != 0x0002 {
		t.Fatalf("restore CIR after an invalid frame = %04x, want 0002", r)
	}

	f.FPCR = 0x10
	io.write(m68kemu.Word, 0x06, 0x0000) // null frame
	if f.FPCR != 0 || f.FrameHeader() != 0 {
		t.Fatalf("a null frame should reset the FPU")
	}
}

func TestCIRAbort(t *testing.T) {
	f := fpu.New(fpu.MC68881)
	io := cirIO{t, fpu.NewCIR(f, cirBase)}
	io.write(m68kemu.Word, 0x0a, 0x5400) // FMOVE.D <ea>,FP0, waiting for 8 bytes
	io.write(m68kemu.Word, 0x02, 0x0001) // abort
	if r := io.read(m68kemu.Word, 0); r != 0x0802 {
		t.Fatalf("response after abort = %04x, want idle", r)
	}
	io.writeLong(0x10, 0x3ff00000)
	io.writeLong(0x10, 0)
	if !f.FP[0].IsNaN() {
		t.Fatalf("operand written after abort was taken: FP0 = %v", f.FP[0])
	}
}
