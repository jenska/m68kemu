package m68kemu

func NewCycleScheduler() *CycleScheduler {
	return &CycleScheduler{}
}

// SetClockRatio makes scheduler time (Now, listener deltas, and event "at"
// values) run in device cycles rather than CPU cycles: an Advance of n CPU
// cycles then moves scheduler time by n*deviceHz/cpuHz, carrying the sub-cycle
// remainder. Passing 0 for either argument, or never calling this, keeps the
// default 1:1 ratio.
func (s *CycleScheduler) SetClockRatio(deviceHz, cpuHz uint64) {
	if s == nil {
		return
	}
	if deviceHz == 0 || cpuHz == 0 {
		s.numer, s.denom = 0, 0
	} else {
		s.numer, s.denom = deviceHz, cpuHz
	}
	s.carry = 0
}

func (s *CycleScheduler) Reset(now uint64) {
	if s == nil {
		return
	}
	s.now = now
	s.carry = 0
	s.events = s.events[:0]
	s.eventHead = 0
}

// toDeviceDelta converts a CPU-cycle delta to a device-cycle delta, retaining
// the fractional remainder across calls.
func (s *CycleScheduler) toDeviceDelta(cpuDelta uint64) uint64 {
	if s.numer == 0 || s.denom == 0 || s.numer == s.denom {
		return cpuDelta
	}
	total := cpuDelta*s.numer + s.carry
	s.carry = total % s.denom
	return total / s.denom
}

func (s *CycleScheduler) Now() uint64 {
	if s == nil {
		return 0
	}
	return s.now
}

func (s *CycleScheduler) AddListener(listener CycleListener) {
	if s == nil || listener == nil {
		return
	}
	s.listeners = append(s.listeners, listener)
}

func (s *CycleScheduler) Schedule(at uint64, fn func(now uint64)) {
	if s == nil || fn == nil {
		return
	}

	event := scheduledEvent{At: at, Fn: fn}
	index := len(s.events)
	s.events = append(s.events, event)
	for index > s.eventHead && s.events[index-1].At > at {
		s.events[index] = s.events[index-1]
		index--
	}
	s.events[index] = event
}

func (s *CycleScheduler) ScheduleAfter(delta uint64, fn func(now uint64)) {
	if s == nil {
		return
	}
	s.Schedule(s.now+delta, fn)
}

func (s *CycleScheduler) Advance(cpuDelta uint64) {
	if s == nil || cpuDelta == 0 {
		return
	}

	delta := s.toDeviceDelta(cpuDelta)
	if delta == 0 {
		return
	}

	target := s.now + delta
	for s.eventHead < len(s.events) && s.events[s.eventHead].At <= target {
		event := s.events[s.eventHead]
		if event.At > s.now {
			s.advanceTo(event.At)
		}
		s.eventHead++
		event.Fn(s.now)
	}

	if s.now < target {
		s.advanceTo(target)
	}
	s.compactEvents()
}

func (s *CycleScheduler) advanceTo(target uint64) {
	if target <= s.now {
		return
	}

	delta := target - s.now
	s.now = target
	for _, listener := range s.listeners {
		listener.AdvanceCycles(delta, s.now)
	}
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
