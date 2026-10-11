package m68kemu

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

const (
	Version           = "1.5.0"
	XBusError         = 2
	XAddressError     = 3
	XIllegal          = 4
	XDivByZero        = 5
	XPrivViolation    = 8
	XLineA            = 10
	XLineF            = 11
	XFormatError      = 14 // MC68010 and later: RTE found an unknown frame format
	XUninitializedInt = 15
	XTrap             = 32

	srCarry         = 0x0001
	srOverflow      = 0x0002
	srZero          = 0x0004
	srNegative      = 0x0008
	srExtend        = 0x0010
	srInterruptMask = 0x0700
	srSupervisor    = 0x2000
	srMaster        = 0x1000 // MC68020 and later: MSP instead of ISP
)

const (
	exceptionCyclesIllegal    uint32 = 34
	exceptionCyclesPrivilege  uint32 = 34
	exceptionCyclesTrapV      uint32 = 34
	exceptionCyclesInterrupt  uint32 = 44
	exceptionCyclesDivByZero  uint32 = 38
	exceptionCyclesCHK        uint32 = 40
	exceptionCyclesBusAddress uint32 = 50
)

const (
	eaMaskDataRegister    uint16 = 0x0800
	eaMaskAddressRegister uint16 = 0x0400
	eaMaskIndirect        uint16 = 0x0200
	eaMaskPostIncrement   uint16 = 0x0100
	eaMaskPreDecrement    uint16 = 0x0080
	eaMaskDisplacement    uint16 = 0x0040
	eaMaskIndex           uint16 = 0x0020
	eaMaskAbsoluteShort   uint16 = 0x0010
	eaMaskAbsoluteLong    uint16 = 0x0008
	eaMaskImmediate       uint16 = 0x0004
	eaMaskPCDisplacement  uint16 = 0x0002
	eaMaskPCIndex         uint16 = 0x0001
)

type (
	instruction func(*CPU) error

	AddressError uint32
	BusError     uint32

	BreakpointType int

	// cycleCalculator builds a static cycle count for a given opcode. Results are
	// stored in the opcodeSet during instruction registration and can be looked
	// up at execution time for fixed-cost instructions.
	cycleCalculator func(opcode uint16) uint32

	// AddressBus for accessing address areas
	AddressBus interface {
		Read(s Size, address uint32) (uint32, error)
		Write(s Size, address uint32, value uint32) error
		Reset()
	}

	// TraceInfo reports the outcome of a single executed instruction.
	TraceInfo struct {
		PC              uint32
		SR              uint16
		Registers       Registers
		BeforeRegisters Registers
		Opcode          uint16
		Bytes           []byte
		Mnemonic        string
		CycleDelta      uint32
		Cycles          uint64
	}

	TraceCallback func(TraceInfo)

	// PreTraceInfo reports an instruction just before execution.
	PreTraceInfo struct {
		PC        uint32
		SR        uint16
		Registers Registers
		Opcode    uint16
		Bytes     []byte
		Mnemonic  string
		Cycles    uint64
	}

	PreTraceCallback func(PreTraceInfo)

	// ExceptionStackFrameFormat identifies the frame layout captured for an exception.
	ExceptionStackFrameFormat int

	// ExceptionStackFrame mirrors the exception frame currently stored on the supervisor stack.
	ExceptionStackFrame struct {
		Format       ExceptionStackFrameFormat
		StackPointer uint32
		// StatusWord is the 68000 group 0 status word or the MC68010
		// special status word.
		StatusWord          uint16
		FaultAddress        uint32
		InstructionRegister uint16
		SR                  uint16
		PC                  uint32
		// VectorOffset is the vector offset from the format/vector word of
		// an MC68010 frame.
		VectorOffset uint16
		// DataOutput, DataInput and InstructionInput are the buffers of an
		// MC68010 format $8 frame; DataOutput and DataInput are also those
		// of an MC68020 format $A or $B frame.
		DataOutput       uint32
		DataInput        uint32
		InstructionInput uint16
		// InstructionAddress is the address of the instruction that caused
		// an MC68020 format $2 or $9 frame.
		InstructionAddress uint32
		// StageBAddress is the address of an MC68020 instruction fetch that
		// faulted (format $B).
		StageBAddress uint32
	}

	// ExceptionInfo describes one taken exception after vectoring has completed.
	ExceptionInfo struct {
		Vector        uint32
		PC            uint32
		NewPC         uint32
		Opcode        uint16
		OpcodeAddress uint32
		FaultAddress  uint32
		FaultValid    bool
		SR            uint16
		NewSR         uint16
		StackPointer  uint32
		Frame         ExceptionStackFrame
		FrameValid    bool
		InterruptMask uint8
		Group0        bool
	}

	ExceptionCallback func(ExceptionInfo)

	// InterruptInfo describes an interrupt that was accepted by the CPU.
	InterruptInfo struct {
		Level      uint8
		Vector     uint32
		AutoVector bool
		PC         uint32
		NewPC      uint32
		SR         uint16
		NewSR      uint16
	}

	InterruptCallback func(InterruptInfo)

	// BusAccessInfo describes one memory transaction observed by the CPU core.
	BusAccessInfo struct {
		Address          uint32
		Size             Size
		Value            uint32
		Write            bool
		InstructionFetch bool
		PC               uint32
		// FunctionCode is the value of the FC2-FC0 pins: 1 user data,
		// 2 user program, 5 supervisor data, 6 supervisor program, or the
		// SFC/DFC value of a MOVES access.
		FunctionCode uint16
	}

	BusAccessCallback func(BusAccessInfo)

	DebugFaultInfo struct {
		Address          uint32
		PC               uint32
		Opcode           uint16
		FunctionCode     uint16
		Write            bool
		InstructionFetch bool
		Valid            bool
	}

	DebugState struct {
		Registers     Registers
		InException   bool
		InterruptMask uint8
		LastFault     DebugFaultInfo
		LastException ExceptionInfo
		HasException  bool
		LastInterrupt InterruptInfo
		HasInterrupt  bool
	}

	AddressRange struct {
		Start uint32
		End   uint32
	}

	// RunUntilOptions controls which conditions stop the instruction runner.
	RunUntilOptions struct {
		MaxInstructions   uint64
		StopOnException   bool
		StopOnIllegal     bool
		StopAtPC          []uint32
		StopOnPCRange     *AddressRange
		StopWhenPCOutside *AddressRange
		StopOnBusAccess   func(BusAccessInfo) bool
		StopPredicate     func(RunPredicateInfo) bool
	}

	RunStopReason int

	// RunResult reports why RunUntil stopped and what the CPU observed while stopping.
	RunResult struct {
		Reason       RunStopReason
		Instructions uint64
		Cycles       uint64
		PC           uint32
		Exception    ExceptionInfo
		HasException bool
		BusAccess    BusAccessInfo
		HasBusAccess bool
		Interrupt    InterruptInfo
		HasInterrupt bool
	}

	// RunPredicateInfo is passed to StopPredicate after each completed instruction.
	RunPredicateInfo struct {
		Registers     Registers
		Instructions  uint64
		Cycles        uint64
		LastException ExceptionInfo
		HasException  bool
		LastBusAccess BusAccessInfo
		HasBusAccess  bool
		LastInterrupt InterruptInfo
		HasInterrupt  bool
	}

	HistoryKind int

	// HistoryEntry stores one recent debug event in the optional rolling history buffer.
	HistoryEntry struct {
		Kind      HistoryKind
		Trace     TraceInfo
		Exception ExceptionInfo
		Interrupt InterruptInfo
		BusAccess BusAccessInfo
	}

	Breakpoint struct {
		Address   uint32
		OnExecute bool
		OnRead    bool
		OnWrite   bool
		Halt      bool
		Callback  func(BreakpointEvent) error
	}

	BreakpointEvent struct {
		Type      BreakpointType
		Address   uint32
		Registers Registers
	}

	BreakpointHit struct {
		Address uint32
		Type    BreakpointType
	}

	// CycleScheduler runs callbacks at points in emulated time. Attached to
	// a CPU with SetScheduler, it follows the CPU's cycle counter: the CPU
	// compares its counter with the time of the next event and only calls
	// into the scheduler once that time is reached, so an idle scheduler
	// costs nothing. Devices that change continuously, such as a video
	// counter, compute their state from Now when it is read.
	CycleScheduler struct {
		now       uint64 // scheduler time, in event ("device") cycles
		numer     uint64 // device-clock Hz  (0 means the default 1:1 ratio)
		denom     uint64 // CPU-clock Hz
		carry     uint64 // sub-cycle remainder of the CPU->device conversion
		events    []scheduledEvent
		eventHead int
		// cpu is the CPU the scheduler follows, or nil. mark is the CPU's
		// cycle count up to which now has been brought.
		cpu  *CPU
		mark uint64
	}

	scheduledEvent struct {
		At uint64
		Fn func(now uint64)
	}

	// Registers represents the programmer visible registers of the 68000 CPU.
	Registers struct {
		D   [8]uint32
		A   [8]uint32
		PC  uint32
		SR  uint16
		SSP uint32
		USP uint32
		IR  uint16 // instruction register
		// VBR, SFC and DFC exist from the MC68010 on and stay zero on the
		// MC68000. Exception vectors are read from VBR + 4*vector.
		VBR uint32
		SFC uint8 // source function code for MOVES
		DFC uint8 // destination function code for MOVES
		// MSP, CACR and CAAR exist from the MC68020 on. There SSP is the
		// interrupt stack pointer (ISP), and the M bit of SR selects MSP
		// or ISP in supervisor mode. Like USP and SSP, MSP holds the value
		// of an inactive stack pointer; the active one is A[7].
		MSP  uint32
		CACR uint32
		CAAR uint32
	}

	faultInfo struct {
		address        uint32
		pc             uint32
		ir             uint16
		functionCode   uint16
		write          bool
		notInstruction bool
		valid          bool
		// size and value describe the access, for the MC68010 special
		// status word and data output buffer.
		size  Size
		value uint32
	}

	// CPU is an emulated 68000-family processor, returned by NewCPU.
	// Different CPUs may run in parallel goroutines, each with its own Bus.
	// A single CPU must not be used from several goroutines at once, except
	// for RequestInterrupt, which any goroutine may call.
	CPU struct {
		regs   Registers
		cycles uint64
		bus    *Bus
		// ops is the dispatch table of the CPU's model, shared with every
		// other CPU of that model.
		ops   *opcodeSet
		model Model
		// addrMask selects the address bits the CPU drives: 24 on the
		// MC68000 and MC68010, 32 from the MC68020 on.
		addrMask uint32
		// cycleRounding is the multiple each instruction's cycles are rounded
		// up to (see WithCycleRounding); 0 or 1 disables it.
		cycleRounding          uint32
		instructionStartCycles uint64
		// fastFetch is true while no breakpoint or tracer watches the bus,
		// so instruction words may come straight from the bus's direct
		// memory (see bus_fastmem.go) without the bookkeeping of readContext.
		// code caches the direct memory of the page the last instruction
		// word came from; it is empty while fastFetch is false.
		fastFetch bool
		code      fastPage
		// data caches the direct memory of the page the last data access
		// went to.
		data fastPage
		// debugActive is set whenever any per-instruction debug hook is live
		// (breakpoints, pre-trace, or instruction/bus tracing), so
		// executeNext can skip all of that bookkeeping on the common path.
		debugActive   bool
		trap          TraceCallback
		preTrap       PreTraceCallback
		exceptionTrap ExceptionCallback
		busTrap       BusAccessCallback
		interruptTrap InterruptCallback
		scheduler     *CycleScheduler
		// nextEvent is the cycle count at which the scheduler's next event
		// is due, or the maximum when nothing is scheduled.
		nextEvent  uint64
		interrupts *interruptController
		irqSource  IRQSource

		stopped bool
		// stepDirty is set once the current step records any of the
		// step-local state below, which resetStepDebugState clears.
		stepDirty bool

		fault              faultInfo
		inException        bool
		lastException      ExceptionInfo
		lastExceptionValid bool
		stepException      ExceptionInfo
		stepExceptionValid bool
		lastInterrupt      InterruptInfo
		lastInterruptValid bool
		stepInterrupt      InterruptInfo
		stepInterruptValid bool
		lastOpcodePC       uint32
		lastOpcodePCValid  bool
		currentOpcodePC    uint32
		currentOpcodeValid bool
		collectStepBus     bool
		traceInstructions  bool
		traceBus           bool
		traceBytes         []byte
		stepBusAccesses    []BusAccessInfo
		breakpoints        map[uint32]Breakpoint
		history            []HistoryEntry
		historyNext        int
		historyCount       int
		// coprocessors are attached by ID through WithCoprocessor.
		coprocessors [8]Coprocessor
		// coprocessorTimings holds each coprocessor that implements
		// CoprocessorTiming, so cpGeneral does not ask on every instruction.
		coprocessorTimings [8]CoprocessorTiming
		// coprocessorBusy is the cycle count at which each coprocessor
		// finishes its current command (see CoprocessorTiming).
		coprocessorBusy [8]uint64
		// cpOperand is reused for the source operands of cpGEN.
		cpOperand []byte
		// operands are the CPU's own effective-address operand objects.
		operands eaTables
		// restartRegs holds the registers at the start of the current
		// instruction on the MC68010, where a bus or address error rolls
		// back to them so RTE can rerun the instruction. It sits at the end,
		// away from the fields the MC68000 touches on every instruction.
		restartRegs Registers
	}
)

