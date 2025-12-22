package m68kemu

// Device represents a memory-mapped peripheral on the address bus.
// Implementations are expected to be safe for repeated Reset calls and
// must internally validate the address ranges they cover.
type Device interface {
	Contains(address uint32) bool
	Read(Size, uint32) (uint32, error)
	Write(Size, uint32, uint32) error
	Reset()
}

// WaitStateDevice optionally advertises additional wait states a device
// imposes per transaction. Implementations may vary their contribution based
// on access size and address.
type WaitStateDevice interface {
	WaitStates(Size, uint32) uint32
}

// WaitHook can be used to simulate wait states or count cycles for bus access.
type WaitHook func(states uint32)

// Bus multiplexes memory access between attached devices and performs common
// checks such as alignment and bus error handling.
type Bus struct {
	devices    []Device
	waitStates uint32
	waitHook   WaitHook
	lastDevice Device
}

// NewBus constructs a bus optionally seeded with devices.
func NewBus(devices ...Device) *Bus {
	return &Bus{devices: devices}
}

// AddDevice attaches an additional device to the bus.
func (b *Bus) AddDevice(device Device) {
	b.devices = append(b.devices, device)
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
	if err := b.validateAlignment(address, s); err != nil {
		return 0, err
	}

	dev := b.findDevice(address)
	if dev == nil {
		return 0, BusError(address)
	}

	b.wait(s, address, dev)
	return dev.Read(s, address)
}

// Write forwards a write to the mapped device after performing alignment and
// mapping checks.
func (b *Bus) Write(s Size, address uint32, value uint32) error {
	if err := b.validateAlignment(address, s); err != nil {
		return err
	}

	dev := b.findDevice(address)
	if dev == nil {
		return BusError(address)
	}

	b.wait(s, address, dev)
	return dev.Write(s, address, value)
}

func (b *Bus) wait(size Size, address uint32, dev Device) {
	if b.waitHook == nil {
		return
	}

	states := b.waitStates
	if wsDev, ok := dev.(WaitStateDevice); ok {
		states += wsDev.WaitStates(size, address)
	}

	if states > 0 {
		b.waitHook(states)
	}
}

func (b *Bus) findDevice(address uint32) Device {
	if b.lastDevice != nil && b.lastDevice.Contains(address) {
		return b.lastDevice
	}

	for _, dev := range b.devices {
		if dev.Contains(address) {
			b.lastDevice = dev
			return dev
		}
	}

	return nil
}

func (b *Bus) validateAlignment(address uint32, s Size) error {
	if (s == Word || s == Long) && address&1 != 0 {
		return AddressError(address)
	}
	return nil
}
