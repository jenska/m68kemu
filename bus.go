package m68kemu

import "fmt"

// Device represents a memory-mapped peripheral on the address bus.
// Implementations must be safe for repeated Reset calls.
//
// A device must also be locatable: implement AddressRangeDevice for a single
// contiguous span, or ContainsDevice for a non-contiguous or state-dependent
// decode (a device may implement both, in which case Contains decides
// membership and AddressRange only bounds it).
type Device interface {
	Read(Size, uint32) (uint32, error)
	Write(Size, uint32, uint32) error
	Reset()
}

// AddressRangeDevice exposes a fixed, inclusive address range [start, end].
type AddressRangeDevice interface {
	AddressRange() (start uint32, end uint32)
}

// ContainsDevice decides, per address, whether the device answers for it.
type ContainsDevice interface {
	Contains(address uint32) bool
}

// deviceContains reports whether dev answers for address, using Contains when
// the device provides it and otherwise its AddressRange compared under mask.
func deviceContains(dev Device, address, mask uint32) bool {
	if c, ok := dev.(ContainsDevice); ok {
		return c.Contains(address)
	}
	if r, ok := dev.(AddressRangeDevice); ok {
		a := address & mask
		start, end := r.AddressRange()
		return a >= start&mask && a <= end&mask
	}
	return false
}

// addressMasker is implemented by devices that compare addresses themselves
// and must know how many address bits the bus decodes.
type addressMasker interface {
	setAddressMask(mask uint32)
}

// WaitStateDevice optionally advertises additional wait states a device
// imposes per transaction. Implementations may vary their contribution based
// on access size and address.
type WaitStateDevice interface {
	WaitStates(Size, uint32) uint32
}

// PeekDevice exposes a side-effect-free read path for debugging and disassembly.
type PeekDevice interface {
	Peek(Size, uint32) (uint32, error)
}

// Bus multiplexes memory access between attached devices and performs common
// checks such as alignment and bus error handling.
type Bus struct {
	// mask selects the address bits the bus decodes: 24 (the default) or
	// 32.
	mask       uint32
	devices    []Device
	waitStates uint32
	waitHook   func(states uint32)
	// cpu is the CPU the bus serves; NewCPU sets it.
	cpu          *CPU
	singleDevice Device
	singleRAM    *RAM
	// fastLow and fastHigh hold the memory the CPU reads and writes
	// directly, per 64 KB page, laid out like low and high (see bus_fastmem.go).
	fastLow             fastTable
	fastHigh            [256]*fastTable
	hasWaitStateDevices bool
	hasPageMap          bool
	// low holds the device ranges of the 64 KB pages of the first 16 MB,
	// all a 24-bit bus decodes. high extends the map to 32 bits: the top
	// address byte selects a pageTable, allocated only where devices sit.
	low  pageTable
	high [256]*pageTable
	// scanList holds every device in lookup order, each paired with its
	// containment test resolved once so the per-access scan does no type
	// assertions. The first rangedCount entries are the page-mapped devices;
	// the rest have no usable AddressRange.
	scanList    []scanEntry
	rangedCount int
}

type scanEntry struct {
	device   Device
	contains func(address uint32) bool
}

// mappedDevice wraps another device with an explicit address range, compared
// in the bus's address width (24 bits until a bus says otherwise).
type mappedDevice struct {
	start  uint32
	end    uint32
	mask   uint32
	device Device
}

type mappedWaitStateDevice struct {
	*mappedDevice
	waitStateDevice WaitStateDevice
}

type pageTable [256][]pageRange

type pageRange struct {
	start  uint32
	end    uint32
	device Device
	// contains is the device's own Contains check, or nil when the range
	// alone decides membership.
	contains func(address uint32) bool
}

// NewBus constructs a bus optionally seeded with devices.
func NewBus(devices ...Device) *Bus {
	b := &Bus{devices: devices, mask: 0xffffff}
	b.refreshTopology()
	return b
}

// setAddressMask sets the address bits the bus decodes: 0xffffff on the
// 24-bit bus of the MC68000 and MC68010, 0xffffffff from the MC68020 on.
// NewCPU sets the mask of its model.
func (b *Bus) setAddressMask(mask uint32) {
	if b.mask != mask {
		b.mask = mask
		b.refreshTopology()
	}
}

