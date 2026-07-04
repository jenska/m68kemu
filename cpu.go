package m68kemu

import (
	"fmt"
	"strings"
)

const (
	Version           = "1.3.0"
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
	instruction func(*CPU) error

	AddressError uint32
	BusError     uint32

	// cycleCalculator builds a static cycle count for a given opcode. Results are
	// stored in OpcodeCycleTable during instruction registration and can be looked
	// up at execution time for fixed-cost instructions.
	cycleCalculator func(opcode uint16) uint32

	instructionRegistration struct {
		ins   instruction
		match uint16
		mask  uint16
		ea    uint16
		calc  cycleCalculator
	}

	opmodeRegistration struct {
		ins         instruction
		base        uint16
		first, last uint16
		mask        uint16
		ea          uint16
		calc        cycleCalculator
	}

	conditionRegistration struct {
		ins  instruction
		base uint16
		mask uint16
		calc cycleCalculator
	}

	// AddressBus for accessing address areas
	AddressBus interface {
		Read(s Size, address uint32) (uint32, error)
		Write(s Size, address uint32, value uint32) error
		Reset()
	}

	// TraceInfo reports the outcome of a single executed instruction.
	TraceInfo struct {
		PC         uint32
		SR         uint16
		Registers  Registers
		Opcode     uint16
		CycleDelta uint32
		Cycles     uint64
	}

	TraceCallback func(TraceInfo)

	// ExceptionStackFrameFormat identifies the 68000 frame layout captured for an exception.
	ExceptionStackFrameFormat int

	// ExceptionStackFrame mirrors the basic frame values pushed for an exception.
	ExceptionStackFrame struct {
		Format       ExceptionStackFrameFormat
		StackPointer uint32
		PC           uint32
	}

	// ExceptionInfo describes the last exception taken by the CPU.
	ExceptionInfo struct {
		Vector       uint32
		PC           uint32
		NewPC        uint32
		StackPointer uint32
		Frame        ExceptionStackFrame
		Group0       bool
	}

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
		LastFault     DebugFaultInfo
		LastException ExceptionInfo
		HasException  bool
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

	faultInfo struct {
		address        uint32
		pc             uint32
		ir             uint16
		functionCode   uint16
		write          bool
		notInstruction bool
		valid          bool
	}

	// CPU is the Motorola 68000 emulator core.
	CPU struct {
		regs       Registers
		cycles     uint64
		bus        AddressBus
		trap       TraceCallback
		scheduler  *CycleScheduler
		interrupts *InterruptController

		stopped bool

		fault              faultInfo
		inException        bool
		lastException      ExceptionInfo
		lastExceptionValid bool
		lastOpcodePC       uint32
		lastOpcodePCValid  bool
		currentOpcodePC    uint32
		currentOpcodeValid bool
		traceInstructions  bool
	}
)

