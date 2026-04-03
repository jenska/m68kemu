package m68kemu

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestExceptionTracerReportsIllegalInstruction(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC
	handler := uint32(0x2200)

	if err := ram.Write(Long, uint32(XIllegal<<2), handler); err != nil {
		t.Fatalf("failed to install illegal vector: %v", err)
	}

	code := assemble(t, "ILLEGAL\n")
	for i, value := range code {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(value)); err != nil {
			t.Fatalf("failed to write opcode byte: %v", err)
		}
	}

	var (
		called bool
		got    ExceptionInfo
		state  DebugState
	)
	cpu.SetExceptionTracer(func(info ExceptionInfo) {
		called = true
		got = info
		state = cpu.DebugState()
	})

	if err := cpu.Step(); err != nil {
		t.Fatalf("step failed: %v", err)
	}

	if !called {
		t.Fatalf("expected exception tracer to be called")
	}
	if got.Vector != XIllegal {
		t.Fatalf("exception vector = %d, want %d", got.Vector, XIllegal)
	}
	if got.NewPC != handler {
		t.Fatalf("exception handler PC = %08x, want %08x", got.NewPC, handler)
	}
	if got.PC != startPC+uint32(Word) {
		t.Fatalf("stacked PC = %08x, want %08x", got.PC, startPC+uint32(Word))
	}
	if got.Opcode != 0x4afc {
		t.Fatalf("exception opcode = %04x, want 4afc", got.Opcode)
	}
	if got.FaultValid {
		t.Fatalf("illegal exception unexpectedly reported a fault address")
	}
	if !state.InException {
		t.Fatalf("debug state should report exception processing inside callback")
	}
	if !state.HasException || state.LastException.Vector != XIllegal {
		t.Fatalf("debug state did not retain the last exception")
	}
}

func TestExceptionTracerReportsBusFaultDetails(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC
	handler := uint32(0x2400)
	faultAddress := uint32(0x800000)

	if err := ram.Write(Long, uint32(XBusError<<2), handler); err != nil {
		t.Fatalf("failed to install bus error vector: %v", err)
	}

	code := assemble(t, "MOVE.B (A0),D0\n")
	for i, value := range code {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(value)); err != nil {
			t.Fatalf("failed to write opcode byte: %v", err)
		}
	}
	cpu.regs.A[0] = faultAddress

	var got ExceptionInfo
	cpu.SetExceptionTracer(func(info ExceptionInfo) {
		got = info
	})

	if err := cpu.Step(); err != nil {
		t.Fatalf("step failed: %v", err)
	}

	if got.Vector != XBusError {
		t.Fatalf("exception vector = %d, want %d", got.Vector, XBusError)
	}
	if !got.FaultValid {
		t.Fatalf("expected bus exception to report a fault address")
	}
	if got.FaultAddress != faultAddress {
		t.Fatalf("fault address = %08x, want %08x", got.FaultAddress, faultAddress)
	}
	if got.NewPC != handler {
		t.Fatalf("handler PC = %08x, want %08x", got.NewPC, handler)
	}
	if got.Opcode != binary.BigEndian.Uint16(code[:2]) {
		t.Fatalf("opcode = %04x, want %04x", got.Opcode, binary.BigEndian.Uint16(code[:2]))
	}

	state := cpu.DebugState()
	if !state.LastFault.Valid {
		t.Fatalf("debug state did not preserve last fault info")
	}
	if state.LastFault.Address != faultAddress {
		t.Fatalf("last fault address = %08x, want %08x", state.LastFault.Address, faultAddress)
	}
	if state.LastFault.Opcode != got.Opcode {
		t.Fatalf("last fault opcode = %04x, want %04x", state.LastFault.Opcode, got.Opcode)
	}
}

