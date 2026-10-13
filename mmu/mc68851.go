package mmu

import "errors"

// MC68851 is the paged memory management unit the MC68020 drives as
// coprocessor 0 (MC68851 User's Manual). Beside translation it has a DMA
// root pointer, task aliases for the address translation cache through an
// eight-entry root pointer table, shared and locked cache entries, access
// level protection, and the breakpoint acknowledge registers that serve the
// MC68020's BKPT instruction.
type MC68851 struct {
	mem           Memory
	tc            uint32
	crp, srp, drp uint64
	psr, pcsr, ac uint16
	cal, val, scc uint8
	bad, bac      [8]uint16
	atc           atc
	rpt           [8]rptEntry
	alias         uint8 // task alias of the current CRP
}

// rptEntry is an entry of the root pointer table, which gives recently used
// CPU root pointers a task alias.
type rptEntry struct {
	rp          uint64
	valid, used bool
}

// PCSR and AC bits (MC68851 User's Manual 6.1.2 and 6.1.7).
const (
	pcsrFlush       = 1 << 15
	pcsrLockWarning = 1 << 14
	acModuleControl = 1 << 7
	acMask          = 0x00b3
	bacEnable       = 1 << 15
	bacMask         = 0x80ff
)

// The MC68851's state frames for PSAVE and PRESTORE (6.2.7.3): a format
// word with the version in the high byte and the length of the rest of the
// frame in the low one.
const (
	frameVersion    = 0xc0
	frameIdle       = 0x20 // 36 bytes in all
	frameMid        = 0x28 // 44 bytes: a coprocessor instruction in progress
	frameBreakpoint = 0x48 // 76 bytes: breakpoints enabled
)

// errFrame reports a state frame PRESTORE rejects.
var errFrame = errors.New("mmu: invalid MC68851 state frame")

// NewMC68851 returns an MC68851 that searches translation tables in mem, in
// its reset state.
func NewMC68851(mem Memory) *MC68851 {
	m := &MC68851{mem: mem, atc: newATC(64)}
	m.rpt[0].valid = true // the CRP the MC68851 starts with
	return m
}

// Reset disables translation, access level checking and the breakpoints.
// It does not flush the address translation cache.
func (m *MC68851) Reset() {
	m.tc &^= tcEnable
	m.ac &^= 0x30 // ALC
	for i := range m.bac {
		m.bac[i] &^= bacEnable
	}
}

// Enabled reports whether the E bit of TC is set.
func (m *MC68851) Enabled() bool { return m.tc&tcEnable != 0 }

// Register returns an MMU register, right-aligned: CRP, SRP and DRP are 64
// bits wide, TC 32, PSR, PCSR, AC, BADn and BACn 16, CAL, VAL and SCC 8.
// TT0 and TT1, which the MC68851 does not have, read as 0.
func (m *MC68851) Register(r Register) uint64 {
	switch {
	case r == TC:
		return uint64(m.tc)
	case r == SRP:
		return m.srp
	case r == CRP:
		return m.crp
	case r == DRP:
		return m.drp
	case r == PSR:
		return uint64(m.psr)
	case r == PCSR:
		return uint64(m.pcsr)
	case r == CAL:
		return uint64(m.cal)
	case r == VAL:
		return uint64(m.val)
	case r == SCC:
		return uint64(m.scc)
	case r == AC:
		return uint64(m.ac)
	case r >= BAD0 && r < BAD0+8:
		return uint64(m.bad[r-BAD0])
	case r >= BAC0 && r < BAC0+8:
		return uint64(m.bac[r-BAC0])
	}
	return 0
}

