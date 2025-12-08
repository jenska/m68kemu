package emu

import "unsafe"

type (
	ea interface {
		init(cpu *CPU, o *Size) (modifier, error)
		computedAddress() uint32
		// cycles() int
	}

	modifier interface {
		computedAddress() uint32
		read() (uint32, error)
		write(uint32) error
	}

	eaRegister struct {
		areg func(cpu *CPU) *uint32
		cpu  *CPU
		size *Size
	}

	eaRegisterIndirect struct {
		eaRegister
		address uint32
	}

	eaPostIncrement struct {
		eaRegisterIndirect
	}

	eaPreDecrement struct {
		eaRegisterIndirect
	}

	eaDisplacement struct {
		eaRegisterIndirect
	}

	eaIndirectIndex struct {
		eaRegisterIndirect
		index func(cpu *CPU, a uint32) (uint32, error)
	}

	eaAbsolute struct {
		cpu     *CPU
		eaSize  *Size
		size    *Size
		address uint32
	}

	eaPCDisplacement struct {
		eaDisplacement
	}

	eaPCIndirectIndex struct {
		eaIndirectIndex
	}

	eaImmediate struct {
		value uint32
	}

	eaStatusRegister struct {
		size *Size
		sr   *uint16
	}
)

var (
	eaSrc = []ea{
		&eaRegister{areg: dy},
		&eaRegister{areg: ay},
		&eaRegisterIndirect{eaRegister{areg: ay}, 0},
		&eaPostIncrement{eaRegisterIndirect{eaRegister{areg: ay}, 0}},
		&eaPreDecrement{eaRegisterIndirect{eaRegister{areg: ay}, 0}},
		&eaDisplacement{eaRegisterIndirect{eaRegister{areg: ay}, 0}},
		&eaIndirectIndex{eaRegisterIndirect{eaRegister{areg: ay}, 0}, ix68000},
		&eaAbsolute{eaSize: Word},
		&eaAbsolute{eaSize: Long},
		&eaPCDisplacement{eaDisplacement{eaRegisterIndirect{eaRegister{areg: nil}, 0}}},
		&eaPCIndirectIndex{eaIndirectIndex{eaRegisterIndirect{eaRegister{areg: nil}, 0}, ix68000}},
		&eaImmediate{},
	}

	eaDst = []ea{
		&eaRegister{areg: dy},
		&eaRegister{areg: ay},
		&eaRegisterIndirect{eaRegister{areg: ay}, 0},
		&eaPostIncrement{eaRegisterIndirect{eaRegister{areg: ay}, 0}},
		&eaPreDecrement{eaRegisterIndirect{eaRegister{areg: ay}, 0}},
		&eaDisplacement{eaRegisterIndirect{eaRegister{areg: ay}, 0}},
		&eaIndirectIndex{eaRegisterIndirect{eaRegister{areg: ay}, 0}, ix68000},
		&eaAbsolute{eaSize: Word},
		&eaAbsolute{eaSize: Long},
		&eaPCDisplacement{eaDisplacement{eaRegisterIndirect{eaRegister{areg: nil}, 0}}},
		&eaPCIndirectIndex{eaIndirectIndex{eaRegisterIndirect{eaRegister{areg: nil}, 0}, ix68000}},
		&eaStatusRegister{},
	}
)

// TODO add cycles
func (cpu *CPU) ResolveSrcEA(o *Size) (modifier, error) {
	mode := (cpu.ir >> 3) & 0x07
	if mode < 7 {
		return eaSrc[mode].init(cpu, o)
	}
	return eaSrc[mode+y(cpu.ir)].init(cpu, o)
}

// TODO add cycles
func (cpu *CPU) ResolveDstEA(o *Size) (modifier, error) {
	mode := (cpu.ir >> 3) & 0x07
	if mode < 7 {
		return eaDst[mode].init(cpu, o)
	}
	return eaDst[mode+y(cpu.ir)].init(cpu, o)
}

func (cpu *CPU) Push(s *Size, value uint32) error {
	cpu.regs.A[7] -= s.size
	return cpu.Write(s, cpu.regs.A[7], value)
}

func (cpu *CPU) Pop(s *Size) (uint32, error) {
	if res, err := cpu.Read(s, cpu.regs.A[7]); err == nil {
		cpu.regs.A[7] += s.size // sometimes odd
		return res, nil
	} else {
		return 0, err
	}
}

func (cpu *CPU) PopPc(s *Size) (uint32, error) {
	if res, err := cpu.Read(s, cpu.regs.PC); err == nil {
		cpu.regs.PC += s.align // never odd
		return res, nil

	} else {
		return 0, err
	}
}