func TestBusTracerDistinguishesInstructionFetchAndDataAccess(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC

	if err := ram.Write(Word, startPC, 0x4e71); err != nil {
		t.Fatalf("failed to seed NOP: %v", err)
	}
	if err := ram.Write(Byte, 0x3000, 0x12); err != nil {
		t.Fatalf("failed to seed data: %v", err)
	}

	var accesses []BusAccessInfo
	cpu.SetBusTracer(func(info BusAccessInfo) {
		accesses = append(accesses, info)
	})

	if _, err := cpu.fetchOpcode(); err != nil {
		t.Fatalf("fetch opcode failed: %v", err)
	}
	if _, err := cpu.read(Byte, 0x3000); err != nil {
		t.Fatalf("data read failed: %v", err)
	}
	if err := cpu.write(Byte, 0x3001, 0x34); err != nil {
		t.Fatalf("data write failed: %v", err)
	}

	if len(accesses) != 3 {
		t.Fatalf("expected 3 traced accesses, got %d", len(accesses))
	}
	if accesses[0].Address != startPC || accesses[0].Size != Word || accesses[0].Value != 0x4e71 || !accesses[0].InstructionFetch || accesses[0].Write {
		t.Fatalf("unexpected fetch trace: %+v", accesses[0])
	}
	if accesses[1].Address != 0x3000 || accesses[1].Size != Byte || accesses[1].Value != 0x12 || accesses[1].InstructionFetch || accesses[1].Write {
		t.Fatalf("unexpected read trace: %+v", accesses[1])
	}
	if accesses[2].Address != 0x3001 || accesses[2].Size != Byte || accesses[2].Value != 0x34 || accesses[2].InstructionFetch || !accesses[2].Write {
		t.Fatalf("unexpected write trace: %+v", accesses[2])
	}
}

func TestTraceInfoIncludesOpcodeBytesAndCycleDelta(t *testing.T) {
	helper := newStepTestHelper(t)
	program := helper.LoadAssembly("MOVEA.L #1,A1\n")
	line1 := program.EntryForLine(t, 1)

	var trace TraceInfo
	helper.cpu.SetTracer(func(info TraceInfo) {
		trace = info
	})

	helper.RunInstructions(1)

	if trace.PC != program.PCForLine(t, 1) {
		t.Fatalf("trace PC = %08x, want %08x", trace.PC, program.PCForLine(t, 1))
	}
	if trace.Opcode != binary.BigEndian.Uint16(line1.Bytes[:2]) {
		t.Fatalf("trace opcode = %04x, want %04x", trace.Opcode, binary.BigEndian.Uint16(line1.Bytes[:2]))
	}
	if !bytes.Equal(trace.Bytes, line1.Bytes) {
		t.Fatalf("trace bytes = % x, want % x", trace.Bytes, line1.Bytes)
	}
	if trace.CycleDelta != uint32(helper.cpu.Cycles()) {
		t.Fatalf("trace cycle delta = %d, want %d", trace.CycleDelta, helper.cpu.Cycles())
	}
	if trace.Registers.A[1] != 1 {
		t.Fatalf("trace A1 = %d, want 1", trace.Registers.A[1])
	}
}

func TestTraceInfoIncludesMnemonicAndBeforeRegisters(t *testing.T) {
	helper := newStepTestHelper(t)
	helper.LoadAssembly("MOVEQ #5,D0\nMOVEQ #6,D0\n")

	var traces []TraceInfo
	helper.cpu.SetTracer(func(info TraceInfo) {
		traces = append(traces, info)
	})

	helper.RunInstructions(2)

	if len(traces) != 2 {
		t.Fatalf("expected 2 traces, got %d", len(traces))
	}

	first := traces[0]
	if first.Mnemonic != "MOVEQ #5, D0" {
		t.Fatalf("first mnemonic = %q, want %q", first.Mnemonic, "MOVEQ #5, D0")
	}
	if first.BeforeRegisters.D[0] != 0 {
		t.Fatalf("first before D0 = %d, want 0", first.BeforeRegisters.D[0])
	}
	if first.Registers.D[0] != 5 {
		t.Fatalf("first after D0 = %d, want 5", first.Registers.D[0])
	}

	second := traces[1]
	if second.Mnemonic != "MOVEQ #6, D0" {
		t.Fatalf("second mnemonic = %q, want %q", second.Mnemonic, "MOVEQ #6, D0")
	}
	if second.BeforeRegisters.D[0] != 5 {
		t.Fatalf("second before D0 = %d, want 5", second.BeforeRegisters.D[0])
	}
	if second.Registers.D[0] != 6 {
		t.Fatalf("second after D0 = %d, want 6", second.Registers.D[0])
	}
}

