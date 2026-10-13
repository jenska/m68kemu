// Package mmu emulates the paged memory management units of the 68000
// family: MC68030 is the on-chip MMU of the MC68030, MC68851 the external
// PMMU of the MC68020.
//
// The CPU of package m68kemu creates and drives the MMU of its model: it
// translates every access through Translate and executes the MMU
// instructions (PMOVE, PFLUSH, PLOAD, PTEST and, on the MC68851, the others)
// with the other methods. A program inspects the MMU through CPU.MMU. An MMU
// must not be used from several goroutines at once.
package mmu

import "errors"

// Register names an MMU register. Not every MMU has every register.
type Register int

const (
	TC    Register   = iota // translation control
	SRP                     // supervisor root pointer
	CRP                     // CPU root pointer
	TT0                     // transparent translation 0 (MC68030)
	TT1                     // transparent translation 1 (MC68030)
	MMUSR                   // MMU status register (PSR on the MC68851)
	DRP                     // DMA root pointer (MC68851)
	PCSR                    // PMMU cache status (MC68851)
	CAL                     // current access level (MC68851)
	VAL                     // validate access level (MC68851)
	SCC                     // stack change control (MC68851)
	AC                      // access control (MC68851)
	BAD0                    // breakpoint acknowledge data 0; BAD0+n is BADn (MC68851)
	BAC0  = BAD0 + 8        // breakpoint acknowledge control 0; BAC0+n is BACn (MC68851)
)

// PSR is the MC68851's name for the MMU status register.
const PSR = MMUSR

// MMU is the interface the CPU drives both MMUs through.
type MMU interface {
	// Reset is the effect of the processor's reset.
	Reset()
	// Enabled reports whether translation is on (the E bit of TC).
	Enabled() bool
	// Translate returns the translation of a read or write at address with
	// function code fc. ok is false when the access must end in a bus
	// error.
	Translate(address uint32, fc uint8, write bool) (t Translation, ok bool)
	// Register returns a register, right-aligned.
	Register(r Register) uint64
	// SetRegister loads a register and, with flush, flushes the address
	// translation cache where the MMU has a flush-disable option (the
	// MC68030's PMOVEFD). It returns ErrConfiguration for a value that
	// takes the MMU configuration exception.
	SetRegister(r Register, value uint64, flush bool) error
	// FlushAll invalidates every cached translation (PFLUSHA).
	FlushAll()
	// Flush invalidates the cached translations whose function code
	// matches fc in the bits set in fcMask, with byAddress only those of
	// the page that holds address, and with shared also the globally
	// shared ones of the MC68851 (PFLUSH, PFLUSHS).
	Flush(fc, fcMask uint8, address uint32, byAddress, shared bool)
	// Load searches the tables for address as a read or write would and
	// caches the translation (PLOAD).
	Load(address uint32, fc uint8, write bool) error
	// Test searches the translation cache (level 0) or the tables down to
	// level and sets the status register (PTEST). It returns the physical
	// address of the last descriptor fetched.
	Test(address uint32, fc uint8, level int, write bool) (descriptor uint32, err error)
}

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

var (
	// ErrConfiguration is returned by SetRegister for a value that makes
	// the CPU take the MMU configuration exception (vector 56).
	ErrConfiguration = errors.New("mmu: configuration error")
	// ErrIllegalOperation is returned by the MC68851's Load and Test while
	// translation is disabled; the CPU takes the PMMU illegal operation
	// exception (vector 57).
	ErrIllegalOperation = errors.New("mmu: illegal operation")
)

// Status register bits: MMUSR on the MC68030, PSR on the MC68851.
const (
	SRBusError     uint16 = 1 << 15
	SRLimit        uint16 = 1 << 14
	SRSupervisor   uint16 = 1 << 13
	SRAccessLevel  uint16 = 1 << 12 // MC68851
	SRWriteProtect uint16 = 1 << 11
	SRInvalid      uint16 = 1 << 10
	SRModified     uint16 = 1 << 9
	SRGate         uint16 = 1 << 8 // MC68851
	SRShared       uint16 = 1 << 7 // MC68851: globally shared
	SRTransparent  uint16 = 1 << 6 // MC68030
	SRLevels       uint16 = 7      // number of levels searched
)

// TC bits (MC68030 User's Manual 9.7.2, MC68851 User's Manual 6.1.3).
const (
	tcEnable = 1 << 31
	tcSRE    = 1 << 25 // supervisor root pointer enable
	tcFCL    = 1 << 24 // function code lookup
	tcMask   = 0x83ffffff
)

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

// pageShift returns log2 of the page size tc selects.
func pageShift(tc uint32) uint32 { return (tc >> 20) & 15 }

// statusOf returns the status register bits both MMUs set from a table
// search for PTEST at levels 1-7.
func statusOf(r *searchResult) uint16 {
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
	return sr | uint16(min(r.levels, 7))
}
