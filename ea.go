package m68kemu

import (
	"unsafe"
)

type (
	ea interface {
		init(cpu *cpu, o Size) (modifier, error)
		computedAddress() uint32
		// cycles() int
	}

	modifier interface {
		computedAddress() uint32
		read() (uint32, error)
		write(uint32) error
	}

	eaRegister struct {
		areg func(cpu *cpu) *uint32
		cpu  *cpu
		size Size
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
		index func(cpu *cpu, a uint32) (uint32, error)
	}

	eaAbsolute struct {
		cpu     *cpu
		eaSize  Size
		size    Size
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
		size Size
		sr   *uint16
	}
)

var (
	eaCycleTable = [8][8]uint32{
		{0, 0, 0, 0, 0, 0, 0, 0},         // Dn
		{0, 0, 0, 0, 0, 0, 0, 0},         // An
		{4, 4, 4, 4, 4, 4, 4, 4},         // (An)
		{4, 4, 4, 4, 4, 4, 4, 4},         // (An)+
		{6, 6, 6, 6, 6, 6, 6, 6},         // -(An)
		{8, 8, 8, 8, 8, 8, 8, 8},         // (d16,An)
		{10, 10, 10, 10, 10, 10, 10, 10}, // (d8,An,Xn)
		{8, 12, 8, 10, 0, 0, 0, 0},       // (xxx).W, (xxx).L, (d16,PC), (d8,PC,Xn), #<data>
	}

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

	eaSrc2 = []ea{
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

	eaDst = []ea{
		&eaRegister{areg: udx},
		&eaRegister{areg: ax},
		&eaRegisterIndirect{eaRegister{areg: ax}, 0},
		&eaPostIncrement{eaRegisterIndirect{eaRegister{areg: ax}, 0}},
		&eaPreDecrement{eaRegisterIndirect{eaRegister{areg: ax}, 0}},
		&eaDisplacement{eaRegisterIndirect{eaRegister{areg: ax}, 0}},
		&eaIndirectIndex{eaRegisterIndirect{eaRegister{areg: ax}, 0}, ix68000},
		&eaAbsolute{eaSize: Word},
		&eaAbsolute{eaSize: Long},
		&eaPCDisplacement{eaDisplacement{eaRegisterIndirect{eaRegister{areg: nil}, 0}}},
		&eaPCIndirectIndex{eaIndirectIndex{eaRegisterIndirect{eaRegister{areg: nil}, 0}, ix68000}},
		&eaStatusRegister{},
	}
)

func (cpu *cpu) ResolveSrcEA(o Size) (modifier, error) {
	mode := (cpu.regs.IR >> 3) & 0x07
	if mode < 7 {
		return eaSrc[mode].init(cpu, o)
	}
	return eaSrc[mode+y(cpu.regs.IR)].init(cpu, o)
}

func (cpu *cpu) ResolveSrcEA2(o Size) (modifier, error) {
	mode := (cpu.regs.IR >> 3) & 0x07
	if mode < 7 {
		return eaSrc2[mode].init(cpu, o)
	}
	return eaSrc2[mode+y(cpu.regs.IR)].init(cpu, o)
}

func (cpu *cpu) ResolveDstEA(o Size) (modifier, error) {
	mode := (cpu.regs.IR >> 6) & 0x07
	if mode < 7 {
		return eaDst[mode].init(cpu, o)
	}
	return eaDst[mode+x(cpu.regs.IR)].init(cpu, o)
}

func x(ir uint16) uint16 { return (ir >> 9) & 0x7 }
func y(ir uint16) uint16 { return ir & 0x7 }

func udx(cpu *cpu) *uint32 { return (*uint32)(unsafe.Pointer(&cpu.regs.D[x(cpu.regs.IR)])) }
func dx(cpu *cpu) *int32   { return &cpu.regs.D[x(cpu.regs.IR)] }
func dy(cpu *cpu) *uint32  { return (*uint32)(unsafe.Pointer(&cpu.regs.D[y(cpu.regs.IR)])) }

func ax(cpu *cpu) *uint32 { return &cpu.regs.A[x(cpu.regs.IR)] }
func ay(cpu *cpu) *uint32 { return &cpu.regs.A[y(cpu.regs.IR)] }

// -------------------------------------------------------------------
// register direct

func (ea *eaRegister) init(cpu *cpu, o Size) (modifier, error) {
	ea.cpu, ea.size = cpu, o
	return ea, nil
}

func (ea *eaRegister) read() (uint32, error) {
	return *ea.areg(ea.cpu) & ea.size.mask(), nil
}

func (ea *eaRegister) write(v uint32) error {
	reg := ea.areg(ea.cpu)
	mask := ea.size.mask()
	*reg = (*reg & ^mask) | (v & mask)
	return nil
}

func (ea *eaRegister) computedAddress() uint32 {
	panic("no address in register addressing mode")
}

// -------------------------------------------------------------------
// Address register indirect

func (ea *eaRegisterIndirect) init(cpu *cpu, o Size) (modifier, error) {
	ea.cpu, ea.size, ea.address = cpu, o, *ea.areg(cpu)
	return ea, nil
}

func (ea *eaRegisterIndirect) read() (uint32, error) {
	return ea.cpu.read(ea.size, ea.address)
}

func (ea *eaRegisterIndirect) write(v uint32) error {
	return ea.cpu.write(ea.size, ea.address, v)
}

func (ea *eaRegisterIndirect) computedAddress() uint32 {
	return ea.address
}

// -------------------------------------------------------------------
// Post increment

func (ea *eaPostIncrement) init(cpu *cpu, o Size) (modifier, error) {
	ea.cpu, ea.size, ea.address = cpu, o, *ea.areg(cpu)
	incr := uint32(o)
	if o == Byte && ea.areg(cpu) == &cpu.regs.A[7] {
		incr = 2
	}
	*ea.areg(cpu) += incr
	return ea, nil
}

func (ea *eaPostIncrement) read() (uint32, error) {
	return ea.cpu.read(ea.size, ea.address)
}

func (ea *eaPostIncrement) write(v uint32) error {
	return ea.cpu.write(ea.size, ea.address, v)
}

// -------------------------------------------------------------------
// Pre decrement

func (ea *eaPreDecrement) init(cpu *cpu, o Size) (modifier, error) {
	decr := uint32(o)
	if o == Byte && ea.areg(cpu) == &cpu.regs.A[7] {
		decr = 2
	}
	*ea.areg(cpu) -= decr
	ea.cpu, ea.size, ea.address = cpu, o, *ea.areg(cpu)
	return ea, nil
}

func (ea *eaPreDecrement) read() (uint32, error) {
	return ea.cpu.read(ea.size, ea.address)
}

func (ea *eaPreDecrement) write(v uint32) error {
	return ea.cpu.write(ea.size, ea.address, v)
}

// -------------------------------------------------------------------
// Displacement

func (ea *eaDisplacement) init(cpu *cpu, o Size) (modifier, error) {
	ea.cpu, ea.size = cpu, o
	offset, err := cpu.popPc(Word)
	if err != nil {
		return nil, err
	}
	ea.address = uint32(int32(*ea.areg(cpu)) + int32(int16(offset)))
	return ea, nil
}

func (ea *eaDisplacement) computedAddress() uint32 {
	return ea.address
}

func (ea *eaPCDisplacement) init(cpu *cpu, o Size) (modifier, error) {
	ea.cpu, ea.size = cpu, o
	offset, err := cpu.popPc(Word)
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

func (ea *eaIndirectIndex) init(cpu *cpu, o Size) (modifier, error) {
	ea.cpu, ea.size = cpu, o
	address, err := ea.index(cpu, *ea.areg(cpu))
	if err != nil {
		return nil, err
	}
	ea.address = address
	return ea, nil
}

func (ea *eaPCIndirectIndex) init(cpu *cpu, o Size) (modifier, error) {
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

func (ea *eaAbsolute) init(cpu *cpu, o Size) (modifier, error) {
	ea.cpu, ea.size = cpu, o
	address, err := cpu.popPc(ea.eaSize)
	if err != nil {
		return nil, err
	}
	ea.address = address
	return ea, nil
}

func (ea *eaAbsolute) read() (uint32, error) {
	return ea.cpu.read(ea.size, ea.address)
}

func (ea *eaAbsolute) write(v uint32) error {
	return ea.cpu.write(ea.size, ea.address, v)
}

func (ea *eaAbsolute) computedAddress() uint32 {
	return ea.address
}

// -------------------------------------------------------------------
// immediate

func (ea *eaImmediate) init(cpu *cpu, o Size) (modifier, error) {
	readSize := o
	if o == Byte {
		readSize = Word
	}
	value, err := cpu.popPc(readSize)
	if err != nil {
		return nil, err
	}
	if o == Byte {
		value &= 0xff
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

func (ea *eaStatusRegister) init(cpu *cpu, o Size) (modifier, error) {
	ea.sr = &cpu.regs.SR
	return ea, nil
}

func (ea *eaStatusRegister) read() (uint32, error) {
	return uint32(*ea.sr) & ea.size.mask(), nil
}

func (ea *eaStatusRegister) write(v uint32) error {
	mask := uint16(ea.size.mask())
	*ea.sr = (*ea.sr & ^mask) | (uint16(v) & mask)
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
func ix68000(c *cpu, a uint32) (uint32, error) {
	ext, err := c.popPc(Word)
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

func eaAccessCycles(mode, reg uint16, size Size) uint32 {
	if mode == 7 && reg == 4 { // #<data>
		switch size {
		case Byte, Word:
			return 4
		case Long:
			return 8
		default:
			return 0
		}
	}

	return eaCycleTable[mode][reg]
}
