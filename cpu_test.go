package m68kemu

import (
	"strings"
	"testing"
)

// peekCountingRAM is a RAM device that tallies Read vs Peek calls so a test can
// assert that a code path uses the side-effect-free Peek route.
type peekCountingRAM struct {
	*RAM
	readCount int
	peekCount int
}

func newPeekCountingRAM(size uint32) *peekCountingRAM {
	return &peekCountingRAM{RAM: NewRAM(0, size)}
}

func (d *peekCountingRAM) Read(s Size, address uint32) (uint32, error) {
	d.readCount++
	return d.RAM.Read(s, address)
}

func (d *peekCountingRAM) Peek(s Size, address uint32) (uint32, error) {
	d.peekCount++
	return d.RAM.Read(s, address)
}

func TestCPUStringIncludesCurrentDisassembly(t *testing.T) {
	cpu, ram := newEnvironment(t)

	code := assemble(t, "MOVEQ #5,D0\n")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program byte: %v", err)
		}
	}

	text := cpu.String()

	if !strings.Contains(text, "DISASM 00002000: MOVEQ #5, D0") {
		t.Fatalf("CPU.String missing disassembly, got:\n%s", text)
	}
	if !strings.Contains(text, "SR 2700 PC 00002000") {
		t.Fatalf("CPU.String missing register dump, got:\n%s", text)
	}
}

func TestCPUStringUsesPeekInsteadOfLiveReads(t *testing.T) {
	mem := newPeekCountingRAM(0x10000)
	mem.RAM.Write(Long, 0, 0x1000)
	mem.RAM.Write(Long, 4, 0x2000)
	mem.RAM.Write(Word, 0x2000, 0x7005)

	processor, err := NewCPU(NewBus(mem))
	if err != nil {
		t.Fatalf("create CPU: %v", err)
	}

	impl, ok := processor.(*cpu)
	if !ok {
		t.Fatalf("unexpected CPU implementation %T", processor)
	}

	readsAfterReset := mem.readCount
	text := impl.String()

	if mem.readCount != readsAfterReset {
		t.Fatalf("CPU.String used Read: got %d reads after reset, want %d", mem.readCount, readsAfterReset)
	}
	if mem.peekCount == 0 {
		t.Fatalf("CPU.String did not use Peek")
	}
	if !strings.Contains(text, "DISASM 00002000: MOVEQ #5, D0") {
		t.Fatalf("CPU.String missing disassembly, got:\n%s", text)
	}
}

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

