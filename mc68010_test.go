package m68kemu

import (
	"testing"

	asm "github.com/jenska/m68kasm"
)

func newEnvironment010(tb testing.TB) (*cpu, *RAM) {
	tb.Helper()

	memory := NewRAM(0, 1024*64)
	bus := NewBus(memory)
	memory.Write(Long, 0, 0x1000)
	memory.Write(Long, 4, 0x2000)
	processor, err := NewCPU(bus, WithModel(M68010))
	if err != nil {
		tb.Fatalf("Failed to create CPU: %v", err)
	}
	return processor.(*cpu), memory
}

// load010 assembles source for the 68010 and stores it at the current PC.
func load010(tb testing.TB, cpu *cpu, ram *RAM, source string) {
	tb.Helper()
	code, err := asm.AssembleStringWithOptions(source, asm.ParseOptions{Target: asm.Target{CPU: asm.CPU68010}})
	if err != nil {
		tb.Fatalf("Assembler failed: %v", err)
	}
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			tb.Fatalf("failed to store program byte: %v", err)
		}
	}
}

func step(tb testing.TB, cpu *cpu) {
	tb.Helper()
	if err := cpu.Step(); err != nil {
		tb.Fatalf("Step failed: %v", err)
	}
}

func TestVectorsRelativeToVBR(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	cpu.regs.VBR = 0x8000
	ram.Write(Long, 0x8000+XTrap<<2, 0x5000)
	ram.Write(Long, XTrap<<2, 0x6000) // must not be used
	load010(t, cpu, ram, "TRAP #0")

	step(t, cpu)
	if cpu.regs.PC != 0x5000 {
		t.Fatalf("PC = %04x, want the TRAP #0 vector from the VBR table at 5000", cpu.regs.PC)
	}
}

func TestResetClearsControlRegisters(t *testing.T) {
	cpu, _ := newEnvironment010(t)
	cpu.regs.VBR, cpu.regs.SFC, cpu.regs.DFC = 0x8000, 5, 6
	if err := cpu.Reset(); err != nil {
		t.Fatal(err)
	}
	if cpu.regs.VBR != 0 || cpu.regs.SFC != 0 || cpu.regs.DFC != 0 {
		t.Fatalf("after reset VBR=%x SFC=%d DFC=%d, want all zero", cpu.regs.VBR, cpu.regs.SFC, cpu.regs.DFC)
	}
}

const (
	illegalHandler = 0x4000
	privHandler    = 0x5000
)

// setTrapVectors points the illegal-instruction and privilege-violation
// vectors of the table at address 0 to their handlers.
func setTrapVectors(ram *RAM) {
	ram.Write(Long, XIllegal<<2, illegalHandler)
	ram.Write(Long, XPrivViolation<<2, privHandler)
}

func enterUserMode(cpu *cpu) {
	cpu.regs.USP = 0x0e00
	cpu.setSR(cpu.regs.SR &^ srSupervisor)
}

func TestMovecControlRegisters(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	load010(t, cpu, ram, `
	MOVE.L #$8000,D0
	MOVEC D0,VBR
	MOVEC VBR,A1
	MOVEQ #-1,D1
	MOVEC D1,SFC
	MOVEC SFC,D2
	MOVE.L #$3000,A2
	MOVEC A2,USP
	MOVEC USP,D3
	MOVEQ #3,D4
	MOVEC D4,DFC
	MOVEC DFC,D5
`)
	for range 12 {
		step(t, cpu)
	}
	r := cpu.regs
	switch {
	case r.VBR != 0x8000 || r.A[1] != 0x8000:
		t.Fatalf("VBR=%x A1=%x, want 8000", r.VBR, r.A[1])
	case r.SFC != 7 || r.D[2] != 7:
		t.Fatalf("SFC=%d D2=%d, want 7 (3 bits of -1)", r.SFC, r.D[2])
	case r.USP != 0x3000 || r.D[3] != 0x3000:
		t.Fatalf("USP=%x D3=%x, want 3000", r.USP, r.D[3])
	case r.DFC != 3 || r.D[5] != 3:
		t.Fatalf("DFC=%d D5=%d, want 3", r.DFC, r.D[5])
	}
}

func TestMovecInUserModeIsPrivileged(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	setTrapVectors(ram)
	load010(t, cpu, ram, "MOVEC VBR,D0")
	enterUserMode(cpu)
	step(t, cpu)
	if cpu.regs.PC != privHandler {
		t.Fatalf("PC = %x, want the privilege-violation handler", cpu.regs.PC)
	}
}

