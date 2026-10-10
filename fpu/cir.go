package fpu

import (
	"encoding/binary"

	"github.com/jenska/m68kemu"
)

// CIR offsets of the coprocessor interface registers.
const (
	cirResponse           = 0x00
	cirControl            = 0x02
	cirSave               = 0x04
	cirRestore            = 0x06
	cirOperationWord      = 0x08
	cirCommand            = 0x0a
	cirCondition          = 0x0e
	cirOperand            = 0x10
	cirRegisterSelect     = 0x14
	cirInstructionAddress = 0x18
	cirOperandAddress     = 0x1c
	cirSize               = 0x20
)

// Response primitives. $8900 (busy) and bit 15 (come again) are what real
// 68881 software checks; the other encodings are not verified against the
// MC68881/MC68882 User's Manual (see doc/fpu.md).
const (
	responseIdle       = 0x0802 // null primitive, processing finished
	responseBusy       = 0x8900 // null primitive, come again
	comeAgain          = 0x8000
	toCPU              = 0x2000 // DR: the coprocessor sends data
	transferData       = 0x1000 // evaluate effective address and transfer data
	transferRegister   = 0x0c00 // transfer single main processor register
	transferMultiple   = 0x0100 // transfer multiple coprocessor registers
	responseException  = 0x1c00 // take pre-instruction exception, + vector
	responseCondition  = 0x0800 // null primitive, condition result in bit 0
	fLineVector        = 11
	controlAcknowledge = 0x0002 // control CIR: exception acknowledge (XA)
	controlAbort       = 0x0001 // control CIR: abort (AB)
	restoreInvalid     = 0x0002 // restore CIR after an invalid frame format
)

const (
	phaseIdle = iota
	phaseRegister
	phaseInput
	phaseOutput
	phaseSave
	phaseRestore
)

// CIR is the memory-mapped coprocessor interface of an MC68881 or MC68882,
// as 68000 machines use it: a bus device whose coprocessor interface
// registers sit at Base+$00 to Base+$1F. The Atari Mega ST (SFP004) and Mega
// STE use base $FFFA40.
//
// Software writes a command word to the command CIR, reads the response CIR
// and moves operands through the operand CIR. Every command finishes at
// once, so the response is never busy.
type CIR struct {
	FPU  *FPU
	Base uint32

	response uint16
	phase    int
	op       Operation
	dynamic  uint32
	data     []byte // operand bytes received or still to send
	header   uint32 // FRESTORE frame header

	restoreStatus  uint16
	registerSelect uint16
	operationWord  uint16
	operandAddress uint32
	instruction    uint32
}

// NewCIR returns the coprocessor interface of f at base.
func NewCIR(f *FPU, base uint32) *CIR {
	c := &CIR{FPU: f, Base: base & 0xffffff}
	c.idle()
	return c
}

// AddressRange implements m68kemu.AddressRangeDevice.
func (c *CIR) AddressRange() (uint32, uint32) { return c.Base, c.Base + cirSize - 1 }

// Reset resets the FPU, as the 68000 RESET line does.
func (c *CIR) Reset() {
	c.FPU.Reset()
	c.idle()
}

func (c *CIR) idle() {
	c.phase, c.data, c.response = phaseIdle, nil, responseIdle
}

// Read implements m68kemu.Device.
func (c *CIR) Read(s m68kemu.Size, address uint32) (uint32, error) {
	offset := (address & 0xffffff) - c.Base
	switch {
	case offset == cirOperand || offset == cirOperand+2 || (s == m68kemu.Byte && offset < cirOperand+4 && offset >= cirOperand):
		return c.readOperand(int(s)), nil
	case offset == cirResponse:
		return uint32(c.response), nil
	case offset == cirSave:
		return uint32(c.save()), nil
	case offset == cirRestore:
		return uint32(c.restoreStatus), nil
	case offset == cirRegisterSelect:
		return uint32(c.registerSelect), nil
	case offset == cirOperandAddress:
		return c.operandAddress >> 16, nil
	case offset == cirOperandAddress+2:
		return c.operandAddress & 0xffff, nil
	}
	return 0, nil
}

// Write implements m68kemu.Device.
func (c *CIR) Write(s m68kemu.Size, address uint32, value uint32) error {
	offset := (address & 0xffffff) - c.Base
	v := uint16(value)
	switch {
	case offset == cirOperand || offset == cirOperand+2 || (s == m68kemu.Byte && offset < cirOperand+4 && offset >= cirOperand):
		c.writeOperand(int(s), value)
	case offset == cirCommand:
		c.command(v)
	case offset == cirCondition:
		c.condition(v)
	case offset == cirControl:
		if v&controlAcknowledge != 0 {
			c.FPU.AcknowledgeException()
		}
		if v&controlAbort != 0 {
			c.idle()
		}
	case offset == cirRestore:
		c.restore(v)
	case offset == cirOperationWord:
		c.operationWord = v
	case offset == cirInstructionAddress:
		c.instruction = c.instruction&0xffff | uint32(v)<<16
	case offset == cirInstructionAddress+2:
		c.instruction = c.instruction&0xffff0000 | uint32(v)
		c.FPU.SetInstructionAddress(c.instruction)
	case offset == cirOperandAddress:
		c.operandAddress = c.operandAddress&0xffff | uint32(v)<<16
	case offset == cirOperandAddress+2:
		c.operandAddress = c.operandAddress&0xffff0000 | uint32(v)
	}
	return nil
}

