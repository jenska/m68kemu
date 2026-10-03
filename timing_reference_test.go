package m68kemu

import (
	"testing"
)

// timingCase is one instruction and its execution time on a 68000 with a
// zero-wait bus, from the M68000 User's Manual, section 8 (instruction
// execution times).
type timingCase struct {
	asm   string
	code  []byte // hand-encoded instruction, for forms m68kasm rejects
	want  uint64
	setup func(*cpu, *RAM)
}

func withSR(sr uint16) func(*cpu, *RAM) {
	return func(c *cpu, _ *RAM) { c.regs.SR = sr }
}

func withD(reg int, value uint32) func(*cpu, *RAM) {
	return func(c *cpu, _ *RAM) { c.regs.D[reg] = int32(value) }
}

const (
	srZ = 0x2704 // supervisor, IPL 7, Z set
	srV = 0x2702 // supervisor, IPL 7, V set
)

var timingReference = []timingCase{
	// MOVE.B/.W <ea>,Dn (table 8-2)
	{asm: "MOVE.W D0,D1", want: 4},
	{asm: "MOVE.W A0,D1", want: 4},
	{asm: "MOVE.W (A0),D1", want: 8},
	{asm: "MOVE.W (A0)+,D1", want: 8},
	{asm: "MOVE.W -(A0),D1", want: 10},
	{asm: "MOVE.W 8(A0),D1", want: 12},
	{asm: "MOVE.W 2(A0,D1.W),D2", want: 14},
	{asm: "MOVE.W $3200.W,D1", want: 12},
	{asm: "MOVE.W $3200.L,D1", want: 16},
	{asm: "MOVE.W 8(PC),D1", want: 12},
	{asm: "MOVE.W 2(PC,D1.W),D2", want: 14},
	{asm: "MOVE.W #1,D1", want: 8},
	{asm: "MOVE.B (A0),D1", want: 8},
	// MOVE.B/.W Dn,<ea>
	{asm: "MOVE.W D1,(A0)", want: 8},
	{asm: "MOVE.W D1,(A0)+", want: 8},
	{asm: "MOVE.W D1,-(A0)", want: 8},
	{asm: "MOVE.W D1,8(A0)", want: 12},
	{asm: "MOVE.W D1,2(A0,D1.W)", want: 14},
	{asm: "MOVE.W D1,$3200.W", want: 12},
	{asm: "MOVE.W D1,$3200.L", want: 16},
	{asm: "MOVE.W (A0),(A1)", want: 12},
	{asm: "MOVE.W -(A0),-(A1)", want: 14},
	{asm: "MOVE.W $3200.L,$3300.L", want: 28},
	{asm: "MOVE.W #1,(A0)", want: 12},
	{asm: "MOVEA.W D0,A1", want: 4},
	// MOVE.L (table 8-3)
	{asm: "MOVE.L D0,D1", want: 4},
	{asm: "MOVE.L (A0),D1", want: 12},
	{asm: "MOVE.L (A0)+,D1", want: 12},
	{asm: "MOVE.L -(A0),D1", want: 14},
	{asm: "MOVE.L 8(A0),D1", want: 16},
	{asm: "MOVE.L 2(A0,D1.W),D2", want: 18},
	{asm: "MOVE.L $3200.W,D1", want: 16},
	{asm: "MOVE.L $3200.L,D1", want: 20},
	{asm: "MOVE.L 8(PC),D1", want: 16},
	{asm: "MOVE.L #1,D1", want: 12},
	{asm: "MOVE.L D1,(A0)", want: 12},
	{asm: "MOVE.L D1,-(A0)", want: 12},
	{asm: "MOVE.L D1,8(A0)", want: 16},
	{asm: "MOVE.L D1,$3200.L", want: 20},
	{asm: "MOVE.L (A0),(A1)", want: 20},
	{asm: "MOVE.L -(A0),-(A1)", want: 22},
	{asm: "MOVE.L $3200.L,$3300.L", want: 36},
	{asm: "MOVE.L #1,(A0)", want: 20},
	{asm: "MOVEA.L D0,A1", want: 4},
	{asm: "MOVEA.L (A0),A1", want: 12},
	{asm: "MOVEQ #1,D1", want: 4},

	// ADD/SUB/AND/OR/CMP/EOR (table 8-4)
	{asm: "ADD.W D0,D1", want: 4},
	{asm: "ADD.L D0,D1", want: 8},
	{asm: "ADD.W (A0),D1", want: 8},
	{asm: "ADD.L (A0),D1", want: 14},
	{asm: "ADD.W D1,(A0)", want: 12},
	{asm: "ADD.L D1,(A0)", want: 20},
	{asm: "ADDA.W D0,A1", want: 8},
	{asm: "ADDA.L D0,A1", want: 8},
	{asm: "ADDA.W (A0),A1", want: 12},
	{asm: "ADDA.L (A0),A1", want: 14},
	{asm: "SUB.L D0,D1", want: 8},
	{asm: "SUB.W D1,(A0)", want: 12},
	{asm: "SUBA.L D0,A1", want: 8},
	{asm: "CMP.W D0,D1", want: 4},
	{asm: "CMP.L D0,D1", want: 6},
	{asm: "CMP.W (A0),D1", want: 8},
	{asm: "CMP.L (A0),D1", want: 14},
	{asm: "CMPA.W D0,A1", want: 6},
	{asm: "CMPA.L D0,A1", want: 6},
	{asm: "AND.W D0,D1", want: 4},
	{asm: "AND.L D0,D1", want: 8},
	{asm: "AND.W (A0),D1", want: 8},
	{asm: "OR.L D1,(A0)", want: 20},
	{asm: "EOR.W D0,D1", want: 4},
	{asm: "EOR.L D0,D1", want: 8},
	{asm: "EOR.L D0,(A0)", want: 20},

	// Immediate (table 8-5)
	{asm: "ADDI.W #1,D1", want: 8},
	{asm: "ADDI.L #1,D1", want: 16},
	{asm: "ADDI.W #1,(A0)", want: 16},
	{asm: "ADDI.L #1,(A0)", want: 28},
	{asm: "SUBI.L #1,D1", want: 16},
	{asm: "CMPI.W #1,D1", want: 8},
	{asm: "CMPI.L #1,D1", want: 14},
	{asm: "CMPI.W #1,(A0)", want: 12},
	{asm: "CMPI.L #1,(A0)", want: 20},
	{asm: "ANDI.L #1,D1", want: 16},
	{asm: "ORI.B #1,D1", want: 8},
	{asm: "EORI.W #1,(A0)", want: 16},
	{asm: "ADDQ.W #1,D1", want: 4},
	{asm: "ADDQ.L #1,D1", want: 8},
	{asm: "SUBQ.L #1,D1", want: 8},
	{asm: "ADDQ.W #1,A1", want: 8},
	{asm: "ADDQ.L #1,A1", want: 8},
	{asm: "ADDQ.W #1,(A0)", want: 12},
	{asm: "ADDQ.L #1,(A0)", want: 20},

	// Single operand (table 8-6)
	{asm: "CLR.W D1", want: 4},
	{asm: "CLR.L D1", want: 6},
	{asm: "CLR.W (A0)", want: 12},
	{asm: "CLR.L (A0)", want: 20},
	{asm: "NEG.L D1", want: 6},
	{asm: "NOT.W (A0)", want: 12},
	{asm: "TST.W D1", want: 4},
	{asm: "TST.L D1", want: 4},
	{asm: "TST.W (A0)", want: 8},
	{asm: "TST.L (A0)", want: 12},
	{asm: "SEQ D1", want: 4},
	{asm: "ST D1", want: 6},
	{asm: "SEQ (A0)", want: 12},
	{asm: "NBCD D1", want: 6},
	{asm: "NBCD -(A0)", want: 14},
	{code: []byte{0x48, 0x10}, asm: "NBCD (A0)", want: 12},
	{code: []byte{0x48, 0x39, 0x00, 0x00, 0x32, 0x00}, asm: "NBCD $3200.L", want: 20},

	// Shift/rotate (table 8-7)
	{asm: "LSL.W #1,D1", want: 8},
	{asm: "LSL.L #1,D1", want: 10},
	{asm: "LSL.W #8,D1", want: 22},
	{asm: "LSL.L D0,D1", want: 16, setup: withD(0, 4)},
	{asm: "ROR.B #2,D1", want: 10},
	{asm: "ASL.W (A0)", want: 12},
	{asm: "ASR.W (A0)", want: 12},
	{asm: "LSR.W 8(A0)", want: 16},
	{asm: "ROXR.W (A0)", want: 12},

	// Bit manipulation (table 8-8); register forms cost 2 less for bits 0-15
	{asm: "BTST D0,D1", want: 6, setup: withD(0, 3)},
	{asm: "BTST #3,D1", want: 10},
	{asm: "BTST D0,(A0)", want: 8, setup: withD(0, 3)},
	{asm: "BTST #5,(A0)", want: 12},
	{asm: "BTST #5,$3200.L", want: 20},
	{asm: "BCHG #3,D1", want: 10},
	{asm: "BCHG #20,D1", want: 12},
	{asm: "BSET D0,D1", want: 6, setup: withD(0, 3)},
	{asm: "BSET D0,D1", want: 8, setup: withD(0, 20)},
	{asm: "BCLR D0,D1", want: 8, setup: withD(0, 3)},
	{asm: "BCLR D0,D1", want: 10, setup: withD(0, 20)},
	{asm: "BCLR #3,D1", want: 12},
	{asm: "BCLR #20,D1", want: 14},
	{asm: "BCHG #5,(A0)", want: 16},
	{asm: "BSET D0,(A0)", want: 12, setup: withD(0, 3)},
	{asm: "BCLR D0,(A0)", want: 12, setup: withD(0, 3)},

	// Conditional (table 8-9)
	{asm: "BRA.S t\nNOP\nt: NOP", want: 10},
	{asm: "BRA.W t\nNOP\nt: NOP", want: 10},
	{asm: "BEQ.S t\nNOP\nt: NOP", want: 10, setup: withSR(srZ)},
	{asm: "BEQ.S t\nNOP\nt: NOP", want: 8},
	{asm: "BEQ.W t\nNOP\nt: NOP", want: 10, setup: withSR(srZ)},
	{asm: "BEQ.W t\nNOP\nt: NOP", want: 12},
	{asm: "BSR.S t\nNOP\nt: NOP", want: 18},
	{asm: "BSR.W t\nNOP\nt: NOP", want: 18},
	{asm: "t: DBRA D0,t", want: 10, setup: withD(0, 5)},
	{asm: "t: DBRA D0,t", want: 14, setup: withD(0, 0)},
	{asm: "t: DBEQ D0,t", want: 12, setup: withSR(srZ)},

	// JMP/JSR/LEA/PEA/MOVEM (table 8-10)
	{asm: "JMP (A0)", want: 8},
	{asm: "JMP 8(A0)", want: 10},
	{asm: "JMP 2(A0,D1.W)", want: 14},
	{asm: "JMP $3200.L", want: 12},
	{asm: "JSR (A0)", want: 16},
	{asm: "JSR 8(A0)", want: 18},
	{asm: "JSR $3200.L", want: 20},
	{asm: "LEA (A0),A1", want: 4},
	{asm: "LEA 8(A0),A1", want: 8},
	{asm: "LEA 2(A0,D1.W),A1", want: 12},
	{asm: "LEA $3200.L,A1", want: 12},
	{asm: "LEA 8(PC),A1", want: 8},
	{asm: "PEA (A0)", want: 12},
	{asm: "PEA 8(A0)", want: 16},
	{asm: "PEA $3200.L", want: 20},
	{asm: "MOVEM.L D0-D3,-(A7)", want: 40},
	{asm: "MOVEM.L (A7)+,D0-D3", want: 44},
	{asm: "MOVEM.W D0-D1,(A0)", want: 16},
	{asm: "MOVEM.W (A0),D0-D1", want: 20},

	// Multi-precision (table 8-11)
	{asm: "ADDX.L D0,D1", want: 8},
	{asm: "ADDX.W -(A0),-(A1)", want: 18},
	{asm: "ADDX.L -(A0),-(A1)", want: 30},
	{asm: "CMPM.W (A0)+,(A1)+", want: 12},
	{asm: "CMPM.L (A0)+,(A1)+", want: 20},
	{asm: "ABCD D0,D1", want: 6},
	{asm: "ABCD -(A0),-(A1)", want: 18},

	// Miscellaneous (table 8-12)
	{asm: "NOP", want: 4},
	{asm: "EXG D0,D1", want: 6},
	{asm: "EXT.W D1", want: 4},
	{asm: "EXT.L D1", want: 4},
	{asm: "SWAP D1", want: 4},
	{asm: "MOVE.W SR,D1", want: 6},
	{asm: "MOVE.W D1,SR", want: 12, setup: withD(1, 0x2700)},
	{asm: "MOVE.W D1,CCR", want: 12},
	{asm: "MOVE.L A0,USP", want: 4},
	{asm: "ANDI.B #0,CCR", want: 20},
	{asm: "ORI.W #$2700,SR", want: 20},
	{asm: "LINK A6,#-8", want: 16},
	{asm: "UNLK A6", want: 12},
	{asm: "RTS", want: 16},
	{asm: "RTR", want: 20},
	{asm: "RTE", want: 20},
	{asm: "TRAPV", want: 4},
	{asm: "CHK.W D0,D1", want: 10, setup: withD(0, 10)},
	{asm: "MULU.W D0,D1", want: 38},
	{asm: "MULU.W D0,D1", want: 70, setup: withD(0, 0xffff)},
	{asm: "MULU.W (A0),D1", want: 42},
	{asm: "MULS.W D0,D1", want: 38},
	{asm: "MULS.W D0,D1", want: 70, setup: withD(0, 0x5555)},
	{asm: "DIVU.W D0,D1", want: 10, setup: func(c *cpu, _ *RAM) { c.regs.D[0], c.regs.D[1] = 1, 0x10000 }},
	{asm: "TAS D1", want: 4},
	{asm: "TAS (A0)", want: 18},
	{asm: "SEQ D1", want: 6, setup: withSR(srZ)},
	{asm: "ADDA.L #1,A1", want: 16},
	{asm: "MOVE.W SR,(A0)", want: 12},
}

