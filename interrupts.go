package m68kemu

import "fmt"

const autoVectorBase = 24

type (
	interruptRequest struct {
		vector     uint8
		autovector bool
	}

	InterruptController struct {
		requests [8]*interruptRequest
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
		ic.requests[level] = &interruptRequest{autovector: true}
		return nil
	}

	ic.requests[level] = &interruptRequest{vector: *vector}
	return nil
}

func (ic *InterruptController) Pending(mask uint16) (uint8, uint32, bool) {
	interruptMask := uint8((mask & srInterruptMask) >> 8)
	if ic.maxLevel <= interruptMask {
		return 0, 0, false
	}

	for level := uint8(7); level > 0; level-- {
		req := ic.requests[level]
		if req == nil {
			continue
		}
		if level <= interruptMask {
			continue
		}

		ic.requests[level] = nil

		// Recalculate maxLevel
		ic.maxLevel = 0
		for l := uint8(7); l > 0; l-- {
			if ic.requests[l] != nil {
				ic.maxLevel = l
				break
			}
		}

		if req.autovector {
			return level, uint32(autoVectorBase + level), true
		}

		return level, uint32(req.vector), true
	}

	return 0, 0, false
}