func TestCycleCounterBasicSequence(t *testing.T) {
	cpu, ram := newEnvironment(t)

	code := assemble(t, "MOVEQ #1,D0\nNOP")
	for i := range code {
		addr := cpu.regs.PC + uint32(i)
		if err := ram.Write(Byte, addr, uint32(code[i])); err != nil {
			t.Fatalf("failed to write byte to %04x: %v", addr, err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("first step failed: %v", err)
	}
	if cpu.Cycles() != 4 {
		t.Fatalf("unexpected cycles after MOVEQ: got %d want 4", cpu.Cycles())
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("second step failed: %v", err)
	}
	if cpu.Cycles() != 8 {
		t.Fatalf("unexpected cycles after NOP: got %d want 8", cpu.Cycles())
	}
}

func TestCycleCounterMemoryMove(t *testing.T) {
	cpu, ram := newEnvironment(t)

	cpu.regs.A[0] = 0x3000
	code := assemble(t, "MOVE.L D0,(A0)")
	for i := range code {
		addr := cpu.regs.PC + uint32(i)
		if err := ram.Write(Byte, addr, uint32(code[i])); err != nil {
			t.Fatalf("failed to write byte to %04x: %v", addr, err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("move failed: %v", err)
	}

	if cpu.Cycles() != 12 {
		t.Fatalf("unexpected cycles for MOVE.L D0,(A0): got %d want 12", cpu.Cycles())
	}
}

func TestCycleCounterWaitStates(t *testing.T) {
	ram := NewRAM(0, 0x1000)
	bus := NewBus(ram)
	bus.SetWaitStates(2)
	if err := ram.Write(Long, 0, 0x100); err != nil {
		t.Fatalf("failed to seed SSP: %v", err)
	}
	if err := ram.Write(Long, 4, 0x200); err != nil {
		t.Fatalf("failed to seed PC: %v", err)
	}

	cpu, err := NewCPU(bus)
	if err != nil {
		t.Fatalf("failed to create CPU: %v", err)
	}

	if err := ram.Write(Word, 0x200, 0x4e71); err != nil { // NOP
		t.Fatalf("failed to write NOP: %v", err)
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("failed to execute NOP: %v", err)
	}

	expected := uint64(6) // 4 base cycles + 2 wait states for the opcode fetch
	if cpu.Cycles() != expected {
		t.Fatalf("unexpected cycles with wait states: got %d want %d", cpu.Cycles(), expected)
	}
}

func TestExecuteInstructionAddsOpcodeCycles(t *testing.T) {
	cpu, _ := newEnvironment(t)

	const opcode = uint16(0x4e71) // NOP

	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("executeInstruction failed: %v", err)
	}

	expected := uint64(opcodesFor(M68000).cycles[opcode])
	if cpu.Cycles() != expected {
		t.Fatalf("unexpected cycles after executeInstruction: got %d want %d", cpu.Cycles(), expected)
	}
}

func TestIllegalInstructionUsesExceptionCycles(t *testing.T) {
	cpu, ram := newEnvironment(t)
	if err := ram.Write(Long, uint32(XIllegal<<2), 0x2200); err != nil {
		t.Fatalf("failed to seed vector: %v", err)
	}
	if err := ram.Write(Word, cpu.regs.PC, 0x4afc); err != nil {
		t.Fatalf("failed to write ILLEGAL: %v", err)
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	if cpu.Cycles() != uint64(exceptionCyclesIllegal) {
		t.Fatalf("unexpected cycles for ILLEGAL: got %d want %d", cpu.Cycles(), exceptionCyclesIllegal)
	}
}

func TestLineExceptionUsesExceptionCycles(t *testing.T) {
	tests := []struct {
		name   string
		opcode uint16
		vector uint32
	}{
		{name: "LineA", opcode: 0xa000, vector: XLineA},
		{name: "LineF", opcode: 0xf000, vector: XLineF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			if err := ram.Write(Long, tt.vector<<2, 0x2200); err != nil {
				t.Fatalf("failed to seed vector: %v", err)
			}
			if err := ram.Write(Word, cpu.regs.PC, uint32(tt.opcode)); err != nil {
				t.Fatalf("failed to write opcode: %v", err)
			}

			if err := cpu.Step(); err != nil {
				t.Fatalf("Step failed: %v", err)
			}
			if cpu.Cycles() != uint64(exceptionCyclesIllegal) {
				t.Fatalf("unexpected cycles for line exception: got %d want %d", cpu.Cycles(), exceptionCyclesIllegal)
			}
		})
	}
}

func TestPrivilegeViolationUsesExceptionCycles(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.SR &^= srSupervisor

	if err := ram.Write(Long, uint32(XPrivViolation<<2), 0x2200); err != nil {
		t.Fatalf("failed to seed vector: %v", err)
	}
	if err := ram.Write(Word, cpu.regs.PC, 0x4e70); err != nil { // RESET
		t.Fatalf("failed to write RESET: %v", err)
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	if cpu.Cycles() != uint64(exceptionCyclesPrivilege) {
		t.Fatalf("unexpected cycles for privilege violation: got %d want %d", cpu.Cycles(), exceptionCyclesPrivilege)
	}
}

func TestInterruptAddsExceptionCycles(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.SR = srSupervisor
	if err := ram.Write(Long, uint32((autoVectorBase+2)<<2), 0x2200); err != nil {
		t.Fatalf("failed to seed vector: %v", err)
	}
	if err := ram.Write(Word, cpu.regs.PC, 0x4e71); err != nil { // NOP
		t.Fatalf("failed to write NOP: %v", err)
	}
	if err := ram.Write(Word, 0x2200, 0x4e71); err != nil {
		t.Fatalf("failed to write handler NOP: %v", err)
	}
	if err := cpu.RequestInterrupt(2, AutoVector); err != nil {
		t.Fatalf("failed to request interrupt: %v", err)
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	expected := uint64(4 + exceptionCyclesInterrupt)
	if cpu.Cycles() != expected {
		t.Fatalf("unexpected cycles with interrupt: got %d want %d", cpu.Cycles(), expected)
	}
}

func TestRunCycles(t *testing.T) {
	cpu, ram := newEnvironment(t)

	code := assemble(t, "MOVEQ #1,D0\nNOP\nNOP")
	for i, b := range code {
		addr := cpu.regs.PC + uint32(i)
		if err := ram.Write(Byte, addr, uint32(b)); err != nil {
			t.Fatalf("failed to seed program byte at %04x: %v", addr, err)
		}
	}

	if err := cpu.RunCycles(6); err != nil {
		t.Fatalf("RunCycles failed: %v", err)
	}

	if cpu.Cycles() != 8 {
		t.Fatalf("unexpected cycle count after RunCycles: got %d want 8", cpu.Cycles())
	}

	if cpu.regs.PC != 0x2004 {
		t.Fatalf("unexpected PC after RunCycles: got %04x want 2004", cpu.regs.PC)
	}
}

func TestRunCyclesDetectsStalledCycles(t *testing.T) {
	cpu, ram := newEnvironment(t)

	code := assemble(t, "NOP")
	for i, b := range code {
		addr := cpu.regs.PC + uint32(i)
		if err := ram.Write(Byte, addr, uint32(b)); err != nil {
			t.Fatalf("failed to seed program byte at %04x: %v", addr, err)
		}
	}

	const nopOpcode = uint16(0x4e71)
	// The table is shared by every 68000, so change a private copy.
	ops := *cpu.ops
	ops.cycles[nopOpcode] = 0
	cpu.ops = &ops

	if err := cpu.RunCycles(1); err == nil {
		t.Fatalf("RunCycles should fail when cycles do not advance")
	}
}

func TestEACycleTable(t *testing.T) {
	tests := []struct {
		name           string
		mode, reg      uint16
		size           Size
		expectedCycles uint32
	}{
		{"Dn", 0, 0, Byte, 0},
		{"An", 1, 0, Word, 0},
		{"(An)", 2, 3, Long, 8},
		{"-(An)", 4, 7, Word, 6},
		{"AbsoluteLong", 7, 1, Word, 12},
		{"PCIndexed", 7, 3, Long, 14},
		{"ImmediateWord", 7, 4, Word, 4},
		{"ImmediateLong", 7, 4, Long, 8},
	}

	for _, tt := range tests {
		if got := eaAccessCycles(tt.mode, tt.reg, tt.size); got != tt.expectedCycles {
			t.Fatalf("%s: unexpected cycle count: got %d want %d", tt.name, got, tt.expectedCycles)
		}
	}
}

func TestOpcodeCycleTable(t *testing.T) {
	code := assemble(t, "MOVE.L D0,(A0)\nLSL.B #1,D0\nLSL.B D1,D0\nASL.W (A0)\nABCD D0,D1")
	assertWordCycles := func(t *testing.T, word uint16, expected uint32) {
		t.Helper()
		if got := opcodesFor(M68000).cycles[word]; got != expected {
			t.Fatalf("opcode %04x: unexpected cycles got %d want %d", word, got, expected)
		}
	}

	moveOpcode := uint16(code[0])<<8 | uint16(code[1])
	assertWordCycles(t, moveOpcode, moveCycles(moveOpcode, Long))

	shiftImmediate := uint16(code[2])<<8 | uint16(code[3])
	assertWordCycles(t, shiftImmediate, shiftRegisterCycleCalculator(shiftImmediate))

	shiftRegister := uint16(code[4])<<8 | uint16(code[5])
	assertWordCycles(t, shiftRegister, 6)

	shiftMemory := uint16(code[6])<<8 | uint16(code[7])
	assertWordCycles(t, shiftMemory, shiftMemoryCycleCalculator(shiftMemory))

	abcdOpcode := uint16(code[8])<<8 | uint16(code[9])
	assertWordCycles(t, abcdOpcode, abcdCycleCalculator(abcdOpcode))
}