const (
	BreakpointExecute BreakpointType = iota
	BreakpointRead
	BreakpointWrite
)

const (
	RunStopNone RunStopReason = iota
	RunStopInstructionLimit
	RunStopPC
	RunStopPCInRange
	RunStopPCOutsideRange
	RunStopBusAccess
	RunStopPredicate
	RunStopException
	RunStopIllegalOpcode
)

const (
	ExceptionStackFrameGroup12 ExceptionStackFrameFormat = iota // 68000: SR, PC
	ExceptionStackFrameGroup0                                   // 68000 bus or address error
	ExceptionStackFrameFormat0                                  // MC68010: SR, PC, format/vector word
	ExceptionStackFrameFormat8                                  // MC68010 bus or address error
	ExceptionStackFrameFormat1                                  // MC68020: throwaway
	ExceptionStackFrameFormat2                                  // MC68020: SR, PC, format/vector word, instruction address
	ExceptionStackFrameFormat9                                  // MC68020: coprocessor mid-instruction
	ExceptionStackFrameFormatA                                  // MC68020: short bus fault
	ExceptionStackFrameFormatB                                  // MC68020: long bus fault
)

const (
	HistoryInstruction HistoryKind = iota
	HistoryException
	HistoryInterrupt
	HistoryBusAccess
)

const (
	functionCodeUserData       uint16 = 1
	functionCodeUserProgram    uint16 = 2
	functionCodeSupervisorData uint16 = 5
	functionCodeSupervisorProg uint16 = 6

	group12ExceptionFrameSize uint32 = uint32(Long + Word)
	group0ExceptionFrameSize  uint32 = 14
)

type accessContext struct {
	functionCode   uint16
	notInstruction bool
	write          bool
}

func (regs *Registers) String() string {
	var result strings.Builder
	fmt.Fprintf(&result, "SR %04x PC %08x USP %08x SSP %08x SP %08x IR %04x\n", regs.SR, regs.PC, regs.USP, regs.SSP, regs.A[7], regs.IR)
	for i := range regs.D {
		fmt.Fprintf(&result, "D%d %08x ", i, regs.D[i])
	}
	result.WriteByte('\n')
	for i := range regs.A {
		fmt.Fprintf(&result, "A%d %08x ", i, regs.A[i])
	}
	result.WriteByte('\n')

	return result.String()
}

func (cpu *CPU) String() string {
	return fmt.Sprintf("%s\n%s", cpu.disassemblyString(), cpu.regs.String())
}

func (ae AddressError) Error() string {
	return fmt.Sprintf("AddressError at %08x", uint32(ae))
}

func (be BusError) Error() string {
	return fmt.Sprintf("BusError at %08x", uint32(be))
}

func (bh BreakpointHit) Error() string {
	return fmt.Sprintf("breakpoint hit at %08x (%v)", bh.Address, bh.Type)
}

func (bt BreakpointType) String() string {
	switch bt {
	case BreakpointExecute:
		return "execute"
	case BreakpointRead:
		return "read"
	case BreakpointWrite:
		return "write"
	default:
		return "unknown"
	}
}

func (reason RunStopReason) String() string {
	switch reason {
	case RunStopInstructionLimit:
		return "instruction-limit"
	case RunStopPC:
		return "pc"
	case RunStopPCInRange:
		return "pc-in-range"
	case RunStopPCOutsideRange:
		return "pc-outside-range"
	case RunStopBusAccess:
		return "bus-access"
	case RunStopPredicate:
		return "predicate"
	case RunStopException:
		return "exception"
	case RunStopIllegalOpcode:
		return "illegal-opcode"
	default:
		return "none"
	}
}

func (cpu *CPU) read(size Size, address uint32) (uint32, error) {
	return cpu.readCtx(size, address, accessContext{functionCode: cpu.dataFunctionCode()})
}

func (cpu *CPU) readProgram(size Size, address uint32) (uint32, error) {
	return cpu.readCtx(size, address, accessContext{functionCode: cpu.programFunctionCode()})
}

func (cpu *CPU) readSystemProgram(size Size, address uint32) (uint32, error) {
	return cpu.readCtx(size, address, accessContext{functionCode: functionCodeSupervisorProg, notInstruction: true})
}

func (cpu *CPU) readCtx(size Size, address uint32, ctx accessContext) (uint32, error) {
	address &= cpu.addrMask
	if address&1 != 0 && size != Byte && cpu.model >= M68020 && !ctx.programSpace() {
		return cpu.readMisaligned(size, address, ctx)
	}
	switch size {
	case Byte, Word, Long:
		if cpu.breakpoints != nil {
			if err := cpu.checkAccessBreakpoint(address, BreakpointRead); err != nil {
				return 0, err
			}
		}
		if idx, ok := cpu.dataIndex(size, address); ok {
			result := cpu.data.read(size, idx)
			if cpu.data.wait != 0 {
				cpu.addCycles(cpu.data.waitFor(size))
			}
			if cpu.shouldTraceBusAccess(ctx) {
				cpu.traceBusAccess(size, address, result, ctx)
			}
			return result, nil
		}
		result, err := cpu.bus.Read(size, address)
		if err != nil {
			cpu.recordFault(faultAddress(address, err), ctx, size, 0)
		} else if cpu.shouldTraceBusAccess(ctx) {
			cpu.traceBusAccess(size, address, result, ctx)
		}
		return uint32(result), err
	default:
		return 0, fmt.Errorf("unknown operand size")

	}
}

