package m68kemu

import (
	"testing"

	asm "github.com/jenska/m68kasm"
)

// newEnvironmentFor returns a CPU of model m, which NewCPU may not accept
// yet, with 64 KB of RAM and the reset vectors SSP=$1000, PC=$2000.
func newEnvironmentFor(tb testing.TB, m Model, devices ...Device) (*cpu, *RAM) {
	tb.Helper()
	ram := NewRAM(0, 0x10000)
	ram.Write(Long, 0, 0x1000)
	ram.Write(Long, 4, 0x2000)
	c, err := newCPU(NewBus(append([]Device{ram}, devices...)...), cpuConfig{model: m})
	if err != nil {
		tb.Fatal(err)
	}
	return c, ram
}

// load020 assembles source for the MC68020 and stores it at the current PC.
func load020(tb testing.TB, cpu *cpu, ram *RAM, source string) {
	tb.Helper()
	code, err := asm.AssembleStringWithOptions(source, asm.ParseOptions{Target: asm.Target{CPU: asm.CPU68020}})
	if err != nil {
		tb.Fatalf("Assembler failed: %v", err)
	}
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}
}

func TestMasterAndInterruptStacks(t *testing.T) {
	cpu, ram := newEnvironmentFor(t, M68020)
	load020(t, cpu, ram, `
	MOVE.L  #$3000,D0
	MOVEC   D0,MSP
	MOVEC   MSP,D1
	ORI.W   #$1000,SR      ; M: switch to the master stack
	MOVEC   ISP,D2
	MOVE.L  #$0E00,D3
	MOVEC   D3,USP
	ANDI.W  #$DFFF,SR      ; user mode
`)
	isp := cpu.regs.A[7]
	for range 4 {
		step(t, cpu)
	}
	if cpu.regs.D[1] != 0x3000 || cpu.regs.A[7] != 0x3000 || cpu.regs.SSP != isp {
		t.Fatalf("after setting M: D1=%x A7=%x ISP=%x; want 3000, 3000, %x", cpu.regs.D[1], cpu.regs.A[7], cpu.regs.SSP, isp)
	}
	for range 4 {
		step(t, cpu)
	}
	if uint32(cpu.regs.D[2]) != isp || cpu.regs.A[7] != 0x0e00 || cpu.regs.MSP != 0x3000 {
		t.Fatalf("in user mode: D2=%x A7=%x MSP=%x; want %x, e00, 3000", cpu.regs.D[2], cpu.regs.A[7], cpu.regs.MSP, isp)
	}
}

func TestCacheControlRegisters(t *testing.T) {
	for _, tt := range []struct {
		model Model
		cacr  uint32
	}{{M68020, 0x3}, {M68030, 0x3313}} {
		cpu, ram := newEnvironmentFor(t, tt.model)
		load020(t, cpu, ram, "MOVEQ #-1,D0\nMOVEC D0,CACR\nMOVEC CACR,D1\nMOVEC D0,CAAR\nMOVEC CAAR,D2")
		for range 5 {
			step(t, cpu)
		}
		if uint32(cpu.regs.D[1]) != tt.cacr || cpu.regs.D[2] != -1 {
			t.Fatalf("%v: CACR=%x CAAR=%x; want %x and ffffffff", tt.model, uint32(cpu.regs.D[1]), uint32(cpu.regs.D[2]), tt.cacr)
		}
	}

	cpu, ram := newEnvironment010(t)
	setTrapVectors(ram)
	ram.Write(Word, cpu.regs.PC, 0x4e7a) // MOVEC CACR,D0 on a 68010
	ram.Write(Word, cpu.regs.PC+2, 0x0002)
	step(t, cpu)
	if cpu.regs.PC != illegalHandler {
		t.Fatalf("MOVEC CACR on an MC68010 should be illegal")
	}
}

func TestFormat2Frame(t *testing.T) {
	cpu, ram := newEnvironmentFor(t, M68020)
	ram.Write(Long, XDivByZero<<2, 0x5000)
	load020(t, cpu, ram, "MOVEQ #0,D0\nDIVU.W D0,D1")
	step(t, cpu)
	start, ssp := cpu.regs.PC, cpu.regs.A[7]
	step(t, cpu)
	frame, _, err := cpu.CurrentExceptionFrame()
	if err != nil {
		t.Fatal(err)
	}
	if frame.Format != ExceptionStackFrameFormat2 || frame.StackPointer != ssp-12 || frame.InstructionAddress != start || frame.VectorOffset != XDivByZero<<2 {
		t.Fatalf("divide by zero frame = %+v, want format 2 at %x with instruction address %x", frame, ssp-12, start)
	}
	if fv, _ := ram.Read(Word, ssp-6); fv != 0x2000|XDivByZero<<2 {
		t.Fatalf("format/vector word = %04x, want %04x", fv, 0x2000|XDivByZero<<2)
	}
}

