package m68kemu

import (
	"github.com/jenska/m68kemu/internal/emu"
)

const (
	Byte = emu.Byte
	Word = emu.Word
	Long = emu.Long
)

type (
	Size       emu.Size
	Registers  emu.Registers
	AddressBus emu.AddressBus

	CPU interface {
		// TODO Registers() Registers
		Step() error
		Reset() error
	}
)

func NewCPU(ab AddressBus) (CPU, error) {
	return emu.NewCPU(ab)
}