// AddressMask returns the address bits the bus decodes: 24 until NewCPU
// attaches an MC68020 or later, 32 after that.
func (b *Bus) AddressMask() uint32 {
	return b.mask
}

// AddDevice attaches an additional device to the bus.
func (b *Bus) AddDevice(device Device) {
	b.devices = append(b.devices, device)
	b.refreshTopology()
}

// SetWaitStates sets the base wait-state count charged to the CPU for every bus
// transaction. A device may add to it by implementing WaitStateDevice.
func (b *Bus) SetWaitStates(states uint32) {
	b.waitStates = states
	b.refreshTopology()
}

// Reset propagates a reset to all attached devices.
func (b *Bus) Reset() {
	for _, dev := range b.devices {
		dev.Reset()
	}
}

// Read forwards a read to the mapped device after performing alignment and
// mapping checks.
func (b *Bus) Read(s Size, address uint32) (uint32, error) {
	address &= b.mask

	if err := b.validateAlignment(address, s); err != nil {
		return 0, err
	}

	if s == Long {
		high, err := b.readCycle(Word, address)
		if err != nil {
			return 0, err
		}
		low, err := b.readCycle(Word, (address+uint32(Word))&b.mask)
		if err != nil {
			return 0, err
		}
		return (high << 16) | low, nil
	}

	return b.readCycle(s, address)
}

// Peek reads from the mapped device without charging wait states. Devices may
// use this for debugger-friendly, side-effect-free inspection.
func (b *Bus) Peek(s Size, address uint32) (uint32, error) {
	address &= b.mask

	if err := b.validateAlignment(address, s); err != nil {
		return 0, err
	}

	if s == Long {
		high, err := b.peekCycle(Word, address)
		if err != nil {
			return 0, err
		}
		low, err := b.peekCycle(Word, (address+uint32(Word))&b.mask)
		if err != nil {
			return 0, err
		}
		return (high << 16) | low, nil
	}

	return b.peekCycle(s, address)
}

// Write forwards a write to the mapped device after performing alignment and
// mapping checks.
func (b *Bus) Write(s Size, address uint32, value uint32) error {
	address &= b.mask

	if err := b.validateAlignment(address, s); err != nil {
		return err
	}

	if s == Long {
		if err := b.writeCycle(Word, address, value>>16); err != nil {
			return err
		}
		return b.writeCycle(Word, (address+uint32(Word))&b.mask, value)
	}

	return b.writeCycle(s, address, value)
}

func (b *Bus) wait(size Size, address uint32, dev Device) {
	if b.waitHook == nil || (b.waitStates == 0 && !b.hasWaitStateDevices) {
		return
	}

	states := b.waitStates
	if b.hasWaitStateDevices {
		if wsDev, ok := dev.(WaitStateDevice); ok {
			states += wsDev.WaitStates(size, address)
		}
	}

	if states > 0 {
		b.waitHook(states)
	}
}

func (b *Bus) readCycle(s Size, address uint32) (uint32, error) {
	dev := b.deviceForAddress(address)
	if dev == nil {
		return 0, BusError(address)
	}

	b.wait(s, address, dev)
	return dev.Read(s, address)
}

func (b *Bus) peekCycle(s Size, address uint32) (uint32, error) {
	dev := b.deviceForAddress(address)
	if dev == nil {
		return 0, BusError(address)
	}

	return peekDevice(dev, s, address)
}

func (b *Bus) writeCycle(s Size, address uint32, value uint32) error {
	dev := b.deviceForAddress(address)
	if dev == nil {
		return BusError(address)
	}

	b.wait(s, address, dev)
	return dev.Write(s, address, value)
}

