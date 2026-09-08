package m68kemu

import "fmt"

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

	interruptController struct {
		requests [8][]pendingInterrupt
		maxLevel uint8
	}
)

func newInterruptController() *interruptController {
	return &interruptController{}
}

func (ic *interruptController) reset() {
	ic.requests = [8][]pendingInterrupt{}
	ic.maxLevel = 0
}

func (ic *interruptController) request(level, vector uint8) error {
	if level > 7 {
		return fmt.Errorf("invalid interrupt level %d", level)
	}
	if level == 0 {
		return nil
	}

	if level > ic.maxLevel {
		ic.maxLevel = level
	}

	if vector == AutoVector {
		ic.requests[level] = append(ic.requests[level], pendingInterrupt{
			vector:     autoVectorBase + level,
			autoVector: true,
		})
		return nil
	}

	ic.requests[level] = append(ic.requests[level], pendingInterrupt{vector: vector})
	return nil
}

// pending pops the highest-priority interrupt above the given SR mask, if any.
func (ic *interruptController) pending(mask uint16) (level uint8, vector uint32, autoVector, ok bool) {
	interruptMask := uint8((mask & srInterruptMask) >> 8)
	if ic.maxLevel <= interruptMask {
		return 0, 0, false, false
	}

	for level := uint8(7); level > 0; level-- {
		queue := ic.requests[level]
		if len(queue) == 0 || level <= interruptMask {
			continue
		}

		interrupt := queue[0]
		ic.requests[level] = queue[1:]

		ic.maxLevel = 0
		for l := uint8(7); l > 0; l-- {
			if len(ic.requests[l]) > 0 {
				ic.maxLevel = l
				break
			}
		}

		return level, uint32(interrupt.vector), interrupt.autoVector, true
	}

	return 0, 0, false, false
}
