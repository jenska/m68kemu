package m68kemu

import "testing"

type resetWaitDevice struct {
	start      uint32
	end        uint32
	resetCount int
	data       map[uint32]uint32
}

func newResetWaitDevice(start, end uint32) *resetWaitDevice {
	return &resetWaitDevice{
		start: start,
		end:   end,
		data:  make(map[uint32]uint32),
	}
}

func (d *resetWaitDevice) Contains(address uint32) bool {
	return address >= d.start && address <= d.end
}

func (d *resetWaitDevice) Read(_ Size, address uint32) (uint32, error) {
	if !d.Contains(address) {
		return 0, BusError(address)
	}
	return d.data[address], nil
}

func (d *resetWaitDevice) Write(_ Size, address uint32, value uint32) error {
	if !d.Contains(address) {
		return BusError(address)
	}
	d.data[address] = value
	return nil
}

func (d *resetWaitDevice) Reset() {
	d.resetCount++
}

func TestBusDeviceResetAndRangeChecks(t *testing.T) {
	dev := newResetWaitDevice(0x1000, 0x10ff)
	bus := NewBus(dev)

	if err := bus.Write(Byte, 0x1002, 0xaa); err != nil {
		t.Fatalf("bus write failed: %v", err)
	}
	if got, err := bus.Read(Byte, 0x1002); err != nil || got != 0xaa {
		t.Fatalf("bus read = (%02x, %v), want (aa, <nil>)", got, err)
	}

	if err := dev.Write(Byte, 0x1100, 0x55); err == nil {
		t.Fatalf("device write above range unexpectedly succeeded")
	}
	if _, err := dev.Read(Byte, 0x0fff); err == nil {
		t.Fatalf("device read below range unexpectedly succeeded")
	}

	bus.Reset()
	if dev.resetCount != 1 {
		t.Fatalf("resetCount = %d, want 1", dev.resetCount)
	}
}

func TestSchedulerNilAndCompactionPaths(t *testing.T) {
	var nilScheduler *CycleScheduler
	nilScheduler.Reset(12)
	nilScheduler.AddListener(nil)
	nilScheduler.Schedule(5, nil)
	nilScheduler.ScheduleAfter(2, nil)
	nilScheduler.Advance(0)
	if got := nilScheduler.Now(); got != 0 {
		t.Fatalf("nil scheduler Now = %d, want 0", got)
	}

	scheduler := &CycleScheduler{
		events: []ScheduledEvent{{At: 1}, {At: 2}, {At: 3}, {At: 4}},
	}
	scheduler.eventHead = 1
	scheduler.compactEvents()
	if scheduler.eventHead != 1 || len(scheduler.events) != 4 {
		t.Fatalf("compactEvents compacted too early: head=%d len=%d", scheduler.eventHead, len(scheduler.events))
	}

	scheduler.eventHead = 2
	scheduler.compactEvents()
	if scheduler.eventHead != 0 || len(scheduler.events) != 2 {
		t.Fatalf("compactEvents did not compact at threshold: head=%d len=%d", scheduler.eventHead, len(scheduler.events))
	}
	if scheduler.events[0].At != 3 || scheduler.events[1].At != 4 {
		t.Fatalf("compacted events = %+v, want At 3 and 4", scheduler.events)
	}

	scheduler.eventHead = len(scheduler.events)
	scheduler.compactEvents()
	if scheduler.eventHead != 0 || len(scheduler.events) != 0 {
		t.Fatalf("compactEvents did not clear consumed queue: head=%d len=%d", scheduler.eventHead, len(scheduler.events))
	}
}

func TestCPUUtilityHelpersAndFetchFaultCoverage(t *testing.T) {
	cpu, ram := newEnvironment(t)

	scheduler := NewCycleScheduler()
	cpu.SetScheduler(scheduler)
	if cpu.Scheduler() != scheduler {
		t.Fatalf("Scheduler getter did not return installed scheduler")
	}

	if got := AddressError(0x1234).Error(); got != "AddressError at 00001234" {
		t.Fatalf("AddressError string = %q", got)
	}
	if got := BusError(0xabcd).Error(); got != "BusError at 0000abcd" {
		t.Fatalf("BusError string = %q", got)
	}

	handler := uint32(0x5000)
	if err := ram.Write(Long, uint32(XAddressError<<2), handler); err != nil {
		t.Fatalf("install address error vector: %v", err)
	}
	cpu.regs.PC = 0x2001

	if err := cpu.Step(); err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	if cpu.Cycles() != uint64(exceptionCyclesBusAddress) {
		t.Fatalf("cycles after fetch fault = %d, want %d", cpu.Cycles(), exceptionCyclesBusAddress)
	}
	if cpu.regs.PC != handler {
		t.Fatalf("PC after fetch fault = %08x, want %08x", cpu.regs.PC, handler)
	}
	state := cpu.DebugState()
	if !state.LastFault.Valid || state.LastFault.Address != 0x2001 {
		t.Fatalf("fault info = valid:%v addr:%08x, want valid 00002001", state.LastFault.Valid, state.LastFault.Address)
	}
}

func TestReadVectorFallbackAndValidation(t *testing.T) {
	cpu, ram := newEnvironment(t)

	if _, err := cpu.readVector(1); err == nil {
		t.Fatalf("odd vector offset unexpectedly succeeded")
	} else if _, ok := err.(AddressError); !ok {
		t.Fatalf("odd vector offset error = %v, want AddressError", err)
	}
	if _, err := cpu.readVector(256 << 2); err == nil {
		t.Fatalf("out-of-range vector unexpectedly succeeded")
	} else if _, ok := err.(AddressError); !ok {
		t.Fatalf("out-of-range vector error = %v, want AddressError", err)
	}

	fallback := uint32(0x4567)
	if err := ram.Write(Long, uint32(XTrap<<2), 0); err != nil {
		t.Fatalf("clear trap vector: %v", err)
	}
	if err := ram.Write(Long, uint32(XUninitializedInt<<2), fallback); err != nil {
		t.Fatalf("install uninitialized vector: %v", err)
	}

	got, err := cpu.readVector(uint32(XTrap << 2))
	if err != nil {
		t.Fatalf("readVector failed: %v", err)
	}
	if got != fallback {
		t.Fatalf("readVector fallback = %08x, want %08x", got, fallback)
	}

}

func TestRAMZeroLengthReset(t *testing.T) {
	ram := NewRAM(0x4000, 0)
	ram.Reset()
}
