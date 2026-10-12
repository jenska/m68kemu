package m68kemu

import (
	"errors"

	"github.com/jenska/m68kemu/mmu"
)

// MMU returns the paged memory management unit of an MC68030, or nil for a
// model without one. The CPU translates every access through it once its TC
// register enables translation, and executes PMOVE, PFLUSH, PLOAD and PTEST
// on it.
func (cpu *CPU) MMU() *mmu.MC68030 { return cpu.mmu }

// tableSearchCycles is the time of one descriptor access during a table
// search. The real cost depends on the bus; this is an approximation.
const tableSearchCycles = 4

// physicalMemory gives the MMU access to the bus without translation.
type physicalMemory struct{ cpu *CPU }

func (p physicalMemory) ReadLong(address uint32) (uint32, error) {
	p.cpu.addCycles(tableSearchCycles)
	return p.cpu.bus.Read(Long, address)
}

func (p physicalMemory) WriteLong(address uint32, value uint32) error {
	p.cpu.addCycles(tableSearchCycles)
	return p.cpu.bus.Write(Long, address, value)
}

// refreshTranslation updates cpu.translating after the MMU's state may have
// changed and drops the fast pages, which may hold translations of the old
// state.
func (cpu *CPU) refreshTranslation() {
	cpu.translating = cpu.mmu != nil && cpu.mmu.Enabled()
	cpu.code, cpu.data = fastPage{}, fastPage{}
}

// readTranslated is readCtx while the MMU translates. It splits accesses at
// odd addresses and across page boundaries, so each part is translated on
// its own, and points the fast pages at the translated page.
func (cpu *CPU) readTranslated(size Size, address uint32, ctx accessContext) (uint32, error) {
	address &= cpu.addrMask
	if size != Byte && address&1 != 0 {
		if !ctx.programSpace() {
			return cpu.readSplit(size, address, ctx)
		}
		return 0, cpu.addressFault(address, ctx, size)
	}
	if cpu.breakpoints != nil {
		if err := cpu.checkAccessBreakpoint(address, BreakpointRead); err != nil {
			return 0, err
		}
	}
	t, ok := cpu.mmu.Translate(address, uint8(ctx.functionCode), false)
	if !ok {
		cpu.recordFault(address, ctx, size, 0)
		return 0, BusError(address)
	}
	if size == Long && address&t.Mask > t.Mask-3 {
		return cpu.readSplit(size, address, ctx)
	}
	var result uint32
	if idx, page, ok := cpu.translatedIndex(size, address, t, ctx); ok {
		result = page.read(size, idx)
		if page.wait != 0 {
			cpu.addCycles(page.waitFor(size))
		}
	} else {
		value, err := cpu.bus.Read(size, t.Physical)
		if err != nil {
			cpu.recordFault(address, ctx, size, 0)
			return 0, BusError(address)
		}
		result = value
	}
	if cpu.shouldTraceBusAccess(ctx) {
		cpu.traceBusAccess(size, address, result, ctx)
	}
	return result, nil
}

// writeTranslated is the write counterpart of readTranslated.
func (cpu *CPU) writeTranslated(size Size, address uint32, value uint32, ctx accessContext) error {
	address &= cpu.addrMask
	if size != Byte && address&1 != 0 {
		return cpu.writeSplit(size, address, value, ctx)
	}
	if cpu.breakpoints != nil {
		if err := cpu.checkAccessBreakpoint(address, BreakpointWrite); err != nil {
			return err
		}
	}
	t, ok := cpu.mmu.Translate(address, uint8(ctx.functionCode), true)
	if !ok {
		cpu.recordFault(address, ctx, size, value)
		return BusError(address)
	}
	if size == Long && address&t.Mask > t.Mask-3 {
		return cpu.writeSplit(size, address, value, ctx)
	}
	if idx, page, ok := cpu.translatedIndex(size, address, t, ctx); ok && !page.readOnly {
		page.write(size, idx, value)
		if page.wait != 0 {
			cpu.addCycles(page.waitFor(size))
		}
	} else if err := cpu.bus.Write(size, t.Physical, value); err != nil {
		cpu.recordFault(address, ctx, size, value)
		return BusError(address)
	}
	if cpu.shouldTraceBusAccess(ctx) {
		cpu.traceBusAccess(size, address, value, ctx)
	}
	return nil
}

