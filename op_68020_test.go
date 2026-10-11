package m68kemu

import "testing"

// run020 assembles source for the MC68020, runs n instructions and returns
// the CPU.
func run020(t *testing.T, source string, n int, setup func(*core, *RAM)) (*CPU, *RAM) {
	t.Helper()
	cpu, ram := newEnvironmentFor(t, M68020)
	setTrapVectors(ram)
	load020(t, cpu, ram, source)
	if setup != nil {
		setup(cpu, ram)
	}
	for range n {
		step(t, cpu)
	}
	return cpu, ram
}

func flagsOf(cpu *CPU) uint16 { return cpu.regs.SR & 0x1f }

// core names the cpu type in closures of tests whose variables are called
// cpu.
type core = CPU

func TestLongBranches(t *testing.T) {
	cpu, _ := run020(t, "\tBRA.L far\n\t.ALIGN $8000\nfar:\tBSR.L sub\n\tNOP\nsub:\tNOP\n", 2, nil)
	far := uint32(0x2000 + 0x8000) // .ALIGN pads the program offset to $8000
	if cpu.regs.PC != far+6+2 {
		t.Fatalf("PC = %x, want sub at %x", cpu.regs.PC, far+6+2)
	}
	if ret, _ := cpu.bus.Read(Long, cpu.regs.A[7]); ret != far+6 {
		t.Fatalf("BSR.L return address %x, want %x", ret, far+6)
	}
}

func TestLinkExtbChk(t *testing.T) {
	cpu, _ := run020(t, "LINK.L A6,#-65536\nMOVE.L #$80,D0\nEXTB.L D0", 3, nil)
	frame := uint32(0x1000 - 4)
	if cpu.regs.A[6] != frame || cpu.regs.A[7] != frame-0x10000 {
		t.Fatalf("LINK.L: A6=%x A7=%x, want %x and %x", cpu.regs.A[6], cpu.regs.A[7], frame, frame-0x10000)
	}
	if cpu.regs.D[0] != 0xffffff80 || flagsOf(cpu) != srNegative {
		t.Fatalf("EXTB.L $80 = %x with flags %02x, want ffffff80 and N", cpu.regs.D[0], flagsOf(cpu))
	}

	for _, tt := range []struct {
		value int32
		trap  bool
	}{{50, false}, {100, false}, {101, true}, {-1, true}} {
		cpu, ram := newEnvironmentFor(t, M68020)
		ram.Write(Long, 6<<2, 0x5000)
		load020(t, cpu, ram, "CHK.L D1,D0")
		cpu.regs.D[0], cpu.regs.D[1] = uint32(tt.value), 100
		step(t, cpu)
		if (cpu.regs.PC == 0x5000) != tt.trap {
			t.Fatalf("CHK.L %d against 100: trap %v, want %v", tt.value, cpu.regs.PC == 0x5000, tt.trap)
		}
	}
}

func TestMulDivLong(t *testing.T) {
	cpu, _ := run020(t, "MULU.L D1,D0\nMULS.L D4,D2:D3", 2, func(c *core, _ *RAM) {
		c.regs.D[0], c.regs.D[1] = 0x10000, 0x10000
		c.regs.D[3], c.regs.D[4] = 0xfffffffd, 5
	})
	if cpu.regs.D[0] != 0 || cpu.regs.D[2] != 0xffffffff || cpu.regs.D[3] != 0xfffffff1 || flagsOf(cpu) != srNegative {
		t.Fatalf("D0=%x D2:D3=%x:%x flags %02x", cpu.regs.D[0], cpu.regs.D[2], cpu.regs.D[3], flagsOf(cpu))
	}
	cpu, _ = run020(t, "MULU.L D1,D0", 1, func(c *core, _ *RAM) { c.regs.D[0], c.regs.D[1] = 0x10000, 0x10000 })
	if flagsOf(cpu) != srOverflow|srZero {
		t.Fatalf("MULU.L overflow flags %02x, want V and Z", flagsOf(cpu))
	}

	cpu, _ = run020(t, "DIVU.L D1,D0\nDIVUL.L D1,D2:D3\nDIVS.L D1,D4:D5", 3, func(c *core, _ *RAM) {
		c.regs.D[0], c.regs.D[1], c.regs.D[3] = 100, 7, 100
		c.regs.D[4], c.regs.D[5] = 0xffffffff, 0xffffff9c // -100 as a 64-bit dividend
	})
	if cpu.regs.D[0] != 14 || cpu.regs.D[3] != 14 || cpu.regs.D[2] != 2 || cpu.regs.D[5] != 0xfffffff2 || cpu.regs.D[4] != 0xfffffffe {
		t.Fatalf("D0=%d D2:D3=%d:%d D4:D5=%d:%d; want 14, 2:14, -2:-14", cpu.regs.D[0], cpu.regs.D[2], cpu.regs.D[3], cpu.regs.D[4], cpu.regs.D[5])
	}

	cpu, _ = run020(t, "DIVU.L D1,D2:D0", 1, func(c *core, _ *RAM) { c.regs.D[2], c.regs.D[0], c.regs.D[1] = 0x100, 0, 1 })
	if cpu.regs.SR&srOverflow == 0 || cpu.regs.D[0] != 0 || cpu.regs.D[2] != 0x100 {
		t.Fatalf("64/32 overflow: V=%v D2:D0=%x:%x, want V set and operands unchanged", cpu.regs.SR&srOverflow != 0, cpu.regs.D[2], cpu.regs.D[0])
	}

	cpu, ram := newEnvironmentFor(t, M68020)
	ram.Write(Long, XDivByZero<<2, 0x5000)
	load020(t, cpu, ram, "DIVS.L D1,D0")
	step(t, cpu)
	if frame, _, _ := cpu.CurrentExceptionFrame(); cpu.regs.PC != 0x5000 || frame.Format != ExceptionStackFrameFormat2 {
		t.Fatalf("DIVS.L by zero: PC=%x format %d, want the handler and a format 2 frame", cpu.regs.PC, frame.Format)
	}
}