func (cpu *CPU) write(size Size, address uint32, value uint32) error {
	return cpu.writeCtx(size, address, value, accessContext{functionCode: cpu.dataFunctionCode(), write: true})
}

func (cpu *CPU) writeSystemData(size Size, address uint32, value uint32) error {
	return cpu.writeCtx(size, address, value, accessContext{functionCode: functionCodeSupervisorData, notInstruction: true, write: true})
}

func (cpu *CPU) writeCtx(size Size, address uint32, value uint32, ctx accessContext) error {
	address &= cpu.addrMask
	if address&1 != 0 && size != Byte && cpu.model >= M68020 && !ctx.programSpace() {
		return cpu.writeMisaligned(size, address, value, ctx)
	}
	switch size {
	case Byte, Word, Long:
		if cpu.breakpoints != nil {
			if err := cpu.checkAccessBreakpoint(address, BreakpointWrite); err != nil {
				return err
			}
		}
		if idx, ok := cpu.dataIndex(size, address); ok && !cpu.data.readOnly {
			cpu.data.write(size, idx, value)
			if cpu.data.wait != 0 {
				cpu.addCycles(cpu.data.waitFor(size))
			}
			if cpu.shouldTraceBusAccess(ctx) {
				cpu.traceBusAccess(size, address, value, ctx)
			}
			return nil
		}
		if err := cpu.bus.Write(size, address, value); err != nil {
			cpu.recordFault(faultAddress(address, err), ctx, size, value)
			return err
		}
		if cpu.shouldTraceBusAccess(ctx) {
			cpu.traceBusAccess(size, address, value, ctx)
		}
		return nil
	default:
		return fmt.Errorf("unknown operand size")

	}
}

// Registers returns a copy of the programmer-visible registers. A[7] is the
// active stack pointer; SSP, USP and MSP hold the inactive ones.
func (cpu *CPU) Registers() Registers {
	return cpu.regs
}

// SetRegisters replaces the registers, with the same convention as
// Registers: A[7] is the stack pointer that SR selects, and SSP, USP and MSP
// hold the others. Registers a model does not have are ignored by it.
func (cpu *CPU) SetRegisters(regs Registers) {
	cpu.regs = regs
}

// SetPC sets the program counter, for example to start a program loaded
// into memory without going through the reset vector. It also ends a STOP.
func (cpu *CPU) SetPC(pc uint32) {
	cpu.regs.PC = pc
	cpu.stopped = false
}

func (cpu *CPU) DebugState() DebugState {
	return DebugState{
		Registers:     cpu.regs,
		InException:   cpu.inException,
		InterruptMask: cpu.interruptMask(),
		LastFault:     cpu.fault.snapshot(),
		LastException: cpu.lastException,
		HasException:  cpu.lastExceptionValid,
		LastInterrupt: cpu.lastInterrupt,
		HasInterrupt:  cpu.lastInterruptValid,
	}
}

func (cpu *CPU) refreshDebugModes() {
	cpu.traceInstructions = cpu.trap != nil || len(cpu.history) != 0
	cpu.traceBus = cpu.busTrap != nil || len(cpu.history) != 0 || cpu.collectStepBus
	cpu.refreshRunModes()
}

// refreshRunModes rebuilds the cached execution fast paths (instruction-fetch
// shortcut and the debug-hook flag). It must be called whenever breakpoints
// or tracing state change; the bus clears the code cache itself when its
// memory map changes.
func (cpu *CPU) refreshRunModes() {
	cpu.debugActive = cpu.breakpoints != nil || cpu.preTrap != nil ||
		cpu.traceInstructions || cpu.traceBus

	cpu.fastFetch = cpu.breakpoints == nil && !cpu.traceInstructions && !cpu.traceBus
	cpu.code = fastPage{}
}

func (cpu *CPU) requireSupervisor() (bool, error) {
	if cpu.regs.SR&srSupervisor != 0 {
		return true, nil
	}
	return false, cpu.exceptionWithCycles(XPrivViolation, exceptionCyclesPrivilege)
}

// SetScheduler attaches s to the CPU, or detaches the current scheduler when
// s is nil. The scheduler restarts at the CPU's cycle count with no events.
// A scheduler follows one CPU at a time: attaching it to another CPU
// detaches it from the first.
func (cpu *CPU) SetScheduler(s *CycleScheduler) {
	if cpu.scheduler != nil {
		cpu.scheduler.cpu = nil
	}
	cpu.scheduler = s
	cpu.nextEvent = noEvent
	if s != nil {
		if s.cpu != nil && s.cpu != cpu {
			s.cpu.scheduler = nil
			s.cpu.nextEvent = noEvent
		}
		s.cpu = cpu
		s.Reset(cpu.cycles)
	}
}

func (cpu *CPU) Scheduler() *CycleScheduler {
	return cpu.scheduler
}

// RequestInterrupt queues an interrupt at the given level (1-7). Pass
// AutoVector for vector to auto-vector it (vector 24+level); any other value is
// taken as the device-supplied vector number.
//
// Unlike the other CPU methods, RequestInterrupt may be called from any
// goroutine, also while the CPU runs. The CPU takes the request at the first
// instruction boundary after it sees it, so a request from another goroutine
// lands at a point in emulated time that depends on the host's scheduling.
// For cycle-exact timing, request the interrupt from a CycleScheduler event,
// which runs on the CPU's goroutine.
func (cpu *CPU) RequestInterrupt(level, vector uint8) error {
	return cpu.interrupts.request(level, vector)
}

func (cpu *CPU) AddBreakpoint(bp Breakpoint) {
	if cpu.breakpoints == nil {
		cpu.breakpoints = make(map[uint32]Breakpoint)
	}
	cpu.breakpoints[bp.Address] = bp
	cpu.refreshRunModes()
}

func (cpu *CPU) SetHistoryLimit(limit int) {
	if limit <= 0 {
		cpu.history = nil
		cpu.historyNext = 0
		cpu.historyCount = 0
		cpu.refreshDebugModes()
		return
	}

	previous := cpu.History()
	cpu.history = make([]HistoryEntry, limit)
	cpu.historyNext = 0
	cpu.historyCount = 0

	if len(previous) > limit {
		previous = previous[len(previous)-limit:]
	}
	for _, entry := range previous {
		cpu.appendHistory(entry)
	}
	cpu.refreshDebugModes()
}

func (cpu *CPU) History() []HistoryEntry {
	if cpu.historyCount == 0 {
		return nil
	}

	result := make([]HistoryEntry, 0, cpu.historyCount)
	start := cpu.historyNext - cpu.historyCount
	if start < 0 {
		start += len(cpu.history)
	}

	for i := range cpu.historyCount {
		index := (start + i) % len(cpu.history)
		result = append(result, cloneHistoryEntry(cpu.history[index]))
	}

	return result
}

func (cpu *CPU) handleBreakpoint(bp Breakpoint, kind BreakpointType, address uint32) error {
	event := BreakpointEvent{Type: kind, Address: address, Registers: cpu.regs}
	if bp.Callback != nil {
		if err := bp.Callback(event); err != nil {
			return err
		}
	}

	if bp.Halt {
		return BreakpointHit{Address: address, Type: kind}
	}

	return nil
}

func (cpu *CPU) rememberOpcodePC(pc uint32) {
	cpu.lastOpcodePC = pc
	cpu.lastOpcodePCValid = true
}

// beginInstructionContext records the instruction boundary so nested helpers,
// fault handling, and bus tracing can all attribute work to the same opcode.
func (cpu *CPU) beginInstructionContext(pc uint32) {
	cpu.currentOpcodePC = pc & cpu.addrMask
	cpu.currentOpcodeValid = true
	cpu.instructionStartCycles = cpu.cycles
}

// beginRestartableContext is beginInstructionContext plus, from the MC68010
// on, saving the registers a bus or address error rolls back to.
func (cpu *CPU) beginRestartableContext(pc uint32) {
	cpu.beginInstructionContext(pc)
	if cpu.model != M68000 {
		cpu.restartRegs = cpu.regs
	}
}

func (cpu *CPU) endInstructionContext() {
	cpu.roundInstructionCycles()
	cpu.currentOpcodeValid = false
}

// roundInstructionCycles pads the current instruction's cycles up to the next
// multiple of cycleRounding. It is idempotent, so the debug path can round
// before reporting the instruction's cycle delta.
func (cpu *CPU) roundInstructionCycles() {
	n := uint64(cpu.cycleRounding)
	if n <= 1 || !cpu.currentOpcodeValid {
		return
	}
	if rem := (cpu.cycles - cpu.instructionStartCycles) % n; rem != 0 {
		cpu.addCycles(uint32(n - rem))
	}
}