func TestPreTracerSeesInstructionBeforeExecution(t *testing.T) {
	helper := newStepTestHelper(t)
	program := helper.LoadAssembly("MOVEQ #5,D0\n")

	var got PreTraceInfo
	helper.cpu.SetPreTracer(func(info PreTraceInfo) {
		got = info
	})

	helper.RunInstructions(1)

	if got.PC != program.PCForLine(t, 1) {
		t.Fatalf("pre-trace PC = %08x, want %08x", got.PC, program.PCForLine(t, 1))
	}
	if got.Opcode != 0x7005 {
		t.Fatalf("pre-trace opcode = %04x, want 7005", got.Opcode)
	}
	if got.Mnemonic != "MOVEQ #5, D0" {
		t.Fatalf("pre-trace mnemonic = %q, want %q", got.Mnemonic, "MOVEQ #5, D0")
	}
	if !bytes.Equal(got.Bytes, program.BytesForLine(t, 1)) {
		t.Fatalf("pre-trace bytes = % x, want % x", got.Bytes, program.BytesForLine(t, 1))
	}
	if got.Registers.D[0] != 0 {
		t.Fatalf("pre-trace D0 = %d, want 0", got.Registers.D[0])
	}
}

func TestRunUntilSupportsCommonStopConditions(t *testing.T) {
	t.Run("instruction limit", func(t *testing.T) {
		cpu, ram := newEnvironment(t)
		startPC := cpu.regs.PC
		code := assemble(t, "NOP\nNOP\nNOP\n")
		for i, value := range code {
			if err := ram.Write(Byte, startPC+uint32(i), uint32(value)); err != nil {
				t.Fatalf("failed to write opcode byte: %v", err)
			}
		}

		result, err := cpu.RunUntil(RunUntilOptions{MaxInstructions: 2})
		if err != nil {
			t.Fatalf("run until failed: %v", err)
		}
		if result.Reason != RunStopInstructionLimit {
			t.Fatalf("stop reason = %v, want %v", result.Reason, RunStopInstructionLimit)
		}
		if result.Instructions != 2 {
			t.Fatalf("instructions = %d, want 2", result.Instructions)
		}
		if result.PC != startPC+2*uint32(Word) {
			t.Fatalf("PC = %08x, want %08x", result.PC, startPC+2*uint32(Word))
		}
	})

	t.Run("pc range", func(t *testing.T) {
		helper := newStepTestHelper(t)
		program := helper.LoadAssembly("NOP\nNOP\n")

		result, err := helper.cpu.RunUntil(RunUntilOptions{
			StopOnPCRange: &AddressRange{Start: program.PCForLine(t, 2), End: program.PCForLine(t, 2)},
		})
		if err != nil {
			t.Fatalf("run until failed: %v", err)
		}
		if result.Reason != RunStopPCInRange {
			t.Fatalf("stop reason = %v, want %v", result.Reason, RunStopPCInRange)
		}
		if result.Instructions != 1 {
			t.Fatalf("instructions = %d, want 1", result.Instructions)
		}
	})

	t.Run("exception and illegal opcode", func(t *testing.T) {
		cpu, ram := newEnvironment(t)
		handler := uint32(0x2600)
		if err := ram.Write(Long, uint32(XIllegal<<2), handler); err != nil {
			t.Fatalf("failed to install illegal vector: %v", err)
		}
		code := assemble(t, "ILLEGAL\n")
		for i, value := range code {
			if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(value)); err != nil {
				t.Fatalf("failed to write opcode byte: %v", err)
			}
		}

		result, err := cpu.RunUntil(RunUntilOptions{StopOnException: true, StopOnIllegal: true})
		if err != nil {
			t.Fatalf("run until failed: %v", err)
		}
		if result.Reason != RunStopIllegalOpcode {
			t.Fatalf("stop reason = %v, want %v", result.Reason, RunStopIllegalOpcode)
		}
		if !result.HasException || result.Exception.Vector != XIllegal {
			t.Fatalf("missing illegal exception details: %+v", result)
		}
	})

	t.Run("exception only", func(t *testing.T) {
		cpu, ram := newEnvironment(t)
		handler := uint32(0x2800)
		if err := ram.Write(Long, uint32(XIllegal<<2), handler); err != nil {
			t.Fatalf("failed to install illegal vector: %v", err)
		}
		code := assemble(t, "ILLEGAL\n")
		for i, value := range code {
			if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(value)); err != nil {
				t.Fatalf("failed to write opcode byte: %v", err)
			}
		}

		result, err := cpu.RunUntil(RunUntilOptions{StopOnException: true})
		if err != nil {
			t.Fatalf("run until failed: %v", err)
		}
		if result.Reason != RunStopException {
			t.Fatalf("stop reason = %v, want %v", result.Reason, RunStopException)
		}
		if !result.HasException || result.Exception.Vector != XIllegal {
			t.Fatalf("missing exception details: %+v", result)
		}
	})

	t.Run("pc leaves range", func(t *testing.T) {
		helper := newStepTestHelper(t)
		program := helper.LoadAssembly("NOP\nNOP\n")
		line1 := program.EntryForLine(t, 1)

		result, err := helper.cpu.RunUntil(RunUntilOptions{
			StopWhenPCOutside: &AddressRange{
				Start: program.PCForLine(t, 1),
				End:   program.PCForLine(t, 1) + uint32(len(line1.Bytes)) - 1,
			},
		})
		if err != nil {
			t.Fatalf("run until failed: %v", err)
		}
		if result.Reason != RunStopPCOutsideRange {
			t.Fatalf("stop reason = %v, want %v", result.Reason, RunStopPCOutsideRange)
		}
		if result.Instructions != 1 {
			t.Fatalf("instructions = %d, want 1", result.Instructions)
		}
	})
}

