package m68kemu

import (
	"testing"
)

// multiDeviceBus builds a bus with two RAM devices so the single-RAM fast path
// in the core is disabled, matching how a full machine emulator wires things.
func multiDeviceBus(tb testing.TB) (*Bus, *RAM) {
	tb.Helper()
	low := NewRAM(0, 0x10000)
	high := NewRAM(0x800000, 0x1000)
	bus := NewBus(low, high)
	low.Write(Long, 0, 0x1000)
	low.Write(Long, 4, 0x2000)
	return bus, low
}

func newMultiDeviceCPU(tb testing.TB, opts ...Option) (*cpu, *Bus, *RAM) {
	tb.Helper()
	bus, low := multiDeviceBus(tb)
	processor, err := NewCPU(bus, opts...)
	if err != nil {
		tb.Fatalf("NewCPU: %v", err)
	}
	return processor.(*cpu), bus, low
}

// newAliasedFastCPU mirrors how a full machine uses SetFastMemory: the fast
// region and a bus RAM device share the same backing slice, so bus-only paths
// such as Reset() see the same bytes the fast path serves.
func newAliasedFastCPU(tb testing.TB, backing []byte) *cpu {
	tb.Helper()
	low := &RAM{offset: 0, mem: backing}
	high := NewRAM(0x800000, 0x1000)
	bus := NewBus(low, high)
	processor, err := NewCPU(bus, WithDeferredReset())
	if err != nil {
		tb.Fatalf("NewCPU: %v", err)
	}
	c := processor.(*cpu)
	c.SetFastMemory(FastRegion{Base: 0, Mem: backing, WaitStates: 4})
	return c
}

func TestWithDeferredResetDoesNotTouchBus(t *testing.T) {
	// A bus whose reset vector cannot be read yet: NewCPU must not fail.
	empty := NewBus()
	processor, err := NewCPU(empty, WithDeferredReset())
	if err != nil {
		t.Fatalf("NewCPU(WithDeferredReset): %v", err)
	}

	// Interrupt controller is usable before the first Reset.
	if err := processor.RequestInterrupt(3, AutoVector); err != nil {
		t.Fatalf("RequestInterrupt before Reset: %v", err)
	}

	// Now give the bus real memory and reset explicitly.
	ram := NewRAM(0, 0x10000)
	ram.Write(Long, 0, 0x2000)
	ram.Write(Long, 4, 0x1234)
	empty.AddDevice(ram)

	if err := processor.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if got := processor.Registers().PC; got != 0x1234 {
		t.Fatalf("PC after Reset = %06x, want 001234", got)
	}
}

func TestWithDeferredResetDefaultStillResets(t *testing.T) {
	_, _, _ = newMultiDeviceCPU(t) // no option: NewCPU resets, PC comes from vector
}

func TestSetHooksInstallsAndClears(t *testing.T) {
	processor, _, ram := newMultiDeviceCPU(t)

	var traces, buses int
	processor.SetHooks(Hooks{
		Trace: func(TraceInfo) { traces++ },
		Bus:   func(BusAccessInfo) { buses++ },
	})

	got := processor.Hooks()
	if got.Trace == nil || got.Bus == nil || got.Exception != nil {
		t.Fatalf("Hooks() did not round-trip: %+v", got)
	}

	// NOP at the reset PC, then run one instruction.
	ram.Write(Word, processor.regs.PC, 0x4e71)
	if err := processor.RunInstructions(1); err != nil {
		t.Fatalf("RunInstructions: %v", err)
	}
	if traces == 0 || buses == 0 {
		t.Fatalf("hooks not called: traces=%d buses=%d", traces, buses)
	}

	processor.SetHooks(Hooks{})
	if h := processor.Hooks(); h.Trace != nil || h.PreTrace != nil || h.Exception != nil || h.Bus != nil || h.Interrupt != nil {
		t.Fatalf("SetHooks(Hooks{}) did not clear all callbacks")
	}
	before := traces
	ram.Write(Word, processor.regs.PC, 0x4e71)
	if err := processor.RunInstructions(1); err != nil {
		t.Fatalf("RunInstructions: %v", err)
	}
	if traces != before {
		t.Fatalf("trace hook still firing after clear")
	}
}

func TestSetFastRAMServesReadsAndWrites(t *testing.T) {
	processor, _, low := newMultiDeviceCPU(t)

	backing := make([]byte, 0x8000)
	processor.SetFastMemory(FastRegion{Base: 0, Mem: backing, WaitStates: 4})

	// Write through the core -> lands in the caller's slice, not on the bus.
	if err := processor.write(Long, 0x40, 0xdeadbeef); err != nil {
		t.Fatalf("write: %v", err)
	}
	if backing[0x40] != 0xde || backing[0x43] != 0xef {
		t.Fatalf("fast write did not reach backing slice: % x", backing[0x40:0x44])
	}
	if v, _ := low.Read(Long, 0x40); v == 0xdeadbeef {
		t.Fatalf("fast write should not have reached bus RAM")
	}

	// Read back through the core.
	got, err := processor.read(Long, 0x40)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != 0xdeadbeef {
		t.Fatalf("fast read = %08x, want deadbeef", got)
	}
}

func TestSetFastRAMChargesWaitStates(t *testing.T) {
	processor, _, _ := newMultiDeviceCPU(t)
	processor.SetFastMemory(FastRegion{Base: 0, Mem: make([]byte, 0x8000), WaitStates: 4})

	start := processor.Cycles()
	if _, err := processor.read(Word, 0x100); err != nil {
		t.Fatalf("read: %v", err)
	}
	if delta := processor.Cycles() - start; delta != 4 {
		t.Fatalf("word wait states charged = %d, want 4", delta)
	}

	// A long access runs as two word bus cycles, so the penalty doubles.
	start = processor.Cycles()
	if _, err := processor.read(Long, 0x200); err != nil {
		t.Fatalf("read: %v", err)
	}
	if delta := processor.Cycles() - start; delta != 8 {
		t.Fatalf("long wait states charged = %d, want 8", delta)
	}
}