func (cpu *CPU) currentOpcodeAddress(fallback uint32) uint32 {
	if cpu.currentOpcodeValid {
		return cpu.currentOpcodePC
	}
	return fallback & cpu.addrMask
}

func (cpu *CPU) debugPC() uint32 {
	if cpu.currentOpcodeValid {
		return cpu.currentOpcodePC
	}
	if cpu.lastOpcodePCValid {
		return cpu.lastOpcodePC & cpu.addrMask
	}
	return cpu.regs.PC & cpu.addrMask
}

func (cpu *CPU) consumeOpcodePC() uint32 {
	pc := cpu.regs.PC
	if cpu.lastOpcodePCValid {
		pc = cpu.lastOpcodePC
	}
	cpu.lastOpcodePCValid = false
	return pc
}

func (cpu *CPU) opcodeException(vector uint32, instructionPC uint32) error {
	if vector == XLineA || vector == XLineF {
		cpu.overrideInstructionCycles(exceptionCyclesIllegal)
		return cpu.raiseExceptionWithPC(vector, cpu.regs.SR|srSupervisor, instructionPC, instructionPC)
	}
	return cpu.exceptionWithCycles(vector, exceptionCyclesIllegal)
}

// ExecuteInstruction runs an instruction without fetching it from memory. This allows
// callers to execute single instructions directly through the API.
func (cpu *CPU) executeInstruction(opcode uint16) error {
	instructionPC := cpu.consumeOpcodePC()
	if cpu.currentOpcodeValid {
		return cpu.dispatchInstruction(opcode, instructionPC)
	}
	return cpu.executeInstructionWithContext(opcode, instructionPC)
}

// executeInstructionWithContext is the slow path for callers (tests, the public
// API) that invoke an opcode without an active instruction context.
func (cpu *CPU) executeInstructionWithContext(opcode uint16, instructionPC uint32) error {
	cpu.beginRestartableContext(instructionPC)
	defer cpu.endInstructionContext()
	return cpu.dispatchInstruction(opcode, instructionPC)
}

// dispatchInstruction runs the opcode handler assuming the instruction context
// is already set up by the caller.
func (cpu *CPU) dispatchInstruction(opcode uint16, instructionPC uint32) error {
	cpu.regs.IR = opcode
	cpu.addCycles(cpu.ops.cycles[opcode])

	handler := cpu.ops.handlers[opcode]
	if handler == nil {
		return cpu.opcodeException(exceptionVectorForOpcode(opcode), instructionPC)
	}

	if err := handler(cpu); err != nil {
		return cpu.handleFaultError(err, true)
	}
	return nil
}

func exceptionVectorForOpcode(opcode uint16) uint32 {
	switch opcode & 0xf000 {
	case 0xa000:
		return XLineA
	case 0xf000:
		return XLineF
	default:
		return XIllegal
	}
}

func (cpu *CPU) raiseException(vector uint32, newSR uint16) error {
	return cpu.raiseExceptionWithPC(vector, newSR, cpu.regs.PC, cpu.regs.PC)
}

// synchronousException takes an exception the instruction raised, stacking
// stackedPC.
func (cpu *CPU) synchronousException(vector uint32, stackedPC uint32) error {
	return cpu.raiseExceptionWithPC(vector, cpu.regs.SR|srSupervisor, stackedPC, stackedPC)
}

func (cpu *CPU) trapException(vector uint32) error {
	stackedPC := cpu.regs.PC & cpu.addrMask
	instructionPC := cpu.currentOpcodeAddress(stackedPC)
	if stackedPC == instructionPC {
		stackedPC = (instructionPC + uint32(Word)) & cpu.addrMask
	}
	return cpu.synchronousException(vector, stackedPC)
}

func (cpu *CPU) raiseExceptionWithPC(vector uint32, newSR uint16, stackedPC uint32, exceptionPC uint32) error {
	if vector > 255 {
		return fmt.Errorf("invalid vector %d", vector)
	}

	vectorOffset := vector << 2
	originalSR := cpu.regs.SR
	opcodeAddress := cpu.currentOpcodeAddress(exceptionPC)
	cpu.inException = true
	defer func() {
		cpu.inException = false
	}()
	cpu.setSR(newSR)

	// 68000 stack frame: PC (long), SR (word). The MC68010 format $0 frame
	// adds the format/vector word above them; the MC68020 format $2 frame
	// also the address of the instruction that caused the exception.
	format := ExceptionStackFrameGroup12
	var instructionAddress uint32
	if cpu.model != M68000 {
		format = ExceptionStackFrameFormat0
		frameFormat := uint16(0)
		if cpu.model >= M68020 && usesFormat2(vector) {
			format, frameFormat = ExceptionStackFrameFormat2, 2
			instructionAddress = opcodeAddress
			if err := cpu.pushException(Long, instructionAddress); err != nil {
				return err
			}
		}
		if err := cpu.pushException(Word, uint32(formatWord(frameFormat, vector))); err != nil {
			return err
		}
	}
	if err := cpu.pushException(Long, stackedPC); err != nil {
		return err
	}
	if err := cpu.pushException(Word, uint32(originalSR)); err != nil {
		return err
	}

	handler, err := cpu.readVector(vectorOffset)
	if err != nil {
		return err
	}

	cpu.regs.PC = handler
	frame := ExceptionStackFrame{
		Format:       format,
		StackPointer: cpu.regs.A[7],
		SR:           originalSR,
		PC:           stackedPC,
	}
	if format != ExceptionStackFrameGroup12 {
		frame.VectorOffset = uint16(vectorOffset)
		frame.InstructionAddress = instructionAddress
	}
	cpu.dispatchException(ExceptionInfo{
		Vector:        vector,
		PC:            exceptionPC,
		NewPC:         handler,
		Opcode:        cpu.regs.IR,
		OpcodeAddress: opcodeAddress,
		SR:            originalSR,
		NewSR:         cpu.regs.SR,
		StackPointer:  cpu.regs.A[7],
		Frame:         frame,
		FrameValid:    true,
		InterruptMask: cpu.interruptMask(),
	})
	return nil
}

func (cpu *CPU) exception(vector uint32) error {
	return cpu.raiseException(vector, cpu.regs.SR|srSupervisor)
}

func (cpu *CPU) exceptionWithCycles(vector uint32, total uint32) error {
	cpu.overrideInstructionCycles(total)
	return cpu.exception(vector)
}

// raiseGroup0Exception takes a bus or address error for the fault recorded
// in cpu.fault.
func (cpu *CPU) raiseGroup0Exception(vector uint32) error {
	if cpu.model != M68000 {
		return cpu.raiseBusFault(vector)
	}

	originalSR := cpu.regs.SR
	opcodeAddress := cpu.currentOpcodeAddress(cpu.fault.pc)
	cpu.inException = true
	defer func() {
		cpu.inException = false
	}()
	cpu.setSR(originalSR | srSupervisor)

	sp := cpu.regs.A[7] - group0ExceptionFrameSize
	cpu.regs.A[7] = sp

	if err := cpu.writeSystemData(Word, sp, uint32(cpu.fault.statusWord())); err != nil {
		return err
	}
	if err := cpu.writeSystemData(Long, sp+2, cpu.fault.address); err != nil {
		return err
	}
	if err := cpu.writeSystemData(Word, sp+6, uint32(cpu.fault.ir)); err != nil {
		return err
	}
	if err := cpu.writeSystemData(Word, sp+8, uint32(originalSR)); err != nil {
		return err
	}
	if err := cpu.writeSystemData(Long, sp+10, cpu.fault.pc); err != nil {
		return err
	}

	handler, err := cpu.readVector(vector << 2)
	if err != nil {
		return err
	}
	cpu.regs.PC = handler
	frame := ExceptionStackFrame{
		Format:              ExceptionStackFrameGroup0,
		StackPointer:        sp,
		StatusWord:          cpu.fault.statusWord(),
		FaultAddress:        cpu.fault.address,
		InstructionRegister: cpu.fault.ir,
		SR:                  originalSR,
		PC:                  cpu.fault.pc,
	}
	cpu.dispatchException(ExceptionInfo{
		Vector:        vector,
		PC:            cpu.fault.pc,
		NewPC:         handler,
		Opcode:        cpu.fault.ir,
		OpcodeAddress: opcodeAddress,
		FaultAddress:  cpu.fault.address,
		FaultValid:    cpu.fault.valid,
		SR:            originalSR,
		NewSR:         cpu.regs.SR,
		StackPointer:  sp,
		Frame:         frame,
		FrameValid:    true,
		InterruptMask: cpu.interruptMask(),
		Group0:        true,
	})
	return nil
}

// setSR keeps USP/SSP in sync when the supervisor bit changes.
func (cpu *CPU) setSR(value uint16) {
	if cpu.model >= M68020 {
		cpu.setSR020(value)
		return
	}
	if (cpu.regs.SR^value)&srSupervisor != 0 {
		if value&srSupervisor != 0 {
			cpu.regs.USP = cpu.regs.A[7]
			cpu.regs.A[7] = cpu.regs.SSP
		} else {
			cpu.regs.SSP = cpu.regs.A[7]
			cpu.regs.A[7] = cpu.regs.USP
		}
	}
	cpu.regs.SR = value
}

