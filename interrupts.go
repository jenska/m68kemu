package m68kemu

import "fmt"

const autoVectorBase = 24

type (
	InterruptController struct {
		requests [8][]uint8
		maxLevel uint8
	}
)

func NewInterruptController() *InterruptController {
	return &InterruptController{}
}

func (ic *InterruptController) Request(level uint8, vector *uint8) error {
	if level > 7 {
		return fmt.Errorf("invalid interrupt level %d", level)
	}
	if level == 0 {
		return nil
	}

	if level > ic.maxLevel {
		ic.maxLevel = level
	}

	if vector == nil {
		ic.requests[level] = append(ic.requests[level], uint8(autoVectorBase+level))
		return nil
	}

	ic.requests[level] = append(ic.requests[level], *vector)
	return nil
}

func (ic *InterruptController) Pending(mask uint16) (uint8, uint32, bool) {
	interruptMask := uint8((mask & srInterruptMask) >> 8)
	if ic.maxLevel <= interruptMask {
		return 0, 0, false
	}

	for level := uint8(7); level > 0; level-- {
		queue := ic.requests[level]
		if len(queue) == 0 {
			continue
		}
		if level <= interruptMask {
			continue
		}

		vector := queue[0]
		ic.requests[level] = queue[1:]

		// Recalculate maxLevel
		ic.maxLevel = 0
		for l := uint8(7); l > 0; l-- {
			if len(ic.requests[l]) > 0 {
				ic.maxLevel = l
				break
			}
		}

		return level, uint32(vector), true
	}

	return 0, 0, false
}