func TestInterruptWithMasterStackPushesThrowawayFrame(t *testing.T) {
	cpu, ram := newEnvironmentFor(t, M68020)
	ram.Write(Long, (autoVectorBase+3)<<2, 0x5000)
	ram.Write(Word, 0x5000, 0x4e73) // RTE
	load020(t, cpu, ram, "NOP\nNOP")
	isp := cpu.regs.A[7]
	cpu.regs.MSP = 0x3000
	cpu.setSR(0x3000) // S and M: master stack, interrupts enabled
	start := cpu.regs.PC

	if err := cpu.RequestInterrupt(3, AutoVector); err != nil {
		t.Fatal(err)
	}
	step(t, cpu) // NOP, then the interrupt
	if cpu.regs.PC != 0x5000 || cpu.regs.SR&srMaster != 0 {
		t.Fatalf("handler PC=%x SR=%04x, want 5000 with M clear", cpu.regs.PC, cpu.regs.SR)
	}
	if cpu.regs.A[7] != isp-8 || cpu.regs.MSP != 0x3000-8 {
		t.Fatalf("ISP=%x MSP=%x, want %x (throwaway) and %x (format 0)", cpu.regs.A[7], cpu.regs.MSP, isp-8, 0x3000-8)
	}
	if fv, _ := ram.Read(Word, isp-2); fv>>12 != 1 {
		t.Fatalf("frame on the ISP has format %d, want 1", fv>>12)
	}

	step(t, cpu) // RTE: throwaway, then the master stack frame
	if cpu.regs.PC != start+2 || cpu.regs.SR != 0x3000 || cpu.regs.A[7] != 0x3000 || cpu.regs.SSP != isp {
		t.Fatalf("after RTE PC=%x SR=%04x A7=%x ISP=%x; want %x, 3000, 3000, %x", cpu.regs.PC, cpu.regs.SR, cpu.regs.A[7], cpu.regs.SSP, start+2, isp)
	}
}

func TestFormatBBusFault(t *testing.T) {
	dev := &flakyDevice{data: map[uint32]uint32{}}
	cpu, ram := newEnvironmentFor(t, M68020, dev)
	ram.Write(Long, XBusError<<2, 0x6000)
	ram.Write(Long, XAddressError<<2, 0x6000)
	ram.Write(Word, 0x6000, 0x4e73) // RTE
	load020(t, cpu, ram, "MOVE.L D0,(A0)+")
	cpu.regs.A[0], cpu.regs.D[0] = 0x20000, 0x12345678
	start, ssp := cpu.regs.PC, cpu.regs.A[7]

	var info ExceptionInfo
	cpu.SetExceptionTracer(func(i ExceptionInfo) { info = i })
	step(t, cpu)
	f := info.Frame
	if f.Format != ExceptionStackFrameFormatB || f.StackPointer != ssp-92 || f.PC != start {
		t.Fatalf("frame = %+v, want format B at %x with PC %x", f, ssp-92, start)
	}
	// DF, a long write in supervisor data space.
	if f.StatusWord != 1<<8|uint16(functionCodeSupervisorData) || f.FaultAddress != 0x20000 || f.DataOutput != 0x12345678 {
		t.Fatalf("SSW %04x, fault address %x, data output %x; want 0105, 20000, 12345678", f.StatusWord, f.FaultAddress, f.DataOutput)
	}
	if read, _, err := cpu.CurrentExceptionFrame(); err != nil || read != f {
		t.Fatalf("frame read back = %+v (%v), want %+v", read, err, f)
	}
	step(t, cpu) // RTE
	step(t, cpu) // the instruction runs again
	if cpu.regs.A[0] != 0x20004 || cpu.regs.A[7] != ssp || dev.data[0x20002] != 0x5678 {
		t.Fatalf("after restart A0=%x SP=%x stored %x; want 20004, %x, 5678", cpu.regs.A[0], cpu.regs.A[7], dev.data[0x20002], ssp)
	}

	cpu.regs.PC = 0x2001 // instruction fetch from an odd address
	step(t, cpu)
	if f := info.Frame; info.Vector != XAddressError || f.StatusWord&(1<<14|1<<12) != 1<<14|1<<12 || f.StageBAddress != 0x2001 {
		t.Fatalf("odd PC: vector %d, frame %+v; want an address error with FB, RB and stage B address 2001", info.Vector, f)
	}
}

func TestMisalignedOperands(t *testing.T) {
	cpu, ram := newEnvironmentFor(t, M68020)
	load020(t, cpu, ram, "MOVE.L D0,(A0)\nMOVE.W (A1),D1\nMOVE.L (A0),D2")
	cpu.regs.A[0], cpu.regs.A[1] = 0x3001, 0x3002
	cpu.regs.D[0] = 0x11223344
	for range 3 {
		step(t, cpu)
	}
	for addr, want := range map[uint32]uint32{0x3001: 0x11, 0x3002: 0x22, 0x3003: 0x33, 0x3004: 0x44} {
		if got, _ := ram.Read(Byte, addr); got != want {
			t.Fatalf("byte at %x = %02x, want %02x", addr, got, want)
		}
	}
	if cpu.regs.D[1]&0xffff != 0x2233 || uint32(cpu.regs.D[2]) != 0x11223344 {
		t.Fatalf("D1=%x D2=%x, want 2233 and 11223344", cpu.regs.D[1]&0xffff, uint32(cpu.regs.D[2]))
	}
}

