package m68kemu

// MemoryDevice is implemented by a device that is plain memory over its whole
// AddressRange: reads and writes have no side effects, and Memory returns the
// live backing store, one byte per address from the start of the range. The
// CPU then reads, writes and fetches instructions from the bytes directly
// instead of calling the device, charging the bus wait states as the bus
// would. A write to read-only memory still goes to the device's Write.
//
// The bus does not ask Contains of a memory device and takes it out of the
// direct path while it implements WaitStateDevice. When a device changes its
// range or backing store, for example on a bank switch, call Bus.Remap.
// Breakpoints and bus tracing see every access either way.
type MemoryDevice interface {
	AddressRangeDevice
	Memory() (mem []byte, readOnly bool)
}

// fastPage is the directly accessible memory of one 64 KB page: mem holds
// the bytes from address base on, and base is even. A page without one has
// an empty mem.
type fastPage struct {
	mem      []byte
	base     uint32
	wait     uint32
	readOnly bool
}

type fastTable [256]fastPage

// noFastPage is returned for the pages of an unmapped top address byte.
var noFastPage fastPage

// fastPageFor returns the direct memory of the page that holds the masked
// address.
func (b *Bus) fastPageFor(address uint32) *fastPage {
	if top := uint8(address >> 24); top != 0 {
		if table := b.fastHigh[top]; table != nil {
			return &table[uint8(address>>16)]
		}
		return &noFastPage
	}
	return &b.fastLow[uint8(address>>16)]
}

// index returns the offset into p.mem of an aligned size-byte access at
// address, and whether the access lies wholly inside it.
func (p *fastPage) index(size Size, address uint32) (uint32, bool) {
	idx := address - p.base
	n := uint32(len(p.mem))
	return idx, idx < n && n-idx >= uint32(size)
}

// waitFor returns the wait states of one access, which the bus charges per
// word cycle: twice for a long.
func (p *fastPage) waitFor(size Size) uint32 {
	if size == Long {
		return p.wait * 2
	}
	return p.wait
}

func (p *fastPage) read(size Size, idx uint32) uint32 {
	mem := p.mem[idx:]
	switch size {
	case Byte:
		return uint32(mem[0])
	case Word:
		return uint32(mem[0])<<8 | uint32(mem[1])
	default:
		return uint32(mem[0])<<24 | uint32(mem[1])<<16 | uint32(mem[2])<<8 | uint32(mem[3])
	}
}

func (p *fastPage) write(size Size, idx, value uint32) {
	mem := p.mem[idx:]
	switch size {
	case Byte:
		mem[0] = uint8(value)
	case Word:
		mem[0] = uint8(value >> 8)
		mem[1] = uint8(value)
	default:
		mem[0] = uint8(value >> 24)
		mem[1] = uint8(value >> 16)
		mem[2] = uint8(value >> 8)
		mem[3] = uint8(value)
	}
}

// memorySpan is the backing store of a memory device, from address start on.
type memorySpan struct {
	start    uint32
	mem      []byte
	readOnly bool
}

// memorySpanOf returns the backing store of dev when the CPU may use it
// directly.
func (b *Bus) memorySpanOf(dev Device) (memorySpan, bool) {
	md, ok := dev.(MemoryDevice)
	if !ok {
		return memorySpan{}, false
	}
	if _, ok := dev.(WaitStateDevice); ok {
		return memorySpan{}, false
	}
	mem, readOnly := md.Memory()
	start, end := md.AddressRange()
	if len(mem) == 0 || start&b.mask != start || end&b.mask != end || end < start {
		return memorySpan{}, false
	}
	if uint64(end-start)+1 != uint64(len(mem)) {
		panic("m68kemu: memory device range does not match its backing store")
	}
	return memorySpan{start: start, mem: mem, readOnly: readOnly}, true
}

// buildFastTables gives every page the largest span of it that the page map
// assigns to a memory device.
func (b *Bus) buildFastTables(spans map[Device]memorySpan) {
	b.fastLow = fastTable{}
	b.fastHigh = [256]*fastTable{}
	if len(spans) == 0 {
		return
	}
	fill := func(dst *fastTable, src *pageTable) {
		for page := range src {
			var best *pageRange
			for i := range src[page] {
				r := &src[page][i]
				if _, ok := spans[r.device]; ok && (best == nil || r.end-r.start > best.end-best.start) {
					best = r
				}
			}
			if best == nil {
				continue
			}
			start := best.start
			if start&1 != 0 {
				start++ // keep the base even; the odd byte goes through the bus
				if start > best.end {
					continue
				}
			}
			span := spans[best.device]
			dst[page] = fastPage{
				mem:      span.mem[start-span.start : best.end-span.start+1],
				base:     start,
				wait:     b.waitStates,
				readOnly: span.readOnly,
			}
		}
	}
	fill(&b.fastLow, &b.low)
	for top, table := range b.high {
		if table != nil {
			b.fastHigh[top] = new(fastTable)
			fill(b.fastHigh[top], table)
		}
	}
}

// Remap rebuilds the bus's address decoding from its devices. Call it after
// a device has changed its address range or, for a MemoryDevice, its
// backing store.
func (b *Bus) Remap() {
	b.refreshTopology()
}