func TestExceptionTracingExposesOpcodeAddressAndFrame(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC
	handler := uint32(0x2600)

	if err := ram.Write(Long, uint32(XIllegal<<2), handler); err != nil {
		t.Fatalf("failed to install illegal vector: %v", err)
	}

	code := assemble(t, "ILLEGAL\n")
	for i, value := range code {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(value)); err != nil {
			t.Fatalf("failed to write opcode byte: %v", err)
		}
	}

	var got ExceptionInfo
	cpu.SetExceptionTracer(func(info ExceptionInfo) {
		got = info
	})

	if err := cpu.Step(); err != nil {
		t.Fatalf("step failed: %v", err)
	}

	if got.OpcodeAddress != startPC {
		t.Fatalf("opcode address = %08x, want %08x", got.OpcodeAddress, startPC)
	}
	if got.StackPointer != cpu.regs.A[7] {
		t.Fatalf("stack pointer = %08x, want %08x", got.StackPointer, cpu.regs.A[7])
	}
	if !got.FrameValid {
		t.Fatalf("expected exception frame to be captured")
	}
	if got.Frame.Format != ExceptionStackFrameGroup12 {
		t.Fatalf("frame format = %v, want %v", got.Frame.Format, ExceptionStackFrameGroup12)
	}
	if got.Frame.SR != 0x2700 {
		t.Fatalf("frame SR = %04x, want 2700", got.Frame.SR)
	}
	if got.Frame.PC != startPC+uint32(Word) {
		t.Fatalf("frame PC = %08x, want %08x", got.Frame.PC, startPC+uint32(Word))
	}

	frame, ok, err := cpu.CurrentExceptionFrame()
	if err != nil {
		t.Fatalf("CurrentExceptionFrame failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected current exception frame")
	}
	if frame != got.Frame {
		t.Fatalf("current frame = %+v, want %+v", frame, got.Frame)
	}
}

