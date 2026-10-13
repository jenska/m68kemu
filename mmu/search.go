package mmu

// Descriptor fields (MC68030 User's Manual 9.5.1, MC68851 User's Manual
// 5.1.5). Short descriptors are one long word; long descriptors put the
// limit and these bits in the first long word and the address in the
// second.
const (
	dtInvalid = 0
	dtPage    = 1
	dtShort   = 2 // valid, the next table holds 4-byte descriptors
	dtLong    = 3 // valid, the next table holds 8-byte descriptors

	descWP   = 1 << 2
	descU    = 1 << 3
	descM    = 1 << 4
	descLock = 1 << 5 // MC68851 page descriptors
	descCI   = 1 << 6
	descGate = 1 << 7 // MC68851 page descriptors
	descS    = 1 << 8 // long descriptors only
	descSG   = 1 << 9 // MC68851 long descriptors and root pointers
)

// searchResult is the outcome of a table search (MC68030 User's Manual
// 9.5.2 and Figures 9-25 to 9-28, MC68851 User's Manual 5.4).
type searchResult struct {
	physical   uint32 // page frame address
	wp         bool   // a descriptor on the way was write protected
	ci         bool
	modified   bool
	gate       bool
	lock       bool
	shared     bool // a descriptor or the root pointer was globally shared
	supervisor bool // a supervisor-only descriptor denied a user access
	access     bool // a read access level denied the access
	writeLevel bool // a write access level denied writes
	invalid    bool // the search ended at an invalid descriptor
	limit      bool // an index was out of a descriptor's limits
	busError   bool // a descriptor fetch ended in a bus error
	levels     int  // number of descriptors fetched
	descriptor uint32
}

// fault reports whether accesses through the result take a bus error.
func (r *searchResult) fault() bool {
	return r.invalid || r.limit || r.busError || r.supervisor || r.access
}

// descriptor is a root pointer or a descriptor fetched from a table.
type descriptor struct {
	status  uint32 // the (first) long word, with the type in bits 1-0
	address uint32 // the address field, not yet masked
	limited bool   // a long descriptor, which limits the next index
}

func (d descriptor) dt() uint32 { return d.status & 3 }

// inLimit reports whether index passes the descriptor's limit: the next
// table's index must not be above an upper limit or below a lower one.
func (d descriptor) inLimit(index uint32) bool {
	if !d.limited {
		return true
	}
	limit := (d.status >> 16) & 0x7fff
	if d.status&0x80000000 != 0 {
		return index >= limit
	}
	return index <= limit
}

// level is one level of the search: the function code or a field of the
// logical address.
type level struct {
	index uint32
	fc    bool   // the function code level, which has no limit check
	below uint32 // number of logical address bits below this field
}

// levels splits address into the table indexes tc defines, after the
// function code when fcl is set.
func levels(tc, address uint32, fc uint8, fcl bool) []level {
	var levels []level
	pos := 32 - (tc>>16)&15 // the initial shift ignores the top bits
	if fcl {
		levels = append(levels, level{index: uint32(fc & 7), fc: true, below: pos})
	}
	for _, ti := range tableIndexes(tc) {
		if ti == 0 {
			break
		}
		pos -= ti
		levels = append(levels, level{index: address >> pos & (1<<ti - 1), below: pos})
	}
	return levels
}

// accessLevel is the MC68851's access level check during a table search:
// with enabled, the access level of the logical address is compared with
// the RAL and WAL fields of long descriptors.
type accessLevel struct {
	enabled bool
	level   uint32
}

// search is a table search, shared by the MMUs.
type search struct {
	mem    Memory
	tc     uint32
	fc     uint8
	write  bool
	update bool // set U and M bits, as an access does; PTEST does not
	al     accessLevel
	r      searchResult
}

