package m68kemu

// This file collects APIs that exist to make embedding the core in a full
// machine emulator (external interrupt controller, construction options,
// observation hooks) less awkward.

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
func (cpu *CPU) SetIRQSource(src IRQSource) {
	cpu.irqSource = src
}

// Option configures CPU construction. Pass options to NewCPU.
type Option func(*cpuConfig)

type cpuConfig struct {
	deferReset    bool
	cycleRounding uint32
	model         Model
	coprocessors  []attachedCoprocessor
	mc68851       bool
}

// WithDeferredReset skips the implicit Reset performed by NewCPU. The caller
// must invoke Reset explicitly once the bus and every device on it are fully
// wired. Use this when the reset vector is not readable at construction time,
// for example when the ROM that supplies it is attached to the bus after the
// CPU is created.
func WithDeferredReset() Option {
	return func(c *cpuConfig) { c.deferReset = true }
}

// WithCycleRounding rounds the cycles of every instruction, together with any
// exception or interrupt processing it triggers, up to a multiple of n. An
// Atari ST needs n = 4: its GLUE and MMU let the CPU onto the bus only on
// 4-cycle boundaries, so an instruction listed at 10 cycles occupies 12.
// n of 0 or 1 disables rounding.
func WithCycleRounding(n uint32) Option {
	return func(c *cpuConfig) { c.cycleRounding = n }
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
func (cpu *CPU) SetHooks(h Hooks) {
	cpu.trap = h.Trace
	cpu.preTrap = h.PreTrace
	cpu.exceptionTrap = h.Exception
	cpu.busTrap = h.Bus
	cpu.interruptTrap = h.Interrupt
	cpu.refreshDebugModes()
}

// Hooks returns the callbacks currently installed.
func (cpu *CPU) Hooks() Hooks {
	return Hooks{
		Trace:     cpu.trap,
		PreTrace:  cpu.preTrap,
		Exception: cpu.exceptionTrap,
		Bus:       cpu.busTrap,
		Interrupt: cpu.interruptTrap,
	}
}