// setSR020 is setSR for the MC68020 and later, which have three stack
// pointers: USP (S clear), ISP (S set, M clear) and MSP (S and M set).
func (cpu *CPU) setSR020(value uint16) {
	if old, now := cpu.stackSlot(cpu.regs.SR), cpu.stackSlot(value); old != now {
		*old = cpu.regs.A[7]
		cpu.regs.A[7] = *now
	}
	cpu.regs.SR = value
}

// stackSlot returns the register that holds the stack pointer SR selects
// while it is inactive.
func (cpu *CPU) stackSlot(sr uint16) *uint32 {
	switch {
	case sr&srSupervisor == 0:
		return &cpu.regs.USP
	case sr&srMaster != 0:
		return &cpu.regs.MSP
	}
	return &cpu.regs.SSP
}

// stackPointer returns the stack pointer slot points to, which is A7 if it
// is the active one.
func (cpu *CPU) stackPointer(slot *uint32) *uint32 {
	if slot == cpu.stackSlot(cpu.regs.SR) {
		return &cpu.regs.A[7]
	}
	return slot
}

func (cpu *CPU) readVector(offset uint32) (uint32, error) {
	if offset&1 != 0 {
		return 0, AddressError(offset)
	}
	if offset >= 256<<2 {
		return 0, AddressError(offset)
	}

	address, err := cpu.readSystemProgram(Long, cpu.regs.VBR+offset)
	if err != nil {
		return 0, err
	}
	if address == 0 {
		return cpu.readSystemProgram(Long, cpu.regs.VBR+XUninitializedInt<<2)
	}
	return address, nil
}

func (cpu *CPU) interrupt(level uint8, vector uint32, autoVector bool) error {
	originalPC := cpu.regs.PC
	originalSR := cpu.regs.SR
	newSR := (cpu.regs.SR & ^uint16(srInterruptMask)) | srSupervisor | (uint16(level) << 8)
	cpu.addCycles(exceptionCyclesInterrupt)
	if err := cpu.raiseException(vector, newSR); err != nil {
		return err
	}
	if cpu.model >= M68020 && cpu.regs.SR&srMaster != 0 {
		// The interrupt frame went onto the master stack. Clear M and leave
		// a throwaway copy on the interrupt stack, whose RTE returns to the
		// master stack.
		throwawaySR := cpu.regs.SR
		cpu.setSR(throwawaySR &^ srMaster)
		for _, w := range []struct {
			size  Size
			value uint32
		}{{Word, uint32(formatWord(1, vector))}, {Long, originalPC}, {Word, uint32(throwawaySR)}} {
			if err := cpu.pushException(w.size, w.value); err != nil {
				return err
			}
		}
	}
	cpu.dispatchInterrupt(InterruptInfo{
		Level:      level,
		Vector:     vector,
		AutoVector: autoVector,
		PC:         originalPC,
		NewPC:      cpu.regs.PC,
		SR:         originalSR,
		NewSR:      cpu.regs.SR,
	})
	return nil
}

func (cpu *CPU) checkInterrupts() error {
	mask := uint8((cpu.regs.SR & srInterruptMask) >> 8)

	if cpu.irqSource != nil {
		if level, vector := cpu.irqSource.PendingIRQ(); level > mask {
			cpu.stopped = false
			autoVector := vector == AutoVector
			resolved := uint32(vector)
			if autoVector {
				resolved = autoVectorBase + uint32(level)
			}
			cpu.irqSource.AckIRQ(level)
			return cpu.interrupt(level, resolved, autoVector)
		}
	}

	if cpu.interrupts == nil || !cpu.interrupts.above(mask) {
		return nil
	}
	return cpu.serviceInterrupt()
}

func (cpu *CPU) serviceInterrupt() error {
	level, vector, autoVector, ok := cpu.interrupts.pending(cpu.regs.SR)
	if !ok {
		return nil
	}

	cpu.stopped = false

	return cpu.interrupt(level, vector, autoVector)
}

// errExceptionTaken ends an instruction that has already taken an
// exception, for example from a reserved addressing mode in the middle of
// decoding an operand.
var errExceptionTaken = errors.New("m68kemu: exception taken")

func (cpu *CPU) handleFaultError(err error, currentInstruction bool) error {
	var vector uint32
	switch err.(type) {
	case BusError:
		vector = XBusError
	case AddressError:
		vector = XAddressError
	default:
		if err == errExceptionTaken {
			return nil
		}
		return err
	}
	if currentInstruction {
		cpu.overrideInstructionCycles(exceptionCyclesBusAddress)
	} else {
		cpu.addCycles(exceptionCyclesBusAddress)
	}
	return cpu.raiseGroup0Exception(vector)
}

// executeNext runs one whole instruction boundary: fetch, execute, and the
// post-instruction interrupt check. Debug bookkeeping (breakpoints, tracing) is
// handled by executeNextDebug, taken only while a debug hook is live.
func (cpu *CPU) executeNext() error {
	if cpu.debugActive {
		return cpu.executeNextDebug()
	}

	cpu.beginRestartableContext(cpu.regs.PC)

	opcode, err := cpu.fetchOpcode()
	if err != nil {
		cpu.endInstructionContext()
		return cpu.handleFaultError(err, false)
	}
	err = cpu.dispatchInstruction(opcode, cpu.consumeOpcodePC())
	if err == nil {
		err = cpu.checkInterrupts()
	}
	cpu.endInstructionContext()
	return err
}

// executeNextDebug is executeNext with the full pre/post instruction debug
// pipeline: execute breakpoints, register/cycle snapshots, pre-trace, and the
// final trace dispatch.
func (cpu *CPU) executeNextDebug() error {
	if cpu.breakpoints != nil {
		if err := cpu.checkExecuteBreakpoint(cpu.regs.PC); err != nil {
			return err
		}
	}

	pc := cpu.regs.PC
	var beforeRegs Registers
	if cpu.preTrap != nil || cpu.traceInstructions {
		beforeRegs = cpu.regs
	}
	var beforeCycles uint64
	if cpu.traceInstructions {
		beforeCycles = cpu.cycles
	}
	cpu.beginRestartableContext(pc)

	opcode, err := cpu.fetchOpcode()
	if err != nil {
		cpu.endInstructionContext()
		return cpu.handleFaultError(err, false)
	}
	if cpu.preTrap != nil {
		cpu.sendPreTrace(pc, opcode, beforeRegs)
	}
	err = cpu.dispatchInstruction(opcode, cpu.consumeOpcodePC())
	if err == nil {
		err = cpu.checkInterrupts()
	}
	if err != nil {
		cpu.endInstructionContext()
		return err
	}
	if cpu.traceInstructions {
		cpu.roundInstructionCycles()
		cpu.sendTrace(pc, beforeRegs, uint32(cpu.cycles-beforeCycles))
	}
	cpu.endInstructionContext()
	return nil
}

// Step fetches the next opcode at the program counter and executes it.
func (cpu *CPU) Step() error {
	if cpu.hasStepDebugState() {
		cpu.resetStepDebugState()
	}

	if cpu.stopped {
		if err := cpu.checkInterrupts(); err != nil {
			return err
		}
		if cpu.stopped {
			return nil
		}
	}

	return cpu.executeNext()
}

// RunCycles executes instructions until at least the requested number of cycles
// have elapsed. Execution may exceed the budget when the final instruction's
// cost pushes the cycle count past the requested amount.
func (cpu *CPU) RunCycles(budget uint64) error {
	start := cpu.cycles
	target := start + budget

	for cpu.cycles < target {
		if cpu.hasStepDebugState() {
			cpu.resetStepDebugState()
		}
		before := cpu.cycles

		// Inline Step() for performance
		if cpu.stopped {
			if err := cpu.checkInterrupts(); err != nil {
				return err
			}
			if cpu.stopped {
				// If still stopped, consume cycles to prevent infinite tight loop without progress
				cpu.addCycles(4)
				continue
			}
		}

		if err := cpu.executeNext(); err != nil {
			return err
		}

		if cpu.cycles == before {
			return fmt.Errorf("execution stalled at %04x: cycles not advancing", cpu.regs.PC)
		}
	}
	return nil
}

func (cpu *CPU) RunUntil(options RunUntilOptions) (RunResult, error) {
	var result RunResult
	previousCollectStepBus := cpu.collectStepBus
	cpu.collectStepBus = options.StopOnBusAccess != nil || options.StopPredicate != nil
	cpu.refreshDebugModes()
	defer func() {
		cpu.collectStepBus = previousCollectStepBus
		cpu.refreshDebugModes()
	}()

	if reason, ok := cpu.runStopReason(options, &result); ok {
		result.Reason = reason
		result.PC = cpu.regs.PC
		return result, nil
	}

	startCycles := cpu.cycles
	for {
		if options.MaxInstructions > 0 && result.Instructions >= options.MaxInstructions {
			result.Reason = RunStopInstructionLimit
			result.PC = cpu.regs.PC
			result.Cycles = cpu.cycles - startCycles
			return result, nil
		}

		if err := cpu.Step(); err != nil {
			return result, err
		}

		result.Instructions++
		result.Cycles = cpu.cycles - startCycles
		result.PC = cpu.regs.PC
		if cpu.stepExceptionValid {
			result.Exception = cpu.stepException
			result.HasException = true
		}
		if matched, ok := cpu.matchStepBusAccess(options); ok {
			result.BusAccess = matched
			result.HasBusAccess = true
		}
		if cpu.stepInterruptValid {
			result.Interrupt = cpu.stepInterrupt
			result.HasInterrupt = true
		}

		if reason, ok := cpu.runStopReason(options, &result); ok {
			result.Reason = reason
			return result, nil
		}
	}
}

