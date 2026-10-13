package mmu

import (
	"errors"
	"testing"
)

// memory is physical memory for the MMU's table searches.
type memory map[uint32]uint32

func (m memory) ReadLong(address uint32) (uint32, error) { return m[address], nil }

func (m memory) WriteLong(address uint32, value uint32) error {
	m[address] = value
	return nil
}

// newTestMC68851 returns an enabled MC68851 with 4 KB pages, a 6-bit
// initial shift (the access level bits stay out of the table index), a
// 2-bit A level and a 12-bit B level. The A table at $1000 holds long
// descriptors: entry 0 is a0 and points to the short B table at $2000,
// which maps page n < 64 to physical n*$1000+$100000.
func newTestMC68851(t *testing.T, a0 uint32) (*MC68851, memory) {
	t.Helper()
	mem := memory{0x1000: a0, 0x1004: 0x2000}
	for page := uint32(0); page < 64; page++ {
		mem[0x2000+page*4] = (0x100000 + page<<12) | dtPage
	}
	m := NewMC68851(mem)
	if err := m.SetRegister(CRP, 0x7fff0003_00001000, true); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRegister(TC, 0x80c62c00, true); err != nil { // E, PS=12, IS=6, TIA=2, TIB=12
		t.Fatal(err)
	}
	return m, mem
}

func TestAccessLevels(t *testing.T) {
	// The A descriptor allows reads from level 2 and writes from level 1.
	// AC enables two level bits; CAL is level 1.
	m, _ := newTestMC68851(t, 0x7fff0000|2<<13|1<<10|dtShort)
	m.SetRegister(AC, 0x00a0, true) // MC, ALC=2
	m.SetRegister(CAL, 1<<6, true)
	const user = 1 // user data
	for _, tt := range []struct {
		address uint32
		write   bool
		ok      bool
	}{
		{0x00003000, false, false}, // level 0: more privileged than CAL
		{0x40003000, false, true},  // level 1
		{0x40003000, true, true},
		{0x80003000, false, true},  // level 2
		{0x80003000, true, false},  // level 2 may not write
		{0xc0003000, false, false}, // level 3 may not read
	} {
		_, ok := m.Translate(tt.address, user, tt.write)
		if ok != tt.ok {
			t.Errorf("%08x write %v: ok %v, want %v", tt.address, tt.write, ok, tt.ok)
		}
	}
	// Supervisor accesses are not checked.
	if _, ok := m.Translate(0xc0003000, 5, true); !ok {
		t.Errorf("supervisor access refused")
	}
	if _, err := m.Test(0x80003000, user, 7, true); err != nil || m.psr&(SRAccessLevel|SRWriteProtect) != SRAccessLevel|SRWriteProtect {
		t.Errorf("PTESTW at level 2: PSR %04x %v, want A and W", m.psr, err)
	}
	if _, _ = m.Test(0x80003000, user, 7, false); m.psr&SRAccessLevel != 0 {
		t.Errorf("PTESTR at level 2: PSR %04x, want no A", m.psr)
	}
}

func TestDMARootPointer(t *testing.T) {
	// FC3 selects DRP, whose tree always starts with a function code table.
	m, mem := newTestMC68851(t, 0x7fff0000|dtShort)
	mem[0x3000+5*4] = 0x200000 | dtPage // FC 5: early termination at $200000
	if err := m.SetRegister(DRP, 0x7fff0002_00003000, true); err != nil {
		t.Fatal(err)
	}
	tr, ok := m.Translate(0x00012345, 8|5, false)
	if !ok || tr.Physical != 0x212345 {
		t.Errorf("DMA translation = %x %v, want 212345", tr.Physical, ok)
	}
	if tr, ok := m.Translate(0x00002345, 5, false); !ok || tr.Physical != 0x102345 {
		t.Errorf("CPU translation = %x %v, want 102345", tr.Physical, ok)
	}
}

func TestSaveRestoreFrames(t *testing.T) {
	m, _ := newTestMC68851(t, 0x7fff0000|dtShort)
	m.SetRegister(BAD0+3, 0x4e71, true)
	m.SetRegister(BAC0+3, 0x8005, true)
	m.SetRegister(CAL, 0x60, true)
	frame := m.Save()
	if len(frame) != 76 || frame[0] != frameVersion || frame[1] != frameBreakpoint {
		t.Fatalf("frame of %d bytes, format %02x%02x, want 76 bytes and c048", len(frame), frame[0], frame[1])
	}
	m.SetRegister(BAC0+3, 0, true)
	m.SetRegister(CAL, 0, true)
	if err := m.Restore(frame); err != nil {
		t.Fatal(err)
	}
	if m.bac[3] != 0x8005 || m.bad[3] != 0x4e71 || m.cal != 0x60 {
		t.Errorf("restored BAC3=%04x BAD3=%04x CAL=%02x", m.bac[3], m.bad[3], m.cal)
	}
	if err := m.Restore([]byte{0, 0, 0, 0}); err != nil || m.cal != 0x60 {
		t.Errorf("null frame: %v, CAL %02x; want no error and registers kept", err, m.cal)
	}
	if err := m.Restore([]byte{frameVersion, 0x10, 0, 0}); !errors.Is(err, errFrame) {
		t.Errorf("frame of unknown length: %v, want errFrame", err)
	}
}

func TestLockedEntries(t *testing.T) {
	// Locked entries are not replaced; the 64th lock request is refused
	// and PCSR warns.
	m, mem := newTestMC68851(t, 0x7fff0000|dtShort)
	for page := uint32(0); page < 64; page++ {
		mem[0x2000+page*4] |= descLock
	}
	for page := uint32(0); page < 16; page++ {
		m.Translate(page<<12, 1, false)
	}
	if got := m.atc.locked(); got != 16 {
		t.Fatalf("%d locked entries, want 16", got)
	}
	for i := 0; i < 64; i++ {
		m.Translate(uint32(i)<<12, 2, false) // other function code, also locked
	}
	if m.atc.locked() != 63 || m.pcsr&pcsrLockWarning == 0 {
		t.Errorf("%d locked entries, PCSR %04x; want 63 and LW", m.atc.locked(), m.pcsr)
	}
}
