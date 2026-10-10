package m68kemu

import (
	"errors"
	"fmt"
)

// Model selects the member of the 68000 family a CPU emulates. Pass it to
// NewCPU with WithModel.
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

// ErrModelUnsupported is returned by NewCPU for a model that is not
// implemented yet. M68000 and M68010 are implemented so far.
var ErrModelUnsupported = errors.New("m68kemu: CPU model not supported")

var modelNames = [modelCount]string{"MC68000", "MC68010", "MC68020", "MC68030", "MC68040", "MC68060"}

func (m Model) String() string {
	if m < 0 || m >= modelCount {
		return fmt.Sprintf("Model(%d)", int(m))
	}
	return modelNames[m]
}

// implemented reports whether NewCPU accepts model m.
func (m Model) implemented() bool {
	return m == M68000 || m == M68010
}

// WithModel selects the CPU model; the default is M68000. NewCPU returns
// ErrModelUnsupported for a model that is not implemented yet.
func WithModel(m Model) Option {
	return func(c *cpuConfig) { c.model = m }
}

// Model returns the model the CPU emulates.
func (cpu *cpu) Model() Model {
	return cpu.model
}