func TestMovecUnknownControlRegisterIsIllegal(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	setTrapVectors(ram)
	start := cpu.regs.PC
	ram.Write(Word, start, 0x4e7a)   // MOVEC Rc,D0
	ram.Write(Word, start+2, 0x0002) // CACR, which the 68010 lacks
	step(t, cpu)
	if cpu.regs.PC != illegalHandler {
		t.Fatalf("PC = %x, want the illegal-instruction handler", cpu.regs.PC)
	}
	// Same stacked PC as an unassigned opcode word: past the opcode, not
	// past the extension word.
	if stacked, _ := ram.Read(Long, cpu.regs.A[7]+2); stacked != start+2 {
		t.Fatalf("stacked PC = %x, want %x", stacked, start+2)
	}
}

func TestMoves(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	load010(t, cpu, ram, `
	MOVES.B (A2),D0
	MOVES.B (A2),A0
	MOVES.W D1,(A3)
`)
	cpu.regs.SFC, cpu.regs.DFC = 1, 5
	cpu.regs.A[2], cpu.regs.A[3] = 0x3000, 0x3010
	cpu.regs.D[0], cpu.regs.D[1] = 0x12345678, 0x0000abcd
	ram.Write(Byte, 0x3000, 0x80)

	var accesses []BusAccessInfo
	cpu.SetBusTracer(func(info BusAccessInfo) {
		if !info.InstructionFetch {
			accesses = append(accesses, info)
		}
	})
	for range 3 {
		step(t, cpu)
	}

	if got := uint32(cpu.regs.D[0]); got != 0x12345680 {
		t.Fatalf("D0 = %08x, want the byte merged into the low byte: 12345680", got)
	}
	if got := cpu.regs.A[0]; got != 0xffffff80 {
		t.Fatalf("A0 = %08x, want the byte sign-extended: ffffff80", got)
	}
	if got, _ := ram.Read(Word, 0x3010); got != 0xabcd {
		t.Fatalf("word at 3010 = %04x, want abcd", got)
	}
	want := []BusAccessInfo{
		{Address: 0x3000, Size: Byte, FunctionCode: 1},
		{Address: 0x3000, Size: Byte, FunctionCode: 1},
		{Address: 0x3010, Size: Word, Write: true, FunctionCode: 5},
	}
	if len(accesses) != len(want) {
		t.Fatalf("data accesses = %+v, want %d", accesses, len(want))
	}
	for i, a := range accesses {
		if a.Address != want[i].Address || a.Size != want[i].Size || a.Write != want[i].Write || a.FunctionCode != want[i].FunctionCode {
			t.Fatalf("access %d = %+v, want %+v", i, a, want[i])
		}
	}
}

func TestMovesInUserModeIsPrivileged(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	setTrapVectors(ram)
	load010(t, cpu, ram, "MOVES.L (A2),D0")
	enterUserMode(cpu)
	step(t, cpu)
	if cpu.regs.PC != privHandler {
		t.Fatalf("PC = %x, want the privilege-violation handler", cpu.regs.PC)
	}
}

func TestRtd(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	load010(t, cpu, ram, "RTD #8")
	cpu.regs.A[7] = 0x0f00
	ram.Write(Long, 0x0f00, 0x2400)
	step(t, cpu)
	if cpu.regs.PC != 0x2400 {
		t.Fatalf("PC = %x, want 2400", cpu.regs.PC)
	}
	if cpu.regs.A[7] != 0x0f00+4+8 {
		t.Fatalf("SP = %x, want %x: return address and 8 bytes of arguments released", cpu.regs.A[7], 0x0f00+4+8)
	}
}

func TestMoveFromCcr(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	load010(t, cpu, ram, "MOVE CCR,D0")
	enterUserMode(cpu) // MOVE from CCR is not privileged
	cpu.regs.SR |= 0x15
	cpu.regs.D[0] = -1
	step(t, cpu)
	if got := uint32(cpu.regs.D[0]); got != 0xffff0015 {
		t.Fatalf("D0 = %08x, want the CCR as a word: ffff0015", got)
	}
}

func TestMoveFromSrPrivilege(t *testing.T) {
	for _, tt := range []struct {
		model      Model
		privileged bool
	}{
		{M68000, false},
		{M68010, true},
	} {
		t.Run(tt.model.String(), func(t *testing.T) {
			ram := NewRAM(0, 0x10000)
			ram.Write(Long, 0, 0x1000)
			ram.Write(Long, 4, 0x2000)
			setTrapVectors(ram)
			ram.Write(Word, 0x2000, 0x40c0) // MOVE SR,D0
			c, err := NewCPU(NewBus(ram), WithModel(tt.model))
			if err != nil {
				t.Fatal(err)
			}
			cpu := c.(*cpu)
			enterUserMode(cpu)
			step(t, cpu)
			if got := cpu.regs.PC == privHandler; got != tt.privileged {
				t.Fatalf("privilege violation = %v, want %v (PC=%x)", got, tt.privileged, cpu.regs.PC)
			}
		})
	}
}

