package m68kemu

import (
	"bytes"
	"errors"
	"testing"
)

func TestTraceCallbackReceivesSnapshot(t *testing.T) {
	helper := newStepTestHelper(t)
	program := helper.LoadAssembly("MOVEA.L #1,A1\n")

	var traces []TraceInfo
	helper.cpu.SetTracer(func(info TraceInfo) {
		traces = append(traces, info)
	})

	helper.RunInstructions(1)

	if len(traces) != 1 {
		t.Fatalf("expected 1 trace entry, got %d", len(traces))
	}

	got := traces[0]
	if got.PC != program.PCForLine(t, 1) {
		t.Fatalf("trace PC = 0x%x, want 0x%x", got.PC, program.PCForLine(t, 1))
	}
	if got.Registers.A[1] != 1 {
		t.Fatalf("trace A1 = %d, want 1", got.Registers.A[1])
	}
	if !bytes.Equal(got.Bytes, program.BytesForLine(t, 1)) {
		t.Fatalf("trace bytes = % x, want % x", got.Bytes, program.BytesForLine(t, 1))
	}
}

func TestExecuteBreakpointHaltsStep(t *testing.T) {
	cpu, _ := newEnvironment(t)

	cpu.AddBreakpoint(Breakpoint{Address: cpu.regs.PC, OnExecute: true, Halt: true})

	err := cpu.Step()
	var hit BreakpointHit
	if !errors.As(err, &hit) {
		t.Fatalf("expected breakpoint hit, got %v", err)
	}

	if hit.Type != BreakpointExecute {
		t.Fatalf("unexpected breakpoint type %v", hit.Type)
	}
	if hit.Address != cpu.regs.PC {
		t.Fatalf("breakpoint address = 0x%x, want 0x%x", hit.Address, cpu.regs.PC)
	}
}

func TestWatchpointFiresOnWrite(t *testing.T) {
	cpu, ram := newEnvironment(t)

	target := uint32(0x3000)
	cpu.AddBreakpoint(Breakpoint{Address: target, OnWrite: true, Halt: true})

	err := cpu.write(Byte, target, 0xAA)
	var hit BreakpointHit
	if !errors.As(err, &hit) {
		t.Fatalf("expected breakpoint hit on write, got %v", err)
	}
	if hit.Type != BreakpointWrite {
		t.Fatalf("unexpected breakpoint type %v", hit.Type)
	}

	if val, _ := ram.Read(Byte, target); val != 0 {
		t.Fatalf("memory changed despite halted write: got 0x%x", val)
	}
}

func TestWatchpointCallbackWithoutHaltAllowsAccess(t *testing.T) {
	cpu, _ := newEnvironment(t)

	target := cpu.regs.PC // any readable address is fine for the bus alignment
	var called int
	cpu.AddBreakpoint(Breakpoint{Address: target, OnRead: true, Halt: false, Callback: func(BreakpointEvent) error {
		called++
		return nil
	}})

	if _, err := cpu.read(Word, target); err != nil {
		t.Fatalf("read failed with callback breakpoint: %v", err)
	}

	if called != 1 {
		t.Fatalf("expected callback to be invoked once, got %d", called)
	}
}