// run walks the tree that root pointer rp describes for address, with a
// function code level when fcl is set. It stops after maxLevels descriptor
// fetches.
func (s *search) run(address uint32, rp uint64, fcl bool, maxLevels int) searchResult {
	r := &s.r
	cur := descriptor{status: uint32(rp >> 32), address: uint32(rp), limited: true}
	r.shared = cur.status&descSG != 0
	levels := levels(s.tc, address, s.fc, fcl)
	pageMask := uint32(1)<<pageShift(s.tc) - 1

	if cur.dt() == dtPage { // no tables: the root pointer holds an offset
		for _, l := range levels {
			if !l.fc {
				r.limit = !cur.inLimit(l.index)
				break
			}
		}
		r.physical = (cur.address&^0xf + address) &^ pageMask
		return *r
	}
	if len(levels) > 0 && levels[0].fc {
		cur.limited = false // the root's limit applies to address indexes only
	}

	var page uint32 // address of the page descriptor
	below := uint32(32)
	for i, l := range levels {
		switch cur.dt() {
		case dtInvalid:
			r.invalid = true
			return *r
		case dtPage: // early termination
			r.limit = !l.fc && !cur.inLimit(l.index)
			r.physical = cur.address&^0xff + address&(1<<below-1)&^pageMask
			s.finishPage(page, cur)
			return *r
		}
		if !l.fc && !cur.inLimit(l.index) {
			r.limit = true
			return *r
		}
		if r.levels == maxLevels {
			return *r
		}
		size := uint32(4)
		if cur.dt() == dtLong {
			size = 8
		}
		at := cur.address&^0xf + l.index*size
		next, ok := s.fetch(at, size == 8, i == len(levels)-1)
		if !ok {
			return *r
		}
		cur, page, below = next, at, l.below
	}

	switch cur.dt() {
	case dtInvalid:
		r.invalid = true
		return *r
	case dtShort, dtLong: // an indirect descriptor in a page table
		if r.levels == maxLevels {
			return *r
		}
		at := cur.address &^ 3
		next, ok := s.fetch(at, cur.dt() == dtLong, false)
		if !ok {
			return *r
		}
		if next.dt() != dtPage {
			r.invalid = true
			return *r
		}
		cur, page = next, at
	}
	r.physical = cur.address&^0xff + address&(1<<below-1)&^pageMask
	s.finishPage(page, cur)
	return *r
}

// fetch reads the descriptor at physical address at. Unless it is an
// indirect descriptor (a table descriptor in a page table, which has no
// status bits), it accumulates its protection and sets its U bit. ok is
// false when the search ends here.
func (s *search) fetch(at uint32, long, pageTable bool) (descriptor, bool) {
	r := &s.r
	hi, err := s.mem.ReadLong(at)
	if err != nil {
		r.busError = true
		return descriptor{}, false
	}
	d := descriptor{status: hi, address: hi}
	if long {
		lo, err := s.mem.ReadLong(at + 4)
		if err != nil {
			r.busError = true
			return descriptor{}, false
		}
		d.address, d.limited = lo, true
	}
	r.levels++
	r.descriptor = at
	if d.dt() == dtInvalid || pageTable && d.dt() != dtPage {
		return d, true
	}
	if hi&descWP != 0 {
		r.wp = true
	}
	if long {
		if hi&descSG != 0 {
			r.shared = true
		}
		if s.al.enabled {
			if ral := (hi >> 13) & 7; s.al.level > ral {
				r.access = true
			}
			if wal := (hi >> 10) & 7; s.al.level > wal {
				r.writeLevel = true
			}
		}
		if hi&descS != 0 && s.fc&4 == 0 {
			r.supervisor = true
			return d, false
		}
	}
	if s.update && hi&descU == 0 {
		d.status |= descU
		if err := s.mem.WriteLong(at, d.status); err != nil {
			r.busError = true
			return descriptor{}, false
		}
	}
	return d, true
}

// finishPage takes the status of the page descriptor d, fetched from at, at
// the end of a successful search and sets its M bit for a write.
func (s *search) finishPage(at uint32, d descriptor) {
	r := &s.r
	r.ci = d.status&descCI != 0
	r.gate = d.status&descGate != 0
	r.lock = d.status&descLock != 0
	r.modified = d.status&descM != 0
	if r.levels == 0 { // early termination at the root pointer
		return
	}
	if s.write && s.update && !r.modified && !r.fault() && !r.wp && !r.writeLevel {
		if err := s.mem.WriteLong(at, d.status|descM); err != nil {
			r.busError = true
			return
		}
		r.modified = true
	}
}