func x(ir uint16) uint16 { return (ir >> 9) & 0x7 }
func y(ir uint16) uint16 { return ir & 0x7 }

func dx(cpu *CPU) *uint32 { return (*uint32)(unsafe.Pointer(&cpu.regs.D[x(cpu.ir)])) }
func dy(cpu *CPU) *uint32 { return (*uint32)(unsafe.Pointer(&cpu.regs.D[y(cpu.ir)])) }

func ax(cpu *CPU) *uint32 { return &cpu.regs.A[x(cpu.ir)] }
func ay(cpu *CPU) *uint32 { return &cpu.regs.A[y(cpu.ir)] }

// -------------------------------------------------------------------
// register direct

func (ea *eaRegister) init(cpu *CPU, o *Size) (modifier, error) {
	ea.cpu, ea.size = cpu, o
	return ea, nil
}

func (ea *eaRegister) read() (uint32, error) {
	return *ea.areg(ea.cpu) & ea.size.mask, nil
}

func (ea *eaRegister) write(v uint32) error {
	ea.size.uset(v, ea.areg(ea.cpu))
	return nil
}

func (ea *eaRegister) computedAddress() uint32 {
	panic("no address in register addressing mode")
}

// -------------------------------------------------------------------
// Address register indirect

func (ea *eaRegisterIndirect) init(cpu *CPU, o *Size) (modifier, error) {
	ea.cpu, ea.size, ea.address = cpu, o, *ea.areg(cpu)
	return ea, nil
}

func (ea *eaRegisterIndirect) read() (uint32, error) {
	return ea.cpu.Read(ea.size, ea.address)
}

func (ea *eaRegisterIndirect) write(v uint32) error {
	return ea.cpu.Write(ea.size, ea.address, v)
}

func (ea *eaRegisterIndirect) computedAddress() uint32 {
	return ea.address
}

// -------------------------------------------------------------------
// Post increment

func (ea *eaPostIncrement) init(cpu *CPU, o *Size) (modifier, error) {
	ea.cpu, ea.size, ea.address = cpu, o, *ea.areg(cpu)
	*ea.areg(cpu) += ea.size.size
	return ea, nil
}

func (ea *eaPostIncrement) read() (uint32, error) {
	return ea.cpu.Read(ea.size, ea.address)
}

func (ea *eaPostIncrement) write(v uint32) error {
	return ea.cpu.Write(ea.size, ea.address, v)
}

// -------------------------------------------------------------------
// Pre decrement

func (ea *eaPreDecrement) init(cpu *CPU, o *Size) (modifier, error) {
	*ea.areg(cpu) -= o.size
	ea.cpu, ea.size, ea.address = cpu, o, *ea.areg(cpu)
	return ea, nil
}

func (ea *eaPreDecrement) read() (uint32, error) {
	return ea.cpu.Read(ea.size, ea.address)
}

func (ea *eaPreDecrement) write(v uint32) error {
	return ea.cpu.Write(ea.size, ea.address, v)
}

// -------------------------------------------------------------------
// Displacement

func (ea *eaDisplacement) init(cpu *CPU, o *Size) (modifier, error) {
	ea.cpu, ea.size = cpu, o
	offset, err := cpu.PopPc(Word)
	if err != nil {
		return nil, err
	}
	ea.address = uint32(int32(*ea.areg(cpu)) + int32(int16(offset)))
	return ea, nil
}

func (ea *eaDisplacement) computedAddress() uint32 {
	return ea.address
}

func (ea *eaPCDisplacement) init(cpu *CPU, o *Size) (modifier, error) {
	ea.cpu, ea.size = cpu, o
	offset, err := cpu.PopPc(Word)
	if err != nil {
		return nil, err
	}
	ea.address = uint32(int32(cpu.regs.PC) + int32(int16(offset)))
	return ea, nil
}

func (ea *eaPCDisplacement) computedAddress() uint32 {
	return ea.address
}

// -------------------------------------------------------------------
// Indirect + index

func (ea *eaIndirectIndex) init(cpu *CPU, o *Size) (modifier, error) {
	ea.cpu, ea.size = cpu, o
	address, err := ea.index(cpu, *ea.areg(cpu))
	if err != nil {
		return nil, err
	}
	ea.address = address
	return ea, nil
}

func (ea *eaPCIndirectIndex) init(cpu *CPU, o *Size) (modifier, error) {
	ea.cpu, ea.size = cpu, o
	address, err := ea.index(cpu, cpu.regs.PC)
	if err != nil {
		return nil, err
	}
	ea.address = address
	return ea, nil
}

