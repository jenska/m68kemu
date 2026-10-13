package mmu

// Root pointers an ATC entry was formed with, so loading one flushes its
// entries.
const (
	rootCRP = iota
	rootSRP
	rootDRP
)

// atcEntry is a cached translation of one page.
type atcEntry struct {
	valid    bool
	used     bool // recently used, for the pseudo-LRU replacement
	fc       uint8
	alias    uint8 // MC68851: task alias of the CPU root pointer
	root     uint8
	logical  uint32 // page address
	physical uint32 // page frame address
	bus      bool   // the access takes a bus error
	ci       bool
	wp       bool
	modified bool
	gate     bool
	shared   bool // MC68851: matches every task alias
	locked   bool // MC68851: exempt from replacement
}

// atc is an address translation cache: fully associative, with pseudo-LRU
// replacement that skips locked entries.
type atc struct {
	entries []atcEntry
}

func newATC(size int) atc { return atc{entries: make([]atcEntry, size)} }

// lookup returns the entry for the page of address with function code fc
// for task alias alias, or nil.
func (a *atc) lookup(address uint32, fc, alias uint8, shift uint32) *atcEntry {
	page := address >> shift
	for i := range a.entries {
		e := &a.entries[i]
		if e.valid && e.fc == fc && e.logical>>shift == page && (e.shared || e.alias == alias) {
			return e
		}
	}
	return nil
}

// victim returns the entry to replace: an invalid one, or else an unlocked
// one not used recently. When every unlocked entry has been used recently,
// the history starts over.
func (a *atc) victim() *atcEntry {
	for i := range a.entries {
		if !a.entries[i].valid {
			return &a.entries[i]
		}
	}
	for i := range a.entries {
		if e := &a.entries[i]; !e.used && !e.locked {
			return e
		}
	}
	var first *atcEntry
	for i := range a.entries {
		if e := &a.entries[i]; !e.locked {
			e.used = false
			if first == nil {
				first = e
			}
		}
	}
	return first
}

// locked returns the number of valid locked entries.
func (a *atc) locked() int {
	n := 0
	for _, e := range a.entries {
		if e.valid && e.locked {
			n++
		}
	}
	return n
}

// flush invalidates the entries match selects.
func (a *atc) flush(match func(*atcEntry) bool) {
	for i := range a.entries {
		if e := &a.entries[i]; e.valid && match(e) {
			e.valid = false
		}
	}
}

// flushAll invalidates every entry.
func (a *atc) flushAll() {
	for i := range a.entries {
		a.entries[i].valid = false
	}
}