// hasStepDebugState reports whether the previous step left scratch state that
// resetStepDebugState must clear.
func (cpu *CPU) hasStepDebugState() bool {
	return cpu.stepDirty
}

// resetStepDebugState clears per-step scratch state before a new Step/RunCycles
// iteration so stop conditions only see events from the current instruction.
func (cpu *CPU) resetStepDebugState() {
	cpu.stepException = ExceptionInfo{}
	cpu.stepExceptionValid = false
	cpu.stepInterrupt = InterruptInfo{}
	cpu.stepInterruptValid = false
	cpu.traceBytes = cpu.traceBytes[:0]
	cpu.stepBusAccesses = cpu.stepBusAccesses[:0]
	cpu.stepDirty = false
}

func (cpu *CPU) sendPreTrace(pc uint32, opcode uint16, regs Registers) {
	if cpu.preTrap == nil {
		return
	}

	bytes := traceInstructionBytes(cpu.bus, pc, opcode)
	cpu.preTrap(PreTraceInfo{
		PC:        pc & cpu.addrMask,
		SR:        regs.SR,
		Registers: regs,
		Opcode:    opcode,
		Bytes:     bytes,
		Mnemonic:  traceInstructionMnemonic(cpu.bus, pc, bytes),
		Cycles:    cpu.cycles,
	})
}

// sendTrace snapshots the final instruction result and also feeds the optional
// history buffer so callers can inspect the recent lead-up to a fault.
func (cpu *CPU) sendTrace(pc uint32, before Registers, cycleDelta uint32) {
	if !cpu.traceInstructions {
		return
	}

	bytes := cpu.traceInstructionBytes(pc)
	info := TraceInfo{
		PC:              pc & cpu.addrMask,
		SR:              cpu.regs.SR,
		Registers:       cpu.regs,
		BeforeRegisters: before,
		Opcode:          cpu.regs.IR,
		Bytes:           bytes,
		Mnemonic:        traceInstructionMnemonic(cpu.bus, pc, bytes),
		CycleDelta:      cycleDelta,
		Cycles:          cpu.cycles,
	}
	cpu.appendHistory(HistoryEntry{Kind: HistoryInstruction, Trace: info})
	if cpu.trap != nil {
		cpu.trap(info)
	}
}

func (cpu *CPU) checkExecuteBreakpoint(pc uint32) error {
	if cpu.breakpoints == nil {
		return nil
	}

	if bp, ok := cpu.breakpoints[pc]; ok && bp.OnExecute {
		return cpu.handleBreakpoint(bp, BreakpointExecute, pc)
	}
	return nil
}

func (cpu *CPU) checkAccessBreakpoint(address uint32, kind BreakpointType) error {
	if cpu.breakpoints == nil {
		return nil
	}

	bp, ok := cpu.breakpoints[address]
	if !ok {
		return nil
	}

	switch kind {
	case BreakpointRead:
		if !bp.OnRead {
			return nil
		}
	case BreakpointWrite:
		if !bp.OnWrite {
			return nil
		}
	}

	return cpu.handleBreakpoint(bp, kind, address)
}

func (cpu *CPU) fetchOpcode() (uint16, error) {
	pc := cpu.regs.PC
	opcode, ok := cpu.readProgramFastWord(pc)
	if !ok {
		value, err := cpu.readProgram(Word, pc)
		if err != nil {
			return 0, err
		}
		opcode = uint16(value)
	}
	cpu.rememberOpcodePC(pc)
	cpu.regs.PC += uint32(Word)
	return opcode, nil
}

func (cpu *CPU) Reset() error {
	cpu.regs = Registers{SR: 0x2700}
	if cpu.interrupts == nil {
		cpu.interrupts = newInterruptController()
	} else {
		cpu.interrupts.reset()
	}
	cpu.stopped = false
	ssp, err := cpu.bus.Read(Long, 0)
	if err != nil {
		return err
	}
	cpu.regs.A[7] = ssp
	cpu.regs.SSP = ssp
	pc, err := cpu.bus.Read(Long, 4)
	if err != nil {
		return err
	}
	cpu.regs.PC = pc
	cpu.cycles = 0
	cpu.coprocessorBusy = [8]uint64{}
	cpu.fault = faultInfo{}
	cpu.inException = false
	cpu.lastException = ExceptionInfo{}
	cpu.lastExceptionValid = false
	cpu.lastInterrupt = InterruptInfo{}
	cpu.lastInterruptValid = false
	cpu.lastOpcodePC = 0
	cpu.lastOpcodePCValid = false
	cpu.currentOpcodePC = 0
	cpu.currentOpcodeValid = false
	cpu.resetStepDebugState()
	cpu.historyNext = 0
	cpu.historyCount = 0
	cpu.refreshDebugModes()
	if cpu.scheduler != nil {
		cpu.scheduler.Reset(0)
	}
	return nil
}

// NewCPU builds a CPU bound to bus. By default it performs a reset (reading the
// vector from the bus); pass WithDeferredReset to skip that.
func NewCPU(bus *Bus, opts ...Option) (*CPU, error) {
	var cfg cpuConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	if !cfg.model.implemented() {
		return nil, fmt.Errorf("%w: %v", ErrModelUnsupported, cfg.model)
	}
	for _, a := range cfg.coprocessors {
		if cfg.model < M68020 || a.id < 0 || a.id > 7 {
			return nil, fmt.Errorf("%w: ID %d on the %v", ErrCoprocessorUnsupported, a.id, cfg.model)
		}
	}
	return newCPU(bus, cfg)
}

// newCPU builds a CPU of any model, implemented or not; tests use it for
// models that NewCPU does not accept yet.
func newCPU(bus *Bus, cfg cpuConfig) (*CPU, error) {
	if bus.cpu != nil {
		return nil, ErrBusInUse
	}
	bus.setAddressMask(cfg.model.addressMask())
	c := CPU{bus: bus, ops: opcodesFor(cfg.model), model: cfg.model, addrMask: cfg.model.addressMask(), interrupts: newInterruptController(), cycleRounding: cfg.cycleRounding, nextEvent: noEvent}
	c.operands.init(cfg.model)
	for _, a := range cfg.coprocessors {
		c.coprocessors[a.id&7] = a.c
		c.coprocessorTimings[a.id&7], _ = a.c.(CoprocessorTiming)
	}
	bus.cpu = &c
	bus.waitHook = func(states uint32) { c.addCycles(states) }

	if cfg.deferReset {
		c.refreshDebugModes()
		return &c, nil
	}
	if err := c.Reset(); err != nil {
		bus.cpu, bus.waitHook = nil, nil
		return nil, err
	}
	return &c, nil
}

func validEA(opcode, mask uint16) bool {
	if mask == 0 {
		return true
	}

	// Modes 0-6 take the mask bits from eaMaskDataRegister down to
	// eaMaskIndex in order; mode 7 has its own bit per register.
	mode, reg := eaFields(opcode)
	if mode < 7 {
		return mask&(eaMaskDataRegister>>mode) != 0
	}
	if reg > 4 {
		return false
	}
	return mask&[...]uint16{eaMaskAbsoluteShort, eaMaskAbsoluteLong, eaMaskPCDisplacement, eaMaskPCIndex, eaMaskImmediate}[reg] != 0
}

func (cpu *CPU) push(s Size, value uint32) error {
	cpu.regs.A[7] -= uint32(s)
	return cpu.write(s, cpu.regs.A[7], value)
}

func (cpu *CPU) pushException(s Size, value uint32) error {
	cpu.regs.A[7] -= uint32(s)
	return cpu.writeSystemData(s, cpu.regs.A[7], value)
}

func (cpu *CPU) pop(s Size) (uint32, error) {
	res, err := cpu.read(s, cpu.regs.A[7])
	if err != nil {
		return 0, err
	}
	cpu.regs.A[7] += uint32(s) // sometimes odd
	return res, nil
}

func (cpu *CPU) popPc(s Size) (uint32, error) {
	var res uint32
	var ok bool
	switch s {
	case Word:
		var w uint16
		w, ok = cpu.readProgramFastWord(cpu.regs.PC)
		res = uint32(w)
	case Long:
		res, ok = cpu.readProgramFastLong(cpu.regs.PC)
	}
	if !ok {
		var err error
		if res, err = cpu.readProgram(s, cpu.regs.PC); err != nil {
			return 0, err
		}
	}
	cpu.regs.PC += uint32(s)
	if cpu.regs.PC&1 != 0 {
		cpu.regs.PC++ // never odd
	}
	return res, nil
}

// popImmediate reads immediate data of size s from the instruction stream. A
// byte is the low half of a word.
func (cpu *CPU) popImmediate(s Size) (uint32, error) {
	if s == Byte {
		v, err := cpu.popPc(Word)
		return v & 0xff, err
	}
	return cpu.popPc(s)
}

