package m68kemu

import "fmt"

// Device represents a memory-mapped peripheral on the address bus.
// Implementations are expected to be safe for repeated Reset calls and
// must internally validate the address ranges they cover.
type Device interface {
	Contains(address uint32) bool
	Read(Size, uint32) (uint32, error)
	Write(Size, uint32, uint32) error
	Reset()
}

// AddressRangeDevice exposes a fixed address range that can be indexed by the bus.
type AddressRangeDevice interface {
	AddressRange() (start uint32, end uint32)
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

// WaitHook can be used to simulate wait states or count cycles for bus access.
type WaitHook func(states uint32)

// Bus multiplexes memory access between attached devices and performs common
// checks such as alignment and bus error handling.
type Bus struct {
	devices             []Device
	waitStates          uint32
	waitHook            WaitHook
	lastDevice          Device
	singleDevice        Device
	singleRAM           *RAM
	hasWaitStateDevices bool
	hasPageMap          bool
	pageDevices         [256][]Device
}

// MappedDevice wraps another device with an explicit 24-bit address range.
type MappedDevice struct {
	start  uint32
	end    uint32
	device Device
}

type mappedWaitStateDevice struct {
	*MappedDevice
	waitStateDevice WaitStateDevice
}

// NewBus constructs a bus optionally seeded with devices.
func NewBus(devices ...Device) *Bus {
	b := &Bus{devices: devices}
	b.refreshTopology()
	return b
}

// AddDevice attaches an additional device to the bus.
func (b *Bus) AddDevice(device Device) {
	b.devices = append(b.devices, device)
	b.refreshTopology()
}

// SetWaitStates defines how many states the bus should report for each
// transaction when a WaitHook is configured.
func (b *Bus) SetWaitStates(states uint32) {
	b.waitStates = states
}

// SetWaitHook installs a callback that receives the configured wait states for
// every transaction. Callers can use this to count cycles or block for a
// desired duration.
func (b *Bus) SetWaitHook(hook WaitHook) {
	b.waitHook = hook
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
	address &= 0xffffff

	if err := b.validateAlignment(address, s); err != nil {
		return 0, err
	}

	if s == Long {
		high, err := b.readCycle(Word, address)
		if err != nil {
			return 0, err
		}
		low, err := b.readCycle(Word, (address+uint32(Word))&0xffffff)
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
	address &= 0xffffff

	if err := b.validateAlignment(address, s); err != nil {
		return 0, err
	}

	if s == Long {
		high, err := b.peekCycle(Word, address)
		if err != nil {
			return 0, err
		}
		low, err := b.peekCycle(Word, (address+uint32(Word))&0xffffff)
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
	address &= 0xffffff

	if err := b.validateAlignment(address, s); err != nil {
		return err
	}

	if s == Long {
		if err := b.writeCycle(Word, address, value>>16); err != nil {
			return err
		}
		return b.writeCycle(Word, (address+uint32(Word))&0xffffff, value)
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
	b.lastDevice = nil
	b.pageDevices = [256][]Device{}

	if len(b.devices) == 1 {
		b.singleDevice = b.devices[0]
		if ram, ok := b.singleDevice.(*RAM); ok {
			b.singleRAM = ram
		}
	}

	for _, dev := range b.devices {
		if _, ok := dev.(WaitStateDevice); ok {
			b.hasWaitStateDevices = true
		}
		if ranged, ok := dev.(AddressRangeDevice); ok {
			start, end := ranged.AddressRange()
			start &= 0xffffff
			end &= 0xffffff
			if end < start {
				continue
			}
			b.hasPageMap = true
			for page := start >> 16; page <= end>>16; page++ {
				b.pageDevices[page] = append(b.pageDevices[page], dev)
			}
		}
	}
}

func (b *Bus) findDevice(address uint32) Device {
	if b.hasPageMap {
		pageDevices := b.pageDevices[(address&0xffffff)>>16]
		for i := len(pageDevices) - 1; i >= 0; i-- {
			dev := pageDevices[i]
			if dev.Contains(address) {
				b.lastDevice = dev
				return dev
			}
		}
	}

	if b.lastDevice != nil && b.lastDevice.Contains(address) {
		return b.lastDevice
	}

	for i := len(b.devices) - 1; i >= 0; i-- {
		dev := b.devices[i]
		if dev.Contains(address) {
			b.lastDevice = dev
			return dev
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
		if dev.Contains(address) {
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

func MapDevice(start, end uint32, device Device) Device {
	mapped := &MappedDevice{start: start & 0xffffff, end: end & 0xffffff, device: device}
	if ws, ok := device.(WaitStateDevice); ok {
		return &mappedWaitStateDevice{MappedDevice: mapped, waitStateDevice: ws}
	}
	return mapped
}

func (d *MappedDevice) AddressRange() (uint32, uint32) {
	return d.start, d.end
}

func (d *MappedDevice) Contains(address uint32) bool {
	address &= 0xffffff
	return address >= d.start && address <= d.end
}

func (d *MappedDevice) Read(size Size, address uint32) (uint32, error) {
	if !d.containsAccess(size, address) {
		return 0, BusError(address & 0xffffff)
	}
	return d.device.Read(size, address)
}

func (d *MappedDevice) Peek(size Size, address uint32) (uint32, error) {
	if !d.containsAccess(size, address) {
		return 0, BusError(address & 0xffffff)
	}
	peekable, ok := d.device.(PeekDevice)
	if !ok {
		return 0, fmt.Errorf("peek unsupported at %08x", address&0xffffff)
	}
	return peekable.Peek(size, address)
}

func (d *MappedDevice) Write(size Size, address uint32, value uint32) error {
	if !d.containsAccess(size, address) {
		return BusError(address & 0xffffff)
	}
	return d.device.Write(size, address, value)
}

func (d *MappedDevice) Reset() {
	d.device.Reset()
}

func (d *mappedWaitStateDevice) WaitStates(size Size, address uint32) uint32 {
	return d.waitStateDevice.WaitStates(size, address)
}

func (d *MappedDevice) containsAccess(size Size, address uint32) bool {
	address &= 0xffffff
	if !d.Contains(address) {
		return false
	}

	end := address + uint32(size) - 1
	if end < address {
		return false
	}

	return end <= d.end
}

func peekDevice(dev Device, size Size, address uint32) (uint32, error) {
	peekable, ok := dev.(PeekDevice)
	if !ok {
		return 0, fmt.Errorf("peek unsupported at %08x", address&0xffffff)
	}
	return peekable.Peek(size, address)
}