func TestRteFormatErrorOn68020(t *testing.T) {
	cpu, ram := newEnvironmentFor(t, M68020)
	ram.Write(Long, XFormatError<<2, 0x5000)
	load020(t, cpu, ram, "RTE")
	sp := cpu.regs.A[7] - 12
	cpu.regs.A[7] = sp
	ram.Write(Word, sp, 0x2700)
	ram.Write(Long, sp+2, 0x3000)
	ram.Write(Word, sp+6, 0x3000) // format $3: MC68040 only
	step(t, cpu)
	if cpu.regs.PC != 0x5000 {
		t.Fatalf("PC = %x, want the format-error handler", cpu.regs.PC)
	}
}

func TestIndexedAddressingModes020(t *testing.T) {
	cpu, ram := newEnvironmentFor(t, M68020)
	setTrapVectors(ram)
	// m68kasm v1.6.2 swaps the preindexed and postindexed encodings, so the
	// memory indirect modes are written as words (m68kdasm decodes them as
	// noted).
	load020(t, cpu, ram, `
	LEA     (8,A0,D1.W*2),A1        ; brief format, scaled index
	MOVE.L  (A0,D1.L*8),D0
	DC.W    $2430,$1D37,0,8,0,12    ; MOVE.L ([8,A0],D1.L*4,12),D2: postindexed
	DC.W    $2630,$1333,0,8,-1,-4   ; MOVE.L ([8,A0,D1.W*2],-4),D3: preindexed
	LEA     (6,PC,D1.W*4),A4
`)
	start := cpu.regs.PC
	cpu.regs.A[0], cpu.regs.D[1] = 0x3000, 2
	ram.Write(Long, 0x3010, 0xaaaa0001) // (A0,D1.L*8)
	ram.Write(Long, 0x3008, 0x4000)     // pointer for the postindexed mode
	ram.Write(Long, 0x4000+8+12, 0xbbbb0002)
	ram.Write(Long, 0x300c, 0x5000) // pointer for the preindexed mode: 3000+8+2*2
	ram.Write(Long, 0x5000-4, 0xcccc0003)
	for range 5 {
		step(t, cpu)
	}
	if cpu.regs.A[1] != 0x3000+8+4 {
		t.Fatalf("LEA (8,A0,D1.W*2) = %x, want %x", cpu.regs.A[1], 0x3000+8+4)
	}
	if uint32(cpu.regs.D[0]) != 0xaaaa0001 || uint32(cpu.regs.D[2]) != 0xbbbb0002 || uint32(cpu.regs.D[3]) != 0xcccc0003 {
		t.Fatalf("D0=%x D2=%x D3=%x, want aaaa0001, bbbb0002, cccc0003", uint32(cpu.regs.D[0]), uint32(cpu.regs.D[2]), uint32(cpu.regs.D[3]))
	}
	lea := start + 4 + 4 + 12 + 12 // address of the last LEA
	if cpu.regs.A[4] != lea+2+6+8 {
		t.Fatalf("LEA (6,PC,D1.W*4) = %x, want %x (extension word address + 6 + 8)", cpu.regs.A[4], lea+2+6+8)
	}

	// Full format with base and index suppressed and a word base
	// displacement: LEA ($1234.W,ZA0,ZD0),A2.
	pc := cpu.regs.PC
	for i, w := range []uint16{0x45f0, 0x01e0, 0x1234} {
		ram.Write(Word, pc+uint32(2*i), uint32(w))
	}
	step(t, cpu)
	if cpu.regs.A[2] != 0x1234 {
		t.Fatalf("LEA ($1234.W,ZA0,ZD0) = %x, want 1234", cpu.regs.A[2])
	}

	// I/IS = 100 is reserved.
	pc = cpu.regs.PC
	ram.Write(Word, pc, 0x45f0)
	ram.Write(Word, pc+2, 0x0114)
	step(t, cpu)
	if cpu.regs.PC != illegalHandler {
		t.Fatalf("reserved I/IS: PC = %x, want the illegal-instruction handler", cpu.regs.PC)
	}
}

func TestBriefExtensionScaleIgnoredOn68000(t *testing.T) {
	cpu, ram := newEnvironment(t)
	ram.Write(Word, cpu.regs.PC, 0x43f0)   // LEA (8,A0,D1.W*2),A1
	ram.Write(Word, cpu.regs.PC+2, 0x1208) // scale bits set
	cpu.regs.A[0], cpu.regs.D[1] = 0x3000, 2
	step(t, cpu)
	if cpu.regs.A[1] != 0x3000+8+2 {
		t.Fatalf("68000 LEA = %x, want %x: the 68000 does not scale the index", cpu.regs.A[1], 0x3000+8+2)
	}
}
