package m68kemu

import "math"

// noEvent is the CPU's nextEvent while no scheduler event is pending.
const noEvent = math.MaxUint64

func NewCycleScheduler() *CycleScheduler {
	return &CycleScheduler{}
}

// SetClockRatio makes scheduler time (Now and event "at" values) run in
// device cycles rather than CPU cycles: n CPU cycles then move scheduler time
// by n*deviceHz/cpuHz, carrying the sub-cycle remainder. Passing 0 for either
// argument, or never calling this, keeps the default 1:1 ratio.
func (s *CycleScheduler) SetClockRatio(deviceHz, cpuHz uint64) {
	if s == nil {
		return
	}
	s.sync()
	if deviceHz == 0 || cpuHz == 0 || deviceHz == cpuHz {
		s.numer, s.denom = 0, 0
	} else {
		g := gcd(deviceHz, cpuHz)
		s.numer, s.denom = deviceHz/g, cpuHz/g
	}
	s.carry = 0
	s.updateDeadline()
}

func gcd(a, b uint64) uint64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// Reset sets scheduler time to now and drops every pending event.
func (s *CycleScheduler) Reset(now uint64) {
	if s == nil {
		return
	}
	s.now = now
	s.carry = 0
	s.events = s.events[:0]
	s.eventHead = 0
	if s.cpu != nil {
		s.mark = s.cpu.cycles
	}
	s.updateDeadline()
}

// toDeviceDelta converts a CPU-cycle delta to a device-cycle delta, retaining
// the fractional remainder across calls.
func (s *CycleScheduler) toDeviceDelta(cpuDelta uint64) uint64 {
	if s.numer == 0 {
		return cpuDelta
	}
	total := cpuDelta*s.numer + s.carry
	s.carry = total % s.denom
	return total / s.denom
}

// Now returns the scheduler time. On an attached scheduler it includes the
// cycles the CPU has run so far, also in the middle of an instruction.
func (s *CycleScheduler) Now() uint64 {
	if s == nil {
		return 0
	}
	s.sync()
	return s.now
}

// Schedule runs fn once scheduler time reaches at. An event at or before the
// current time runs at the next cycle the CPU spends (or the next Advance).
func (s *CycleScheduler) Schedule(at uint64, fn func(now uint64)) {
	if s == nil || fn == nil {
		return
	}
	s.sync()

	event := scheduledEvent{At: at, Fn: fn}
	index := len(s.events)
	s.events = append(s.events, event)
	for index > s.eventHead && s.events[index-1].At > at {
		s.events[index] = s.events[index-1]
		index--
	}
	s.events[index] = event
	if index == s.eventHead {
		s.updateDeadline()
	}
}

// ScheduleAfter runs fn delta scheduler cycles from now.
func (s *CycleScheduler) ScheduleAfter(delta uint64, fn func(now uint64)) {
	if s == nil {
		return
	}
	s.Schedule(s.Now()+delta, fn)
}

// Advance moves scheduler time forward by cpuDelta CPU cycles and runs the
// events that fall due. An attached scheduler advances with its CPU on its
// own; Advance drives a scheduler that is not attached to one.
func (s *CycleScheduler) Advance(cpuDelta uint64) {
	if s == nil || cpuDelta == 0 {
		return
	}
	s.sync()
	s.advance(cpuDelta)
	s.updateDeadline()
}

// runDue brings an attached scheduler up to its CPU's cycle count, running
// the events that have fallen due. The CPU calls it once its counter reaches
// nextEvent.
func (s *CycleScheduler) runDue() {
	delta := s.cpu.cycles - s.mark
	s.mark = s.cpu.cycles
	s.advance(delta)
	s.updateDeadline()
}

// sync brings an attached scheduler's time up to its CPU's cycle count. No
// event can fall due on the way: the CPU would have called runDue.
func (s *CycleScheduler) sync() {
	if s.cpu == nil || s.cpu.cycles == s.mark {
		return
	}
	delta := s.cpu.cycles - s.mark
	s.mark = s.cpu.cycles
	s.now += s.toDeviceDelta(delta)
}

func (s *CycleScheduler) advance(cpuDelta uint64) {
	target := s.now + s.toDeviceDelta(cpuDelta)
	for s.eventHead < len(s.events) && s.events[s.eventHead].At <= target {
		event := s.events[s.eventHead]
		if event.At > s.now {
			s.now = event.At
		}
		s.eventHead++
		event.Fn(s.now)
	}
	if s.now < target {
		s.now = target
	}
	s.compactEvents()
}

// updateDeadline tells the attached CPU the cycle count at which the next
// event falls due.
func (s *CycleScheduler) updateDeadline() {
	if s.cpu == nil {
		return
	}
	if s.eventHead == len(s.events) {
		s.cpu.nextEvent = noEvent
		return
	}
	at := s.events[s.eventHead].At
	if at <= s.now {
		s.cpu.nextEvent = s.mark
		return
	}
	s.cpu.nextEvent = s.mark + s.cpuCyclesUntil(at-s.now)
}

// cpuCyclesUntil returns the fewest CPU cycles that move scheduler time
// forward by deviceDelta (> 0).
func (s *CycleScheduler) cpuCyclesUntil(deviceDelta uint64) uint64 {
	if s.numer == 0 {
		return deviceDelta
	}
	// Smallest n with (n*numer + carry) / denom >= deviceDelta.
	need := deviceDelta*s.denom - s.carry
	return (need + s.numer - 1) / s.numer
}

func (s *CycleScheduler) compactEvents() {
	if s.eventHead == 0 {
		return
	}
	if s.eventHead == len(s.events) {
		s.events = s.events[:0]
		s.eventHead = 0
		return
	}
	if s.eventHead < len(s.events)/2 {
		return
	}

	copy(s.events, s.events[s.eventHead:])
	s.events = s.events[:len(s.events)-s.eventHead]
	s.eventHead = 0
}