func runTimingCase(t *testing.T, tc timingCase) uint64 {
	t.Helper()
	cpu, ram := newEnvironment(t)
	for i := range cpu.regs.A[:7] {
		cpu.regs.A[i] = 0x3000 + uint32(i)*0x100
	}
	cpu.regs.D[1] = 2
	// Stack frames for RTS/RTR/RTE: SR/CCR word then the return address.
	sp := cpu.regs.A[7]
	_ = ram.Write(Word, sp, 0x2700)
	_ = ram.Write(Long, sp+2, cpu.regs.PC)
	_ = ram.Write(Long, sp+6, cpu.regs.PC)
	if tc.asm == "RTS" {
		_ = ram.Write(Long, sp, cpu.regs.PC)
	}
	if tc.setup != nil {
		tc.setup(cpu, ram)
	}
	code := tc.code
	if code == nil {
		code = assemble(t, tc.asm)
	}
	for i, b := range code {
		_ = ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}
	start := cpu.Cycles()
	if err := cpu.Step(); err != nil {
		t.Fatalf("%s: %v", tc.asm, err)
	}
	return cpu.Cycles() - start
}

func TestInstructionTimingMatchesMotorolaTables(t *testing.T) {
	for _, tc := range timingReference {
		if got := runTimingCase(t, tc); got != tc.want {
			t.Errorf("%-24s %3d cycles, want %3d", tc.asm, got, tc.want)
		}
	}
}