// readSplit reads a word or long that may cross a page boundary in parts
// that do not: bytes and words at an odd address, as readMisaligned does,
// or two words.
func (cpu *CPU) readSplit(size Size, address uint32, ctx accessContext) (uint32, error) {
	if address&1 == 0 { // a long across a page boundary
		hi, err := cpu.readTranslated(Word, address, ctx)
		if err != nil {
			return 0, err
		}
		lo, err := cpu.readTranslated(Word, address+2, ctx)
		return hi<<16 | lo, err
	}
	hi, err := cpu.readTranslated(Byte, address, ctx)
	if err != nil {
		return 0, err
	}
	if size == Word {
		lo, err := cpu.readTranslated(Byte, address+1, ctx)
		return hi<<8 | lo, err
	}
	mid, err := cpu.readTranslated(Word, address+1, ctx)
	if err != nil {
		return 0, err
	}
	lo, err := cpu.readTranslated(Byte, address+3, ctx)
	return hi<<24 | mid<<8 | lo, err
}

// writeSplit is the write counterpart of readSplit.
func (cpu *CPU) writeSplit(size Size, address, value uint32, ctx accessContext) error {
	if address&1 == 0 {
		if err := cpu.writeTranslated(Word, address, value>>16, ctx); err != nil {
			return err
		}
		return cpu.writeTranslated(Word, address+2, value, ctx)
	}
	if size == Word {
		if err := cpu.writeTranslated(Byte, address, value>>8, ctx); err != nil {
			return err
		}
		return cpu.writeTranslated(Byte, address+1, value, ctx)
	}
	if err := cpu.writeTranslated(Byte, address, value>>24, ctx); err != nil {
		return err
	}
	if err := cpu.writeTranslated(Word, address+1, value>>8, ctx); err != nil {
		return err
	}
	return cpu.writeTranslated(Byte, address+3, value, ctx)
}

// addressFault records an address error for an odd instruction fetch.
func (cpu *CPU) addressFault(address uint32, ctx accessContext, size Size) error {
	cpu.recordFault(address, ctx, size, 0)
	return AddressError(address)
}

// translatedIndex returns the offset of a size-byte access at address into
// the page translatedPage returns for it.
func (cpu *CPU) translatedIndex(size Size, address uint32, t mmu.Translation, ctx accessContext) (uint32, *fastPage, bool) {
	page, ok := cpu.translatedPage(address, t, ctx)
	if !ok {
		return 0, nil, false
	}
	idx, ok := page.index(size, address)
	return idx, &page, ok
}

// translatedPage returns the direct memory that backs the translated block
// around address, as a fast page with logical addresses, and makes it the
// data or code page when the access uses the function code the fast path
// assumes. ok is false when no direct memory backs the physical address.
func (cpu *CPU) translatedPage(address uint32, t mmu.Translation, ctx accessContext) (fastPage, bool) {
	p := cpu.bus.fastPageFor(t.Physical & cpu.addrMask)
	if _, ok := p.index(Byte, t.Physical); !ok {
		return fastPage{}, false
	}
	physBase := t.Physical &^ t.Mask
	start := max(physBase, p.base)
	end := min(physBase+t.Mask, p.base+uint32(len(p.mem))-1) // inclusive
	page := fastPage{
		mem:      p.mem[start-p.base : end-p.base+1],
		base:     (address &^ t.Mask) + (start - physBase),
		wait:     p.wait,
		readOnly: p.readOnly || !t.Writable,
	}
	switch {
	case ctx.programSpace() && ctx.functionCode == cpu.programFunctionCode():
		if cpu.fastFetch {
			cpu.code = page
		}
	case !ctx.programSpace() && ctx.functionCode == cpu.dataFunctionCode():
		cpu.data = page
	}
	return page, true
}

