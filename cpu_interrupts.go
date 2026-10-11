package m68kemu

import (
	"fmt"
	"sync"
	"sync/atomic"
)

const autoVectorBase = 24

// AutoVector is the vector value that asks the CPU to auto-vector an interrupt
// request (using vector 24+level) instead of taking a device-supplied vector.
// It is the zero value, so a request with no explicit vector auto-vectors.
const AutoVector uint8 = 0

type (
	pendingInterrupt struct {
		vector     uint8
		autoVector bool
	}

	// interruptController queues the requests made through RequestInterrupt.
	// Other goroutines may add requests while the CPU runs: mu guards the
	// queues, and maxLevel, the highest queued level, is atomic so the CPU
	// can check it after every instruction without taking the lock.
	interruptController struct {
		mu       sync.Mutex
		requests [8][]pendingInterrupt
		maxLevel atomic.Uint32
	}
)

func newInterruptController() *interruptController {
	return &interruptController{}
}

func (ic *interruptController) reset() {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.requests = [8][]pendingInterrupt{}
	ic.maxLevel.Store(0)
}

// above reports whether a request above the interrupt mask (0-7) is queued.
func (ic *interruptController) above(mask uint8) bool {
	return ic.maxLevel.Load() > uint32(mask)
}

func (ic *interruptController) request(level, vector uint8) error {
	if level > 7 {
		return fmt.Errorf("invalid interrupt level %d", level)
	}
	if level == 0 {
		return nil
	}

	interrupt := pendingInterrupt{vector: vector}
	if vector == AutoVector {
		interrupt = pendingInterrupt{vector: autoVectorBase + level, autoVector: true}
	}

	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.requests[level] = append(ic.requests[level], interrupt)
	if uint32(level) > ic.maxLevel.Load() {
		ic.maxLevel.Store(uint32(level))
	}
	return nil
}

// pending pops the highest-priority interrupt above the given SR mask, if any.
func (ic *interruptController) pending(mask uint16) (level uint8, vector uint32, autoVector, ok bool) {
	interruptMask := uint8((mask & srInterruptMask) >> 8)
	if !ic.above(interruptMask) {
		return 0, 0, false, false
	}

	ic.mu.Lock()
	defer ic.mu.Unlock()

	for level := uint8(7); level > 0; level-- {
		queue := ic.requests[level]
		if len(queue) == 0 || level <= interruptMask {
			continue
		}

		interrupt := queue[0]
		ic.requests[level] = queue[1:]

		highest := uint32(0)
		for l := uint8(7); l > 0; l-- {
			if len(ic.requests[l]) > 0 {
				highest = uint32(l)
				break
			}
		}
		ic.maxLevel.Store(highest)

		return level, uint32(interrupt.vector), interrupt.autoVector, true
	}

	return 0, 0, false, false
}