func TestTstCmpiTrapcc(t *testing.T) {
	cpu, _ := run020(t, "TST.W A0\nTST.L (6,PC)\nCMPI.W #$4E71,(2,PC)\nNOP\nDC.L $80000000", 3, nil)
	if flagsOf(cpu) != srZero {
		t.Fatalf("CMPI.W #$4E71 against a NOP: flags %02x, want Z", flagsOf(cpu))
	}

	cpu, _ = run020(t, "MOVEQ #1,D0\nTRAPEQ.W #1\nTRAPNE", 3, nil)
	if cpu.regs.PC != illegalHandler && cpu.regs.PC == 0x2000+2+4+2 {
		t.Fatalf("TRAPNE did not trap")
	}
	frame, _, _ := cpu.CurrentExceptionFrame()
	if frame.Format != ExceptionStackFrameFormat2 || frame.VectorOffset != 7<<2 || frame.PC != 0x2000+2+4+2 || frame.InstructionAddress != 0x2000+2+4 {
		t.Fatalf("TRAPNE frame %+v, want format 2, vector 7, PC after it and its own address", frame)
	}
}

func TestPackUnpk(t *testing.T) {
	cpu, ram := run020(t, "PACK D0,D1,#0\nUNPK D2,D3,#$3030\nPACK -(A0),-(A1),#0\nUNPK -(A2),-(A3),#$3030", 4, func(c *core, r *RAM) {
		c.regs.D[0], c.regs.D[1], c.regs.D[2], c.regs.D[3] = 0x0304, 0xffffffff, 0x34, 0xffffffff
		c.regs.A[0], c.regs.A[1], c.regs.A[2], c.regs.A[3] = 0x3002, 0x3011, 0x3021, 0x3032
		r.Write(Word, 0x3000, 0x0506)
		r.Write(Byte, 0x3020, 0x78)
	})
	if cpu.regs.D[1] != 0xffffff34 || cpu.regs.D[3] != 0xffff3334 {
		t.Fatalf("PACK = %x, UNPK = %x; want ffffff34 and ffff3334", cpu.regs.D[1], cpu.regs.D[3])
	}
	if b, _ := ram.Read(Byte, 0x3010); b != 0x56 || cpu.regs.A[0] != 0x3000 || cpu.regs.A[1] != 0x3010 {
		t.Fatalf("PACK -(A0),-(A1) = %02x with A0=%x A1=%x, want 56, 3000, 3010", b, cpu.regs.A[0], cpu.regs.A[1])
	}
	if w, _ := ram.Read(Word, 0x3030); w != 0x3738 || cpu.regs.A[3] != 0x3030 {
		t.Fatalf("UNPK -(A2),-(A3) = %04x with A3=%x, want 3738 and 3030", w, cpu.regs.A[3])
	}
}

