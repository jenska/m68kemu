package m68kemu

import (
	"errors"
	"testing"
)

func TestTraceCallbackReceivesSnapshot(t *testing.T) {
	cpu, ram := newEnvironment(t)

	code := assemble(t, "MOVEA.L #1,A1\n")
	for i := range code {
		addr := cpu.regs.PC + uint32(i)
		if err := ram.Write(Byte, addr, uint32(code[i])); err != nil {
			t.Fatalf("failed to place instruction: %v", err)
		}
	}

	var traces []TraceInfo
	cpu.SetTracer(func(info TraceInfo) {
		traces = append(traces, info)
	})

	if err := cpu.Step(); err != nil {
		t.Fatalf("step failed: %v", err)
	}

	if len(traces) != 1 {
		t.Fatalf("expected 1 trace entry, got %d", len(traces))
	}

	got := traces[0]
	if got.PC != 0x2000 {
		t.Fatalf("trace PC = 0x%x, want 0x2000", got.PC)
	}
	if got.Registers.A[1] != 1 {
		t.Fatalf("trace A1 = %d, want 1", got.Registers.A[1])
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
