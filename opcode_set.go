package m68kemu

import (
	"fmt"
	"sync"
)

// Model selects the member of the 68000 family a CPU emulates.
type Model int

const (
	M68000 Model = iota
	M68010
	M68020
	M68030
	M68040
	M68060
	modelCount
)

// opcodeSet is the immutable dispatch table for one model, shared by every
// CPU of that model.
type opcodeSet struct {
	handlers [0x10000]instruction
	cycles   [0x10000]uint32
}

// tableBuilder fills an opcodeSet; registrars can branch on model.
type tableBuilder struct {
	model Model
	set   *opcodeSet
}

// registrars fill an opcodeSet in this order, which is the order the init
// functions they replace used to run in. add panics on a slot that is already
// taken, but registerLogicalInstruction skips taken slots and
// registerExtendInstruction, registerExgInstruction and registerMoveUsp
// overwrite them, so the order can matter where those overlap.
var registrars = []func(*tableBuilder){
	registerArithmetic,
	registerCompare,
	registerAddxSubxNegx,
	registerBCD,
	registerClrTst,
	registerLogical,
	registerBitOps,
	registerShiftRotate,
	registerBranches,
	registerJumpLink,
	registerSubroutine,
	registerMoves,
	registerMovem,
	registerMovep,
	registerLeaPea,
	registerSystem,
	registerTrap,
}

var opcodeSets [modelCount]struct {
	once sync.Once
	set  *opcodeSet
}

// opcodesFor returns the dispatch table of model m, building it on first use:
// it runs every registrar, then drops each handler on a word that is not a
// valid opcode for m, so such a word raises the illegal-instruction exception
// (or line A/F) like on the real CPU.
func opcodesFor(m Model) *opcodeSet {
	entry := &opcodeSets[m]
	entry.once.Do(func() {
		b := tableBuilder{model: m, set: new(opcodeSet)}
		for _, register := range registrars {
			register(&b)
		}
		for op := range b.set.handlers {
			if b.set.handlers[op] != nil && op != 0x4AFC && !validFor(m, uint16(op)) {
				b.set.handlers[op] = nil
				b.set.cycles[op] = 0
			}
		}
		entry.set = b.set
	})
	return entry.set
}

// add registers ins for each opcode value that matches the mask and whose
// effective address is allowed by eaMask, and records the precomputed cycle
// count from calc.
func (b *tableBuilder) add(ins instruction, match, mask uint16, eaMask uint16, calc cycleCalculator) {
	for value := uint16(0); ; {
		index := match | value
		if validEA(index, eaMask) {
			if b.set.handlers[index] != nil {
				panic(fmt.Errorf("instruction 0x%04x already registered (existing %p new %p)", index, b.set.handlers[index], ins))
			}
			b.set.handlers[index] = ins
			if calc != nil {
				b.set.cycles[index] = calc(index)
			}
		}

		value = ((value | mask) + 1) & ^mask
		if value == 0 {
			break
		}
	}
}
