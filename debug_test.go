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
