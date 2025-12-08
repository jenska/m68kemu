package emu

import (
	"fmt"
)

type (
	AddressError uint32
	BusError     uint32

	// AddressBus for accessing address areas
	AddressBus interface {
		ReadLongFrom(address uint32) (uint32, error)
		WriteLongTo(address uint32, value uint32) error
		ReadWordFrom(address uint32) (uint16, error)
		WriteWordTo(address uint32, value uint16) error
		ReadByteFrom(address uint32) (uint8, error)
		WriteByteTo(address uint32, value uint8) error
		Reset()
	}

	// Registers represents the programmer visible registers of the 68000 CPU.
	Registers struct {
		D   [8]int32
		A   [8]uint32
		PC  uint32
		SR  uint16
		USP uint32
	}

	Instruction func(*CPU) error

	//  CPU core
	CPU struct {
		regs Registers
		bus  AddressBus

		ir uint16 // instruction register
	}
)

var instructions [0x10000]Instruction

func (regs *Registers) String() string {
	result := fmt.Sprintf("SR %04x PC %08x USP %08x SP %08x\n", regs.SR, regs.PC, regs.USP, regs.A[7])
	for i := range regs.D {
		result += fmt.Sprintf("D%d %08x ", i, uint32(regs.D[i]))
	}
	result += "\n"
	for i := range regs.A {
		result += fmt.Sprintf("A%d %08x ", i, uint32(regs.A[i]))
	}
	result += "\n"

	return result
}

func (cpu *CPU) String() string {
	// TODO add current instruction
	return cpu.regs.String()
}

func (ae AddressError) Error() string {
	return fmt.Sprintln("AddressError at %08x", uint32(ae))
}

func (be BusError) Error() string {
	return fmt.Sprintln("BusError at %08x", uint32(be))
}

func (cpu *CPU) Read(size *Size, address uint32) (uint32, error) {
	address &= 0xffffff // 24bit address bus of 68000
	// TODO return AddressError if address is odd and size != Byte
	switch size {
	case Byte:
		result, err := cpu.bus.ReadByteFrom(address)
		return uint32(result), err
	case Word:
		result, err := cpu.bus.ReadWordFrom(address)
		return uint32(result), err
	case Long:
		return cpu.bus.ReadLongFrom(address)
	default:
		return 0, fmt.Errorf("unknown operand size")
	}
}

func (cpu *CPU) Write(size *Size, address uint32, value uint32) error {
	address &= 0xffffff // 24bit address bus of 68000
	// TODO return AddressError if address is odd and size != Byte
	switch size {
	case Byte:
		return cpu.bus.WriteByteTo(address, uint8(value))
	case Word:
		return cpu.bus.WriteWordTo(address, uint16(value))
	case Long:
		return cpu.bus.WriteLongTo(address, value)
	default:
		return fmt.Errorf("unknown operand size")

	}
}

func (cpu *CPU) Registers() Registers {
	return cpu.regs
}

// ExecuteInstruction runs an instruction without fetching it from memory. This allows
// callers to execute single instructions directly through the API.
func (cpu *CPU) executeInstruction(opcode uint16) error {
	cpu.ir = opcode
	if handler := instructions[opcode]; handler == nil {
		return fmt.Errorf("unknown opcode 0x%04x", opcode)
	} else {
		return handler(cpu)
	}
}

// Step fetches the next opcode at the program counter and executes it.
func (cpu *CPU) Step() error {
	if opcode, err := cpu.fetchOpcode(); err == nil {
		return cpu.executeInstruction(opcode)
	} else {
		return err
	}
}

func (cpu *CPU) fetchOpcode() (uint16, error) {
	if opcode, err := cpu.Read(Word, cpu.regs.PC); err == nil {
		cpu.regs.PC += uint32(Word.size)
		return uint16(opcode), nil
	} else {
		return 0, err
	}
}

func NewCPU(bus AddressBus, ssp, pc uint32) (*CPU, error) {
	regs := Registers{}
	regs.A[7] = ssp
	regs.PC = pc
	regs.SR = 0x2700
	return &CPU{regs: regs, bus: bus}, nil
}

// RegisterInstruction adds an opcode handler to the CPU.
func RegisterInstruction(ins Instruction, match, mask uint16, eaMask uint16) {
	for value := uint16(0); ; {
		index := match | value
		if validEA(index, eaMask) {
			if instructions[index] != nil {
				panic(fmt.Errorf("instruction 0x%04x already registered", index))
			}
			instructions[index] = ins
		}

		value = ((value | mask) + 1) & ^mask
		if value == 0 {
			break
		}
	}
}

func validEA(opcode, mask uint16) bool {
	const (
		eaMaskDataRegister    = 0x0800
		eaMaskAddressRegister = 0x0400
		eaMaskIndirect        = 0x0200
		eaMaskPostIncrement   = 0x0100
		eaMaskPreDecrement    = 0x0080
		eaMaskDisplacement    = 0x0040
		eaMaskIndex           = 0x0020
		eaMaskAbsoluteShort   = 0x0010
		eaMaskAbsoluteLong    = 0x0008
		eaMaskImmediate       = 0x0004
		eaMaskPCDisplacement  = 0x0002
		eaMaskPCIndex         = 0x0001
	)

	if mask == 0 {
		return true
	}

	switch opcode & 0x3f {
	case 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07:
		return (mask & eaMaskDataRegister) != 0
	case 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f:
		return (mask & eaMaskAddressRegister) != 0
	case 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17:
		return (mask & eaMaskIndirect) != 0
	case 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f:
		return (mask & eaMaskPostIncrement) != 0
	case 0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27:
		return (mask & eaMaskPreDecrement) != 0
	case 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f:
		return (mask & eaMaskDisplacement) != 0
	case 0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37:
		return (mask & eaMaskIndex) != 0
	case 0x38:
		return (mask & eaMaskAbsoluteShort) != 0
	case 0x39:
		return (mask & eaMaskAbsoluteLong) != 0
	case 0x3a:
		return (mask & eaMaskPCDisplacement) != 0
	case 0x3b:
		return (mask & eaMaskPCIndex) != 0
	case 0x3c:
		return (mask & eaMaskImmediate) != 0
	}
	return false
}