func TestBusTracerIncludesCurrentInstructionPC(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC
	code := assemble(t, "MOVE.B (A0),D0\n")
	for i, value := range code {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(value)); err != nil {
			t.Fatalf("failed to write opcode byte: %v", err)
		}
	}
	cpu.regs.A[0] = 0x3000
	if err := ram.Write(Byte, 0x3000, 0x12); err != nil {
		t.Fatalf("failed to write data byte: %v", err)
	}

	var accesses []BusAccessInfo
	cpu.SetBusTracer(func(info BusAccessInfo) {
		accesses = append(accesses, info)
	})

	if err := cpu.Step(); err != nil {
		t.Fatalf("step failed: %v", err)
	}

	if len(accesses) < 2 {
		t.Fatalf("expected at least 2 bus accesses, got %d", len(accesses))
	}
	if accesses[0].PC != startPC {
		t.Fatalf("fetch PC = %08x, want %08x", accesses[0].PC, startPC)
	}
	if accesses[1].PC != startPC {
		t.Fatalf("data access PC = %08x, want %08x", accesses[1].PC, startPC)
	}
}

func TestInterruptTracerReportsAcceptance(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC
	handler := uint32(0x3000)

	if err := ram.Write(Long, uint32((autoVectorBase+2)<<2), handler); err != nil {
		t.Fatalf("failed to install interrupt vector: %v", err)
	}

	code := assemble(t, "NOP\n")
	for i, value := range code {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(value)); err != nil {
			t.Fatalf("failed to write opcode byte: %v", err)
		}
	}

	var got InterruptInfo
	cpu.SetInterruptTracer(func(info InterruptInfo) {
		got = info
	})
	cpu.setSR(srSupervisor)

	if err := cpu.RequestInterrupt(2, nil); err != nil {
		t.Fatalf("failed to request interrupt: %v", err)
	}
	if err := cpu.Step(); err != nil {
		t.Fatalf("step failed: %v", err)
	}

	if got.Level != 2 {
		t.Fatalf("interrupt level = %d, want 2", got.Level)
	}
	if got.Vector != autoVectorBase+2 {
		t.Fatalf("interrupt vector = %d, want %d", got.Vector, autoVectorBase+2)
	}
	if !got.AutoVector {
		t.Fatalf("interrupt should report autovector")
	}
	if got.PC != startPC+uint32(Word) {
		t.Fatalf("interrupt PC = %08x, want %08x", got.PC, startPC+uint32(Word))
	}
	if got.NewPC != handler {
		t.Fatalf("interrupt new PC = %08x, want %08x", got.NewPC, handler)
	}
	if got.SR != srSupervisor {
		t.Fatalf("interrupt SR = %04x, want %04x", got.SR, uint16(srSupervisor))
	}
	if got.NewSR&srInterruptMask != uint16(2<<8) {
		t.Fatalf("interrupt new SR = %04x, want mask %04x", got.NewSR, uint16(2<<8))
	}
}

func TestRunUntilSupportsExactPCBusAccessAndPredicateStops(t *testing.T) {
	t.Run("exact pc", func(t *testing.T) {
		helper := newStepTestHelper(t)
		program := helper.LoadAssembly("NOP\nNOP\n")

		result, err := helper.cpu.RunUntil(RunUntilOptions{StopAtPC: []uint32{program.PCForLine(t, 2)}})
		if err != nil {
			t.Fatalf("run until failed: %v", err)
		}
		if result.Reason != RunStopPC {
			t.Fatalf("stop reason = %v, want %v", result.Reason, RunStopPC)
		}
		if result.Instructions != 1 {
			t.Fatalf("instructions = %d, want 1", result.Instructions)
		}
	})

	t.Run("bus access", func(t *testing.T) {
		cpu, ram := newEnvironment(t)
		code := assemble(t, "MOVE.B (A0),D0\n")
		for i, value := range code {
			if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(value)); err != nil {
				t.Fatalf("failed to write opcode byte: %v", err)
			}
		}
		cpu.regs.A[0] = 0x3000
		if err := ram.Write(Byte, 0x3000, 0x12); err != nil {
			t.Fatalf("failed to seed data: %v", err)
		}

		result, err := cpu.RunUntil(RunUntilOptions{
			StopOnBusAccess: func(info BusAccessInfo) bool {
				return !info.InstructionFetch && !info.Write && info.Address == 0x3000
			},
		})
		if err != nil {
			t.Fatalf("run until failed: %v", err)
		}
		if result.Reason != RunStopBusAccess {
			t.Fatalf("stop reason = %v, want %v", result.Reason, RunStopBusAccess)
		}
		if !result.HasBusAccess {
			t.Fatalf("expected matching bus access in result")
		}
		if result.BusAccess.Address != 0x3000 {
			t.Fatalf("bus access address = %08x, want 00003000", result.BusAccess.Address)
		}
	})

	t.Run("predicate", func(t *testing.T) {
		helper := newStepTestHelper(t)
		helper.LoadAssembly("MOVEQ #1,D0\nADDQ.B #1,D0\n")

		result, err := helper.cpu.RunUntil(RunUntilOptions{
			StopPredicate: func(info RunPredicateInfo) bool {
				return info.Registers.D[0] == 1
			},
		})
		if err != nil {
			t.Fatalf("run until failed: %v", err)
		}
		if result.Reason != RunStopPredicate {
			t.Fatalf("stop reason = %v, want %v", result.Reason, RunStopPredicate)
		}
		if result.Instructions != 1 {
			t.Fatalf("instructions = %d, want 1", result.Instructions)
		}
	})
}