func (b *Bus) refreshTopology() {
	b.singleDevice = nil
	b.singleRAM = nil
	b.hasWaitStateDevices = false
	b.hasPageMap = false
	b.low = pageTable{}
	b.high = [256]*pageTable{}
	b.scanList = b.scanList[:0]
	b.rangedCount = 0
	var unranged []scanEntry
	spans := make(map[Device]memorySpan)

	if len(b.devices) == 1 {
		b.singleDevice = b.devices[0]
		if ram, ok := b.singleDevice.(*RAM); ok {
			b.singleRAM = ram
		}
	}

	for _, dev := range b.devices {
		if m, ok := dev.(addressMasker); ok {
			m.setAddressMask(b.mask)
		}
		if _, ok := dev.(WaitStateDevice); ok {
			b.hasWaitStateDevices = true
		}

		var contains func(uint32) bool
		if span, ok := b.memorySpanOf(dev); ok {
			spans[dev] = span // its range decides; Contains is not asked
		} else if c, ok := dev.(ContainsDevice); ok {
			contains = c.Contains
		}
		ranged, hasRange := dev.(AddressRangeDevice)
		if contains == nil && !hasRange {
			panic(fmt.Sprintf("m68kemu: device %T is not locatable: implement AddressRangeDevice or ContainsDevice", dev))
		}

		var start, end uint32
		if hasRange {
			start, end = ranged.AddressRange()
			start &= b.mask
			end &= b.mask
		}
		if !hasRange || end < start {
			if contains == nil {
				contains = rangeContains(start, end, b.mask)
			}
			unranged = append(unranged, scanEntry{device: dev, contains: contains})
			continue
		}

		// Any device with a usable range goes into the page map, earlier
		// devices taking priority. A Contains method may still decode the
		// range sparsely, so the page entry keeps it as a filter.
		b.scanList = append(b.scanList, scanEntry{device: dev, contains: boundedContains(start, end, b.mask, contains)})
		b.hasPageMap = true
		for page := start >> 16; page <= end>>16; page++ {
			pageStart := page << 16
			b.addPageRange(page, max(start, pageStart), min(end, pageStart|0xffff), dev, contains)
		}
	}

	b.rangedCount = len(b.scanList)
	b.scanList = append(b.scanList, unranged...)
	b.buildFastTables(spans)
	if b.cpu != nil {
		// The CPU's caches may hold memory the new map no longer has.
		b.cpu.code, b.cpu.data = fastPage{}, fastPage{}
	}
}

// boundedContains returns a check for a device whose AddressRange bounds its
// optional Contains decode.
func boundedContains(start, end, mask uint32, contains func(uint32) bool) func(uint32) bool {
	if contains == nil {
		return rangeContains(start, end, mask)
	}
	return func(address uint32) bool {
		a := address & mask
		return a >= start && a <= end && contains(address)
	}
}

func rangeContains(start, end, mask uint32) func(uint32) bool {
	return func(address uint32) bool {
		a := address & mask
		return a >= start && a <= end
	}
}

func (b *Bus) findDevice(address uint32) Device {
	scan := b.scanList[b.rangedCount:]
	if b.hasPageMap {
		// Read, Peek and Write have masked the address already.
		if r := findInRanges(b.pageRanges(address), address); r != nil {
			if r.contains == nil || r.contains(address) {
				return r.device
			}
			// The address falls in a hole of a sparsely decoded device, so a
			// later overlapping device may still answer for it.
			scan = b.scanList
		}
	}

	for i := range scan {
		if scan[i].contains(address) {
			return scan[i].device
		}
	}

	return nil
}

func (b *Bus) deviceForAddress(address uint32) Device {
	if ram := b.singleRAM; ram != nil {
		if ram.Contains(address) {
			return ram
		}
		return nil
	}

	if dev := b.singleDevice; dev != nil {
		if deviceContains(dev, address, b.mask) {
			return dev
		}
		return nil
	}

	return b.findDevice(address)
}

func (b *Bus) validateAlignment(address uint32, s Size) error {
	if (s == Word || s == Long) && address&1 != 0 {
		return AddressError(address)
	}
	return nil
}

// MapDevice places device at the address range [start, end]. The range is
// compared in the address width of the bus the result is attached to.
func MapDevice(start, end uint32, device Device) Device {
	mapped := &mappedDevice{start: start, end: end, mask: 0xffffff, device: device}
	if ws, ok := device.(WaitStateDevice); ok {
		return &mappedWaitStateDevice{mappedDevice: mapped, waitStateDevice: ws}
	}
	return mapped
}

