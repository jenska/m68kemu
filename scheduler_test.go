package m68kemu

import "testing"

type countingListener struct {
	delta uint64
	now   uint64
}

func (l *countingListener) AdvanceCycles(delta uint64, now uint64) {
	l.delta += delta
	l.now = now
}

func TestCycleSchedulerAdvancesWithCPU(t *testing.T) {
	cpu, ram := newEnvironment(t)
	scheduler := NewCycleScheduler()
	listener := &countingListener{}
	scheduler.AddListener(listener)
	cpu.SetScheduler(scheduler)

	code := assemble(t, "NOP")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	triggered := false
	scheduler.ScheduleAfter(4, func(now uint64) {
		if now != 4 {
			t.Fatalf("event fired at %d, want 4", now)
		}
		triggered = true
	})

	if err := cpu.Step(); err != nil {
		t.Fatalf("Step failed: %v", err)
	}

	if scheduler.Now() != 4 {
		t.Fatalf("scheduler did not advance with CPU: got %d want 4", scheduler.Now())
	}
	if listener.delta != 4 || listener.now != 4 {
		t.Fatalf("listener saw delta=%d now=%d, want 4/4", listener.delta, listener.now)
	}
	if !triggered {
		t.Fatalf("scheduled event did not fire")
	}
}

func TestCycleSchedulerFiresEventsAtScheduledTimeWithinLargeAdvance(t *testing.T) {
	scheduler := NewCycleScheduler()
	fired := make([]uint64, 0, 2)

	scheduler.ScheduleAfter(4, func(now uint64) {
		fired = append(fired, now)
		scheduler.ScheduleAfter(1, func(now uint64) {
			fired = append(fired, now)
		})
	})

	scheduler.Advance(10)

	if scheduler.Now() != 10 {
		t.Fatalf("scheduler time = %d, want 10", scheduler.Now())
	}
	if len(fired) != 2 {
		t.Fatalf("fired %d events, want 2", len(fired))
	}
	if fired[0] != 4 || fired[1] != 5 {
		t.Fatalf("events fired at %v, want [4 5]", fired)
	}
}

func TestCycleSchedulerClockRatioScalesAndCarries(t *testing.T) {
	scheduler := NewCycleScheduler()
	listener := &countingListener{}
	scheduler.AddListener(listener)

	// Device runs at 2 MHz against an 8 MHz CPU: 4 CPU cycles per device cycle.
	scheduler.SetClockRatio(2_000_000, 8_000_000)

	scheduler.Advance(4)
	if scheduler.Now() != 1 || listener.delta != 1 {
		t.Fatalf("after 4 CPU cycles: now=%d delta=%d, want 1/1", scheduler.Now(), listener.delta)
	}

	// 6 more CPU cycles => 1 device cycle now, 2 CPU cycles carried.
	scheduler.Advance(6)
	if scheduler.Now() != 2 {
		t.Fatalf("after +6 CPU cycles: now=%d, want 2", scheduler.Now())
	}

	// 2 carried + 2 new = 4 => exactly 1 more device cycle.
	scheduler.Advance(2)
	if scheduler.Now() != 3 {
		t.Fatalf("after +2 CPU cycles: now=%d, want 3", scheduler.Now())
	}
}

func TestCycleSchedulerClockRatioEventTiming(t *testing.T) {
	scheduler := NewCycleScheduler()
	scheduler.SetClockRatio(1_000_000, 2_000_000) // 2 CPU cycles per device cycle

	var firedAt uint64 = ^uint64(0)
	scheduler.ScheduleAfter(3, func(now uint64) { firedAt = now })

	scheduler.Advance(5) // 2 device cycles, not yet
	if firedAt != ^uint64(0) {
		t.Fatalf("event fired early at %d", firedAt)
	}
	scheduler.Advance(3) // +1 (carry 1) => device time 3
	if firedAt != 3 {
		t.Fatalf("event fired at %d, want 3", firedAt)
	}
}