// flakyDevice answers for $20000-$200FF and fails its first write with a bus
// error, like a page that a virtual-memory handler first has to map in.
type flakyDevice struct {
	failed bool
	data   map[uint32]uint32
}

func (d *flakyDevice) AddressRange() (uint32, uint32) { return 0x20000, 0x200ff }
func (d *flakyDevice) Read(s Size, a uint32) (uint32, error) {
	return d.data[a], nil
}
func (d *flakyDevice) Write(s Size, a uint32, v uint32) error {
	if !d.failed {
		d.failed = true
		return BusError(a)
	}
	d.data[a] = v & s.mask()
	return nil
}
func (d *flakyDevice) Reset() {}

// newFaultEnvironment010 returns a 68010 with RAM at 0, a flaky device, and
// bus and address error handlers that consist of a single RTE.
func newFaultEnvironment010(t *testing.T) (*cpu, *RAM, *flakyDevice) {
	t.Helper()
	ram := NewRAM(0, 0x10000)
	dev := &flakyDevice{data: map[uint32]uint32{}}
	ram.Write(Long, 0, 0x1000)
	ram.Write(Long, 4, 0x2000)
	ram.Write(Long, XBusError<<2, 0x6000)
	ram.Write(Long, XAddressError<<2, 0x6000)
	ram.Write(Word, 0x6000, 0x4e73) // RTE
	c, err := NewCPU(NewBus(ram, dev), WithModel(M68010))
	if err != nil {
		t.Fatal(err)
	}
	return c.(*cpu), ram, dev
}

func TestFormat0FrameAndRte(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	cpu.regs.VBR = 0x8000
	ram.Write(Long, 0x8000+(XTrap+3)<<2, 0x5000)
	ram.Write(Word, 0x5000, 0x4e73) // RTE
	load010(t, cpu, ram, "TRAP #3")
	start, ssp := cpu.regs.PC, cpu.regs.A[7]

	step(t, cpu)
	frame, ok, err := cpu.CurrentExceptionFrame()
	if err != nil || !ok {
		t.Fatalf("CurrentExceptionFrame: ok=%v err=%v", ok, err)
	}
	want := ExceptionStackFrame{
		Format:       ExceptionStackFrameFormat0,
		StackPointer: ssp - 8,
		SR:           0x2700,
		PC:           start + 2,
		VectorOffset: (XTrap + 3) << 2,
	}
	if frame != want {
		t.Fatalf("frame = %+v, want %+v", frame, want)
	}
	if fv, _ := ram.Read(Word, ssp-2); fv != (XTrap+3)<<2 {
		t.Fatalf("format/vector word = %04x, want format 0 and offset %x", fv, (XTrap+3)<<2)
	}

	step(t, cpu) // RTE
	if cpu.regs.PC != start+2 || cpu.regs.A[7] != ssp {
		t.Fatalf("after RTE PC=%x SP=%x, want PC=%x SP=%x", cpu.regs.PC, cpu.regs.A[7], start+2, ssp)
	}
}

func TestInterruptPushesFormat0Frame(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	ram.Write(Long, (autoVectorBase+3)<<2, 0x5000)
	load010(t, cpu, ram, "NOP")
	cpu.regs.SR = 0x2000
	if err := cpu.RequestInterrupt(3, AutoVector); err != nil {
		t.Fatal(err)
	}
	step(t, cpu)
	if cpu.regs.PC != 0x5000 {
		t.Fatalf("PC = %x, want the level 3 autovector handler", cpu.regs.PC)
	}
	if fv, _ := ram.Read(Word, cpu.regs.A[7]+6); fv != (autoVectorBase+3)<<2 {
		t.Fatalf("format/vector word = %04x, want %04x", fv, (autoVectorBase+3)<<2)
	}
}

func TestRteFormatError(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	ram.Write(Long, XFormatError<<2, 0x5000)
	load010(t, cpu, ram, "RTE")
	sp := cpu.regs.A[7] - 8
	cpu.regs.A[7] = sp
	ram.Write(Word, sp, 0x2700)
	ram.Write(Long, sp+2, 0x3000)
	ram.Write(Word, sp+6, 0x2000) // format $2, which the 68010 does not have
	step(t, cpu)
	if cpu.regs.PC != 0x5000 {
		t.Fatalf("PC = %x, want the format-error handler", cpu.regs.PC)
	}
}

