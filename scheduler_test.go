package m68kemu

import (
	"strings"
	"testing"
)

func TestCycleSchedulerAdvancesWithCPU(t *testing.T) {
	cpu, ram := newEnvironment(t)
	scheduler := NewCycleScheduler()
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
	// Device runs at 2 MHz against an 8 MHz CPU: 4 CPU cycles per device cycle.
	scheduler.SetClockRatio(2_000_000, 8_000_000)

	scheduler.Advance(4)
	if scheduler.Now() != 1 {
		t.Fatalf("after 4 CPU cycles: now=%d, want 1", scheduler.Now())
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

// nopCPU returns a CPU running a loop of 16 NOPs (4 cycles each, so the
// first 64 cycles are NOPs only) and a BRA.
func nopCPU(t *testing.T) *CPU {
	t.Helper()
	cpu, ram := newEnvironment(t)
	code := assemble(t, "loop:"+strings.Repeat(" NOP\n", 16)+" BRA.W loop\n")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}
	return cpu
}

func TestAttachedSchedulerFiresAtCPUCycle(t *testing.T) {
	cpu := nopCPU(t)
	scheduler := NewCycleScheduler()
	cpu.SetScheduler(scheduler)
	scheduler.SetClockRatio(1, 3) // 3 CPU cycles per device cycle

	type firing struct{ now, cycles uint64 }
	var fired []firing
	record := func(now uint64) { fired = append(fired, firing{now, cpu.Cycles()}) }
	scheduler.Schedule(5, func(now uint64) {
		record(now)
		scheduler.ScheduleAfter(6, record)
	})

	if err := cpu.RunCycles(40); err != nil {
		t.Fatal(err)
	}
	// Device time 5 needs 15 CPU cycles, reached by the NOP ending at 16;
	// device time 11 needs 33, reached at 36.
	want := []firing{{5, 16}, {11, 36}}
	if len(fired) != len(want) || fired[0] != want[0] || fired[1] != want[1] {
		t.Fatalf("events fired at %+v, want %+v", fired, want)
	}
	if got, want := scheduler.Now(), cpu.Cycles()/3; got != want {
		t.Fatalf("Now() = %d after %d CPU cycles, want %d", got, cpu.Cycles(), want)
	}
}

func TestAttachedSchedulerNowFollowsCPUWithoutEvents(t *testing.T) {
	cpu := nopCPU(t)
	scheduler := NewCycleScheduler()
	cpu.SetScheduler(scheduler)

	if err := cpu.RunCycles(100); err != nil {
		t.Fatal(err)
	}
	if scheduler.Now() != cpu.Cycles() {
		t.Fatalf("Now() = %d, want the CPU's %d cycles", scheduler.Now(), cpu.Cycles())
	}

	// An event scheduled in the past runs on the next cycle the CPU spends.
	fired := false
	scheduler.Schedule(1, func(uint64) { fired = true })
	if err := cpu.Step(); err != nil {
		t.Fatal(err)
	}
	if !fired {
		t.Fatal("overdue event did not fire on the next instruction")
	}
}

func TestSchedulerMovesBetweenCPUs(t *testing.T) {
	first, second := nopCPU(t), nopCPU(t)
	scheduler := NewCycleScheduler()
	first.SetScheduler(scheduler)
	second.SetScheduler(scheduler)
	if first.Scheduler() != nil || second.Scheduler() != scheduler {
		t.Fatal("scheduler should follow only the CPU it was attached to last")
	}

	fired := 0
	scheduler.ScheduleAfter(8, func(uint64) { fired++ })
	if err := first.RunCycles(40); err != nil {
		t.Fatal(err)
	}
	if fired != 0 {
		t.Fatal("the detached CPU drove the scheduler")
	}
	if err := second.RunCycles(40); err != nil {
		t.Fatal(err)
	}
	if fired != 1 {
		t.Fatalf("event fired %d times, want 1", fired)
	}

	second.SetScheduler(nil)
	if err := second.RunCycles(40); err != nil {
		t.Fatal(err)
	}
}