// readProgramFastWord serves an instruction word from the bus's direct
// memory. It reports ok=false when that does not apply (a debug hook watches
// the bus, the address is odd or not in direct memory), and the caller falls
// back to readProgram.
func (cpu *CPU) readProgramFastWord(address uint32) (uint16, bool) {
	address &= cpu.addrMask
	mem := cpu.code.mem
	// The base of direct memory is even, so for an even address idx+1
	// cannot wrap.
	idx := address - cpu.code.base
	if address&1 != 0 || idx+1 >= uint32(len(mem)) || cpu.code.wait != 0 {
		return cpu.readProgramWordSlow(address)
	}
	return uint16(mem[idx])<<8 | uint16(mem[idx+1]), true
}

// readProgramWordSlow is readProgramFastWord for a fetch outside the cached
// page or from memory with wait states.
func (cpu *CPU) readProgramWordSlow(address uint32) (uint16, bool) {
	idx := address - cpu.code.base
	if address&1 != 0 || uint64(idx)+2 > uint64(len(cpu.code.mem)) {
		if !cpu.loadCodePage(Word, address) {
			return 0, false
		}
		idx = address - cpu.code.base
	}
	if cpu.code.wait != 0 {
		cpu.addCycles(cpu.code.wait)
	}
	mem := cpu.code.mem[idx : idx+2]
	return uint16(mem[0])<<8 | uint16(mem[1]), true
}

func (cpu *CPU) readProgramFastLong(address uint32) (uint32, bool) {
	address &= cpu.addrMask
	idx := address - cpu.code.base
	if address&1 != 0 || uint64(idx)+4 > uint64(len(cpu.code.mem)) {
		if !cpu.loadCodePage(Long, address) {
			return 0, false
		}
		idx = address - cpu.code.base
	}
	if cpu.code.wait != 0 {
		cpu.addCycles(cpu.code.wait * 2)
	}
	mem := cpu.code.mem[idx : idx+4]
	return uint32(mem[0])<<24 | uint32(mem[1])<<16 | uint32(mem[2])<<8 | uint32(mem[3]), true
}

// dataIndex returns the offset into cpu.data of a size-byte data access at
// address, first pointing the data cache at the direct memory of the
// address's page when needed. ok is false for an odd word or long access and
// for an address outside direct memory: the bus then decides.
func (cpu *CPU) dataIndex(size Size, address uint32) (uint32, bool) {
	idx := address - cpu.data.base
	if uint64(idx)+uint64(size) <= uint64(len(cpu.data.mem)) && (address&1 == 0 || size == Byte) {
		return idx, true
	}
	return cpu.loadDataPage(size, address)
}

// loadDataPage is the miss path of dataIndex.
func (cpu *CPU) loadDataPage(size Size, address uint32) (uint32, bool) {
	if address&1 != 0 && size != Byte {
		return 0, false
	}
	p := cpu.bus.fastPageFor(address)
	idx, ok := p.index(size, address)
	if ok {
		cpu.data = *p
	}
	return idx, ok
}

// loadCodePage points the code cache at the direct memory that holds the
// aligned size-byte fetch at address, if there is one and fetches may use it.
func (cpu *CPU) loadCodePage(size Size, address uint32) bool {
	if !cpu.fastFetch || address&1 != 0 {
		return false
	}
	p := cpu.bus.fastPageFor(address)
	if _, ok := p.index(size, address); !ok {
		return false
	}
	cpu.code = *p
	return true
}

// addCycles increments the CPU cycle counter using a uint32 input to keep call
// sites close to the 68k reference values while storing the counter as a wider
// type.
func (cpu *CPU) addCycles(c uint32) {
	cpu.cycles += uint64(c)
	if cpu.cycles >= cpu.nextEvent {
		cpu.scheduler.runDue()
	}
}

// overrideInstructionCycles replaces the current instruction's table time
// with total, the time of the exception it raises instead. The difference
// goes through addCycles so an attached scheduler stays in step. Every
// instruction that can raise such an exception is tabled at or below that
// exception's time (RESET charges its long reset pulse only once the
// privilege check passed), so the difference never needs to be taken back.
func (cpu *CPU) overrideInstructionCycles(total uint32) {
	if current := cpu.ops.cycles[cpu.regs.IR]; total > current {
		cpu.addCycles(total - current)
	}
}

// Cycles returns the total number of cycles executed since the last reset.
func (cpu *CPU) Cycles() uint64 {
	return cpu.cycles
}

func constantCycles(c uint32) cycleCalculator {
	return func(uint16) uint32 {
		return c
	}
}

func (cpu *CPU) disassemblyString() string {
	line, err := DisassembleInstruction(cpu.bus, cpu.regs.PC)
	if err != nil {
		return fmt.Sprintf("DISASM %08x: <unavailable>", cpu.regs.PC)
	}
	return fmt.Sprintf("DISASM %08x: %s", cpu.regs.PC, line.Assembly)
}

func (cpu *CPU) dataFunctionCode() uint16 {
	if cpu.regs.SR&srSupervisor != 0 {
		return functionCodeSupervisorData
	}
	return functionCodeUserData
}

func (cpu *CPU) programFunctionCode() uint16 {
	if cpu.regs.SR&srSupervisor != 0 {
		return functionCodeSupervisorProg
	}
	return functionCodeUserProgram
}

func (cpu *CPU) recordFault(address uint32, ctx accessContext, size Size, value uint32) {
	cpu.fault = faultInfo{
		address:        address & cpu.addrMask,
		pc:             cpu.regs.PC,
		ir:             cpu.regs.IR,
		functionCode:   ctx.functionCode & 0x7,
		write:          ctx.write,
		notInstruction: ctx.notInstruction,
		valid:          true,
		size:           size,
		value:          value,
	}
}

func (cpu *CPU) interruptMask() uint8 {
	return uint8((cpu.regs.SR & srInterruptMask) >> 8)
}

func (cpu *CPU) dispatchException(info ExceptionInfo) {
	cpu.lastException = info
	cpu.lastExceptionValid = true
	cpu.stepException = info
	cpu.stepExceptionValid = true
	cpu.stepDirty = true
	cpu.appendHistory(HistoryEntry{Kind: HistoryException, Exception: info})
	if cpu.exceptionTrap != nil {
		cpu.exceptionTrap(info)
	}
}

func (cpu *CPU) dispatchInterrupt(info InterruptInfo) {
	cpu.lastInterrupt = info
	cpu.lastInterruptValid = true
	cpu.stepInterrupt = info
	cpu.stepInterruptValid = true
	cpu.stepDirty = true
	cpu.appendHistory(HistoryEntry{Kind: HistoryInterrupt, Interrupt: info})
	if cpu.interruptTrap != nil {
		cpu.interruptTrap(info)
	}
}

func (cpu *CPU) shouldTraceBusAccess(ctx accessContext) bool {
	if ctx.instructionFetch() && !ctx.write && cpu.traceInstructions {
		return true
	}
	return cpu.traceBus
}

// traceBusAccess is the single funnel for memory access reporting. It updates
// fetched-byte tracking, step-local stop data, the rolling history buffer, and
// the external callback with the same normalized record.
func (cpu *CPU) traceBusAccess(size Size, address uint32, value uint32, ctx accessContext) {
	info := BusAccessInfo{
		Address:          address & cpu.addrMask,
		Size:             size,
		Value:            value & size.mask(),
		Write:            ctx.write,
		InstructionFetch: ctx.instructionFetch(),
		PC:               cpu.debugPC(),
		FunctionCode:     ctx.functionCode & 7,
	}

	if ctx.instructionFetch() && !ctx.write {
		cpu.traceBytes = appendTraceValue(cpu.traceBytes, size, value)
		cpu.stepDirty = true
	}
	if cpu.collectStepBus {
		cpu.stepBusAccesses = append(cpu.stepBusAccesses, info)
		cpu.stepDirty = true
	}
	if len(cpu.history) != 0 {
		cpu.appendHistory(HistoryEntry{Kind: HistoryBusAccess, BusAccess: info})
	}

	if cpu.busTrap == nil {
		return
	}

	cpu.busTrap(info)
}

func (cpu *CPU) runStopReason(options RunUntilOptions, result *RunResult) (RunStopReason, bool) {
	if options.StopPredicate != nil && options.StopPredicate(cpu.runPredicateInfo(result)) {
		return RunStopPredicate, true
	}
	if matched, ok := cpu.matchStopAtPC(options); ok {
		if result != nil {
			result.PC = matched
		}
		return RunStopPC, true
	}
	if matched, ok := cpu.matchStepBusAccess(options); ok {
		if result != nil {
			result.BusAccess = matched
			result.HasBusAccess = true
		}
		return RunStopBusAccess, true
	}
	if options.StopOnIllegal && cpu.stepExceptionValid && isIllegalException(cpu.stepException.Vector) {
		return RunStopIllegalOpcode, true
	}
	if options.StopOnException && cpu.stepExceptionValid {
		return RunStopException, true
	}
	return cpu.pcStopReason(options)
}