func TestHistoryRingBufferKeepsRecentEvents(t *testing.T) {
	helper := newStepTestHelper(t)
	program := helper.LoadAssembly("NOP\nNOP\n")
	helper.cpu.SetHistoryLimit(2)

	helper.RunInstructions(2)

	history := helper.cpu.History()
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
	if history[0].Kind != HistoryBusAccess {
		t.Fatalf("oldest history kind = %v, want %v", history[0].Kind, HistoryBusAccess)
	}
	if history[1].Kind != HistoryInstruction {
		t.Fatalf("newest history kind = %v, want %v", history[1].Kind, HistoryInstruction)
	}
	if history[1].Trace.PC != program.PCForLine(t, 2) {
		t.Fatalf("history trace PC = %08x, want %08x", history[1].Trace.PC, program.PCForLine(t, 2))
	}
}

func TestStepTestHelperSupportsSmallRegressionScenarios(t *testing.T) {
	helper := newStepTestHelper(t)
	program := helper.LoadAssembly("MOVEQ #5,D0\nADDQ.B #1,D0\n")
	helper.SetRegisters(func(regs *Registers) {
		regs.A[0] = 0x3000
	})

	helper.RunInstructions(2)
	helper.AssertState(func(t *testing.T, cpu *cpu, _ *RAM) {
		if cpu.regs.D[0] != 6 {
			t.Fatalf("D0 = %d, want 6", cpu.regs.D[0])
		}
		if cpu.regs.PC != program.PCForLine(t, 2)+uint32(len(program.BytesForLine(t, 2))) {
			t.Fatalf("PC = %08x, want %08x", cpu.regs.PC, program.PCForLine(t, 2)+uint32(len(program.BytesForLine(t, 2))))
		}
		if cpu.regs.A[0] != 0x3000 {
			t.Fatalf("A0 = %08x, want 00003000", cpu.regs.A[0])
		}
	})
}

func TestAssembleProgramListingTracksPerLineAddresses(t *testing.T) {
	helper := newStepTestHelper(t)
	program := helper.LoadAssembly("NOP\nMOVEQ #1,D0\n")

	if len(program.Listing) != 2 {
		t.Fatalf("listing entries = %d, want 2", len(program.Listing))
	}
	if program.PCForLine(t, 1) != helper.cpu.regs.PC {
		t.Fatalf("line 1 PC = %08x, want %08x", program.PCForLine(t, 1), helper.cpu.regs.PC)
	}
	if program.PCForLine(t, 2) != helper.cpu.regs.PC+uint32(len(program.BytesForLine(t, 1))) {
		t.Fatalf("line 2 PC = %08x, want %08x", program.PCForLine(t, 2), helper.cpu.regs.PC+uint32(len(program.BytesForLine(t, 1))))
	}
}