// mmuInstruction runs an F-line instruction with coprocessor ID 0 on a CPU
// with an MMU: PMOVE, PFLUSH, PLOAD or PTEST. They are privileged; other
// instructions take the line-F exception.
func (cpu *CPU) mmuInstruction(instructionPC uint32) error {
	if ok, err := cpu.requireSupervisor(); !ok {
		return err
	}
	if (cpu.regs.IR>>6)&7 != 0 {
		return cpu.opcodeException(XLineF, instructionPC)
	}
	cmd, err := cpu.popPc(Word)
	if err != nil {
		return err
	}
	ok, err := cpu.mmuCommand(uint16(cmd))
	cpu.refreshTranslation()
	if err != nil || ok {
		return err
	}
	return cpu.opcodeException(XLineF, instructionPC)
}

// mmuCommand decodes and runs the command word of an MMU instruction. ok is
// false for an encoding the MMU does not implement.
func (cpu *CPU) mmuCommand(cmd uint16) (ok bool, err error) {
	switch {
	case cmd&0xe0ff == 0x4000 || cmd&0xe0ff == 0x0000: // PMOVE TC, SRP, CRP, TT0, TT1
		reg, size, ok := pmoveRegister(cmd)
		if !ok || !mmuControlAlterable(cpu.regs.IR) {
			return false, nil
		}
		return true, cpu.pmove(reg, size, cmd&0x0200 != 0, cmd&0x0100 == 0)
	case cmd&0xfdff == 0x6000: // PMOVE MMUSR
		if !mmuControlAlterable(cpu.regs.IR) {
			return false, nil
		}
		return true, cpu.pmove(mmu.MMUSR, 2, cmd&0x0200 != 0, false)
	case cmd&0xfde0 == 0x2000: // PLOADR, PLOADW
		fc, ok := cpu.mmuFunctionCode(cmd)
		if !ok {
			return false, nil
		}
		address, ok, err := cpu.mmuAddress()
		if !ok || err != nil {
			return ok, err
		}
		cpu.mmu.Load(address, fc, cmd&0x0200 == 0)
		cpu.addCycles(16)
		return true, nil
	case cmd&0xe300 == 0x2000: // PFLUSHA, PFLUSH
		mode, mask := (cmd>>10)&7, uint8(cmd>>5)&7
		switch mode {
		case 1: // PFLUSHA
			if cmd&0xff != 0 {
				return false, nil
			}
			cpu.mmu.Flush(0, 0, 0, false)
		case 4, 6:
			fc, ok := cpu.mmuFunctionCode(cmd)
			if !ok {
				return false, nil
			}
			var address uint32
			if mode == 6 {
				var err error
				if address, ok, err = cpu.mmuAddress(); !ok || err != nil {
					return ok, err
				}
			}
			cpu.mmu.Flush(fc, mask, address, mode == 6)
		default:
			return false, nil
		}
		cpu.addCycles(8)
		return true, nil
	case cmd&0xe000 == 0x8000: // PTESTR, PTESTW
		level, withRegister, reg := int(cmd>>10)&7, cmd&0x0100 != 0, (cmd>>5)&7
		if level == 0 && cmd&0x01e0 != 0 || !withRegister && reg != 0 {
			return false, nil
		}
		fc, ok := cpu.mmuFunctionCode(cmd)
		if !ok {
			return false, nil
		}
		address, ok, err := cpu.mmuAddress()
		if !ok || err != nil {
			return ok, err
		}
		descriptor := cpu.mmu.Test(address, fc, level, cmd&0x0200 == 0)
		if withRegister {
			cpu.regs.A[reg] = descriptor
		}
		cpu.addCycles(12)
		return true, nil
	}
	return false, nil
}