func (cpu *CPU) pcStopReason(options RunUntilOptions) (RunStopReason, bool) {
	if options.StopOnPCRange != nil && options.StopOnPCRange.contains(cpu.regs.PC, cpu.addrMask) {
		return RunStopPCInRange, true
	}
	if options.StopWhenPCOutside != nil && !options.StopWhenPCOutside.contains(cpu.regs.PC, cpu.addrMask) {
		return RunStopPCOutsideRange, true
	}
	return RunStopNone, false
}

func faultAddress(address uint32, err error) uint32 {
	switch fault := err.(type) {
	case BusError:
		return uint32(fault)
	case AddressError:
		return uint32(fault)
	default:
		return address
	}
}

func (f faultInfo) statusWord() uint16 {
	var word uint16
	if !f.write {
		word |= 1 << 4
	}
	if f.notInstruction {
		word |= 1 << 3
	}
	word |= f.functionCode & 0x7
	return word
}

func (f faultInfo) snapshot() DebugFaultInfo {
	return DebugFaultInfo{
		Address:          f.address,
		PC:               f.pc,
		Opcode:           f.ir,
		FunctionCode:     f.functionCode,
		Write:            f.write,
		InstructionFetch: !f.notInstruction && !f.write,
		Valid:            f.valid,
	}
}

// programSpace reports whether the access is to program space, where the
// MC68020 still requires even addresses.
func (ctx accessContext) programSpace() bool {
	fc := ctx.functionCode & 7
	return fc == functionCodeUserProgram || fc == functionCodeSupervisorProg
}

// readMisaligned reads a word or long at an odd address, which the MC68020
// and later allow for operands, as aligned byte and word accesses: a byte
// and a byte for a word, a byte, a word and a byte for a long.
func (cpu *CPU) readMisaligned(size Size, address uint32, ctx accessContext) (uint32, error) {
	hi, err := cpu.readCtx(Byte, address, ctx)
	if err != nil {
		return 0, err
	}
	if size == Word {
		lo, err := cpu.readCtx(Byte, address+1, ctx)
		return hi<<8 | lo, err
	}
	mid, err := cpu.readCtx(Word, address+1, ctx)
	if err != nil {
		return 0, err
	}
	lo, err := cpu.readCtx(Byte, address+3, ctx)
	return hi<<24 | mid<<8 | lo, err
}

// writeMisaligned is the write counterpart of readMisaligned.
func (cpu *CPU) writeMisaligned(size Size, address, value uint32, ctx accessContext) error {
	if size == Word {
		if err := cpu.writeCtx(Byte, address, value>>8, ctx); err != nil {
			return err
		}
		return cpu.writeCtx(Byte, address+1, value, ctx)
	}
	if err := cpu.writeCtx(Byte, address, value>>24, ctx); err != nil {
		return err
	}
	if err := cpu.writeCtx(Word, address+1, value>>8, ctx); err != nil {
		return err
	}
	return cpu.writeCtx(Byte, address+3, value, ctx)
}

func (ctx accessContext) instructionFetch() bool {
	return !ctx.notInstruction && !ctx.write &&
		(ctx.functionCode == functionCodeUserProgram || ctx.functionCode == functionCodeSupervisorProg)
}

// Contains reports whether the 24-bit address lies in r.
func (r AddressRange) Contains(address uint32) bool {
	return r.contains(address, 0xffffff)
}

// contains reports whether address lies in r, comparing the bits in mask.
func (r AddressRange) contains(address, mask uint32) bool {
	address &= mask
	return address >= r.Start&mask && address <= r.End&mask
}

func isIllegalException(vector uint32) bool {
	switch vector {
	case XIllegal, XLineA, XLineF:
		return true
	default:
		return false
	}
}

func (cpu *CPU) matchStopAtPC(options RunUntilOptions) (uint32, bool) {
	for _, target := range options.StopAtPC {
		if cpu.regs.PC&cpu.addrMask == target&cpu.addrMask {
			return cpu.regs.PC & cpu.addrMask, true
		}
	}
	return 0, false
}

func (cpu *CPU) matchStepBusAccess(options RunUntilOptions) (BusAccessInfo, bool) {
	if options.StopOnBusAccess == nil {
		return BusAccessInfo{}, false
	}
	for _, access := range cpu.stepBusAccesses {
		if options.StopOnBusAccess(access) {
			return access, true
		}
	}
	return BusAccessInfo{}, false
}

func (cpu *CPU) runPredicateInfo(result *RunResult) RunPredicateInfo {
	info := RunPredicateInfo{
		Registers:     cpu.regs,
		Cycles:        cpu.cycles,
		LastException: cpu.stepException,
		HasException:  cpu.stepExceptionValid,
		LastInterrupt: cpu.stepInterrupt,
		HasInterrupt:  cpu.stepInterruptValid,
	}
	if result != nil {
		info.Instructions = result.Instructions
	}
	if len(cpu.stepBusAccesses) != 0 {
		info.LastBusAccess = cpu.stepBusAccesses[len(cpu.stepBusAccesses)-1]
		info.HasBusAccess = true
	}
	return info
}

func (cpu *CPU) appendHistory(entry HistoryEntry) {
	if len(cpu.history) == 0 {
		return
	}
	cpu.history[cpu.historyNext] = cloneHistoryEntry(entry)
	cpu.historyNext = (cpu.historyNext + 1) % len(cpu.history)
	if cpu.historyCount < len(cpu.history) {
		cpu.historyCount++
	}
}

func cloneHistoryEntry(entry HistoryEntry) HistoryEntry {
	entry.Trace.Bytes = slices.Clone(entry.Trace.Bytes)
	return entry
}

// ReadExceptionStackFrame decodes an exception frame of the given format
// directly from memory without requiring the caller to know the byte layout.
func ReadExceptionStackFrame(bus AddressBus, sp uint32, format ExceptionStackFrameFormat) (ExceptionStackFrame, error) {
	frame := ExceptionStackFrame{
		Format:       format,
		StackPointer: sp & 0xffffff,
	}
	var err error
	read := func(s Size, offset uint32) uint32 {
		if err != nil {
			return 0
		}
		var v uint32
		v, err = bus.Read(s, sp+offset)
		return v
	}

	switch format {
	case ExceptionStackFrameGroup12:
		frame.SR = uint16(read(Word, 0))
		frame.PC = read(Long, 2)
	case ExceptionStackFrameGroup0:
		frame.StatusWord = uint16(read(Word, 0))
		frame.FaultAddress = read(Long, 2)
		frame.InstructionRegister = uint16(read(Word, 6))
		frame.SR = uint16(read(Word, 8))
		frame.PC = read(Long, 10)
	case ExceptionStackFrameFormat0, ExceptionStackFrameFormat8, ExceptionStackFrameFormat1,
		ExceptionStackFrameFormat2, ExceptionStackFrameFormat9, ExceptionStackFrameFormatA, ExceptionStackFrameFormatB:
		frame.SR = uint16(read(Word, 0))
		frame.PC = read(Long, 2)
		frame.VectorOffset = uint16(read(Word, 6)) & 0x0fff
		switch format {
		case ExceptionStackFrameFormat8:
			frame.StatusWord = uint16(read(Word, 8))
			frame.FaultAddress = read(Long, 10)
			frame.DataOutput = read(Word, 16)
			frame.DataInput = read(Word, 20)
			frame.InstructionInput = uint16(read(Word, 24))
		case ExceptionStackFrameFormat2, ExceptionStackFrameFormat9:
			frame.InstructionAddress = read(Long, 8)
		case ExceptionStackFrameFormatA, ExceptionStackFrameFormatB:
			frame.StatusWord = uint16(read(Word, 10))
			frame.DataOutput = read(Long, 24)
			if frame.StatusWord&(1<<8) != 0 {
				frame.FaultAddress = read(Long, 16)
			}
			if format == ExceptionStackFrameFormatB {
				frame.StageBAddress = read(Long, 36)
				frame.DataInput = read(Long, 44)
				if frame.StatusWord&(1<<8) == 0 {
					frame.FaultAddress = frame.StageBAddress
				}
			}
		}
	default:
		return ExceptionStackFrame{}, fmt.Errorf("unknown exception stack frame format %d", format)
	}
	if err != nil {
		return ExceptionStackFrame{}, err
	}
	return frame, nil
}

// CurrentExceptionFrame rereads the most recent exception frame from the stack,
// which is useful after a handler has inspected or modified it in memory.
func (cpu *CPU) CurrentExceptionFrame() (ExceptionStackFrame, bool, error) {
	if !cpu.lastExceptionValid || !cpu.lastException.FrameValid {
		return ExceptionStackFrame{}, false, nil
	}

	frame, err := ReadExceptionStackFrame(cpu.bus, cpu.lastException.StackPointer, cpu.lastException.Frame.Format)
	if err != nil {
		return ExceptionStackFrame{}, true, err
	}
	return frame, true, nil
}

func (cpu *CPU) traceInstructionBytes(pc uint32) []byte {
	if len(cpu.traceBytes) != 0 {
		return slices.Clone(cpu.traceBytes)
	}
	return traceInstructionBytes(cpu.bus, pc, cpu.regs.IR)
}

func appendTraceValue(dst []byte, size Size, value uint32) []byte {
	switch size {
	case Byte:
		return append(dst, byte(value))
	case Word:
		return append(dst, byte(value>>8), byte(value))
	case Long:
		return append(dst, byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
	default:
		return dst
	}
}