func TestSetFastRAMOutOfWindowFallsThroughToBus(t *testing.T) {
	processor, _, _ := newMultiDeviceCPU(t)
	// Window covers only low 0x8000; the bus low RAM still spans 0x10000 and the
	// high device lives at 0x800000.
	processor.SetFastMemory(FastRegion{Base: 0, Mem: make([]byte, 0x8000)})

	// Address beyond the window but on another bus device.
	if err := processor.write(Word, 0x800000, 0x1234); err != nil {
		t.Fatalf("write to bus device beyond window: %v", err)
	}
	if got, err := processor.read(Word, 0x800000); err != nil || got != 0x1234 {
		t.Fatalf("bus round-trip beyond window = %04x err=%v", got, err)
	}

	// Address beyond the window but still inside the bus low RAM.
	if err := processor.write(Word, 0xC000, 0x5678); err != nil {
		t.Fatalf("write past window into bus low RAM: %v", err)
	}
	if got, err := processor.read(Word, 0xC000); err != nil || got != 0x5678 {
		t.Fatalf("bus low RAM round-trip past window = %04x err=%v", got, err)
	}

	// Access with no backing device anywhere -> bus error via the fall-through.
	if _, err := processor.read(Long, 0x900000); err == nil {
		t.Fatalf("expected bus error for unmapped address past window")
	} else {
		expectBusError(t, err)
	}
}

func TestSetFastRAMMisalignedAccess(t *testing.T) {
	processor, _, _ := newMultiDeviceCPU(t)
	processor.SetFastMemory(FastRegion{Base: 0, Mem: make([]byte, 0x8000)})

	if _, err := processor.read(Word, 0x101); err == nil {
		t.Fatalf("expected AddressError on odd word read")
	} else {
		expectAddressError(t, err)
	}
}

func TestSetFastRAMExecutesInstructions(t *testing.T) {
	backing := make([]byte, 0x10000)
	// reset vector: SSP=0x2000, PC=0x1000
	putLong(backing, 0, 0x2000)
	putLong(backing, 4, 0x1000)
	// 0x1000: MOVEQ #$7f,D0 ; 0x1002: NOP
	putWord(backing, 0x1000, 0x707f)
	putWord(backing, 0x1002, 0x4e71)

	processor := newAliasedFastCPU(t, backing)
	if err := processor.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if processor.regs.PC != 0x1000 {
		t.Fatalf("PC after reset = %06x, want 001000", processor.regs.PC)
	}

	start := processor.Cycles()
	if err := processor.RunInstructions(2); err != nil {
		t.Fatalf("RunInstructions: %v", err)
	}
	if processor.regs.D[0] != 0x7f {
		t.Fatalf("D0 = %08x, want 0000007f", uint32(processor.regs.D[0]))
	}
	if processor.regs.PC != 0x1004 {
		t.Fatalf("PC = %06x, want 001004", processor.regs.PC)
	}
	// Two instruction words fetched from the window -> at least 2*4 wait cycles
	// charged on top of the instruction costs.
	if delta := processor.Cycles() - start; delta < 8 {
		t.Fatalf("cycles advanced %d, expected fetch wait states to be charged", delta)
	}
}

func TestSetFastMemoryReadOnlyRegionRoutesWritesToBus(t *testing.T) {
	processor, _, low := newMultiDeviceCPU(t)

	rom := make([]byte, 0x1000)
	putWord(rom, 0x10, 0xcafe)
	// Map the ROM image over an address the bus low RAM also covers, read-only.
	processor.SetFastMemory(FastRegion{Base: 0x2000, Mem: rom, WaitStates: 4, ReadOnly: true})

	// Reads come from the region.
	if got, err := processor.read(Word, 0x2010); err != nil || got != 0xcafe {
		t.Fatalf("read-only region read = %04x err=%v, want cafe", got, err)
	}

	// Writes fall through to the bus and do not touch the region slice.
	if err := processor.write(Word, 0x2010, 0x1111); err != nil {
		t.Fatalf("write through read-only region: %v", err)
	}
	if rom[0x10] != 0xca || rom[0x11] != 0xfe {
		t.Fatalf("read-only region slice was modified: % x", rom[0x10:0x12])
	}
	if v, _ := low.Read(Word, 0x2010); v != 0x1111 {
		t.Fatalf("write should have reached bus RAM, got %04x", v)
	}
	// Subsequent reads still come from the region, not the bus.
	if got, _ := processor.read(Word, 0x2010); got != 0xcafe {
		t.Fatalf("read-only region read after write = %04x, want cafe", got)
	}
}

func TestSetFastRAMClearedByEmpty(t *testing.T) {
	processor, _, low := newMultiDeviceCPU(t)
	processor.SetFastMemory(FastRegion{Base: 0, Mem: make([]byte, 0x8000)})
	processor.SetFastMemory() // clear

	if err := processor.write(Word, 0x50, 0xabcd); err != nil {
		t.Fatalf("write: %v", err)
	}
	if v, _ := low.Read(Word, 0x50); v != 0xabcd {
		t.Fatalf("after clear, write should reach bus RAM, got %04x", v)
	}
}

func putWord(b []byte, off int, v uint16) {
	b[off] = byte(v >> 8)
	b[off+1] = byte(v)
}

func putLong(b []byte, off int, v uint32) {
	b[off] = byte(v >> 24)
	b[off+1] = byte(v >> 16)
	b[off+2] = byte(v >> 8)
	b[off+3] = byte(v)
}