func TestBusErrorRestartsInstruction(t *testing.T) {
	cpu, ram, dev := newFaultEnvironment010(t)
	load010(t, cpu, ram, "MOVE.L D0,(A0)+")
	cpu.regs.A[0], cpu.regs.D[0] = 0x20000, 0x12345678
	start, ssp := cpu.regs.PC, cpu.regs.A[7]

	var info ExceptionInfo
	var a0 uint32
	cpu.SetExceptionTracer(func(i ExceptionInfo) { info, a0 = i, cpu.regs.A[0] })

	step(t, cpu) // the write faults
	if cpu.regs.PC != 0x6000 || info.Vector != XBusError {
		t.Fatalf("PC=%x vector=%d, want the bus error handler", cpu.regs.PC, info.Vector)
	}
	want := ExceptionStackFrame{
		Format:       ExceptionStackFrameFormat8,
		StackPointer: ssp - 58,
		StatusWord:   uint16(functionCodeSupervisorData), // a long write: no RW, IF, DF, BY or HB
		FaultAddress: 0x20000,
		SR:           0x2700,
		PC:           start,
		VectorOffset: XBusError << 2,
		DataOutput:   0x1234,
	}
	if info.Frame != want {
		t.Fatalf("frame = %+v, want %+v", info.Frame, want)
	}
	if frame, _, err := cpu.CurrentExceptionFrame(); err != nil || frame != want {
		t.Fatalf("frame read back = %+v (err %v), want %+v", frame, err, want)
	}
	if a0 != 0x20000 {
		t.Fatalf("A0 in the handler = %x, want 20000: the post-increment is rolled back", a0)
	}

	step(t, cpu) // RTE
	if cpu.regs.PC != start || cpu.regs.A[7] != ssp {
		t.Fatalf("after RTE PC=%x SP=%x, want PC=%x SP=%x", cpu.regs.PC, cpu.regs.A[7], start, ssp)
	}
	step(t, cpu) // the instruction runs again and now succeeds
	// The bus splits the long write into two word writes.
	if dev.data[0x20000] != 0x1234 || dev.data[0x20002] != 0x5678 || cpu.regs.A[0] != 0x20004 {
		t.Fatalf("after restart: stored %04x %04x, A0=%x; want 1234 5678 and A0 incremented once to 20004",
			dev.data[0x20000], dev.data[0x20002], cpu.regs.A[0])
	}
}

func TestFormat8SpecialStatusWord(t *testing.T) {
	for _, tt := range []struct {
		name    string
		source  string
		setup   func(*cpu)
		ssw     uint16
		address uint32
		dob     uint16
	}{
		{"ByteWriteHighByte", "MOVE.B D0,(A0)", func(c *cpu) { c.regs.A[0], c.regs.D[0] = 0x20000, 0xab },
			1<<10 | 1<<9 | 5, 0x20000, 0xab},
		{"WordReadOddAddress", "MOVE.W (A0),D0", func(c *cpu) { c.regs.A[0] = 0x3001 },
			1<<12 | 1<<8 | 5, 0x3001, 0},
		{"InstructionFetchOddPC", "NOP", func(c *cpu) { c.regs.PC = 0x2001 },
			1<<13 | 1<<8 | 6, 0x2001, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram, _ := newFaultEnvironment010(t)
			load010(t, cpu, ram, tt.source)
			tt.setup(cpu)
			var info ExceptionInfo
			cpu.SetExceptionTracer(func(i ExceptionInfo) { info = i })
			step(t, cpu)
			f := info.Frame
			if f.Format != ExceptionStackFrameFormat8 || f.StatusWord != tt.ssw || f.FaultAddress != tt.address || f.DataOutput != tt.dob {
				t.Fatalf("frame = %+v, want format 8, SSW %04x, fault address %x, data output %04x", f, tt.ssw, tt.address, tt.dob)
			}
		})
	}
}

func TestM68000FramesUnchanged(t *testing.T) {
	cpu, ram := newEnvironment(t)
	ram.Write(Long, XTrap<<2, 0x5000)
	ram.Write(Word, cpu.regs.PC, 0x4e40) // TRAP #0
	ssp := cpu.regs.A[7]
	step(t, cpu)
	if cpu.regs.A[7] != ssp-6 || cpu.lastException.Frame.Format != ExceptionStackFrameGroup12 {
		t.Fatalf("68000 TRAP frame: SP=%x format=%d, want SP=%x and the 6-byte group 1/2 frame", cpu.regs.A[7], cpu.lastException.Frame.Format, ssp-6)
	}
}