const (
	ExceptionStackFrameGroup12 ExceptionStackFrameFormat = iota
	ExceptionStackFrameGroup0
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

func (ae AddressError) Error() string {
	return fmt.Sprintf("AddressError at %08x", uint32(ae))
}

func (be BusError) Error() string {
	return fmt.Sprintf("BusError at %08x", uint32(be))
}

func (cpu *CPU) read(size Size, address uint32) (uint32, error) {
	return cpu.readContext(size, address, accessContext{functionCode: cpu.dataFunctionCode()})
}

func (cpu *CPU) readProgram(size Size, address uint32) (uint32, error) {
	return cpu.readContext(size, address, accessContext{functionCode: cpu.programFunctionCode()})
}

func (cpu *CPU) readSystemProgram(size Size, address uint32) (uint32, error) {
	return cpu.readContext(size, address, accessContext{functionCode: functionCodeSupervisorProg, notInstruction: true})
}

func (cpu *CPU) readContext(size Size, address uint32, ctx accessContext) (uint32, error) {
	address &= 0xffffff // 24bit address bus of 68000
	switch size {
	case Byte, Word, Long:
		result, err := cpu.bus.Read(size, address)
		if err != nil {
			cpu.recordFault(faultAddress(address, err), ctx)
		}
		return uint32(result), err
	default:
		return 0, fmt.Errorf("unknown operand size")

	}
}

func (cpu *CPU) write(size Size, address uint32, value uint32) error {
	return cpu.writeContext(size, address, value, accessContext{functionCode: cpu.dataFunctionCode(), write: true})
}

func (cpu *CPU) writeSystemData(size Size, address uint32, value uint32) error {
	return cpu.writeContext(size, address, value, accessContext{functionCode: functionCodeSupervisorData, notInstruction: true, write: true})
}

func (cpu *CPU) writeContext(size Size, address uint32, value uint32, ctx accessContext) error {
	address &= 0xffffff // 24bit address bus of 68000
	switch size {
	case Byte, Word, Long:
		if err := cpu.bus.Write(size, address, value); err != nil {
			cpu.recordFault(faultAddress(address, err), ctx)
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown operand size")

	}
}

func (cpu *CPU) Registers() Registers {
	return cpu.regs
}

func (cpu *CPU) DebugState() DebugState {
	return DebugState{
		LastFault:     cpu.fault.snapshot(),
		LastException: cpu.lastException,
		HasException:  cpu.lastExceptionValid,
	}
}

func (cpu *CPU) SetTracer(cb TraceCallback) {
	cpu.trap = cb
	cpu.traceInstructions = cb != nil
}

func (cpu *CPU) requireSupervisor() (bool, error) {
	if cpu.regs.SR&srSupervisor != 0 {
		return true, nil
	}
	return false, cpu.exceptionWithCycles(XPrivViolation, exceptionCyclesPrivilege)
}

func (cpu *CPU) SetScheduler(s *CycleScheduler) {
	cpu.scheduler = s
	if s != nil {
		s.Reset(cpu.cycles)
	}
}

func (cpu *CPU) Scheduler() *CycleScheduler {
	return cpu.scheduler
}

func (cpu *CPU) RequestInterrupt(level uint8, vector *uint8) error {
	return cpu.interrupts.Request(level, vector)
}

func (cpu *CPU) rememberOpcodePC(pc uint32) {
	cpu.lastOpcodePC = pc
	cpu.lastOpcodePCValid = true
}

// beginInstructionContext records the instruction boundary so nested helpers,
// fault handling, and bus tracing can all attribute work to the same opcode.
func (cpu *CPU) beginInstructionContext(pc uint32) {
	cpu.currentOpcodePC = pc & 0xffffff
	cpu.currentOpcodeValid = true
}

func (cpu *CPU) endInstructionContext() {
	cpu.currentOpcodeValid = false
}

func (cpu *CPU) currentOpcodeAddress(fallback uint32) uint32 {
	if cpu.currentOpcodeValid {
		return cpu.currentOpcodePC
	}
	return fallback & 0xffffff
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

func (cpu *CPU) raiseException(vector uint32, newSR uint16) error {
	return cpu.raiseExceptionWithPC(vector, newSR, cpu.regs.PC, cpu.regs.PC)
}

func (cpu *CPU) synchronousException(vector uint32, newSR uint16, stackedPC uint32) error {
	return cpu.raiseExceptionWithPC(vector, newSR, stackedPC, stackedPC)
}

func (cpu *CPU) trapException(vector uint32) error {
	stackedPC := cpu.regs.PC & 0xffffff
	instructionPC := cpu.currentOpcodeAddress(stackedPC)
	if stackedPC == instructionPC {
		stackedPC = (instructionPC + uint32(Word)) & 0xffffff
	}
	return cpu.synchronousException(vector, cpu.regs.SR|srSupervisor, stackedPC)
}

func (cpu *CPU) raiseExceptionWithPC(vector uint32, newSR uint16, stackedPC uint32, exceptionPC uint32) error {
	if vector > 255 {
		return fmt.Errorf("invalid vector %d", vector)
	}

	vectorOffset := vector << 2
	originalSR := cpu.regs.SR
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
	cpu.lastException = ExceptionInfo{
		Vector:       vector,
		PC:           exceptionPC,
		NewPC:        handler,
		StackPointer: cpu.regs.A[7],
		Frame: ExceptionStackFrame{
			Format:       ExceptionStackFrameGroup12,
			StackPointer: cpu.regs.A[7],
			PC:           stackedPC,
		},
	}
	cpu.lastExceptionValid = true
	return nil
}

func (cpu *CPU) exception(vector uint32) error {
	return cpu.raiseException(vector, cpu.regs.SR|srSupervisor)
}

func (cpu *CPU) exceptionWithCycles(vector uint32, total uint32) error {
	cpu.overrideInstructionCycles(total)
	return cpu.exception(vector)
}

func (cpu *CPU) raiseGroup0Exception(vector uint32, newSR uint16) error {
	if vector > 255 {
		return fmt.Errorf("invalid vector %d", vector)
	}

	originalSR := cpu.regs.SR
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
	cpu.lastException = ExceptionInfo{
		Vector:       vector,
		PC:           cpu.fault.pc,
		NewPC:        handler,
		StackPointer: sp,
		Frame: ExceptionStackFrame{
			Format:       ExceptionStackFrameGroup0,
			StackPointer: sp,
			PC:           cpu.fault.pc,
		},
		Group0: true,
	}
	cpu.lastExceptionValid = true
	return nil
}

func (cpu *CPU) group0ExceptionForCurrentInstruction(vector uint32, total uint32) error {
	cpu.overrideInstructionCycles(total)
	return cpu.raiseGroup0Exception(vector, cpu.regs.SR|srSupervisor)
}

func (cpu *CPU) group0ExceptionWithoutInstruction(vector uint32, total uint32) error {
	cpu.addCycles(total)
	return cpu.raiseGroup0Exception(vector, cpu.regs.SR|srSupervisor)
}

// setSR keeps USP/SSP in sync when the supervisor bit changes.
func (cpu *CPU) setSR(value uint16) {
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

func (cpu *CPU) readVector(offset uint32) (uint32, error) {
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

func (cpu *CPU) interrupt(level uint8, vector uint32) error {
	newSR := (cpu.regs.SR & ^uint16(srInterruptMask)) | srSupervisor | (uint16(level) << 8)
	cpu.addCycles(exceptionCyclesInterrupt)
	return cpu.raiseException(vector, newSR)
}

func (cpu *CPU) checkInterrupts() error {
	if cpu.interrupts == nil || !cpu.interrupts.HasPending(cpu.regs.SR) {
		return nil
	}

	level, vector, _, ok := cpu.interrupts.Pending(cpu.regs.SR)
	if !ok {
		return nil
	}

	cpu.stopped = false

	return cpu.interrupt(level, vector)
}

func (cpu *CPU) handleFaultError(err error, currentInstruction bool) error {
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
func (cpu *CPU) executeNext() error {
	pc := cpu.regs.PC
	var beforeCycles uint64
	if cpu.traceInstructions {
		beforeCycles = cpu.cycles
	}
	cpu.beginInstructionContext(pc)

	opcode, err := cpu.fetchOpcode()
	if err != nil {
		cpu.endInstructionContext()
		return cpu.handleFaultError(err, false)
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
		cpu.sendTrace(pc, uint32(cpu.cycles-beforeCycles))
	}
	cpu.endInstructionContext()
	return nil
}

// Step fetches the next opcode at the program counter and executes it.
func (cpu *CPU) Step() error {
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

func (cpu *CPU) RunInstructions(count uint64) error {
	for range count {
		if err := cpu.Step(); err != nil {
			return err
		}
	}
	return nil
}

func (cpu *CPU) sendTrace(pc uint32, cycleDelta uint32) {
	if !cpu.traceInstructions {
		return
	}

	info := TraceInfo{
		PC:         pc & 0xffffff,
		SR:         cpu.regs.SR,
		Registers:  cpu.regs,
		Opcode:     cpu.regs.IR,
		CycleDelta: cycleDelta,
		Cycles:     cpu.cycles,
	}
	if cpu.trap != nil {
		cpu.trap(info)
	}
}

func (cpu *CPU) fetchOpcode() (uint16, error) {
	fetchPC := cpu.regs.PC
	if opcode, err := cpu.readProgram(Word, cpu.regs.PC); err == nil {
		cpu.rememberOpcodePC(fetchPC)
		cpu.regs.PC += uint32(Word)
		return uint16(opcode), nil
	} else {
		return 0, err
	}
}

func (cpu *CPU) Reset() error {
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
	cpu.lastOpcodePC = 0
	cpu.lastOpcodePCValid = false
	cpu.currentOpcodePC = 0
	cpu.currentOpcodeValid = false
	if cpu.scheduler != nil {
		cpu.scheduler.Reset(0)
	}
	return nil
}

func NewCPU(bus AddressBus) (*CPU, error) {
	c := CPU{bus: bus}

	if b, ok := bus.(*Bus); ok {
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

func registerInstructions(regs ...instructionRegistration) {
	for _, reg := range regs {
		registerInstruction(reg.ins, reg.match, reg.mask, reg.ea, reg.calc)
	}
}

func registerOpmodes(regs ...opmodeRegistration) {
	for _, reg := range regs {
		for opmode := reg.first; opmode <= reg.last; opmode++ {
			registerInstruction(reg.ins, reg.base|(opmode<<6), reg.mask, reg.ea, reg.calc)
		}
	}
}

func registerConditions(regs ...conditionRegistration) {
	for _, reg := range regs {
		for cond := range uint16(16) {
			registerInstruction(reg.ins, reg.base|(cond<<8), reg.mask, 0, reg.calc)
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

func (cpu *CPU) push(s Size, value uint32) error {
	cpu.regs.A[7] -= uint32(s)
	return cpu.write(s, cpu.regs.A[7], value)
}

func (cpu *CPU) pushException(s Size, value uint32) error {
	cpu.regs.A[7] -= uint32(s)
	return cpu.writeSystemData(s, cpu.regs.A[7], value)
}

func (cpu *CPU) pop(s Size) (uint32, error) {
	if res, err := cpu.read(s, cpu.regs.A[7]); err == nil {
		cpu.regs.A[7] += uint32(s) // sometimes odd
		return res, nil
	} else {
		return 0, err
	}
}

func (cpu *CPU) popPc(s Size) (uint32, error) {
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

// addCycles increments the CPU cycle counter using a uint32 input to keep call
// sites close to the 68k reference values while storing the counter as a wider
// type.
func (cpu *CPU) addCycles(c uint32) {
	cpu.cycles += uint64(c)
	if cpu.scheduler != nil {
		cpu.scheduler.Advance(uint64(c))
	}
}

func (cpu *CPU) overrideInstructionCycles(total uint32) {
	current := opcodeCycleTable[cpu.regs.IR]
	if total >= current {
		cpu.cycles += uint64(total - current)
		return
	}
	cpu.cycles -= uint64(current - total)
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

func (cpu *CPU) recordFault(address uint32, ctx accessContext) {
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

func (cpu *CPU) interruptMask() uint8 {
	return uint8((cpu.regs.SR & srInterruptMask) >> 8)
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
