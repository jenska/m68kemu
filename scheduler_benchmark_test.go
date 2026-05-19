package m68kemu

import "testing"

func BenchmarkCycleSchedulerAdvanceBurst(b *testing.B) {
	const eventCount = 1024

	b.ReportAllocs()
	b.StopTimer()
	for i := 0; i < b.N; i++ {
		scheduler := NewCycleScheduler()
		for event := 0; event < eventCount; event++ {
			at := uint64(event + 1)
			scheduler.Schedule(at, func(uint64) {})
		}

		b.StartTimer()
		scheduler.Advance(eventCount)
		b.StopTimer()
	}
}

func BenchmarkBusReadMappedRanges(b *testing.B) {
	const (
		deviceCount = 64
		regionSize  = 0x0400
		targetIndex = deviceCount - 1
	)

	devices := make([]Device, 0, deviceCount)
	for i := 0; i < deviceCount; i++ {
		start := uint32(i * regionSize)
		end := start + regionSize - 1
		devices = append(devices, newStubMappedDevice(start, end))
	}

	targetAddress := uint32(targetIndex*regionSize + 0x12)
	targetDevice := devices[targetIndex].(*stubMappedDevice)
	targetDevice.data[targetAddress] = 0x5a

	bus := NewBus(devices...)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value, err := bus.Read(Byte, targetAddress)
		if err != nil {
			b.Fatalf("read failed: %v", err)
		}
		if value != 0x5a {
			b.Fatalf("read = %02x, want 5a", value)
		}
	}
}

func BenchmarkRunEightMillionCycles(b *testing.B) {
	const cycleBudget = 8_000_000
	cpu, ram := newEnvironment(b)
	code := assemble(b, "loop: ADDQ.L #1, D0\nMOVE.L D0, D1\nBRA.S loop")
	for offset, value := range code {
		addr := cpu.regs.PC + uint32(offset)
		if err := ram.Write(Byte, addr, uint32(value)); err != nil {
			b.Fatalf("failed to seed program byte at %04x: %v", addr, err)
		}
	}

	for b.Loop() {
		if err := cpu.RunCycles(cycleBudget); err != nil {
			b.Fatalf("RunCycles failed: %v", err)
		}
	}
}
