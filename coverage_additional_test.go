package m68kemu

import (
	"errors"
	"strings"
	"testing"
)

type resetWaitDevice struct {
	start      uint32
	end        uint32
	wait       uint32
	resetCount int
	data       map[uint32]uint32
}

func newResetWaitDevice(start, end uint32, wait uint32) *resetWaitDevice {
	return &resetWaitDevice{
		start: start,
		end:   end,
		wait:  wait,
		data:  make(map[uint32]uint32),
	}
}

func (d *resetWaitDevice) AddressRange() (uint32, uint32) {
	return d.start, d.end
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

func (d *resetWaitDevice) WaitStates(_ Size, _ uint32) uint32 {
	return d.wait
}

func TestBusAddDeviceAndMappedHelpers(t *testing.T) {
	dev := newResetWaitDevice(0x1000, 0x10ff, 7)
	mapped := MapDevice(0x1000, 0x10ff, dev)
	bus := NewBus()
	bus.AddDevice(mapped)

	if err := bus.Write(Byte, 0x1002, 0xaa); err != nil {
		t.Fatalf("bus write failed: %v", err)
	}
	if got, err := bus.Read(Byte, 0x1002); err != nil || got != 0xaa {
		t.Fatalf("bus read = (%02x, %v), want (aa, <nil>)", got, err)
	}

	wsDev, ok := mapped.(WaitStateDevice)
	if !ok {
		t.Fatalf("mapped device does not expose wait states")
	}
	if got := wsDev.WaitStates(Byte, 0x1002); got != 7 {
		t.Fatalf("WaitStates = %d, want 7", got)
	}

	peekable, ok := mapped.(interface {
		Peek(Size, uint32) (uint32, error)
	})
	if !ok {
		t.Fatalf("mapped device does not expose Peek")
	}
	if _, err := peekable.Peek(Byte, 0x1002); err == nil || !strings.Contains(err.Error(), "peek unsupported") {
		t.Fatalf("Peek error = %v, want unsupported peek error", err)
	}
	if _, err := peekable.Peek(Byte, 0x0fff); err == nil {
		t.Fatalf("Peek below range unexpectedly succeeded")
	} else {
		var be BusError
		if !errors.As(err, &be) {
			t.Fatalf("Peek below range error = %v, want BusError", err)
		}
	}

	if err := mapped.Write(Byte, 0x1100, 0x55); err == nil {
		t.Fatalf("mapped write above range unexpectedly succeeded")
	}
	if _, err := mapped.Read(Byte, 0x0fff); err == nil {
		t.Fatalf("mapped read below range unexpectedly succeeded")
	}

	mapped.Reset()
	if dev.resetCount != 1 {
		t.Fatalf("resetCount = %d, want 1", dev.resetCount)
	}
}

func TestBusPeekLongAcrossAdjacentDevices(t *testing.T) {
	low := NewRAM(0x0000, 0x0004)
	high := NewRAM(0x0004, 0x0004)
	bus := NewBus()
	bus.AddDevice(low)
	bus.AddDevice(high)

	if err := low.Write(Word, 0x0002, 0xaabb); err != nil {
		t.Fatalf("seed low word: %v", err)
	}
	if err := high.Write(Word, 0x0004, 0xccdd); err != nil {
		t.Fatalf("seed high word: %v", err)
	}

	got, err := bus.Peek(Long, 0x0002)
	if err != nil {
		t.Fatalf("Peek long failed: %v", err)
	}
	if got != 0xaabbccdd {
		t.Fatalf("Peek long = %08x, want aabbccdd", got)
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
	if got := (BreakpointHit{Address: 0x10, Type: BreakpointWrite}).Error(); got != "breakpoint hit at 00000010 (write)" {
		t.Fatalf("BreakpointHit string = %q", got)
	}
	if got := BreakpointType(99).String(); got != "unknown" {
		t.Fatalf("BreakpointType unknown string = %q", got)
	}
	if got := RunStopReason(99).String(); got != "none" {
		t.Fatalf("RunStopReason default string = %q", got)
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
	if !state.HasException || state.LastException.Vector != XAddressError || !state.LastException.Group0 {
		t.Fatalf("unexpected exception state: %+v", state.LastException)
	}
	if !state.LastException.FaultValid || state.LastException.FaultAddress != 0x2001 {
		t.Fatalf("fault info = valid:%v addr:%08x, want valid 00002001", state.LastException.FaultValid, state.LastException.FaultAddress)
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

	if _, err := ReadExceptionStackFrame(cpu.bus, 0x1000, ExceptionStackFrameFormat(99)); err == nil {
		t.Fatalf("unknown exception stack frame format unexpectedly succeeded")
	}
}

func TestVerboseHelperFallbackPaths(t *testing.T) {
	cpu, ram := newEnvironment(t)

	code := assemble(t, "MOVEQ #5,D0\n")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("seed code: %v", err)
		}
	}

	line, err := traceDisassemblyLine(cpu.bus, TraceInfo{
		PC:       cpu.regs.PC,
		Bytes:    append([]byte(nil), code...),
		Mnemonic: "MOVEQ #5, D0",
	})
	if err != nil {
		t.Fatalf("traceDisassemblyLine with mnemonic failed: %v", err)
	}
	if line.Assembly != "MOVEQ #5, D0" {
		t.Fatalf("traceDisassemblyLine assembly = %q", line.Assembly)
	}

	line, err = traceDisassemblyLine(cpu.bus, TraceInfo{
		PC:    cpu.regs.PC,
		Bytes: append([]byte(nil), code...),
	})
	if err != nil {
		t.Fatalf("traceDisassemblyLine decode-bytes failed: %v", err)
	}
	if line.Assembly == "" {
		t.Fatalf("traceDisassemblyLine decode-bytes returned empty assembly")
	}

	if got := traceInstructionMnemonic(cpu.bus, cpu.regs.PC, append([]byte(nil), code...)); got != "MOVEQ #5, D0" {
		t.Fatalf("traceInstructionMnemonic(bytes) = %q", got)
	}
	if got := traceInstructionMnemonic(cpu.bus, cpu.regs.PC, nil); got != "MOVEQ #5, D0" {
		t.Fatalf("traceInstructionMnemonic(fallback) = %q", got)
	}

	if got := formatMemoryRangeLabel(MemoryRange{Label: "program"}); got != "program" {
		t.Fatalf("formatMemoryRangeLabel(label) = %q", got)
	}
	if got := formatMemoryRangeLabel(MemoryRange{Start: 0x2000}); got != "00002000" {
		t.Fatalf("formatMemoryRangeLabel(single) = %q", got)
	}
	if got := formatMemoryRangeLabel(MemoryRange{Start: 0x2000, Length: 0x10}); got != "00002000-0000200f" {
		t.Fatalf("formatMemoryRangeLabel(range) = %q", got)
	}
}

func TestRAMZeroLengthAddressRangeAndReset(t *testing.T) {
	ram := NewRAM(0x4000, 0)
	start, end := ram.AddressRange()
	if start != 0x4000 || end != 0x4000 {
		t.Fatalf("AddressRange for empty RAM = (%08x, %08x), want (00004000, 00004000)", start, end)
	}

	ram.Reset()
}
