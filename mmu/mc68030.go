// Package mmu emulates the paged memory management units of the 68000
// family. MC68030 is the on-chip MMU of the MC68030.
//
// The CPU of package m68kemu creates and drives the MMU of its model: it
// translates every access through Translate and executes the MMU
// instructions (PMOVE, PFLUSH, PLOAD, PTEST) with the other methods. A
// program inspects the MMU through CPU.MMU. An MMU must not be used from
// several goroutines at once.
package mmu

import "errors"

// Register names an MMU register.
type Register int

const (
	TC    Register = iota // translation control
	SRP                   // supervisor root pointer
	CRP                   // CPU root pointer
	TT0                   // transparent translation 0
	TT1                   // transparent translation 1
	MMUSR                 // MMU status register
)

// Translation is the translation of a logical address.
type Translation struct {
	// Physical is the physical address.
	Physical uint32
	// Mask selects the low address bits the translation passes through
	// unchanged: it applies to the naturally aligned block of Mask+1 bytes
	// around the address (a page, or a transparent block).
	Mask uint32
	// Writable reports whether writes to the block may use the same
	// translation without asking the MMU again.
	Writable bool
}

// Memory is the physical memory an MMU reads and updates its translation
// tables in, a long word at a time.
type Memory interface {
	ReadLong(address uint32) (uint32, error)
	WriteLong(address uint32, value uint32) error
}

// ErrConfiguration is returned by SetRegister for a value that makes the CPU
// take the MMU configuration exception.
var ErrConfiguration = errors.New("mmu: configuration error")

// TC bits (MC68030 User's Manual, 9.7.2).
const (
	tcEnable = 1 << 31
	tcSRE    = 1 << 25 // supervisor root pointer enable
	tcFCL    = 1 << 24 // function code lookup
	tcMask   = 0x83ffffff
)

// Transparent translation register bits (9.7.3).
const (
	ttEnable = 1 << 15
	ttCI     = 1 << 10
	ttRW     = 1 << 9 // 1: reads are transparent, 0: writes
	ttRWM    = 1 << 8 // R/W ignored
	ttMask   = 0xffff8777
)

// Descriptor fields (9.5.1). Short descriptors are one long word; long
// descriptors put the limit and these bits in the first long word and the
// address in the second.
const (
	dtInvalid = 0
	dtPage    = 1
	dtShort   = 2 // valid, the next table holds 4-byte descriptors
	dtLong    = 3 // valid, the next table holds 8-byte descriptors

	descWP = 1 << 2
	descU  = 1 << 3
	descM  = 1 << 4
	descCI = 1 << 6
	descS  = 1 << 8 // long descriptors only
)

// MMUSR bits (9.7.4).
const (
	SRBusError     uint16 = 1 << 15
	SRLimit        uint16 = 1 << 14
	SRSupervisor   uint16 = 1 << 13
	SRWriteProtect uint16 = 1 << 11
	SRInvalid      uint16 = 1 << 10
	SRModified     uint16 = 1 << 9
	SRTransparent  uint16 = 1 << 6
	SRLevels       uint16 = 7 // number of levels searched
)

// atcSize is the number of entries of the MC68030's address translation
// cache.
const atcSize = 22

// atcEntry is a cached translation of one page.
type atcEntry struct {
	valid    bool
	used     bool // recently used, for the pseudo-LRU replacement
	fc       uint8
	logical  uint32 // page address
	physical uint32 // page frame address
	bus      bool   // the access takes a bus error
	ci       bool
	wp       bool
	modified bool
}

// MC68030 is the MMU of the MC68030.
type MC68030 struct {
	mem      Memory
	tc       uint32
	crp, srp uint64
	tt       [2]uint32
	mmusr    uint16
	atc      [atcSize]atcEntry
}

// NewMC68030 returns an MC68030 MMU that searches translation tables in mem.
// Translation and the transparent translation registers are disabled.
func NewMC68030(mem Memory) *MC68030 {
	return &MC68030{mem: mem}
}

