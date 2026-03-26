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