func (d *mappedDevice) setAddressMask(mask uint32) {
	d.mask = mask
}

func (d *mappedDevice) AddressRange() (uint32, uint32) {
	return d.start & d.mask, d.end & d.mask
}

func (d *mappedDevice) Contains(address uint32) bool {
	address &= d.mask
	return address >= d.start&d.mask && address <= d.end&d.mask
}

func (d *mappedDevice) Read(size Size, address uint32) (uint32, error) {
	if !d.containsAccess(size, address) {
		return 0, BusError(address & d.mask)
	}
	return d.device.Read(size, address)
}

func (d *mappedDevice) Peek(size Size, address uint32) (uint32, error) {
	if !d.containsAccess(size, address) {
		return 0, BusError(address & d.mask)
	}
	return peekDevice(d.device, size, address)
}

func (d *mappedDevice) Write(size Size, address uint32, value uint32) error {
	if !d.containsAccess(size, address) {
		return BusError(address & d.mask)
	}
	return d.device.Write(size, address, value)
}

func (d *mappedDevice) Reset() {
	d.device.Reset()
}

func (d *mappedWaitStateDevice) WaitStates(size Size, address uint32) uint32 {
	return d.waitStateDevice.WaitStates(size, address)
}

func (d *mappedDevice) containsAccess(size Size, address uint32) bool {
	address &= d.mask
	if !d.Contains(address) {
		return false
	}

	end := address + uint32(size) - 1
	if end < address {
		return false
	}

	return end <= d.end&d.mask
}

func peekDevice(dev Device, size Size, address uint32) (uint32, error) {
	peekable, ok := dev.(PeekDevice)
	if !ok {
		return 0, fmt.Errorf("peek unsupported at %08x", address)
	}
	return peekable.Peek(size, address)
}

func (b *Bus) addPageRange(page, start, end uint32, dev Device, contains func(uint32) bool) {
	if start > end {
		return
	}

	table := &b.low
	if top := page >> 8; top != 0 {
		if b.high[top] == nil {
			b.high[top] = new(pageTable)
		}
		table = b.high[top]
	}
	existing := table[page&0xff]
	uncovered := make([]pageRange, 0, 1)
	cursor := start
	for _, current := range existing {
		if current.end < cursor {
			continue
		}
		if current.start > end {
			break
		}
		if cursor < current.start {
			uncovered = append(uncovered, pageRange{
				start:    cursor,
				end:      min(end, current.start-1),
				device:   dev,
				contains: contains,
			})
		}
		if current.end >= end {
			cursor = end + 1
			break
		}
		cursor = current.end + 1
	}
	if cursor <= end {
		uncovered = append(uncovered, pageRange{start: cursor, end: end, device: dev, contains: contains})
	}
	if len(uncovered) == 0 {
		return
	}

	merged := make([]pageRange, 0, len(existing)+len(uncovered))
	i, j := 0, 0
	for i < len(existing) && j < len(uncovered) {
		if existing[i].start <= uncovered[j].start {
			merged = append(merged, existing[i])
			i++
			continue
		}
		merged = append(merged, uncovered[j])
		j++
	}
	merged = append(merged, existing[i:]...)
	merged = append(merged, uncovered[j:]...)
	table[page&0xff] = merged
}

// pageRanges returns the device ranges of the page that holds the masked
// address. The uint8 indexes keep the lookups free of bounds checks.
func (b *Bus) pageRanges(address uint32) []pageRange {
	if top := uint8(address >> 24); top != 0 {
		if table := b.high[top]; table != nil {
			return table[uint8(address>>16)]
		}
		return nil
	}
	return b.low[uint8(address>>16)]
}

// findInRanges returns the range of a page that holds address. It is kept
// separate from pageRanges so both stay small enough to inline.
func findInRanges(ranges []pageRange, address uint32) *pageRange {
	lo, hi := 0, len(ranges)
	for lo < hi {
		mid := lo + (hi-lo)/2
		current := &ranges[mid]
		if address < current.start {
			hi = mid
			continue
		}
		if address > current.end {
			lo = mid + 1
			continue
		}
		return current
	}
	return nil
}