// -------------------------------------------------------------------
// absolute word and long

func (ea *eaAbsolute) init(cpu *CPU, o *Size) (modifier, error) {
	ea.cpu, ea.size = cpu, o
	address, err := cpu.PopPc(ea.eaSize)
	if err != nil {
		return nil, err
	}
	ea.address = address
	return ea, nil
}

func (ea *eaAbsolute) read() (uint32, error) {
	return ea.cpu.Read(ea.size, ea.address)
}

func (ea *eaAbsolute) write(v uint32) error {
	return ea.cpu.Write(ea.size, ea.address, v)
}

func (ea *eaAbsolute) computedAddress() uint32 {
	return ea.address
}

// -------------------------------------------------------------------
// immediate

func (ea *eaImmediate) init(cpu *CPU, o *Size) (modifier, error) {
	value, err := cpu.PopPc(o)
	if err != nil {
		return nil, err
	}
	ea.value = value
	return ea, nil
}

func (ea *eaImmediate) read() (uint32, error) {
	return ea.value, nil
}

func (ea *eaImmediate) write(v uint32) error {
	panic("write on immediate addressing mode")
}

func (ea *eaImmediate) computedAddress() uint32 {
	panic("no adress in immediate addressing mode")
}

// -------------------------------------------------------------------
// sr

func (ea *eaStatusRegister) init(cpu *CPU, o *Size) (modifier, error) {
	ea.sr = &cpu.regs.SR
	return ea, nil
}

func (ea *eaStatusRegister) read() (uint32, error) {
	if ea.size == Byte {
		return uint32(*ea.sr & 0xff), nil
	}
	return uint32(*ea.sr), nil
}

func (ea *eaStatusRegister) write(v uint32) error {
	if ea.size == Byte {
		*ea.sr = (*ea.sr & 0xff00) | (uint16(v) & 0xff)
	} else {
		*ea.sr = uint16(v)
	}
	return nil
}

func (ea *eaStatusRegister) computedAddress() uint32 {
	panic("no adress in status register addressing mode")
}

// -------------------------------------------------------------------
// Indexed addressing modes are encoded as follows:
//
// Base instruction format:
// F E D C B A 9 8 7 6 | 5 4 3 | 2 1 0
// x x x x x x x x x x | 1 1 0 | BASE REGISTER      (An)
//
// Base instruction format for destination EA in move instructions:
// F E D C | B A 9    | 8 7 6 | 5 4 3 2 1 0
// x x x x | BASE REG | 1 1 0 | X X X X X X       (An)
//
// Brief extension format:
//
//	F  |  E D C   |  B  |  A 9  | 8 | 7 6 5 4 3 2 1 0
//
// D/A | REGISTER | W/L | SCALE | 0 |  DISPLACEMENT
//
// Full extension format:
//
//	F     E D C      B     A 9    8   7    6    5 4       3   2 1 0
//
// D/A | REGISTER | W/L | SCALE | 1 | BS | IS | BD SIZE | 0 | I/IS
// BASE DISPLACEMENT (0, 16, 32 bit)                (bd)
// OUTER DISPLACEMENT (0, 16, 32 bit)               (od)
//
// D/A:     0 = Dn, 1 = An                          (Xn)
// W/L:     0 = W (sign extend), 1 = L              (.SIZE)
// SCALE:   00=1, 01=2, 10=4, 11=8                  (*SCALE)
// BS:      0=add base reg, 1=suppress base reg     (An suppressed)
// IS:      0=add index, 1=suppress index           (Xn suppressed)
// BD SIZE: 00=reserved, 01=NULL, 10=Word, 11=Long  (size of bd)
//
// IS I/IS Operation
// 0  000  No Memory Indirect
// 0  001  indir prex with null outer
// 0  010  indir prex with word outer
// 0  011  indir prex with long outer
// 0  100  reserved
// 0  101  indir postx with null outer
// 0  110  indir postx with word outer
// 0  111  indir postx with long outer
// 1  000  no memory indirect
// 1  001  mem indir with null outer
// 1  010  mem indir with word outer
// 1  011  mem indir with long outer
// 1  100-111  reserved
func ix68000(c *CPU, a uint32) (uint32, error) {
	ext, err := c.PopPc(Word)
	if err != nil {
		return 0, err
	}

	var xn int32
	index := ext >> 12
	if index < 8 {
		xn = c.regs.D[index]
	} else {
		xn = int32(c.regs.A[index-8])
	}
	if (ext & 0x800) == 0 {
		xn = int32(int16(ext))
	}
	return uint32(int32(a) + xn + int32(int8(ext))), nil
}