// SetRegister loads an MMU register, with the side effects of PMOVE: a
// root pointer with descriptor type invalid is refused and reported as
// ErrConfiguration, as is a TC with E set whose fields are inconsistent,
// which is loaded with E clear. Loading TC with E clear flushes the address
// translation cache, SRP and DRP flush the entries formed with them, and CRP
// selects a task alias. The MC68851 has no flush-disable option, so flush
// is ignored; PCSR is read-only.
func (m *MC68851) SetRegister(r Register, value uint64, flush bool) error {
	switch {
	case r == TC:
		m.tc = uint32(value) & tcMask
		if m.tc&tcEnable == 0 {
			m.atc.flushAll()
		} else if !validTC(m.tc) {
			m.tc &^= tcEnable
			return ErrConfiguration
		}
	case r == CRP || r == SRP || r == DRP:
		rp := value & 0xffff_0203_ffff_fff0 // L/U, limit, SG, DT and the table address
		if (rp>>32)&3 == dtInvalid {
			return ErrConfiguration
		}
		m.setRootPointer(r, rp)
	case r == PSR:
		m.psr = uint16(value)
	case r == CAL:
		m.cal = uint8(value) & 0xe0
	case r == VAL:
		m.val = uint8(value) & 0xe0
	case r == SCC:
		m.scc = uint8(value)
	case r == AC:
		m.ac = uint16(value) & acMask
	case r >= BAD0 && r < BAD0+8:
		m.bad[r-BAD0] = uint16(value)
	case r >= BAC0 && r < BAC0+8:
		m.bac[r-BAC0] = uint16(value) & bacMask
	}
	return nil
}

// setRootPointer loads CRP, SRP or DRP with their side effects on the
// cache.
func (m *MC68851) setRootPointer(r Register, rp uint64) {
	switch r {
	case SRP:
		m.srp = rp
		m.atc.flush(func(e *atcEntry) bool { return e.root == rootSRP })
	case DRP:
		m.drp = rp
		m.atc.flush(func(e *atcEntry) bool { return e.root == rootDRP })
	case CRP:
		m.crp = rp
		m.selectAlias(rp)
	}
}

// selectAlias looks the CPU root pointer up in the root pointer table and
// makes its entry the current task alias (5.3.1). A new root pointer, or
// one that differs only in its limit and descriptor type, takes over an
// entry and flushes the cached translations of that alias; PCSR reports the
// alias and the flush.
func (m *MC68851) selectAlias(rp uint64) {
	const sameTable = 0x8000_0000_ffff_fff0 // L/U and the table address
	flushed := false
	i := -1
	for j := range m.rpt {
		if m.rpt[j].valid && m.rpt[j].rp == rp {
			i = j
			break
		}
	}
	if i < 0 {
		for j := range m.rpt {
			if m.rpt[j].valid && m.rpt[j].rp&sameTable == rp&sameTable {
				i = j
				break
			}
		}
		if i < 0 {
			i = m.rptVictim()
		}
		m.rpt[i] = rptEntry{rp: rp, valid: true}
		alias := uint8(i)
		m.atc.flush(func(e *atcEntry) bool { return e.alias == alias && !e.shared })
		flushed = true
	}
	m.rpt[i].used = true
	m.alias = uint8(i)
	m.pcsr = m.pcsr&pcsrLockWarning | uint16(i)
	if flushed {
		m.pcsr |= pcsrFlush
	}
}

// rptVictim returns the root pointer table entry to replace, with the
// cache's pseudo-LRU algorithm.
func (m *MC68851) rptVictim() int {
	for i := range m.rpt {
		if !m.rpt[i].valid {
			return i
		}
	}
	for i := range m.rpt {
		if !m.rpt[i].used {
			return i
		}
	}
	for i := range m.rpt {
		m.rpt[i].used = false
	}
	return 0
}

// FlushRoot invalidates the root pointer table entry of a previous CPU root
// pointer and the cached translations of its task alias, except the shared
// ones (PFLUSHR). Nothing happens if rp is not in the table.
func (m *MC68851) FlushRoot(rp uint64) {
	for i := range m.rpt {
		if m.rpt[i].valid && m.rpt[i].rp == rp {
			m.rpt[i].valid = false
			alias := uint8(i)
			m.atc.flush(func(e *atcEntry) bool { return e.alias == alias && !e.shared })
			return
		}
	}
}

// accessLevelBits returns the number of upper logical address bits that
// hold the access level, 0 while access levels are disabled.
func (m *MC68851) accessLevelBits() uint32 { return uint32(m.ac>>4) & 3 }

// Translate returns the translation of a read or write at address with
// function code fc (FC3 set for DMA). ok is false when the access must end
// in a bus error.
func (m *MC68851) Translate(address uint32, fc uint8, write bool) (Translation, bool) {
	if fc == 7 || m.tc&tcEnable == 0 {
		return Translation{Physical: address, Mask: 0x00ffffff, Writable: true}, true
	}
	if n := m.accessLevelBits(); n > 0 && fc&0xc == 0 {
		// A user access may not ask for more privilege than CAL.
		if address>>(32-n) < uint32(m.cal)>>(8-n) {
			return Translation{}, false
		}
	}
	return translateATC(&m.atc, address, fc, m.alias, write, pageShift(m.tc), m.load)
}

