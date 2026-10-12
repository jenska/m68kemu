package mmu

// searchResult is the outcome of a table search (MC68030 User's Manual,
// 9.5.2 and Figures 9-25 to 9-28).
type searchResult struct {
	physical   uint32 // page frame address
	wp         bool   // a descriptor on the way was write protected
	ci         bool
	modified   bool
	supervisor bool // a supervisor-only descriptor denied a user access
	invalid    bool // the search ended at an invalid descriptor
	limit      bool // an index was out of a descriptor's limits
	busError   bool // a descriptor fetch ended in a bus error
	levels     int  // number of descriptors fetched
	descriptor uint32
}

// fault reports whether accesses through the result take a bus error.
func (r *searchResult) fault() bool {
	return r.invalid || r.limit || r.busError || r.supervisor
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

// levels splits address into the table indexes TC defines.
func (m *MC68030) levels(address uint32, fc uint8) []level {
	var levels []level
	pos := 32 - (m.tc>>16)&15 // the initial shift ignores the top bits
	if m.tc&tcFCL != 0 {
		levels = append(levels, level{index: uint32(fc), fc: true, below: pos})
	}
	for _, ti := range tableIndexes(m.tc) {
		if ti == 0 {
			break
		}
		pos -= ti
		levels = append(levels, level{index: address >> pos & (1<<ti - 1), below: pos})
	}
	return levels
}

// search walks the translation tree for address. With update it sets the U
// bits of the descriptors it fetches and, for a write, the M bit of the page
// descriptor, as an access does; PTEST does not. It stops after maxLevels
// descriptor fetches.
func (m *MC68030) search(address uint32, fc uint8, write, update bool, maxLevels int) searchResult {
	var r searchResult
	rp := m.crp
	if m.tc&tcSRE != 0 && fc&4 != 0 {
		rp = m.srp
	}
	cur := descriptor{status: uint32(rp >> 32), address: uint32(rp), limited: true}
	levels := m.levels(address, fc)
	pageMask := uint32(1)<<m.pageShift() - 1

	if cur.dt() == dtPage { // no tables: the root pointer holds an offset
		for _, l := range levels {
			if !l.fc {
				r.limit = !cur.inLimit(l.index)
				break
			}
		}
		r.physical = (cur.address&^0xf + address) &^ pageMask
		return r
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
			return r
		case dtPage: // early termination
			r.limit = !l.fc && !cur.inLimit(l.index)
			r.physical = cur.address&^0xff + address&(1<<below-1)&^pageMask
			m.finishPage(&r, page, cur, write, update)
			return r
		}
		if !l.fc && !cur.inLimit(l.index) {
			r.limit = true
			return r
		}
		if r.levels == maxLevels {
			return r
		}
		size := uint32(4)
		if cur.dt() == dtLong {
			size = 8
		}
		at := cur.address&^0xf + l.index*size
		next, ok := m.fetch(&r, at, size == 8, i == len(levels)-1, fc, update)
		if !ok {
			return r
		}
		cur, page, below = next, at, l.below
	}

	switch cur.dt() {
	case dtInvalid:
		r.invalid = true
		return r
	case dtShort, dtLong: // an indirect descriptor in a page table
		if r.levels == maxLevels {
			return r
		}
		at := cur.address &^ 3
		next, ok := m.fetch(&r, at, cur.dt() == dtLong, false, fc, update)
		if !ok {
			return r
		}
		if next.dt() != dtPage {
			r.invalid = true
			return r
		}
		cur, page = next, at
	}
	r.physical = cur.address&^0xff + address&(1<<below-1)&^pageMask
	m.finishPage(&r, page, cur, write, update)
	return r
}

// fetch reads the descriptor at physical address at. Unless it is an
// indirect descriptor (a table descriptor in a page table, which has no
// status bits), it accumulates its write protection, checks its supervisor
// protection and sets its U bit. ok is false when the search ends here.
func (m *MC68030) fetch(r *searchResult, at uint32, long, pageTable bool, fc uint8, update bool) (descriptor, bool) {
	hi, err := m.mem.ReadLong(at)
	if err != nil {
		r.busError = true
		return descriptor{}, false
	}
	d := descriptor{status: hi, address: hi}
	if long {
		lo, err := m.mem.ReadLong(at + 4)
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
	if long && hi&descS != 0 && fc&4 == 0 {
		r.supervisor = true
		return d, false
	}
	if update && hi&descU == 0 {
		d.status |= descU
		if err := m.mem.WriteLong(at, d.status); err != nil {
			r.busError = true
			return descriptor{}, false
		}
	}
	return d, true
}

// finishPage takes the status of the page descriptor d, fetched from at, at
// the end of a successful search and sets its M bit for a write.
func (m *MC68030) finishPage(r *searchResult, at uint32, d descriptor, write, update bool) {
	r.ci = d.status&descCI != 0
	r.modified = d.status&descM != 0
	if write && update && !r.modified && !r.fault() && !r.wp {
		if err := m.mem.WriteLong(at, d.status|descM); err != nil {
			r.busError = true
			return
		}
		r.modified = true
	}
}
