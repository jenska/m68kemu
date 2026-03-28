package m68kemu

import "testing"

func TestFetchOpcodeFastPathRecordsSupervisorProgramFault(t *testing.T) {
	cpu, _ := newEnvironment(t)
	cpu.regs.PC = 0x2001 // odd opcode fetch => address fault

	_, err := cpu.fetchOpcode()
	expectAddressError(t, err)

	state := cpu.DebugState()
	if !state.LastFault.Valid {
		t.Fatalf("expected valid fault info")
	}
	if state.LastFault.FunctionCode != functionCodeSupervisorProg {
		t.Fatalf("function code = %d, want %d", state.LastFault.FunctionCode, functionCodeSupervisorProg)
	}
	if !state.LastFault.InstructionFetch {
		t.Fatalf("expected instruction fetch fault")
	}
	if state.LastFault.Address != 0x2001 {
		t.Fatalf("fault address = %08x, want %08x", state.LastFault.Address, 0x2001)
	}
}

func TestFetchOpcodeFastPathRecordsUserProgramFault(t *testing.T) {
	cpu, _ := newEnvironment(t)
	cpu.regs.SR &^= srSupervisor // user mode
	cpu.regs.PC = 0x800000       // outside test RAM => bus error

	_, err := cpu.fetchOpcode()
	expectBusError(t, err)

	state := cpu.DebugState()
	if !state.LastFault.Valid {
		t.Fatalf("expected valid fault info")
	}
	if state.LastFault.FunctionCode != functionCodeUserProgram {
		t.Fatalf("function code = %d, want %d", state.LastFault.FunctionCode, functionCodeUserProgram)
	}
	if !state.LastFault.InstructionFetch {
		t.Fatalf("expected instruction fetch fault")
	}
	if state.LastFault.Address != 0x800000 {
		t.Fatalf("fault address = %08x, want %08x", state.LastFault.Address, 0x800000)
	}
}

func TestPopPcFastLongFaultUsesSecondCycleAddress(t *testing.T) {
	cpu, _ := newEnvironment(t)
	cpu.regs.PC = 0x0000fffe // aligned; first word in range, second word out of range

	_, err := cpu.popPc(Long)
	expectBusError(t, err)

	state := cpu.DebugState()
	if !state.LastFault.Valid {
		t.Fatalf("expected valid fault info")
	}
	if state.LastFault.Address != 0x00010000 {
		t.Fatalf("fault address = %08x, want %08x", state.LastFault.Address, 0x00010000)
	}
	if state.LastFault.FunctionCode != functionCodeSupervisorProg {
		t.Fatalf("function code = %d, want %d", state.LastFault.FunctionCode, functionCodeSupervisorProg)
	}
}