// command starts the general instruction cmd. An enabled exception from an
// earlier instruction is reported first, and the command is not executed.
func (c *CIR) command(cmd uint16) {
	if vector, ok := c.FPU.PendingException(); ok {
		c.phase, c.data = phaseIdle, nil
		c.response = responseException | uint16(vector)
		return
	}
	op, err := Decode(cmd)
	if err != nil {
		c.phase, c.data = phaseIdle, nil
		c.response = responseException | fLineVector
		return
	}
	c.op, c.dynamic, c.data = op, 0, nil
	if op.Register >= 0 {
		c.phase = phaseRegister
		c.response = comeAgain | transferRegister | uint16(op.Register)
		return
	}
	c.start()
}

// start runs the command once the dynamic register value, if any, is known.
func (c *CIR) start() {
	c.op.SetDynamic(c.dynamic)
	multiple := c.op.Command>>13 >= classLoadRegisters
	if multiple {
		c.registerSelect = uint16(c.op.registerList(c.dynamic)) << 8
	}
	if c.op.In > 0 {
		c.phase, c.data = phaseInput, nil
		c.response = comeAgain | transferPrimitive(multiple, c.op.In)
		return
	}
	c.execute(nil)
}

func transferPrimitive(multiple bool, length int) uint16 {
	if multiple {
		return transferMultiple | 12
	}
	return transferData | uint16(length)
}

func (c *CIR) execute(in []byte) {
	out, err := c.FPU.Execute(c.op, c.dynamic, in)
	if err != nil {
		c.phase, c.data = phaseIdle, nil
		c.response = responseException | fLineVector
		return
	}
	if len(out) == 0 {
		c.idle()
		return
	}
	multiple := c.op.Command>>13 >= classLoadRegisters
	c.phase, c.data = phaseOutput, out
	c.response = comeAgain | toCPU | transferPrimitive(multiple, len(out))
}

func (c *CIR) condition(pred uint16) {
	if vector, ok := c.FPU.PendingException(); ok {
		c.response = responseException | uint16(vector)
		return
	}
	result, err := c.FPU.Condition(pred & 0x3f)
	if err != nil {
		c.response = responseException | fLineVector
		return
	}
	c.response = responseCondition
	if result {
		c.response |= 1
	}
}

// writeOperand takes size bytes of value for the current transfer. The
// operand CIR is a byte stream: the 68000 bus splits a long access into two
// word accesses.
func (c *CIR) writeOperand(size int, value uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], value<<(32-8*size))
	c.data = append(c.data, b[:size]...)
	switch c.phase {
	case phaseRegister:
		if len(c.data) >= 4 {
			c.dynamic = binary.BigEndian.Uint32(c.data)
			c.data = nil
			c.start()
		}
	case phaseInput:
		if len(c.data) >= c.op.In {
			c.execute(c.data)
		}
	case phaseRestore:
		if size := int(c.header >> 16 & 0xff); len(c.data) >= size {
			c.finishRestore(append(binary.BigEndian.AppendUint32(nil, c.header), c.data[:size]...))
		}
	default:
		c.data = nil
	}
}

// readOperand returns the next size bytes of the current transfer.
func (c *CIR) readOperand(size int) uint32 {
	if c.phase != phaseOutput && c.phase != phaseSave {
		return 0
	}
	var b [4]byte
	n := copy(b[:size], c.data)
	c.data = c.data[n:]
	if len(c.data) == 0 {
		c.idle()
	}
	return binary.BigEndian.Uint32(b[:]) >> (32 - 8*size)
}

// save starts an FSAVE: it returns the frame's format word (version and
// size) and queues the rest of the frame for the operand CIR.
func (c *CIR) save() uint16 {
	frame := c.FPU.Save()
	header := binary.BigEndian.Uint32(frame)
	if len(frame) > 4 {
		c.phase, c.data = phaseSave, frame[4:]
	} else {
		c.idle()
	}
	return uint16(header >> 16)
}

// restore starts an FRESTORE with the frame's format word. The rest of the
// frame follows through the operand CIR.
func (c *CIR) restore(format uint16) {
	c.header = uint32(format) << 16
	if format>>8 == 0 || format&0xff == 0 {
		c.finishRestore(binary.BigEndian.AppendUint32(nil, c.header))
		return
	}
	c.phase, c.data = phaseRestore, nil
}

func (c *CIR) finishRestore(frame []byte) {
	c.restoreStatus = uint16(c.header >> 16)
	if err := c.FPU.Restore(frame); err != nil {
		c.restoreStatus = restoreInvalid
	}
	c.idle()
}
