package m68kemu

import (
	"github.com/jenska/m68kemu/internal/emu"
)

const (
	Byte              = emu.Byte
	Word              = emu.Word
	Long              = emu.Long
	BreakpointExecute = emu.BreakpointExecute
	BreakpointRead    = emu.BreakpointRead
	BreakpointWrite   = emu.BreakpointWrite
)

type (
	Size            = emu.Size
	Registers       = emu.Registers
	AddressBus      = emu.AddressBus
	Breakpoint      = emu.Breakpoint
	BreakpointEvent = emu.BreakpointEvent
	BreakpointType  = emu.BreakpointType
	TraceInfo       = emu.TraceInfo
	TraceCallback   = emu.TraceCallback
	BreakpointHit   = emu.BreakpointHit

	CPU interface {
		Registers() Registers
		Step() error
		Reset() error
		SetTracer(TraceCallback)
		AddBreakpoint(Breakpoint)
		RequestInterrupt(level uint8, vector *uint8) error
	}
)

func NewCPU(ab AddressBus) (CPU, error) {
	return emu.NewCPU(ab)
}
