package m68kemu

// This file collects APIs that exist to make embedding the core in a full
// machine emulator (bus with many memory-mapped devices, external scheduler,
// external interrupt controller) less awkward. They are all additive; the
// pre-existing setters and NewCPU signature keep working.

// IRQSource is a level-sensitive interrupt line the CPU samples at every
// instruction boundary. It suits a machine whose peripherals hold an interrupt
// asserted until it is serviced, rather than posting one-shot requests through
// RequestInterrupt.
type IRQSource interface {
	// PendingIRQ reports the highest interrupt level currently asserted (0 for
	// none) and the vector to take (AutoVector to auto-vector). It is called
	// once per instruction, so it must be cheap.
	PendingIRQ() (level, vector uint8)
	// AckIRQ is called with the level the CPU has just accepted, so the source
	// can lower that line. A source with edge/pulse semantics uses this to clear
	// the request it just delivered.
	AckIRQ(level uint8)
}

// SetIRQSource installs (or, with nil, removes) the level-sensitive interrupt
// line. A request queued through RequestInterrupt is still honoured and takes
// priority equal to its level; the source is consulted first.
func (cpu *cpu) SetIRQSource(src IRQSource) {
	cpu.irqSource = src
}

// Option configures CPU construction. Pass options to NewCPU.
type Option func(*cpuConfig)

type cpuConfig struct {
	deferReset bool
}

// WithDeferredReset skips the implicit Reset performed by NewCPU. The caller
// must invoke Reset explicitly once the bus and every device on it are fully
// wired. Use this when the reset vector is not readable at construction time,
// for example when the ROM that supplies it is attached to the bus after the
// CPU is created.
func WithDeferredReset() Option {
	return func(c *cpuConfig) { c.deferReset = true }
}

// Hooks bundles every observation callback the core supports. A nil field means
// "no callback". SetHooks installs all of them in one call and runs the
// internal trace/run-mode refresh exactly once, which lets a caller swap a
// whole tracing configuration atomically instead of touching five setters.
type Hooks struct {
	Trace     TraceCallback
	PreTrace  PreTraceCallback
	Exception ExceptionCallback
	Bus       BusAccessCallback
	Interrupt InterruptCallback
}

// SetHooks replaces all observation callbacks at once. Passing the zero Hooks
// clears every callback.
func (cpu *cpu) SetHooks(h Hooks) {
	cpu.trap = h.Trace
	cpu.preTrap = h.PreTrace
	cpu.exceptionTrap = h.Exception
	cpu.busTrap = h.Bus
	cpu.interruptTrap = h.Interrupt
	cpu.refreshDebugModes()
}

// Hooks returns the callbacks currently installed.
func (cpu *cpu) Hooks() Hooks {
	return Hooks{
		Trace:     cpu.trap,
		PreTrace:  cpu.preTrap,
		Exception: cpu.exceptionTrap,
		Bus:       cpu.busTrap,
		Interrupt: cpu.interruptTrap,
	}
}

// FastRegion describes one flat, directly-addressable span of guest memory the
// interpreter may service without going through the bus, for data reads, data
// writes (unless ReadOnly), and instruction/operand fetches whose whole access
// falls inside [Base, Base+len(Mem)).
//
// The caller guarantees that, while the region is installed, it behaves as
// plain memory: accesses have no side effects, the address is not remapped (no
// MMU banking, no overlay), and Mem is the live backing store (the core reads
// and, when not ReadOnly, writes it in place). Accesses not fully contained in
// any region fall through to the bus unchanged, and a write to a ReadOnly
// region also falls through, so it is safe to map only the sub-range that
// currently satisfies these guarantees and to re-install the set whenever that
// changes.
//
// WaitStates is the per-word-transfer penalty the bus would otherwise charge
// for this region (bus wait states plus the device's fixed contribution). It is
// added to the cycle counter on every access, doubled for a long access to
// match the bus running that as two word cycles. Address-dependent penalties
// (e.g. video bus contention) are not modelled on a fast region; leave such
// ranges on the bus if they must stay cycle-exact.
//
// Regions are bypassed automatically whenever a breakpoint or bus/instruction
// tracer is active, so debugging still observes every access.
type FastRegion struct {
	Base       uint32
	Mem        []byte
	WaitStates uint32
	ReadOnly   bool
}

type fastMemRegion struct {
	base     uint32
	last     uint32 // address of the final valid byte (base+len-1), 24-bit
	mem      []byte
	wait     uint32
	readOnly bool
}

// SetFastMemory replaces the set of fast regions. Call with no arguments to
// clear it. Regions should not overlap; if they do, the first one given wins.
func (cpu *cpu) SetFastMemory(regions ...FastRegion) {
	cpu.fastRegions = cpu.fastRegions[:0]
	for _, r := range regions {
		if len(r.Mem) == 0 {
			continue
		}
		base := r.Base & 0xffffff
		cpu.fastRegions = append(cpu.fastRegions, fastMemRegion{
			base:     base,
			last:     (base + uint32(len(r.Mem)) - 1) & 0xffffff,
			mem:      r.Mem,
			wait:     r.WaitStates,
			readOnly: r.ReadOnly,
		})
	}
	cpu.refreshRunModes()
}

// fastRegionFor returns the region wholly containing the size-byte access at
// address, or nil. The set is expected to hold only a handful of entries.
func (cpu *cpu) fastRegionFor(size Size, address uint32) *fastMemRegion {
	for i := range cpu.fastRegions {
		if cpu.fastRegions[i].contains(size, address) {
			return &cpu.fastRegions[i]
		}
	}
	return nil
}

// waitFor returns the wait-state charge for one access of the given size,
// mirroring the bus, which runs a long access as two word bus cycles and so
// charges its per-transfer penalty twice.
func (f *fastMemRegion) waitFor(size Size) uint32 {
	if size == Long {
		return f.wait * 2
	}
	return f.wait
}

// contains reports whether the whole size-byte access at address fits in the
// region without wrapping the 24-bit address space.
func (f *fastMemRegion) contains(size Size, address uint32) bool {
	end := address + uint32(size) - 1
	return end >= address && address >= f.base && end <= f.last
}

func (f *fastMemRegion) read(size Size, address uint32) (uint32, error) {
	if size != Byte && address&1 != 0 {
		return 0, AddressError(address)
	}
	idx := address - f.base
	switch size {
	case Byte:
		return uint32(f.mem[idx]), nil
	case Word:
		return uint32(f.mem[idx])<<8 | uint32(f.mem[idx+1]), nil
	default:
		return uint32(f.mem[idx])<<24 | uint32(f.mem[idx+1])<<16 |
			uint32(f.mem[idx+2])<<8 | uint32(f.mem[idx+3]), nil
	}
}

func (f *fastMemRegion) write(size Size, address, value uint32) error {
	if size != Byte && address&1 != 0 {
		return AddressError(address)
	}
	idx := address - f.base
	switch size {
	case Byte:
		f.mem[idx] = uint8(value)
	case Word:
		f.mem[idx] = uint8(value >> 8)
		f.mem[idx+1] = uint8(value)
	default:
		f.mem[idx] = uint8(value >> 24)
		f.mem[idx+1] = uint8(value >> 16)
		f.mem[idx+2] = uint8(value >> 8)
		f.mem[idx+3] = uint8(value)
	}
	return nil
}
