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

func newMultiDeviceCPU(tb testing.TB, opts ...Option) (*CPU, *Bus, *RAM) {
	tb.Helper()
	bus, low := multiDeviceBus(tb)
	processor, err := NewCPU(bus, opts...)
	if err != nil {
		tb.Fatalf("NewCPU: %v", err)
	}
	return processor, bus, low
}

// newBackedCPU returns a CPU, not yet reset, whose RAM at 0 is backing.
func newBackedCPU(tb testing.TB, backing []byte) *CPU {
	tb.Helper()
	bus := NewBus(&RAM{mem: backing}, NewRAM(0x800000, 0x1000))
	processor, err := NewCPU(bus, WithDeferredReset())
	if err != nil {
		tb.Fatalf("NewCPU: %v", err)
	}
	return processor
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
	if err := processor.Step(); err != nil {
		t.Fatalf("Step: %v", err)
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
	if err := processor.Step(); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if traces != before {
		t.Fatalf("trace hook still firing after clear")
	}
}

// countingMemory is a RAM that counts the calls the bus makes to it, so tests
// can tell direct accesses from bus accesses.
type countingMemory struct {
	*RAM
	reads, writes int
}

func (m *countingMemory) Read(s Size, address uint32) (uint32, error) {
	m.reads++
	return m.RAM.Read(s, address)
}

func (m *countingMemory) Write(s Size, address uint32, value uint32) error {
	m.writes++
	return m.RAM.Write(s, address, value)
}

func newCountingCPU(tb testing.TB) (*CPU, *Bus, *countingMemory) {
	tb.Helper()
	mem := &countingMemory{RAM: NewRAM(0, 0x10000)}
	putLong(mem.Bytes(), 0, 0x2000)
	putLong(mem.Bytes(), 4, 0x1000)
	bus := NewBus(mem, NewRAM(0x800000, 0x1000))
	processor, err := NewCPU(bus)
	if err != nil {
		tb.Fatalf("NewCPU: %v", err)
	}
	mem.reads, mem.writes = 0, 0
	return processor, bus, mem
}

func TestMemoryDeviceIsAccessedDirectly(t *testing.T) {
	processor, _, mem := newCountingCPU(t)

	if err := processor.write(Long, 0x40, 0xdeadbeef); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got, err := processor.read(Long, 0x40); err != nil || got != 0xdeadbeef {
		t.Fatalf("read = (%08x, %v), want deadbeef", got, err)
	}
	if b := mem.Bytes()[0x40:0x44]; b[0] != 0xde || b[3] != 0xef {
		t.Fatalf("write did not reach the backing store: % x", b)
	}
	if mem.reads != 0 || mem.writes != 0 {
		t.Fatalf("bus called the device %d/%d times, want none", mem.reads, mem.writes)
	}
}

func TestMemoryDeviceChargesBusWaitStates(t *testing.T) {
	processor, bus, mem := newCountingCPU(t)
	bus.SetWaitStates(4)

	for _, tt := range []struct {
		size Size
		want uint64
	}{{Byte, 4}, {Word, 4}, {Long, 8}} { // a long runs as two word cycles
		start := processor.Cycles()
		if _, err := processor.read(tt.size, 0x100); err != nil {
			t.Fatalf("read: %v", err)
		}
		if delta := processor.Cycles() - start; delta != tt.want {
			t.Fatalf("size %d: wait states charged = %d, want %d", tt.size, delta, tt.want)
		}
	}
	if mem.reads != 0 {
		t.Fatalf("wait states took the RAM off the direct path")
	}
}

func TestMemoryDeviceMisalignedAccess(t *testing.T) {
	processor, _, _ := newCountingCPU(t)
	if _, err := processor.read(Word, 0x101); err == nil {
		t.Fatalf("expected AddressError on odd word read")
	} else {
		expectAddressError(t, err)
	}
	if err := processor.write(Long, 0x103, 0); err == nil {
		t.Fatalf("expected AddressError on odd long write")
	} else {
		expectAddressError(t, err)
	}
}

func TestMemoryDeviceExecutesInstructions(t *testing.T) {
	processor, bus, mem := newCountingCPU(t)
	bus.SetWaitStates(4)
	putWord(mem.Bytes(), 0x1000, 0x707f) // MOVEQ #$7f,D0
	putWord(mem.Bytes(), 0x1002, 0x4e71) // NOP

	start := processor.Cycles()
	if _, err := processor.RunUntil(RunUntilOptions{MaxInstructions: 2}); err != nil {
		t.Fatalf("RunUntil: %v", err)
	}
	if processor.regs.D[0] != 0x7f || processor.regs.PC != 0x1004 {
		t.Fatalf("D0 = %08x, PC = %06x; want 0000007f, 001004", processor.regs.D[0], processor.regs.PC)
	}
	// MOVEQ 4 + NOP 4, plus 4 wait states for each instruction word.
	if delta := processor.Cycles() - start; delta != 16 {
		t.Fatalf("cycles advanced %d, want 16", delta)
	}
	if mem.reads != 0 {
		t.Fatalf("instruction fetch went through the bus %d times", mem.reads)
	}
}

func TestROMIsReadDirectlyAndRejectsWrites(t *testing.T) {
	image := make([]byte, 0x1000)
	putWord(image, 0x10, 0xcafe)
	rom := NewROM(0x2000, image)
	ram := NewRAM(0, 0x2000)
	putLong(ram.Bytes(), 0, 0x1000)
	putLong(ram.Bytes(), 4, 0x1000)
	processor, err := NewCPU(NewBus(ram, rom))
	if err != nil {
		t.Fatalf("NewCPU: %v", err)
	}
	image[0x10] = 0 // NewROM copied the image

	if got, err := processor.read(Word, 0x2010); err != nil || got != 0xcafe {
		t.Fatalf("ROM read = (%04x, %v), want cafe", got, err)
	}
	if err := processor.write(Word, 0x2010, 0x1111); err == nil {
		t.Fatalf("write to ROM succeeded, want a bus error")
	} else {
		expectBusError(t, err)
	}
	if got, _ := processor.read(Word, 0x2010); got != 0xcafe {
		t.Fatalf("ROM changed by a write: %04x", got)
	}
}

// bankedMemory switches between two backing stores, like a machine that
// pages memory in and out.
type bankedMemory struct {
	*RAM
	banks [2][]byte
}

func (m *bankedMemory) selectBank(i int) { m.RAM.mem = m.banks[i] }

func TestBusRemapPicksUpNewBackingStore(t *testing.T) {
	m := &bankedMemory{RAM: NewRAM(0x10000, 0x100), banks: [2][]byte{make([]byte, 0x100), make([]byte, 0x100)}}
	m.banks[0][0], m.banks[1][0] = 0xaa, 0xbb
	m.selectBank(0)
	ram := NewRAM(0, 0x1000)
	putLong(ram.Bytes(), 0, 0x1000)
	putLong(ram.Bytes(), 4, 0x100)
	bus := NewBus(ram, m)
	processor, err := NewCPU(bus)
	if err != nil {
		t.Fatalf("NewCPU: %v", err)
	}

	if got, _ := processor.read(Byte, 0x10000); got != 0xaa {
		t.Fatalf("bank 0 read = %02x, want aa", got)
	}
	m.selectBank(1)
	bus.Remap()
	if got, _ := processor.read(Byte, 0x10000); got != 0xbb {
		t.Fatalf("bank 1 read after Remap = %02x, want bb", got)
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

// fakeIRQ is a level-sensitive interrupt source for tests.
type fakeIRQ struct {
	level, vector uint8
	acked         []uint8
}

func (f *fakeIRQ) PendingIRQ() (uint8, uint8) { return f.level, f.vector }
func (f *fakeIRQ) AckIRQ(level uint8)         { f.acked = append(f.acked, level) }

func TestIRQSourceTakenWhenUnmaskedAndAcked(t *testing.T) {
	backing := make([]byte, 0x10000)
	putLong(backing, 0, 0x2000)
	putLong(backing, 4, 0x1000)
	putLong(backing, int(autoVectorBase+4)<<2, 0x1500) // level-4 autovector handler
	putWord(backing, 0x1000, 0x4e71)                   // NOP
	putWord(backing, 0x1002, 0x4e71)
	putWord(backing, 0x1500, 0x4e73) // RTE

	c := newBackedCPU(t, backing)
	src := &fakeIRQ{}
	c.SetIRQSource(src)
	if err := c.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	c.regs.SR |= 0x0700 // mask all

	if err := c.Step(); err != nil { // NOP, IRQ masked
		t.Fatalf("Step: %v", err)
	}
	src.level = 4 // assert HBL/VBL-style autovector line
	if err := c.Step(); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if c.regs.PC == 0x1500 {
		t.Fatalf("IRQ taken while masked")
	}

	c.regs.SR &^= 0x0700 // unmask
	if err := c.Step(); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if c.regs.PC != 0x1500 {
		t.Fatalf("IRQ not taken after unmask: PC=%06x", c.regs.PC)
	}
	if len(src.acked) != 1 || src.acked[0] != 4 {
		t.Fatalf("AckIRQ not called with level 4: %v", src.acked)
	}
}

func TestIRQSourceVectoredRequest(t *testing.T) {
	backing := make([]byte, 0x10000)
	putLong(backing, 0, 0x2000)
	putLong(backing, 4, 0x1000)
	putLong(backing, 0x46<<2, 0x1500)
	putWord(backing, 0x1000, 0x4e71)
	putWord(backing, 0x1500, 0x4e73)

	c := newBackedCPU(t, backing)
	src := &fakeIRQ{level: 6, vector: 0x46}
	c.SetIRQSource(src)
	if err := c.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	c.regs.SR &^= 0x0700

	if err := c.Step(); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if c.regs.PC != 0x1500 {
		t.Fatalf("vectored IRQ not taken: PC=%06x", c.regs.PC)
	}
}

func TestSetRegistersAndSetPC(t *testing.T) {
	processor, _, ram := newMultiDeviceCPU(t)

	regs := processor.Registers()
	regs.D[3] = 0xfffffffe
	regs.A[2] = 0x4000
	processor.SetRegisters(regs)
	if got := processor.Registers(); got.D[3] != 0xfffffffe || got.A[2] != 0x4000 {
		t.Fatalf("SetRegisters did not take: D3 %08x A2 %08x", got.D[3], got.A[2])
	}

	if err := ram.Load(0x3000, []byte{0x52, 0x83}); err != nil { // ADDQ.L #1,D3
		t.Fatal(err)
	}
	processor.SetPC(0x3000)
	if err := processor.Step(); err != nil {
		t.Fatal(err)
	}
	if got := processor.Registers(); got.D[3] != 0xffffffff || got.PC != 0x3002 {
		t.Fatalf("after ADDQ: D3 %08x PC %06x, want ffffffff 003002", got.D[3], got.PC)
	}
}

func TestRAMLoadAndBytes(t *testing.T) {
	ram := NewRAM(0x1000, 0x10)
	if err := ram.Load(0x100e, []byte{1, 2}); err != nil {
		t.Fatalf("Load at the end: %v", err)
	}
	if b := ram.Bytes(); b[14] != 1 || b[15] != 2 {
		t.Fatalf("Bytes() = % x", b)
	}
	for _, address := range []uint32{0x100f, 0xfff, 0x2000} {
		if err := ram.Load(address, []byte{1, 2}); err == nil {
			t.Fatalf("Load at %x past the RAM succeeded", address)
		} else {
			expectBusError(t, err)
		}
	}
}