// search runs a table search for address through the root pointer the
// function code selects: DRP for DMA, SRP for supervisor accesses with SRE,
// else CRP.
func (m *MC68851) search(address uint32, fc uint8, write, update bool, maxLevels int) (searchResult, uint8) {
	rp, root, fcl := m.crp, uint8(rootCRP), m.tc&tcFCL != 0
	switch {
	case fc&8 != 0:
		rp, root, fcl = m.drp, rootDRP, true
	case m.tc&tcSRE != 0 && fc&4 != 0:
		rp, root = m.srp, rootSRP
	}
	s := search{mem: m.mem, tc: m.tc, fc: fc, write: write, update: update}
	if n := m.accessLevelBits(); n > 0 && fc&4 == 0 {
		s.al = accessLevel{enabled: true, level: address >> (32 - n)}
	}
	return s.run(address, rp, fcl, maxLevels), root
}

// load searches the tables for address, as a read or write would, and
// caches the result in place of any entry for the page.
func (m *MC68851) load(address uint32, fc uint8, write bool) *atcEntry {
	shift := pageShift(m.tc)
	if e := m.atc.lookup(address, fc, m.alias, shift); e != nil {
		e.valid = false
	}
	r, root := m.search(address, fc, write, true, 7)
	e := m.atc.victim()
	*e = atcEntry{
		valid:    true,
		fc:       fc,
		alias:    m.alias,
		root:     root,
		logical:  address >> shift << shift,
		physical: r.physical >> shift << shift,
		bus:      r.fault(),
		ci:       r.ci,
		wp:       r.wp || r.writeLevel,
		modified: r.modified,
		gate:     r.gate,
		shared:   r.shared,
		locked:   r.lock && m.atc.locked() < len(m.atc.entries)-1,
	}
	if m.atc.locked() >= len(m.atc.entries)-1 {
		m.pcsr |= pcsrLockWarning
	} else {
		m.pcsr &^= pcsrLockWarning
	}
	return e
}

// FlushAll invalidates every cached translation (PFLUSHA).
func (m *MC68851) FlushAll() { m.atc.flushAll() }

// Flush invalidates the cached translations of the current task alias whose
// function code matches fc in the bits set in fcMask, and with byAddress
// only those of the page that holds address. Globally shared entries go
// only with shared (PFLUSHS).
func (m *MC68851) Flush(fc, fcMask uint8, address uint32, byAddress, shared bool) {
	shift := pageShift(m.tc)
	m.atc.flush(func(e *atcEntry) bool {
		if e.shared && !shared || !e.shared && e.alias != m.alias {
			return false
		}
		return (e.fc^fc)&fcMask == 0 && (!byAddress || e.logical>>shift == address>>shift)
	})
}

// Load searches the tables for address as a read or write would and caches
// the translation (PLOAD). It returns ErrIllegalOperation while translation
// is disabled.
func (m *MC68851) Load(address uint32, fc uint8, write bool) error {
	if m.tc&tcEnable == 0 {
		return ErrIllegalOperation
	}
	m.load(address, fc, write)
	return nil
}

// Test searches the translation cache (level 0) or the translation tables
// down to level and sets PSR (PTEST). It returns the physical address of
// the last descriptor fetched, or ErrIllegalOperation while translation is
// disabled.
func (m *MC68851) Test(address uint32, fc uint8, level int, write bool) (uint32, error) {
	if m.tc&tcEnable == 0 {
		return 0, ErrIllegalOperation
	}
	if level == 0 {
		e := m.atc.lookup(address, fc, m.alias, pageShift(m.tc))
		if e == nil {
			m.psr = SRInvalid
			return 0, nil
		}
		m.psr = atcStatus(e)
		if e.gate {
			m.psr |= SRGate
		}
		if e.shared {
			m.psr |= SRShared
		}
		return 0, nil
	}
	r, _ := m.search(address, fc, write, false, level)
	m.psr = statusOf(&r)
	if r.access || write && r.writeLevel {
		m.psr |= SRAccessLevel
	}
	if r.writeLevel {
		m.psr |= SRWriteProtect
	}
	if r.gate && !r.fault() {
		m.psr |= SRGate
	}
	if r.shared {
		m.psr |= SRShared
	}
	return r.descriptor, nil
}