func TestBitFields(t *testing.T) {
	cpu, ram := run020(t, `
	BFEXTU  D0{4:8},D1
	BFEXTS  D0{8:4},D2
	BFFFO   D0{0:32},D4
	BFINS   D3,D6{28:8}
	BFEXTU  (A0){4:12},D5
	BFSET   (A0){12:8}
	BFEXTU  (A0){D7:8},D7
	BFTST   (A0){0:4}
`, 8, func(c *core, r *RAM) {
		c.regs.D[0] = 0x12345678
		c.regs.D[3], c.regs.D[6] = 0xab, 0
		c.regs.D[7] = 0xfffffff8
		c.regs.A[0] = 0x3001
		r.Write(Byte, 0x3000, 0x99)
		r.Write(Byte, 0x3001, 0x12)
		r.Write(Byte, 0x3002, 0x34)
		r.Write(Byte, 0x3003, 0x56)
	})
	if cpu.regs.D[1] != 0x23 || cpu.regs.D[2] != 3 || cpu.regs.D[4] != 3 {
		t.Fatalf("BFEXTU %x, BFEXTS %x, BFFFO %d; want 23, 3, 3", cpu.regs.D[1], cpu.regs.D[2], cpu.regs.D[4])
	}
	if cpu.regs.D[6] != 0xb000000a {
		t.Fatalf("BFINS $AB into D6{28:8} = %08x, want b000000a (the field wraps)", cpu.regs.D[6])
	}
	if cpu.regs.D[5] != 0x234 {
		t.Fatalf("BFEXTU (A0){4:12} = %x, want 234", cpu.regs.D[5])
	}
	for addr, want := range map[uint32]uint32{0x3002: 0x3f, 0x3003: 0xf6} {
		if b, _ := ram.Read(Byte, addr); b != want {
			t.Fatalf("after BFSET (A0){12:8} byte %x = %02x, want %02x", addr, b, want)
		}
	}
	if cpu.regs.D[7] != 0x99 {
		t.Fatalf("BFEXTU (A0){-8:8} = %x, want the byte before A0 (99)", cpu.regs.D[7])
	}
	if flagsOf(cpu) != 0 {
		t.Fatalf("BFTST (A0){0:4} of $1 flags %02x, want none", flagsOf(cpu))
	}
}

func TestCasAndCas2(t *testing.T) {
	cpu, ram := run020(t, "CAS.W D1,D2,(A0)\nCAS.W D3,D4,(A0)", 2, func(c *core, r *RAM) {
		c.regs.A[0] = 0x3000
		c.regs.D[1], c.regs.D[2], c.regs.D[3], c.regs.D[4] = 5, 9, 1, 7
		r.Write(Word, 0x3000, 5)
	})
	if w, _ := ram.Read(Word, 0x3000); w != 9 || cpu.regs.D[3] != 9 || flagsOf(cpu)&srZero != 0 {
		t.Fatalf("CAS: memory %d, D3=%d, Z=%v; want 9 (first CAS stored), 9 (second loaded), Z clear", w, cpu.regs.D[3], flagsOf(cpu)&srZero != 0)
	}

	cpu, ram = run020(t, "CAS2.L D0:D1,D2:D3,(A0):(A1)", 1, func(c *core, r *RAM) {
		c.regs.A[0], c.regs.A[1] = 0x3000, 0x3010
		c.regs.D[0], c.regs.D[1], c.regs.D[2], c.regs.D[3] = 1, 2, 10, 20
		r.Write(Long, 0x3000, 1)
		r.Write(Long, 0x3010, 2)
	})
	a, _ := ram.Read(Long, 0x3000)
	b, _ := ram.Read(Long, 0x3010)
	if a != 10 || b != 20 || flagsOf(cpu)&srZero == 0 {
		t.Fatalf("CAS2 with both equal: memory %d, %d; want 10, 20 and Z", a, b)
	}
}

func TestChk2Cmp2(t *testing.T) {
	for _, tt := range []struct {
		value int32
		flags uint16
	}{{15, 0}, {10, srZero}, {25, srCarry}, {5, srCarry}} {
		cpu, _ := run020(t, "CMP2.B (A0),D0", 1, func(c *core, r *RAM) {
			c.regs.A[0], c.regs.D[0] = 0x3000, uint32(tt.value)
			r.Write(Byte, 0x3000, 10)
			r.Write(Byte, 0x3001, 20)
		})
		if got := flagsOf(cpu) & (srZero | srCarry); got != tt.flags {
			t.Fatalf("CMP2 %d against 10..20: flags %02x, want %02x", tt.value, got, tt.flags)
		}
	}
	// Unsigned byte bounds $80..$FF: sign-extended they wrap.
	cpu, _ := run020(t, "CMP2.B (A0),D0", 1, func(c *core, r *RAM) {
		c.regs.A[0], c.regs.D[0] = 0x3000, 0x90
		r.Write(Byte, 0x3000, 0x80)
		r.Write(Byte, 0x3001, 0x7f)
	})
	if flagsOf(cpu)&srCarry != 0 {
		t.Fatalf("CMP2 $90 against $80..$7F (wrapping) should be in range")
	}

	cpu, ram := newEnvironmentFor(t, M68020)
	ram.Write(Long, 6<<2, 0x5000)
	load020(t, cpu, ram, "CHK2.W (A0),A1")
	cpu.regs.A[0], cpu.regs.A[1] = 0x3000, 0x8000
	ram.Write(Word, 0x3000, 0)
	ram.Write(Word, 0x3002, 0x1000)
	step(t, cpu)
	if cpu.regs.PC != 0x5000 {
		t.Fatalf("CHK2 out of range should trap, PC=%x", cpu.regs.PC)
	}
}
