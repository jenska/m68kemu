package mmu

// Transparent translation register bits (MC68030 User's Manual 9.7.3).
const (
	ttEnable = 1 << 15
	ttCI     = 1 << 10
	ttRW     = 1 << 9 // 1: reads are transparent, 0: writes
	ttRWM    = 1 << 8 // R/W ignored
	ttMask   = 0xffff8777
)

// MC68030 is the MMU of the MC68030 (MC68030 User's Manual, section 9).
type MC68030 struct {
	mem      Memory
	tc       uint32
	crp, srp uint64
	tt       [2]uint32
	mmusr    uint16
	atc      atc
}

// NewMC68030 returns an MC68030 MMU that searches translation tables in mem.
// Translation and the transparent translation registers are disabled.
func NewMC68030(mem Memory) *MC68030 {
	return &MC68030{mem: mem, atc: newATC(22)}
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
// bits wide, CRP and SRP 64, MMUSR 16. Registers of the MC68851 read as 0.
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
	default:
		return nil
	}
	if flush {
		m.atc.flushAll()
	}
	return err
}

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
	return translateATC(&m.atc, address, fc, 0, write, pageShift(m.tc), m.load)
}

// translateATC translates address through cache a, calling load on a miss
// or on the first write to a page, which sets its M bit.
func translateATC(a *atc, address uint32, fc, alias uint8, write bool, shift uint32, load func(uint32, uint8, bool) *atcEntry) (Translation, bool) {
	e := a.lookup(address, fc, alias, shift)
	if e == nil || write && !e.modified && !e.wp && !e.bus {
		e = load(address, fc, write)
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

// search runs a table search for address.
func (m *MC68030) search(address uint32, fc uint8, write, update bool, maxLevels int) searchResult {
	rp := m.crp
	if m.tc&tcSRE != 0 && fc&4 != 0 {
		rp = m.srp
	}
	s := search{mem: m.mem, tc: m.tc, fc: fc, write: write, update: update}
	return s.run(address, rp, m.tc&tcFCL != 0, maxLevels)
}

// load searches the tables for address, as a read or write would, and
// caches the result in place of any entry for the page.
func (m *MC68030) load(address uint32, fc uint8, write bool) *atcEntry {
	shift := pageShift(m.tc)
	if e := m.atc.lookup(address, fc, 0, shift); e != nil {
		e.valid = false
	}
	r := m.search(address, fc, write, true, 7)
	e := m.atc.victim()
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

// FlushAll invalidates every cached translation (PFLUSHA).
func (m *MC68030) FlushAll() { m.atc.flushAll() }

// Flush invalidates the cached translations whose function code matches fc
// in the bits set in fcMask, and with byAddress only those of the page that
// holds address (PFLUSH). The MC68030 has no shared entries.
func (m *MC68030) Flush(fc, fcMask uint8, address uint32, byAddress, shared bool) {
	shift := pageShift(m.tc)
	m.atc.flush(func(e *atcEntry) bool {
		return (e.fc^fc)&fcMask == 0 && (!byAddress || e.logical>>shift == address>>shift)
	})
}

// Load searches the tables for address as a read or write would, even while
// translation is disabled, and caches the translation (PLOAD).
func (m *MC68030) Load(address uint32, fc uint8, write bool) error {
	m.load(address, fc, write)
	return nil
}

// Test searches the translation cache (level 0) or the translation tables
// down to level and sets MMUSR (PTEST). It returns the physical address of
// the last descriptor fetched.
func (m *MC68030) Test(address uint32, fc uint8, level int, write bool) (uint32, error) {
	if level > 0 {
		r := m.search(address, fc, write, false, level)
		m.mmusr = statusOf(&r)
		return r.descriptor, nil
	}
	switch e := m.atc.lookup(address, fc, 0, pageShift(m.tc)); {
	case m.transparent(address, fc, write):
		m.mmusr = SRTransparent
	case e == nil:
		m.mmusr = SRInvalid
	default:
		m.mmusr = atcStatus(e)
	}
	return 0, nil
}

// atcStatus returns the status register bits PTEST at level 0 sets from a
// cache entry.
func atcStatus(e *atcEntry) uint16 {
	var sr uint16
	if e.bus {
		sr |= SRBusError | SRInvalid
	}
	if e.wp {
		sr |= SRWriteProtect
	}
	if e.modified {
		sr |= SRModified
	}
	return sr
}