// pmoveRegister decodes the register and operand size of PMOVE to or from
// TC, SRP, CRP, TT0 or TT1.
func pmoveRegister(cmd uint16) (mmu.Register, uint32, bool) {
	preg := (cmd >> 10) & 7
	if cmd&0xe000 == 0x4000 {
		switch preg {
		case 0:
			return mmu.TC, 4, true
		case 2:
			return mmu.SRP, 8, true
		case 3:
			return mmu.CRP, 8, true
		}
		return 0, 0, false
	}
	switch preg {
	case 2:
		return mmu.TT0, 4, true
	case 3:
		return mmu.TT1, 4, true
	}
	return 0, 0, false
}

// pmove moves an MMU register of size bytes to (toMemory) or from the
// control alterable effective address. Loading a register flushes the
// address translation cache unless the flush-disable bit is set.
func (cpu *CPU) pmove(reg mmu.Register, size uint32, toMemory, flush bool) error {
	operand, err := cpu.resolveSrcEA(Long)
	if err != nil {
		return err
	}
	address := operand.computedAddress()
	if toMemory {
		value := cpu.mmu.Register(reg)
		switch size {
		case 2:
			err = cpu.write(Word, address, uint32(value))
		case 4:
			err = cpu.write(Long, address, uint32(value))
		default:
			if err = cpu.write(Long, address, uint32(value>>32)); err == nil {
				err = cpu.write(Long, address+4, uint32(value))
			}
		}
		cpu.addCycles(8)
		return err
	}
	var value uint64
	switch size {
	case 2:
		v, err := cpu.read(Word, address)
		if err != nil {
			return err
		}
		value = uint64(v)
	case 4:
		v, err := cpu.read(Long, address)
		if err != nil {
			return err
		}
		value = uint64(v)
	default:
		hi, err := cpu.read(Long, address)
		if err != nil {
			return err
		}
		lo, err := cpu.read(Long, address+4)
		if err != nil {
			return err
		}
		value = uint64(hi)<<32 | uint64(lo)
	}
	cpu.addCycles(12)
	if err := cpu.mmu.SetRegister(reg, value, flush); errors.Is(err, mmu.ErrConfiguration) {
		cpu.refreshTranslation()
		return cpu.exceptionWithCycles(XMMUConfiguration, exceptionCyclesIllegal)
	} else if err != nil {
		return err
	}
	return nil
}

// mmuFunctionCode decodes the function code field (bits 4-0) of an MMU
// command word: immediate, from a data register, or SFC or DFC.
func (cpu *CPU) mmuFunctionCode(cmd uint16) (uint8, bool) {
	field := cmd & 0x1f
	switch {
	case field&0x18 == 0x10:
		return uint8(field & 7), true
	case field&0x18 == 0x08:
		return uint8(cpu.regs.D[field&7] & 7), true
	case field == 0:
		return uint8(cpu.regs.SFC & 7), true
	case field == 1:
		return uint8(cpu.regs.DFC & 7), true
	}
	return 0, false
}

// mmuAddress evaluates the control alterable effective address of PFLUSH,
// PLOAD or PTEST, the logical address the instruction is about. ok is false
// for another addressing mode.
func (cpu *CPU) mmuAddress() (address uint32, ok bool, err error) {
	if !mmuControlAlterable(cpu.regs.IR) {
		return 0, false, nil
	}
	operand, err := cpu.resolveSrcEA(Long)
	if err != nil {
		return 0, true, err
	}
	return operand.computedAddress(), true, nil
}

// mmuControlAlterable reports whether the effective address of an MMU
// instruction uses a control alterable addressing mode, the only ones the
// MC68030 accepts.
func mmuControlAlterable(op uint16) bool {
	return specIn(op, specControl&^(specPCDisp|specPCIndex))
}