// Reset disables translation and transparent translation. It does not flush
// the address translation cache.
func (m *MC68030) Reset() {
	m.tc &^= tcEnable
	m.tt[0] &^= ttEnable
	m.tt[1] &^= ttEnable
}

// Enabled reports whether the E bit of TC is set.
func (m *MC68030) Enabled() bool { return m.tc&tcEnable != 0 }

// Register returns an MMU register, right-aligned: TC, TT0 and TT1 are 32
// bits wide, CRP and SRP 64, MMUSR 16.
func (m *MC68030) Register(r Register) uint64 {
	switch r {
	case TC:
		return uint64(m.tc)
	case SRP:
		return m.srp
	case CRP:
		return m.crp
	case TT0:
		return uint64(m.tt[0])
	case TT1:
		return uint64(m.tt[1])
	case MMUSR:
		return uint64(m.mmusr)
	}
	return 0
}

// SetRegister loads an MMU register and, with flush, flushes the address
// translation cache. A TC with E set whose page size is reserved or whose
// IS, PS and table index fields do not add up to 32, and a root pointer with
// descriptor type invalid, are loaded and then reported as ErrConfiguration;
// TC is loaded with E clear.
func (m *MC68030) SetRegister(r Register, value uint64, flush bool) error {
	var err error
	switch r {
	case TC:
		m.tc = uint32(value) & tcMask
		if m.tc&tcEnable != 0 && !validTC(m.tc) {
			m.tc &^= tcEnable
			err = ErrConfiguration
		}
	case SRP, CRP:
		rp := value & 0xffff0003_fffffff0
		if r == SRP {
			m.srp = rp
		} else {
			m.crp = rp
		}
		if (rp>>32)&3 == dtInvalid {
			err = ErrConfiguration
		}
	case TT0:
		m.tt[0] = uint32(value) & ttMask
	case TT1:
		m.tt[1] = uint32(value) & ttMask
	case MMUSR:
		m.mmusr = uint16(value)
		return nil
	}
	if flush {
		m.Flush(0, 0, 0, false)
	}
	return err
}

// validTC reports whether the page size of tc is defined and its fields
// cover the 32 address bits.
func validTC(tc uint32) bool {
	ps, is := (tc>>20)&15, (tc>>16)&15
	if ps < 8 {
		return false
	}
	sum := ps + is
	for _, ti := range tableIndexes(tc) {
		if ti == 0 {
			break
		}
		sum += ti
	}
	return sum == 32 && (tc>>12)&15 != 0
}

// tableIndexes returns the TIA, TIB, TIC and TID fields of tc.
func tableIndexes(tc uint32) [4]uint32 {
	return [4]uint32{(tc >> 12) & 15, (tc >> 8) & 15, (tc >> 4) & 15, tc & 15}
}

// pageShift returns log2 of the page size.
func (m *MC68030) pageShift() uint32 { return (m.tc >> 20) & 15 }

// transparent reports whether a transparent translation register maps the
// access.
func (m *MC68030) transparent(address uint32, fc uint8, write bool) bool {
	for _, tt := range m.tt {
		if tt&ttEnable == 0 {
			continue
		}
		base, mask := tt>>24, (tt>>16)&0xff
		fcBase, fcMask := uint8(tt>>4)&7, uint8(tt)&7
		if (address>>24^base)&^mask&0xff != 0 || (fc^fcBase)&^fcMask&7 != 0 {
			continue
		}
		if tt&ttRWM != 0 || (tt&ttRW != 0) == !write {
			return true
		}
	}
	return false
}

// Translate returns the translation of a read or write at address with
// function code fc. ok is false when the access must end in a bus error.
func (m *MC68030) Translate(address uint32, fc uint8, write bool) (Translation, bool) {
	if fc == 7 || m.transparent(address, fc, write) || m.tc&tcEnable == 0 {
		// CPU space and transparent blocks are not translated.
		return Translation{
			Physical: address,
			Mask:     0x00ffffff,
			Writable: fc == 7 || m.transparent(address, fc, true) && m.transparent(address, fc, false),
		}, true
	}
	shift := m.pageShift()
	e := m.lookup(address, fc)
	if e == nil || write && !e.modified && !e.wp && !e.bus {
		// A miss, or the first write to the page, which sets M.
		e = m.load(address, fc, write)
	}
	e.used = true
	if e.bus || write && e.wp {
		return Translation{}, false
	}
	pageMask := uint32(1)<<shift - 1
	return Translation{
		Physical: e.physical | address&pageMask,
		Mask:     pageMask,
		Writable: e.modified && !e.wp,
	}, true
}

