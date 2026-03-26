package m68kemu

func NewCycleScheduler() *CycleScheduler {
	return &CycleScheduler{}
}

func (s *CycleScheduler) Reset(now uint64) {
	if s == nil {
		return
	}
	s.now = now
	s.events = s.events[:0]
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

	event := ScheduledEvent{At: at, Fn: fn}
	index := len(s.events)
	s.events = append(s.events, event)
	for index > 0 && s.events[index-1].At > at {
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

func (s *CycleScheduler) Advance(delta uint64) {
	if s == nil || delta == 0 {
		return
	}

	target := s.now + delta
	for len(s.events) > 0 && s.events[0].At <= target {
		event := s.events[0]
		if event.At > s.now {
			s.advanceTo(event.At)
		}
		copy(s.events, s.events[1:])
		s.events = s.events[:len(s.events)-1]
		event.Fn(s.now)
	}

	if s.now < target {
		s.advanceTo(target)
	}
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
