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