// lookup returns the cache entry for the page of address, or nil.
func (m *MC68030) lookup(address uint32, fc uint8) *atcEntry {
	page := address >> m.pageShift()
	for i := range m.atc {
		e := &m.atc[i]
		if e.valid && e.fc == fc && e.logical>>m.pageShift() == page {
			return e
		}
	}
	return nil
}

// load searches the tables for address, as a read or write would, and
// caches the result in place of any entry for the page.
func (m *MC68030) load(address uint32, fc uint8, write bool) *atcEntry {
	if e := m.lookup(address, fc); e != nil {
		e.valid = false
	}
	r := m.search(address, fc, write, true, 7)
	shift := m.pageShift()
	e := m.victim()
	*e = atcEntry{
		valid:    true,
		fc:       fc,
		logical:  address >> shift << shift,
		physical: r.physical >> shift << shift,
		bus:      r.fault(),
		ci:       r.ci,
		wp:       r.wp,
		modified: r.modified,
	}
	return e
}

// victim returns the cache entry to replace: an invalid one, or else one not
// used recently. When every entry has been used recently, the history starts
// over.
func (m *MC68030) victim() *atcEntry {
	for i := range m.atc {
		if !m.atc[i].valid {
			return &m.atc[i]
		}
	}
	for i := range m.atc {
		if !m.atc[i].used {
			return &m.atc[i]
		}
	}
	for i := range m.atc {
		m.atc[i].used = false
	}
	return &m.atc[0]
}

// Flush invalidates the cached translations whose function code matches fc
// in the bits set in fcMask, and with byAddress only those of the page that
// holds address (PFLUSH). A zero fcMask flushes them all (PFLUSHA).
func (m *MC68030) Flush(fc, fcMask uint8, address uint32, byAddress bool) {
	page := address >> m.pageShift()
	for i := range m.atc {
		e := &m.atc[i]
		if (e.fc^fc)&fcMask != 0 || byAddress && e.logical>>m.pageShift() != page {
			continue
		}
		e.valid = false
	}
}

// Load searches the tables for address as a read or write would, even while
// translation is disabled, and caches the translation (PLOAD).
func (m *MC68030) Load(address uint32, fc uint8, write bool) {
	m.load(address, fc, write)
}

// Test searches the translation cache (level 0) or the translation tables
// down to level and sets MMUSR (PTEST). It returns the physical address of
// the last descriptor fetched.
func (m *MC68030) Test(address uint32, fc uint8, level int, write bool) uint32 {
	if level == 0 {
		switch e := m.lookup(address, fc); {
		case m.transparent(address, fc, write):
			m.mmusr = SRTransparent
		case e == nil:
			m.mmusr = SRInvalid
		default:
			m.mmusr = 0
			if e.bus {
				m.mmusr |= SRBusError | SRInvalid
			}
			if e.wp {
				m.mmusr |= SRWriteProtect
			}
			if e.modified {
				m.mmusr |= SRModified
			}
		}
		return 0
	}
	r := m.search(address, fc, write, false, level)
	var sr uint16
	if r.busError {
		sr |= SRBusError
	}
	if r.limit {
		sr |= SRLimit
	}
	if r.supervisor {
		sr |= SRSupervisor
	}
	if r.wp {
		sr |= SRWriteProtect
	}
	if r.invalid || r.busError || r.limit {
		sr |= SRInvalid
	}
	if r.modified {
		sr |= SRModified
	}
	m.mmusr = sr | uint16(min(r.levels, 7))
	return r.descriptor
}
