package m68kemu

import "testing"

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

	if cpu.Cycles() != 8 {
		t.Fatalf("unexpected cycles for MOVE.L D0,(A0): got %d want 8", cpu.Cycles())
	}
}

func TestCycleCounterWaitStates(t *testing.T) {
	ram := NewRAM(0, 0x1000)
	bus := NewBus(&ram)
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

	expected := uint64(opcodeCycleTable[opcode])
	if cpu.Cycles() != expected {
		t.Fatalf("unexpected cycles after executeInstruction: got %d want %d", cpu.Cycles(), expected)
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
	originalCycles := opcodeCycleTable[nopOpcode]
	opcodeCycleTable[nopOpcode] = 0
	defer func() { opcodeCycleTable[nopOpcode] = originalCycles }()

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
		{"(An)", 2, 3, Long, 4},
		{"-(An)", 4, 7, Word, 6},
		{"AbsoluteLong", 7, 1, Word, 12},
		{"PCIndexed", 7, 3, Long, 10},
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
		if got := opcodeCycleTable[word]; got != expected {
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