// TestDivisionTimingStaysWithinManualMaximum checks DIVU/DIVS against the
// worst cases the manual gives (140 and 158 cycles plus the EA time).
func TestDivisionTimingStaysWithinManualMaximum(t *testing.T) {
	for dividend := uint32(0); dividend < 0x40000000; dividend = dividend*3 + 7 {
		for _, divisor := range []uint16{1, 2, 3, 7, 0x00ff, 0x1234, 0x7fff, 0x8000, 0xffff} {
			if got := divuCycles(dividend, divisor); got > 140 || got < 10 {
				t.Fatalf("divuCycles(%#x, %#x) = %d, want 10..140", dividend, divisor, got)
			}
			if got := divsCycles(int32(dividend), int16(divisor)); got > 158 || got < 12 {
				t.Fatalf("divsCycles(%#x, %#x) = %d, want 12..158", dividend, divisor, got)
			}
			if got := divsCycles(-int32(dividend), int16(divisor)); got > 158 || got < 12 {
				t.Fatalf("divsCycles(%#x, %#x) = %d, want 12..158", -int32(dividend), divisor, got)
			}
		}
	}
}

func TestCycleRoundingPadsEachInstruction(t *testing.T) {
	ram := NewRAM(0, 0x10000)
	bus := NewBus(ram)
	_ = ram.Write(Long, 0, 0x1000)
	_ = ram.Write(Long, 4, 0x2000)
	processor, err := NewCPU(bus, WithCycleRounding(4))
	if err != nil {
		t.Fatalf("create CPU: %v", err)
	}
	code := assemble(t, "NOP\nEXG D0,D1\nt: DBRA D0,t")
	for i, b := range code {
		_ = ram.Write(Byte, 0x2000+uint32(i), uint32(b))
	}
	processor.(*cpu).regs.D[1] = 1                // EXG moves it into D0: DBRA branches once, then expires
	for i, want := range []uint64{4, 8, 12, 16} { // NOP 4, EXG 6, DBRA taken 10, DBRA expired 14
		start := processor.Cycles()
		if err := processor.Step(); err != nil {
			t.Fatalf("step: %v", err)
		}
		if got := processor.Cycles() - start; got != want {
			t.Fatalf("instruction %d: %d cycles, want %d", i, got, want)
		}
	}
}
