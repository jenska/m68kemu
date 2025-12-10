package emu

import (
	"fmt"
)

const (
	Byte Size = 1
	Word Size = 2
	Long Size = 4

	XBusError         = 2
	XAddresError      = 3
	XIllegal          = 4
	XDivByZero        = 5
	XPrivViolation    = 8
	XUninitializedInt = 15
	XTrap             = 32

	srCarry      = 0x0001
	srOverflow   = 0x0002
	srZero       = 0x0004
	srNegative   = 0x0008
	srExtend     = 0x0010
	srSupervisor = 0x2000
)

const (
	eaMaskDataRegister    uint16 = 0x0800
	eaMaskAddressRegister uint16 = 0x0400
	eaMaskIndirect        uint16 = 0x0200
	eaMaskPostIncrement   uint16 = 0x0100
	eaMaskPreDecrement    uint16 = 0x0080
	eaMaskDisplacement    uint16 = 0x0040
	eaMaskIndex           uint16 = 0x0020
	eaMaskAbsoluteShort   uint16 = 0x0010
	eaMaskAbsoluteLong    uint16 = 0x0008
	eaMaskImmediate       uint16 = 0x0004
	eaMaskPCDisplacement  uint16 = 0x0002
	eaMaskPCIndex         uint16 = 0x0001
)

var InstructionTable [0x10000]Instruction

type (
	Size uint32

	Instruction func(*CPU) error

	AddressError uint32
	BusError     uint32

	// AddressBus for accessing address areas
	AddressBus interface {
		Read(s Size, address uint32) (uint32, error)
		Write(s Size, address uint32, value uint32) error
		Reset()
	}

	// Registers represents the programmer visible registers of the 68000 CPU.
	Registers struct {
		D   [8]int32
		A   [8]uint32
		PC  uint32
		SR  uint16
		USP uint32
		IR  uint16 // instruction register
	}

	//  CPU core
	CPU struct {
		regs Registers
		bus  AddressBus
	}
)

func (regs *Registers) String() string {
	// TODO show IR as disassembly
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
	return cpu.regs.String()
}

func (ae AddressError) Error() string {
	return fmt.Sprintln("AddressError at %08x", uint32(ae))
}

func (be BusError) Error() string {
	return fmt.Sprintln("BusError at %08x", uint32(be))
}

func (cpu *CPU) Read(size Size, address uint32) (uint32, error) {
	address &= 0xffffff // 24bit address bus of 68000
	switch size {
	case Byte:
		result, err := cpu.bus.Read(Byte, address)
		return uint32(result), err
	case Word, Long:
		if address&1 != 0 {
			return 0, AddressError(address)
		}
		result, err := cpu.bus.Read(size, address)
		return uint32(result), err
	default:
		return 0, fmt.Errorf("unknown operand size")
	}
}

func (cpu *CPU) Write(size Size, address uint32, value uint32) error {
	address &= 0xffffff // 24bit address bus of 68000
	switch size {
	case Byte:
		return cpu.bus.Write(Byte, address, value)
	case Word, Long:
		if address&1 != 0 {
			return AddressError(address)
		}
		return cpu.bus.Write(size, address, value)
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
	cpu.regs.IR = opcode
	handler := InstructionTable[opcode]
	if handler == nil {
		// TODO 68000 exception handling
		return fmt.Errorf("unknown opcode 0x%04x", opcode)
	}

	if err := handler(cpu); err != nil {
		switch err.(type) {
		case BusError:
			return cpu.Exception(XBusError)
		case AddressError:
			return cpu.Exception(XAddresError)
		default:
			return err
		}

	}
	return nil
}

func (cpu *CPU) Exception(vector uint32) error {
	if vector > 255 {
		return fmt.Errorf("invalid vector %d", vector)
	}

	vectorOffset := vector << 2
	originalSR := cpu.regs.SR

	// TODO: track separate supervisor/user stacks once privilege modes are modeled.
	cpu.regs.SR |= srSupervisor

	// 68000 format 0 stack frame: vector offset (word), PC (long), SR (word).
	if err := cpu.Push(Word, vectorOffset); err != nil {
		return err
	}
	if err := cpu.Push(Long, cpu.regs.PC); err != nil {
		return err
	}
	if err := cpu.Push(Word, uint32(originalSR)); err != nil {
		return err
	}

	handler, err := cpu.readVector(vectorOffset)
	if err != nil {
		return err
	}

	cpu.regs.PC = handler
	return nil
}

func (cpu *CPU) readVector(offset uint32) (uint32, error) {
	if offset&1 != 0 {
		return 0, AddressError(offset)
	}
	if offset >= 256<<2 {
		return 0, AddressError(offset)
	}

	address, err := cpu.Read(Long, offset)
	if err != nil {
		return 0, err
	}
	if address == 0 {
		return cpu.Read(Long, XUninitializedInt<<2)
	}
	return address, nil
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
		cpu.regs.PC += uint32(Word)
		return uint16(opcode), nil
	} else {
		return 0, err
	}
}

func (cpu *CPU) Reset() error {
	cpu.regs = Registers{SR: 0x2700}
	ssp, err := cpu.bus.Read(Long, 0)
	if err != nil {
		return err
	}
	cpu.regs.A[7] = ssp
	pc, err := cpu.bus.Read(Long, 4)
	if err != nil {
		return err
	}
	cpu.regs.PC = pc
	return nil
}

func NewCPU(bus AddressBus) (*CPU, error) {
	cpu := CPU{regs: Registers{SR: 0x2700}, bus: bus}
	if err := cpu.Reset(); err != nil {
		return nil, err
	}
	return &cpu, nil
}

var cnt int

// RegisterInstruction adds an opcode handler to the CPU.
func RegisterInstruction(ins Instruction, match, mask uint16, eaMask uint16) {
	for value := uint16(0); ; {
		index := match | value
		if validEA(index, eaMask) {
			if InstructionTable[index] != nil {
				panic(fmt.Errorf("instruction 0x%04x already registered", index))
			}
			InstructionTable[index] = ins
			cnt++
		}

		value = ((value | mask) + 1) & ^mask
		if value == 0 {
			break
		}
	}
	fmt.Printf("# instructions %d\n", cnt)
}

func validEA(opcode, mask uint16) bool {
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

func (cpu *CPU) Push(s Size, value uint32) error {
	cpu.regs.A[7] -= uint32(s)
	return cpu.Write(s, cpu.regs.A[7], value)
}

func (cpu *CPU) Pop(s Size) (uint32, error) {
	if res, err := cpu.Read(s, cpu.regs.A[7]); err == nil {
		cpu.regs.A[7] += uint32(s) // sometimes odd
		return res, nil
	} else {
		return 0, err
	}
}

func (cpu *CPU) PopPc(s Size) (uint32, error) {
	if res, err := cpu.Read(s, cpu.regs.PC); err == nil {
		cpu.regs.PC += uint32(s)
		if cpu.regs.PC&1 != 0 {
			cpu.regs.PC++ // never odd
		}
		return res, nil

	} else {
		return 0, err
	}
}
