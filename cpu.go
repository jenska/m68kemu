package m68kemu

import (
	"fmt"
	"strings"
)

const (
	Version           = "1.2.3"
	XBusError         = 2
	XAddressError     = 3
	XIllegal          = 4
	XDivByZero        = 5
	XPrivViolation    = 8
	XLineA            = 10
	XLineF            = 11
	XUninitializedInt = 15
	XTrap             = 32

	srCarry         = 0x0001
	srOverflow      = 0x0002
	srZero          = 0x0004
	srNegative      = 0x0008
	srExtend        = 0x0010
	srInterruptMask = 0x0700
	srSupervisor    = 0x2000
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

var opcodeTable [0x10000]instruction
var opcodeCycleTable [0x10000]uint32

type (
	instruction func(*cpu) error

	AddressError uint32
	BusError     uint32

	BreakpointType int

	// cycleCalculator builds a static cycle count for a given opcode. Results are
	// stored in OpcodeCycleTable during instruction registration and can be looked
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

	// ExceptionStackFrameFormat identifies the 68000 frame layout captured for an exception.
	ExceptionStackFrameFormat int

	// ExceptionStackFrame mirrors the exception frame currently stored on the supervisor stack.
	ExceptionStackFrame struct {
		Format              ExceptionStackFrameFormat
		StackPointer        uint32
		StatusWord          uint16
		FaultAddress        uint32
		InstructionRegister uint16
		SR                  uint16
		PC                  uint32
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

	CycleScheduler struct {
		now       uint64
		listeners []CycleListener
		events    []ScheduledEvent
		eventHead int
	}

	CycleListener interface {
		AdvanceCycles(delta uint64, now uint64)
	}

	ScheduledEvent struct {
		At uint64
		Fn func(now uint64)
	}

	// Registers represents the programmer visible registers of the 68000 CPU.
	Registers struct {
		D   [8]int32
		A   [8]uint32
		PC  uint32
		SR  uint16
		SSP uint32
		USP uint32
		IR  uint16 // instruction register
	}

	// CPU exposes the minimal interface for interacting with the emulator core.
	CPU interface {
		Registers() Registers
		DebugState() DebugState
		Step() error
		RunCycles(budget uint64) error
		RunInstructions(count uint64) error
		RunUntil(options RunUntilOptions) (RunResult, error)
		Reset() error
		SetTracer(TraceCallback)
		SetPreTracer(PreTraceCallback)
		SetExceptionTracer(ExceptionCallback)
		SetBusTracer(BusAccessCallback)
		SetInterruptTracer(InterruptCallback)
		SetScheduler(*CycleScheduler)
		Scheduler() *CycleScheduler
		AddBreakpoint(Breakpoint)
		RequestInterrupt(level uint8, vector *uint8) error
		Cycles() uint64
		SetHistoryLimit(limit int)
		History() []HistoryEntry
		CurrentExceptionFrame() (ExceptionStackFrame, bool, error)
	}

	faultInfo struct {
		address        uint32
		pc             uint32
		ir             uint16
		functionCode   uint16
		write          bool
		notInstruction bool
		valid          bool
	}

	//  CPU core
	cpu struct {
		regs          Registers
		cycles        uint64
		bus           AddressBus
		busFast       *Bus
		trap          TraceCallback
		preTrap       PreTraceCallback
		exceptionTrap ExceptionCallback
		busTrap       BusAccessCallback
		interruptTrap InterruptCallback
		scheduler     *CycleScheduler
		interrupts    *InterruptController

		stopped bool

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
	ExceptionStackFrameGroup12 ExceptionStackFrameFormat = iota
	ExceptionStackFrameGroup0
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
		fmt.Fprintf(&result, "D%d %08x ", i, uint32(regs.D[i]))
	}
	result.WriteByte('\n')
	for i := range regs.A {
		fmt.Fprintf(&result, "A%d %08x ", i, uint32(regs.A[i]))
	}
	result.WriteByte('\n')

	return result.String()
}

func (cpu *cpu) String() string {
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

func (cpu *cpu) read(size Size, address uint32) (uint32, error) {
	return cpu.readContext(size, address, accessContext{functionCode: cpu.dataFunctionCode()})
}

func (cpu *cpu) readProgram(size Size, address uint32) (uint32, error) {
	return cpu.readContext(size, address, accessContext{functionCode: cpu.programFunctionCode()})
}

func (cpu *cpu) readSystemProgram(size Size, address uint32) (uint32, error) {
	return cpu.readContext(size, address, accessContext{functionCode: functionCodeSupervisorProg, notInstruction: true})
}

func (cpu *cpu) readContext(size Size, address uint32, ctx accessContext) (uint32, error) {
	address &= 0xffffff // 24bit address bus of 68000
	switch size {
	case Byte, Word, Long:
		if cpu.breakpoints != nil {
			if err := cpu.checkAccessBreakpoint(address, BreakpointRead); err != nil {
				return 0, err
			}
		}
		if result, ok, err := cpu.fastRAMRead(size, address); ok {
			if err != nil {
				cpu.recordFault(faultAddress(address, err), ctx)
			} else if cpu.shouldTraceBusAccess(ctx) {
				cpu.traceBusAccess(size, address, result, ctx)
			}
			return result, err
		}
		result, err := cpu.bus.Read(size, address)
		if err != nil {
			cpu.recordFault(faultAddress(address, err), ctx)
		} else if cpu.shouldTraceBusAccess(ctx) {
			cpu.traceBusAccess(size, address, result, ctx)
		}
		return uint32(result), err
	default:
		return 0, fmt.Errorf("unknown operand size")

	}
}

func (cpu *cpu) write(size Size, address uint32, value uint32) error {
	return cpu.writeContext(size, address, value, accessContext{functionCode: cpu.dataFunctionCode(), write: true})
}

func (cpu *cpu) writeSystemData(size Size, address uint32, value uint32) error {
	return cpu.writeContext(size, address, value, accessContext{functionCode: functionCodeSupervisorData, notInstruction: true, write: true})
}

func (cpu *cpu) writeContext(size Size, address uint32, value uint32, ctx accessContext) error {
	address &= 0xffffff // 24bit address bus of 68000
	switch size {
	case Byte, Word, Long:
		if cpu.breakpoints != nil {
			if err := cpu.checkAccessBreakpoint(address, BreakpointWrite); err != nil {
				return err
			}
		}
		if ok, err := cpu.fastRAMWrite(size, address, value); ok {
			if err != nil {
				cpu.recordFault(faultAddress(address, err), ctx)
			} else if cpu.shouldTraceBusAccess(ctx) {
				cpu.traceBusAccess(size, address, value, ctx)
			}
			return err
		}
		if err := cpu.bus.Write(size, address, value); err != nil {
			cpu.recordFault(faultAddress(address, err), ctx)
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

func (cpu *cpu) fastRAMDevice() *RAM {
	if cpu.busFast == nil {
		return nil
	}
	if cpu.busFast.waitStates != 0 || cpu.busFast.hasWaitStateDevices {
		return nil
	}
	return cpu.busFast.singleRAM
}

func (cpu *cpu) fastRAMRead(size Size, address uint32) (uint32, bool, error) {
	ram := cpu.fastRAMDevice()
	if ram == nil {
		return 0, false, nil
	}

	memLen := uint32(len(ram.mem))
	if address < ram.offset {
		return 0, true, BusError(address)
	}
	idx := address - ram.offset

	switch size {
	case Byte:
		if idx >= memLen {
			return 0, true, BusError(address)
		}
		return uint32(ram.mem[idx]), true, nil
	case Word:
		if address&1 != 0 {
			return 0, true, AddressError(address)
		}
		if idx+1 >= memLen {
			return 0, true, BusError(address)
		}
		return uint32(ram.mem[idx])<<8 | uint32(ram.mem[idx+1]), true, nil
	case Long:
		if address&1 != 0 {
			return 0, true, AddressError(address)
		}
		if idx+1 >= memLen {
			return 0, true, BusError(address)
		}
		if idx+3 >= memLen {
			return 0, true, BusError((address + uint32(Word)) & 0xffffff)
		}
		return uint32(ram.mem[idx])<<24 |
			uint32(ram.mem[idx+1])<<16 |
			uint32(ram.mem[idx+2])<<8 |
			uint32(ram.mem[idx+3]), true, nil
	default:
		return 0, false, nil
	}
}

func (cpu *cpu) fastRAMWrite(size Size, address uint32, value uint32) (bool, error) {
	ram := cpu.fastRAMDevice()
	if ram == nil {
		return false, nil
	}

	memLen := uint32(len(ram.mem))
	if address < ram.offset {
		return true, BusError(address)
	}
	idx := address - ram.offset

	switch size {
	case Byte:
		if idx >= memLen {
			return true, BusError(address)
		}
		ram.mem[idx] = uint8(value)
		return true, nil
	case Word:
		if address&1 != 0 {
			return true, AddressError(address)
		}
		if idx+1 >= memLen {
			return true, BusError(address)
		}
		ram.mem[idx] = uint8(value >> 8)
		ram.mem[idx+1] = uint8(value)
		return true, nil
	case Long:
		if address&1 != 0 {
			return true, AddressError(address)
		}
		if idx+1 >= memLen {
			return true, BusError(address)
		}
		ram.mem[idx] = uint8(value >> 24)
		ram.mem[idx+1] = uint8(value >> 16)
		if idx+3 >= memLen {
			return true, BusError((address + uint32(Word)) & 0xffffff)
		}
		ram.mem[idx+2] = uint8(value >> 8)
		ram.mem[idx+3] = uint8(value)
		return true, nil
	default:
		return false, nil
	}
}

func (cpu *cpu) Registers() Registers {
	return cpu.regs
}

func (cpu *cpu) DebugState() DebugState {
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

func (cpu *cpu) SetTracer(cb TraceCallback) {
	cpu.trap = cb
	cpu.refreshDebugModes()
}

func (cpu *cpu) SetPreTracer(cb PreTraceCallback) {
	cpu.preTrap = cb
}

func (cpu *cpu) SetExceptionTracer(cb ExceptionCallback) {
	cpu.exceptionTrap = cb
}

func (cpu *cpu) SetBusTracer(cb BusAccessCallback) {
	cpu.busTrap = cb
	cpu.refreshDebugModes()
}

func (cpu *cpu) SetInterruptTracer(cb InterruptCallback) {
	cpu.interruptTrap = cb
}

func (cpu *cpu) refreshDebugModes() {
	cpu.traceInstructions = cpu.trap != nil || len(cpu.history) != 0
	cpu.traceBus = cpu.busTrap != nil || len(cpu.history) != 0 || cpu.collectStepBus
}

func (cpu *cpu) requireSupervisor() (bool, error) {
	if cpu.regs.SR&srSupervisor != 0 {
		return true, nil
	}
	return false, cpu.exceptionWithCycles(XPrivViolation, exceptionCyclesPrivilege)
}

func (cpu *cpu) SetScheduler(s *CycleScheduler) {
	cpu.scheduler = s
	if s != nil {
		s.Reset(cpu.cycles)
	}
}

func (cpu *cpu) Scheduler() *CycleScheduler {
	return cpu.scheduler
}

func (cpu *cpu) RequestInterrupt(level uint8, vector *uint8) error {
	return cpu.interrupts.Request(level, vector)
}

func (cpu *cpu) AddBreakpoint(bp Breakpoint) {
	if cpu.breakpoints == nil {
		cpu.breakpoints = make(map[uint32]Breakpoint)
	}
	cpu.breakpoints[bp.Address] = bp
}

func (cpu *cpu) SetHistoryLimit(limit int) {
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

func (cpu *cpu) History() []HistoryEntry {
	if len(cpu.history) == 0 || cpu.historyCount == 0 {
		return nil
	}

	result := make([]HistoryEntry, 0, cpu.historyCount)
	start := cpu.historyNext - cpu.historyCount
	if start < 0 {
		start += len(cpu.history)
	}

	for i := 0; i < cpu.historyCount; i++ {
		index := (start + i) % len(cpu.history)
		result = append(result, cloneHistoryEntry(cpu.history[index]))
	}

	return result
}

func (cpu *cpu) handleBreakpoint(bp Breakpoint, kind BreakpointType, address uint32) error {
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

func (cpu *cpu) rememberOpcodePC(pc uint32) {
	cpu.lastOpcodePC = pc
	cpu.lastOpcodePCValid = true
}

// beginInstructionContext records the instruction boundary so nested helpers,
// fault handling, and bus tracing can all attribute work to the same opcode.
func (cpu *cpu) beginInstructionContext(pc uint32) {
	cpu.currentOpcodePC = pc & 0xffffff
	cpu.currentOpcodeValid = true
}

func (cpu *cpu) endInstructionContext() {
	cpu.currentOpcodeValid = false
}

func (cpu *cpu) currentOpcodeAddress(fallback uint32) uint32 {
	if cpu.currentOpcodeValid {
		return cpu.currentOpcodePC
	}
	return fallback & 0xffffff
}

func (cpu *cpu) debugPC() uint32 {
	if cpu.currentOpcodeValid {
		return cpu.currentOpcodePC
	}
	if cpu.lastOpcodePCValid {
		return cpu.lastOpcodePC & 0xffffff
	}
	return cpu.regs.PC & 0xffffff
}

func (cpu *cpu) consumeOpcodePC() uint32 {
	pc := cpu.regs.PC
	if cpu.lastOpcodePCValid {
		pc = cpu.lastOpcodePC
	}
	cpu.lastOpcodePCValid = false
	return pc
}

func (cpu *cpu) opcodeException(vector uint32, instructionPC uint32) error {
	if vector == XLineA || vector == XLineF {
		cpu.overrideInstructionCycles(exceptionCyclesIllegal)
		return cpu.raiseExceptionWithPC(vector, cpu.regs.SR|srSupervisor, instructionPC, instructionPC)
	}
	return cpu.exceptionWithCycles(vector, exceptionCyclesIllegal)
}

// ExecuteInstruction runs an instruction without fetching it from memory. This allows
// callers to execute single instructions directly through the API.
func (cpu *cpu) executeInstruction(opcode uint16) error {
	instructionPC := cpu.consumeOpcodePC()
	startedContext := false
	if !cpu.currentOpcodeValid {
		cpu.beginInstructionContext(instructionPC)
		startedContext = true
	}
	if startedContext {
		defer cpu.endInstructionContext()
	}

	cpu.regs.IR = opcode

	cpu.addCycles(opcodeCycleTable[opcode])

	handler := opcodeTable[opcode]
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

func (cpu *cpu) raiseException(vector uint32, newSR uint16) error {
	return cpu.raiseExceptionWithPC(vector, newSR, cpu.regs.PC, cpu.regs.PC)
}

func (cpu *cpu) raiseExceptionWithPC(vector uint32, newSR uint16, stackedPC uint32, exceptionPC uint32) error {
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

	// 68000 stack frame: PC (long), SR (word).
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
		Format:       ExceptionStackFrameGroup12,
		StackPointer: cpu.regs.A[7],
		SR:           originalSR,
		PC:           stackedPC,
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

func (cpu *cpu) exception(vector uint32) error {
	return cpu.raiseException(vector, cpu.regs.SR|srSupervisor)
}

func (cpu *cpu) exceptionWithCycles(vector uint32, total uint32) error {
	cpu.overrideInstructionCycles(total)
	return cpu.exception(vector)
}

func (cpu *cpu) raiseGroup0Exception(vector uint32, newSR uint16) error {
	if vector > 255 {
		return fmt.Errorf("invalid vector %d", vector)
	}

	originalSR := cpu.regs.SR
	opcodeAddress := cpu.currentOpcodeAddress(cpu.fault.pc)
	cpu.inException = true
	defer func() {
		cpu.inException = false
	}()
	cpu.setSR(newSR)

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

func (cpu *cpu) group0ExceptionForCurrentInstruction(vector uint32, total uint32) error {
	cpu.overrideInstructionCycles(total)
	return cpu.raiseGroup0Exception(vector, cpu.regs.SR|srSupervisor)
}

func (cpu *cpu) group0ExceptionWithoutInstruction(vector uint32, total uint32) error {
	cpu.addCycles(total)
	return cpu.raiseGroup0Exception(vector, cpu.regs.SR|srSupervisor)
}

// setSR keeps USP/SSP in sync when the supervisor bit changes.
func (cpu *cpu) setSR(value uint16) {
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

func (cpu *cpu) readVector(offset uint32) (uint32, error) {
	if offset&1 != 0 {
		return 0, AddressError(offset)
	}
	if offset >= 256<<2 {
		return 0, AddressError(offset)
	}

	address, err := cpu.readSystemProgram(Long, offset)
	if err != nil {
		return 0, err
	}
	if address == 0 {
		return cpu.readSystemProgram(Long, XUninitializedInt<<2)
	}
	return address, nil
}

func (cpu *cpu) interrupt(level uint8, vector uint32, autoVector bool) error {
	originalPC := cpu.regs.PC
	originalSR := cpu.regs.SR
	newSR := (cpu.regs.SR & ^uint16(srInterruptMask)) | srSupervisor | (uint16(level) << 8)
	cpu.addCycles(exceptionCyclesInterrupt)
	if err := cpu.raiseException(vector, newSR); err != nil {
		return err
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

func (cpu *cpu) checkInterrupts() error {
	if cpu.interrupts == nil || !cpu.interrupts.HasPending(cpu.regs.SR) {
		return nil
	}

	level, vector, autoVector, ok := cpu.interrupts.Pending(cpu.regs.SR)
	if !ok {
		return nil
	}

	cpu.stopped = false

	return cpu.interrupt(level, vector, autoVector)
}

func (cpu *cpu) handleFaultError(err error, currentInstruction bool) error {
	switch err.(type) {
	case BusError:
		if currentInstruction {
			return cpu.group0ExceptionForCurrentInstruction(XBusError, exceptionCyclesBusAddress)
		}
		return cpu.group0ExceptionWithoutInstruction(XBusError, exceptionCyclesBusAddress)
	case AddressError:
		if currentInstruction {
			return cpu.group0ExceptionForCurrentInstruction(XAddressError, exceptionCyclesBusAddress)
		}
		return cpu.group0ExceptionWithoutInstruction(XAddressError, exceptionCyclesBusAddress)
	default:
		return err
	}
}

// executeNext runs one whole instruction boundary: fetch, optional pre-trace,
// execute, post-instruction interrupt check, and final trace dispatch.
func (cpu *cpu) executeNext() error {
	if cpu.breakpoints != nil {
		if err := cpu.checkExecuteBreakpoint(cpu.regs.PC); err != nil {
			return err
		}
	}

	pc := cpu.regs.PC
	beforeRegs := cpu.regs
	beforeCycles := cpu.cycles
	cpu.beginInstructionContext(pc)

	opcode, err := cpu.fetchOpcode()
	if err != nil {
		cpu.endInstructionContext()
		return cpu.handleFaultError(err, false)
	}
	if cpu.preTrap != nil {
		cpu.sendPreTrace(pc, opcode, beforeRegs)
	}
	if err := cpu.executeInstruction(opcode); err != nil {
		cpu.endInstructionContext()
		return err
	}
	if err := cpu.checkInterrupts(); err != nil {
		cpu.endInstructionContext()
		return err
	}
	if cpu.traceInstructions {
		cpu.sendTrace(pc, beforeRegs, uint32(cpu.cycles-beforeCycles))
	}
	cpu.endInstructionContext()
	return nil
}

// Step fetches the next opcode at the program counter and executes it.
func (cpu *cpu) Step() error {
	if cpu.stepExceptionValid || cpu.stepInterruptValid || len(cpu.traceBytes) != 0 || len(cpu.stepBusAccesses) != 0 {
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
func (cpu *cpu) RunCycles(budget uint64) error {
	start := cpu.cycles
	target := start + budget

	for cpu.cycles < target {
		if cpu.stepExceptionValid || cpu.stepInterruptValid || len(cpu.traceBytes) != 0 || len(cpu.stepBusAccesses) != 0 {
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

func (cpu *cpu) RunInstructions(count uint64) error {
	for i := uint64(0); i < count; i++ {
		if err := cpu.Step(); err != nil {
			return err
		}
	}
	return nil
}

func (cpu *cpu) RunUntil(options RunUntilOptions) (RunResult, error) {
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

// resetStepDebugState clears per-step scratch state before a new Step/RunCycles
// iteration so stop conditions only see events from the current instruction.
func (cpu *cpu) resetStepDebugState() {
	cpu.stepException = ExceptionInfo{}
	cpu.stepExceptionValid = false
	cpu.stepInterrupt = InterruptInfo{}
	cpu.stepInterruptValid = false
	cpu.traceBytes = cpu.traceBytes[:0]
	cpu.stepBusAccesses = cpu.stepBusAccesses[:0]
}

func (cpu *cpu) sendPreTrace(pc uint32, opcode uint16, regs Registers) {
	if cpu.preTrap == nil {
		return
	}

	bytes := traceInstructionBytes(cpu.bus, pc, opcode)
	cpu.preTrap(PreTraceInfo{
		PC:        pc & 0xffffff,
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
func (cpu *cpu) sendTrace(pc uint32, before Registers, cycleDelta uint32) {
	if !cpu.traceInstructions {
		return
	}

	bytes := cpu.traceInstructionBytes(pc)
	info := TraceInfo{
		PC:              pc & 0xffffff,
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

func (cpu *cpu) checkExecuteBreakpoint(pc uint32) error {
	if cpu.breakpoints == nil {
		return nil
	}

	if bp, ok := cpu.breakpoints[pc]; ok && bp.OnExecute {
		return cpu.handleBreakpoint(bp, BreakpointExecute, pc)
	}
	return nil
}

func (cpu *cpu) checkAccessBreakpoint(address uint32, kind BreakpointType) error {
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

func (cpu *cpu) fetchOpcode() (uint16, error) {
	fetchPC := cpu.regs.PC
	if opcode, ok, err := cpu.readProgramFastWord(cpu.regs.PC); ok {
		if err != nil {
			cpu.recordProgramFault(cpu.regs.PC, err)
			return 0, err
		}
		cpu.rememberOpcodePC(fetchPC)
		cpu.regs.PC += uint32(Word)
		return opcode, nil
	}

	if opcode, err := cpu.readProgram(Word, cpu.regs.PC); err == nil {
		cpu.rememberOpcodePC(fetchPC)
		cpu.regs.PC += uint32(Word)
		return uint16(opcode), nil
	} else {
		return 0, err
	}
}

func (cpu *cpu) Reset() error {
	cpu.regs = Registers{SR: 0x2700}
	if cpu.interrupts == nil {
		cpu.interrupts = NewInterruptController()
	} else {
		cpu.interrupts.Reset()
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
	cpu.fault = faultInfo{}
	cpu.inException = false
	cpu.lastException = ExceptionInfo{}
	cpu.lastExceptionValid = false
	cpu.stepException = ExceptionInfo{}
	cpu.stepExceptionValid = false
	cpu.lastInterrupt = InterruptInfo{}
	cpu.lastInterruptValid = false
	cpu.stepInterrupt = InterruptInfo{}
	cpu.stepInterruptValid = false
	cpu.lastOpcodePC = 0
	cpu.lastOpcodePCValid = false
	cpu.currentOpcodePC = 0
	cpu.currentOpcodeValid = false
	cpu.traceBytes = cpu.traceBytes[:0]
	cpu.stepBusAccesses = cpu.stepBusAccesses[:0]
	cpu.historyNext = 0
	cpu.historyCount = 0
	cpu.refreshDebugModes()
	if cpu.scheduler != nil {
		cpu.scheduler.Reset(0)
	}
	return nil
}

func NewCPU(bus AddressBus) (CPU, error) {
	c := cpu{bus: bus}

	if b, ok := bus.(*Bus); ok {
		c.busFast = b
		previous := b.waitHook
		b.SetWaitHook(func(states uint32) {
			if previous != nil {
				previous(states)
			}
			c.addCycles(states)
		})
	}

	if err := c.Reset(); err != nil {
		return nil, err
	}
	return &c, nil
}

// registerInstruction adds an opcode handler to the CPU and records the
// precomputed cycle count for each opcode value that matches the mask.
func registerInstruction(ins instruction, match, mask uint16, eaMask uint16, calc cycleCalculator) {
	for value := uint16(0); ; {
		index := match | value
		if validEA(index, eaMask) {
			if opcodeTable[index] != nil {
				panic(fmt.Errorf("instruction 0x%04x already registered (existing %p new %p)", index, opcodeTable[index], ins))
			}
			opcodeTable[index] = ins
			if calc != nil {
				opcodeCycleTable[index] = calc(index)
			}
		}

		value = ((value | mask) + 1) & ^mask
		if value == 0 {
			break
		}
	}
}

func validEA(opcode, mask uint16) bool {
	if mask == 0 {
		return true
	}

	switch opcode & 0x3f {
	case 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07:
		return (mask & eaMaskDataRegister) != 0
	case 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f:
		return (mask & eaMaskAddressRegister) != 0
	case 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17:
		return (mask & eaMaskIndirect) != 0
	case 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f:
		return (mask & eaMaskPostIncrement) != 0
	case 0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27:
		return (mask & eaMaskPreDecrement) != 0
	case 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f:
		return (mask & eaMaskDisplacement) != 0
	case 0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37:
		return (mask & eaMaskIndex) != 0
	case 0x38:
		return (mask & eaMaskAbsoluteShort) != 0
	case 0x39:
		return (mask & eaMaskAbsoluteLong) != 0
	case 0x3a:
		return (mask & eaMaskPCDisplacement) != 0
	case 0x3b:
		return (mask & eaMaskPCIndex) != 0
	case 0x3c:
		return (mask & eaMaskImmediate) != 0
	}
	return false
}

func (cpu *cpu) push(s Size, value uint32) error {
	cpu.regs.A[7] -= uint32(s)
	return cpu.write(s, cpu.regs.A[7], value)
}

func (cpu *cpu) pushException(s Size, value uint32) error {
	cpu.regs.A[7] -= uint32(s)
	return cpu.writeSystemData(s, cpu.regs.A[7], value)
}

func (cpu *cpu) pop(s Size) (uint32, error) {
	if res, err := cpu.read(s, cpu.regs.A[7]); err == nil {
		cpu.regs.A[7] += uint32(s) // sometimes odd
		return res, nil
	} else {
		return 0, err
	}
}

func (cpu *cpu) popPc(s Size) (uint32, error) {
	switch s {
	case Word:
		if res, ok, err := cpu.readProgramFastWord(cpu.regs.PC); ok {
			if err != nil {
				cpu.recordProgramFault(cpu.regs.PC, err)
				return 0, err
			}
			cpu.regs.PC += uint32(s)
			if cpu.regs.PC&1 != 0 {
				cpu.regs.PC++ // never odd
			}
			return uint32(res), nil
		}
	case Long:
		if res, ok, err := cpu.readProgramFastLong(cpu.regs.PC); ok {
			if err != nil {
				cpu.recordProgramFault(cpu.regs.PC, err)
				return 0, err
			}
			cpu.regs.PC += uint32(s)
			if cpu.regs.PC&1 != 0 {
				cpu.regs.PC++ // never odd
			}
			return res, nil
		}
	}

	if res, err := cpu.readProgram(s, cpu.regs.PC); err == nil {
		cpu.regs.PC += uint32(s)
		if cpu.regs.PC&1 != 0 {
			cpu.regs.PC++ // never odd
		}
		return res, nil

	} else {
		return 0, err
	}
}

// readProgramFastWord keeps the single-RAM fast path active while still
// reporting instruction fetches through the debug bus hook.
func (cpu *cpu) readProgramFastWord(address uint32) (uint16, bool, error) {
	if cpu.breakpoints != nil {
		return 0, false, nil
	}

	ram := cpu.fastRAMDevice()
	if ram == nil {
		return 0, false, nil
	}

	address &= 0xffffff
	if address&1 != 0 {
		return 0, true, AddressError(address)
	}
	if address < ram.offset {
		return 0, true, BusError(address)
	}

	idx := address - ram.offset
	memLen := uint32(len(ram.mem))
	if idx+1 >= memLen {
		return 0, true, BusError(address)
	}

	value := uint16(ram.mem[idx])<<8 | uint16(ram.mem[idx+1])
	ctx := accessContext{functionCode: cpu.programFunctionCode()}
	if cpu.shouldTraceBusAccess(ctx) {
		cpu.traceBusAccess(Word, address, uint32(value), ctx)
	}
	return value, true, nil
}

func (cpu *cpu) readProgramFastLong(address uint32) (uint32, bool, error) {
	if cpu.breakpoints != nil {
		return 0, false, nil
	}

	ram := cpu.fastRAMDevice()
	if ram == nil {
		return 0, false, nil
	}

	address &= 0xffffff
	if address&1 != 0 {
		return 0, true, AddressError(address)
	}
	if address < ram.offset {
		return 0, true, BusError(address)
	}

	idx := address - ram.offset
	memLen := uint32(len(ram.mem))
	if idx+1 >= memLen {
		return 0, true, BusError(address)
	}
	if idx+3 >= memLen {
		return 0, true, BusError((address + uint32(Word)) & 0xffffff)
	}

	value := uint32(ram.mem[idx])<<24 |
		uint32(ram.mem[idx+1])<<16 |
		uint32(ram.mem[idx+2])<<8 |
		uint32(ram.mem[idx+3])
	ctx := accessContext{functionCode: cpu.programFunctionCode()}
	if cpu.shouldTraceBusAccess(ctx) {
		cpu.traceBusAccess(Long, address, value, ctx)
	}
	return value, true, nil
}

func (cpu *cpu) recordProgramFault(address uint32, err error) {
	cpu.recordFault(faultAddress(address, err), accessContext{functionCode: cpu.programFunctionCode()})
}

// addCycles increments the CPU cycle counter using a uint32 input to keep call
// sites close to the 68k reference values while storing the counter as a wider
// type.
func (cpu *cpu) addCycles(c uint32) {
	cpu.cycles += uint64(c)
	if cpu.scheduler != nil {
		cpu.scheduler.Advance(uint64(c))
	}
}

func (cpu *cpu) overrideInstructionCycles(total uint32) {
	current := opcodeCycleTable[cpu.regs.IR]
	if total >= current {
		cpu.cycles += uint64(total - current)
		return
	}
	cpu.cycles -= uint64(current - total)
}

// Cycles returns the total number of cycles executed since the last reset.
func (cpu *cpu) Cycles() uint64 {
	return cpu.cycles
}

func constantCycles(c uint32) cycleCalculator {
	return func(uint16) uint32 {
		return c
	}
}

func (cpu *cpu) disassemblyString() string {
	line, err := DisassembleInstruction(cpu.bus, cpu.regs.PC)
	if err != nil {
		return fmt.Sprintf("DISASM %08x: <unavailable>", cpu.regs.PC)
	}
	return fmt.Sprintf("DISASM %08x: %s", cpu.regs.PC, line.Assembly)
}

func (cpu *cpu) dataFunctionCode() uint16 {
	if cpu.regs.SR&srSupervisor != 0 {
		return functionCodeSupervisorData
	}
	return functionCodeUserData
}

func (cpu *cpu) programFunctionCode() uint16 {
	if cpu.regs.SR&srSupervisor != 0 {
		return functionCodeSupervisorProg
	}
	return functionCodeUserProgram
}

func (cpu *cpu) recordFault(address uint32, ctx accessContext) {
	cpu.fault = faultInfo{
		address:        address & 0xffffff,
		pc:             cpu.regs.PC,
		ir:             cpu.regs.IR,
		functionCode:   ctx.functionCode & 0x7,
		write:          ctx.write,
		notInstruction: ctx.notInstruction,
		valid:          true,
	}
}

func (cpu *cpu) interruptMask() uint8 {
	return uint8((cpu.regs.SR & srInterruptMask) >> 8)
}

func (cpu *cpu) dispatchException(info ExceptionInfo) {
	cpu.lastException = info
	cpu.lastExceptionValid = true
	cpu.stepException = info
	cpu.stepExceptionValid = true
	cpu.appendHistory(HistoryEntry{Kind: HistoryException, Exception: info})
	if cpu.exceptionTrap != nil {
		cpu.exceptionTrap(info)
	}
}

func (cpu *cpu) dispatchInterrupt(info InterruptInfo) {
	cpu.lastInterrupt = info
	cpu.lastInterruptValid = true
	cpu.stepInterrupt = info
	cpu.stepInterruptValid = true
	cpu.appendHistory(HistoryEntry{Kind: HistoryInterrupt, Interrupt: info})
	if cpu.interruptTrap != nil {
		cpu.interruptTrap(info)
	}
}

func (cpu *cpu) shouldTraceBusAccess(ctx accessContext) bool {
	if ctx.instructionFetch() && !ctx.write && cpu.traceInstructions {
		return true
	}
	return cpu.traceBus
}

// traceBusAccess is the single funnel for memory access reporting. It updates
// fetched-byte tracking, step-local stop data, the rolling history buffer, and
// the external callback with the same normalized record.
func (cpu *cpu) traceBusAccess(size Size, address uint32, value uint32, ctx accessContext) {
	info := BusAccessInfo{
		Address:          address & 0xffffff,
		Size:             size,
		Value:            value & size.mask(),
		Write:            ctx.write,
		InstructionFetch: ctx.instructionFetch(),
		PC:               cpu.debugPC(),
	}

	if ctx.instructionFetch() && !ctx.write {
		cpu.traceBytes = appendTraceValue(cpu.traceBytes, size, value)
	}
	if cpu.collectStepBus {
		cpu.stepBusAccesses = append(cpu.stepBusAccesses, info)
	}
	if len(cpu.history) != 0 {
		cpu.appendHistory(HistoryEntry{Kind: HistoryBusAccess, BusAccess: info})
	}

	if cpu.busTrap == nil {
		return
	}

	cpu.busTrap(info)
}

func (cpu *cpu) runStopReason(options RunUntilOptions, result *RunResult) (RunStopReason, bool) {
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

func (cpu *cpu) pcStopReason(options RunUntilOptions) (RunStopReason, bool) {
	if options.StopOnPCRange != nil && options.StopOnPCRange.Contains(cpu.regs.PC) {
		return RunStopPCInRange, true
	}
	if options.StopWhenPCOutside != nil && !options.StopWhenPCOutside.Contains(cpu.regs.PC) {
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

func (ctx accessContext) instructionFetch() bool {
	return !ctx.notInstruction && !ctx.write &&
		(ctx.functionCode == functionCodeUserProgram || ctx.functionCode == functionCodeSupervisorProg)
}

func (r AddressRange) Contains(address uint32) bool {
	address &= 0xffffff
	return address >= (r.Start&0xffffff) && address <= (r.End&0xffffff)
}

func isIllegalException(vector uint32) bool {
	switch vector {
	case XIllegal, XLineA, XLineF:
		return true
	default:
		return false
	}
}

func (cpu *cpu) matchStopAtPC(options RunUntilOptions) (uint32, bool) {
	for _, target := range options.StopAtPC {
		if cpu.regs.PC&0xffffff == target&0xffffff {
			return cpu.regs.PC & 0xffffff, true
		}
	}
	return 0, false
}

func (cpu *cpu) matchStepBusAccess(options RunUntilOptions) (BusAccessInfo, bool) {
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

func (cpu *cpu) runPredicateInfo(result *RunResult) RunPredicateInfo {
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

func (cpu *cpu) appendHistory(entry HistoryEntry) {
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
	entry.Trace = cloneTraceInfo(entry.Trace)
	entry.Exception = cloneExceptionInfo(entry.Exception)
	return entry
}

func cloneTraceInfo(info TraceInfo) TraceInfo {
	info.Bytes = append([]byte(nil), info.Bytes...)
	return info
}

func cloneExceptionInfo(info ExceptionInfo) ExceptionInfo {
	return info
}

// ReadExceptionStackFrame decodes a 68000 exception frame directly from memory
// without requiring the caller to know the byte layout.
func ReadExceptionStackFrame(bus AddressBus, sp uint32, format ExceptionStackFrameFormat) (ExceptionStackFrame, error) {
	frame := ExceptionStackFrame{
		Format:       format,
		StackPointer: sp & 0xffffff,
	}

	switch format {
	case ExceptionStackFrameGroup12:
		sr, err := bus.Read(Word, sp)
		if err != nil {
			return ExceptionStackFrame{}, err
		}
		pc, err := bus.Read(Long, sp+uint32(Word))
		if err != nil {
			return ExceptionStackFrame{}, err
		}
		frame.SR = uint16(sr)
		frame.PC = pc
		return frame, nil
	case ExceptionStackFrameGroup0:
		statusWord, err := bus.Read(Word, sp)
		if err != nil {
			return ExceptionStackFrame{}, err
		}
		faultAddress, err := bus.Read(Long, sp+2)
		if err != nil {
			return ExceptionStackFrame{}, err
		}
		ir, err := bus.Read(Word, sp+6)
		if err != nil {
			return ExceptionStackFrame{}, err
		}
		sr, err := bus.Read(Word, sp+8)
		if err != nil {
			return ExceptionStackFrame{}, err
		}
		pc, err := bus.Read(Long, sp+10)
		if err != nil {
			return ExceptionStackFrame{}, err
		}
		frame.StatusWord = uint16(statusWord)
		frame.FaultAddress = faultAddress
		frame.InstructionRegister = uint16(ir)
		frame.SR = uint16(sr)
		frame.PC = pc
		return frame, nil
	default:
		return ExceptionStackFrame{}, fmt.Errorf("unknown exception stack frame format %d", format)
	}
}

// CurrentExceptionFrame rereads the most recent exception frame from the stack,
// which is useful after a handler has inspected or modified it in memory.
func (cpu *cpu) CurrentExceptionFrame() (ExceptionStackFrame, bool, error) {
	if !cpu.lastExceptionValid || !cpu.lastException.FrameValid {
		return ExceptionStackFrame{}, false, nil
	}

	frame, err := ReadExceptionStackFrame(cpu.bus, cpu.lastException.StackPointer, cpu.lastException.Frame.Format)
	if err != nil {
		return ExceptionStackFrame{}, true, err
	}
	return frame, true, nil
}

func (cpu *cpu) traceInstructionBytes(pc uint32) []byte {
	if len(cpu.traceBytes) != 0 {
		return append([]byte(nil), cpu.traceBytes...)
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