// Condition evaluates an MC68851 condition for PBcc, PDBcc, PScc and
// PTRAPcc: the even conditions test a PSR bit (B, L, S, A, W, I, G, C) for
// set, the odd ones for clear.
func (m *MC68851) Condition(cond uint16) (bool, error) {
	if cond > 15 {
		return false, errors.New("mmu: undefined MC68851 condition")
	}
	bits := [...]uint16{SRBusError, SRLimit, SRSupervisor, SRAccessLevel, SRWriteProtect, SRInvalid, SRGate, SRShared}
	set := m.psr&bits[cond>>1] != 0
	return set == (cond&1 == 0), nil
}

// Validate reports whether a pointer passes PVALID: the access level in the
// upper bits of address may not be more privileged (numerically less) than
// that in the upper bits of source, the VAL register or an address
// register. It fails while module operations are disabled.
func (m *MC68851) Validate(source, address uint32) bool {
	if m.ac&acModuleControl == 0 {
		return false
	}
	n := m.accessLevelBits()
	if n == 0 {
		return true
	}
	return source>>(32-n) <= address>>(32-n)
}

// Breakpoint answers the breakpoint acknowledge cycle of BKPT #n: while
// BACn is enabled and its skip count is not zero, it counts down and
// returns BADn as the opcode the CPU executes in place of the BKPT. ok is
// false otherwise, and the CPU takes the illegal instruction exception.
func (m *MC68851) Breakpoint(n int) (opcode uint16, ok bool) {
	n &= 7
	if m.bac[n]&bacEnable == 0 || m.bac[n]&0xff == 0 {
		return 0, false
	}
	m.bac[n]--
	return m.bad[n], true
}

// Save returns the state frame of PSAVE: the breakpoint registers when a
// breakpoint is enabled, PSR, SCC, CAL, VAL, SRP and CRP.
func (m *MC68851) Save() []byte {
	breakpoints := false
	for _, bac := range m.bac {
		breakpoints = breakpoints || bac&bacEnable != 0
	}
	length := frameIdle
	if breakpoints {
		length = frameBreakpoint
	}
	frame := make([]byte, 4+length)
	frame[0], frame[1] = frameVersion, byte(length)
	at := 4
	if breakpoints {
		for i := range m.bac {
			putLong(frame[at:], uint32(m.bac[i])<<16|uint32(m.bad[i]))
			at += 4
		}
		at += 8 // internal data
	}
	m.putRegisters(frame[at:])
	return frame
}

// putRegisters writes the registers of a state frame: PSR, SCC, CAL and VAL
// in one long word, then SRP and CRP, each with its low long word first.
func (m *MC68851) putRegisters(b []byte) {
	putLong(b, uint32(m.psr)<<16|uint32(m.scc)<<8|uint32(m.cal&0xe0)|uint32(m.val&0xe0)>>4)
	putLong(b[4:], uint32(m.srp))
	putLong(b[8:], uint32(m.srp>>32))
	putLong(b[12:], uint32(m.crp))
	putLong(b[16:], uint32(m.crp>>32))
}

// Restore loads a state frame for PRESTORE. A frame of format 0 (the null
// frame) only resets the MC68851's internal state; a frame of another
// version or length is rejected.
func (m *MC68851) Restore(frame []byte) error {
	if len(frame) < 4 {
		return errFrame
	}
	if frame[0] == 0 {
		return nil
	}
	if frame[0] != frameVersion || len(frame) != 4+int(frame[1]) {
		return errFrame
	}
	at := 4
	switch frame[1] {
	case frameIdle:
	case frameMid:
		at += 8 // internal data
	case frameBreakpoint:
		for i := range m.bac {
			v := getLong(frame[at:])
			m.bac[i], m.bad[i] = uint16(v>>16)&bacMask, uint16(v)
			at += 4
		}
		at += 8
	default:
		return errFrame
	}
	b := frame[at:]
	regs := getLong(b)
	m.psr, m.scc = uint16(regs>>16), uint8(regs>>8)
	m.cal, m.val = uint8(regs)&0xe0, uint8(regs<<4)&0xe0
	m.setRootPointer(SRP, uint64(getLong(b[8:]))<<32|uint64(getLong(b[4:])))
	m.setRootPointer(CRP, uint64(getLong(b[16:]))<<32|uint64(getLong(b[12:])))
	return nil
}

func putLong(b []byte, v uint32) {
	b[0], b[1], b[2], b[3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
}

func getLong(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
