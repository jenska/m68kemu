package m68kemu

import (
	"reflect"
	"testing"
)

func TestClrAndTst(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.SR |= srExtend

	ram.Write(Long, 0x3000, 0xdeadbeef)
	code := assemble(t, "CLR.L $3000\nTST.L $3000\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("CLR failed: %v", err)
	}
	if got, _ := ram.Read(Long, 0x3000); got != 0 {
		t.Fatalf("expected memory to be cleared, got %08x", got)
	}
	if cpu.regs.SR&srZero == 0 || cpu.regs.SR&srExtend == 0 {
		t.Fatalf("expected zero set and extend preserved, SR=%04x", cpu.regs.SR)
	}

	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("TST failed: %v", err)
	}
	if cpu.regs.SR&srZero == 0 {
		t.Fatalf("expected zero flag after TST on cleared memory")
	}
}

func TestTrapStacksExceptionFrameAndJumps(t *testing.T) {
	cpu, ram := newEnvironment(t)

	// Simulate user mode and preset a distinct handler address.
	cpu.regs.SR = 0x0700
	handler := uint32(0x123456)
	vector := uint32(1)
	vectorNumber := XTrap + vector
	vectorAddress := vectorNumber << 2

	if err := ram.Write(Long, vectorAddress, handler); err != nil {
		t.Fatalf("failed to write vector: %v", err)
	}

	code := assemble(t, "TRAP #1\n")
	for i := range code {
		addr := cpu.regs.PC + uint32(i)
		if err := ram.Write(Byte, addr, uint32(code[i])); err != nil {
			t.Fatalf("failed to write byte to %04x: %v", addr, err)
		}
	}

	opcode, err := cpu.fetchOpcode()
	if err != nil {
		t.Fatalf("failed to fetch opcode: %v", err)
	}
	returnPC := cpu.regs.PC

	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("failed to execute TRAP: %v", err)
	}

	expectedSP := uint32(0x1000 - (Word + Long))
	if cpu.regs.A[7] != expectedSP {
		t.Fatalf("unexpected SP after trap: got %08x want %08x", cpu.regs.A[7], expectedSP)
	}

	stackedVector, err := ram.Read(Word, expectedSP)
	if err != nil {
		t.Fatalf("failed reading stacked vector: %v", err)
	}
	if stackedVector != 0x0700 {
		t.Fatalf("stacked SR mismatch: got %04x want %04x", stackedVector, 0x0700)
	}

	stackedPC, err := ram.Read(Long, expectedSP+uint32(Word))
	if err != nil {
		t.Fatalf("failed reading stacked PC: %v", err)
	}
	if stackedPC != returnPC {
		t.Fatalf("stacked PC mismatch: got %08x want %08x", stackedPC, returnPC)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("PC did not jump to handler: got %08x want %08x", cpu.regs.PC, handler)
	}
	if cpu.regs.SR&srSupervisor == 0 {
		t.Fatalf("trap did not set supervisor bit; SR=%04x", cpu.regs.SR)
	}
}

func TestExecuteInstructionTrapUsesSharedSynchronousExceptionFrame(t *testing.T) {
	cpu, ram := newEnvironment(t)

	cpu.regs.SR = 0x0700
	cpu.regs.USP = 0x4000
	cpu.regs.A[7] = cpu.regs.USP
	cpu.regs.SSP = 0x1800

	handler := uint32(0x654321)
	vectorNumber := uint32(XTrap + 5)
	if err := ram.Write(Long, vectorNumber<<2, handler); err != nil {
		t.Fatalf("failed to write vector: %v", err)
	}

	startPC := cpu.regs.PC
	if err := cpu.executeInstruction(0x4e45); err != nil {
		t.Fatalf("failed to execute TRAP directly: %v", err)
	}

	expectedSP := uint32(0x1800 - exceptionFrameSize)
	if cpu.regs.A[7] != expectedSP {
		t.Fatalf("unexpected SP after direct trap: got %08x want %08x", cpu.regs.A[7], expectedSP)
	}

	stackedSR, err := ram.Read(Word, expectedSP)
	if err != nil {
		t.Fatalf("failed reading stacked SR: %v", err)
	}
	if stackedSR != 0x0700 {
		t.Fatalf("stacked SR mismatch: got %04x want %04x", stackedSR, 0x0700)
	}

	stackedPC, err := ram.Read(Long, expectedSP+uint32(Word))
	if err != nil {
		t.Fatalf("failed reading stacked PC: %v", err)
	}
	if want := startPC + uint32(Word); stackedPC != want {
		t.Fatalf("stacked PC mismatch: got %08x want %08x", stackedPC, want)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("PC did not jump to handler: got %08x want %08x", cpu.regs.PC, handler)
	}
	if cpu.regs.SR&srSupervisor == 0 {
		t.Fatalf("trap did not set supervisor bit; SR=%04x", cpu.regs.SR)
	}
	if cpu.regs.USP != 0x4000 {
		t.Fatalf("USP changed after trap: got %08x want %08x", cpu.regs.USP, uint32(0x4000))
	}

	state := cpu.DebugState()
	if !state.HasException || state.LastException.Vector != vectorNumber {
		t.Fatalf("expected trap exception state, got %+v", state.LastException)
	}
	want := startPC + uint32(Word)
	if state.LastException.PC != want {
		t.Fatalf("exception PC mismatch: got %08x want %08x", state.LastException.PC, want)
	}
	if state.LastException.Frame.PC != want {
		t.Fatalf("frame PC mismatch: got %08x want %08x", state.LastException.Frame.PC, want)
	}
}

func TestNopAdvancesPCAndLeavesState(t *testing.T) {
	cpu, ram := newEnvironment(t)

	opcode := uint16(0x4e71)
	if err := ram.Write(Word, cpu.regs.PC, uint32(opcode)); err != nil {
		t.Fatalf("failed to write NOP opcode: %v", err)
	}

	initialPC := cpu.regs.PC
	initialSR := cpu.regs.SR
	initialD := cpu.regs.D
	initialA := cpu.regs.A

	if err := cpu.Step(); err != nil {
		t.Fatalf("failed to execute NOP: %v", err)
	}

	if cpu.regs.PC != initialPC+uint32(Word) {
		t.Fatalf("PC not advanced after NOP: got %08x want %08x", cpu.regs.PC, initialPC+uint32(Word))
	}

	if cpu.regs.SR != initialSR {
		t.Fatalf("SR changed after NOP: got %04x want %04x", cpu.regs.SR, initialSR)
	}

	if cpu.regs.D != initialD {
		t.Fatalf("data registers changed after NOP: got %+v want %+v", cpu.regs.D, initialD)
	}

	if cpu.regs.A != initialA {
		t.Fatalf("address registers changed after NOP: got %+v want %+v", cpu.regs.A, initialA)
	}
}

func TestNegExtSwapExg(t *testing.T) {
	cpu, ram := newEnvironment(t)

	// Prepare registers.
	cpu.regs.D[0] = 1
	cpu.regs.D[1] = 0x00000080
	cpu.regs.D[2] = 0x12345678
	cpu.regs.D[3] = -0x55555556
	cpu.regs.D[4] = -0x44444445
	cpu.regs.A[0] = 0x11111111

	// Program: NEG.B D0; EXT.W D1; EXT.L D1; SWAP D2; EXG D3,D4; EXG D0,A0
	program := []byte{
		0x44, 0x00, // NEG.B D0
		0x48, 0x81, // EXT.W D1
		0x48, 0xC1, // EXT.L D1
		0x48, 0x42, // SWAP D2
		0xC9, 0x43, // EXG D3,D4
		0xC1, 0x88, // EXG D0,A0
	}

	for i, b := range program {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to load program: %v", err)
		}
	}

	step := func() uint16 {
		opcode, err := cpu.fetchOpcode()
		if err != nil {
			t.Fatalf("failed to fetch opcode: %v", err)
		}
		if err := cpu.executeInstruction(opcode); err != nil {
			t.Fatalf("execution failed for %04x: %v", opcode, err)
		}
		return opcode
	}

	step()
	if cpu.regs.D[0]&0xff != 0xff {
		t.Fatalf("NEG.B did not produce expected result, got %02x", cpu.regs.D[0]&0xff)
	}
	if cpu.regs.SR&(srCarry|srExtend) == 0 {
		t.Fatalf("NEG.B should set carry/extend flags")
	}

	step()
	if cpu.regs.D[1]&0xffff != 0xff80 {
		t.Fatalf("EXT.W did not sign-extend byte, got %04x", cpu.regs.D[1]&0xffff)
	}
	if cpu.regs.SR&srNegative == 0 || cpu.regs.SR&(srOverflow|srCarry) != 0 {
		t.Fatalf("EXT.W flags incorrect: %04x", cpu.regs.SR)
	}

	step()
	if cpu.regs.D[1] != -0x80 {
		t.Fatalf("EXT.L did not sign-extend word, got %08x", cpu.regs.D[1])
	}

	step()
	if cpu.regs.D[2] != 0x56781234 {
		t.Fatalf("SWAP did not swap words, got %08x", cpu.regs.D[2])
	}
	if cpu.regs.SR&(srOverflow|srCarry) != 0 {
		t.Fatalf("SWAP should clear V and C, SR=%04x", cpu.regs.SR)
	}
	if cpu.regs.SR&srExtend == 0 {
		t.Fatalf("SWAP should preserve extend flag")
	}

	handler := opcodeTable[0xc943]
	if handler == nil {
		t.Fatalf("EXG Dx,Dy opcode not registered")
	}
	if reflect.ValueOf(handler).Pointer() != reflect.ValueOf(exgInstruction).Pointer() {
		t.Fatalf("EXG opcode mapped to unexpected handler")
	}
	if opcode := step(); opcode != 0xc943 {
		t.Fatalf("expected EXG Dx,Dy opcode, got %04x", opcode)
	}
	if cpu.regs.D[3] != -0x44444445 || cpu.regs.D[4] != -0x55555556 {
		t.Fatalf("EXG Dx,Dy failed: D3=%08x D4=%08x", cpu.regs.D[3], cpu.regs.D[4])
	}

	step()
	if cpu.regs.D[0] != 0x11111111 || cpu.regs.A[0] != 0x000000ff {
		t.Fatalf("EXG Dx,Ay failed: D0=%08x A0=%08x", cpu.regs.D[0], cpu.regs.A[0])
	}
}

func TestTrapvResetAndStop(t *testing.T) {
	cpu, ram := newEnvironment(t)

	ram.Write(Long, 7<<2, 0x2222)
	ram.Write(Long, (autoVectorBase+2)<<2, 0x2008)

	code := assemble(t, "TRAPV\nRESET\nSTOP #$2000\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	cpu.regs.SR |= srOverflow
	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("TRAPV failed: %v", err)
	}
	if cpu.regs.PC != 0x2222 {
		t.Fatalf("expected TRAPV to vector to 0x2222, PC=%04x", cpu.regs.PC)
	}

	// Execute RESET
	cpu.regs.PC = 0x2002
	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("RESET failed: %v", err)
	}
	if cpu.regs.SR != 0x2702 {
		t.Fatalf("RESET should preserve SR bits, got %04x", cpu.regs.SR)
	}
	if val, _ := ram.Read(Word, 0x2004); val != 0x4e72 {
		t.Fatalf("RESET should not clear guest RAM, STOP opcode missing: %04x", val)
	}
	handler := assemble(t, "NOP")
	for i, b := range handler {
		ram.Write(Byte, 0x2008+uint32(i), uint32(b))
	}

	// Execute STOP and resume via interrupt
	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("STOP failed: %v", err)
	}
	if !cpu.stopped {
		t.Fatalf("CPU should be stopped")
	}

	if err := cpu.RequestInterrupt(2, nil); err != nil {
		t.Fatalf("failed to request interrupt: %v", err)
	}
	if err := cpu.Step(); err != nil {
		t.Fatalf("failed to service interrupt: %v", err)
	}
	if cpu.stopped {
		t.Fatalf("CPU should resume after interrupt")
	}
	if cpu.regs.PC != 0x200a {
		t.Fatalf("expected autovector handler to execute, PC=%04x", cpu.regs.PC)
	}
}

func TestStopInterruptRunsHandlerInstruction(t *testing.T) {
	cpu, ram := newEnvironment(t)

	handlerAddr := uint32(0x2010)
	ram.Write(Long, (autoVectorBase+2)<<2, handlerAddr)

	code := assemble(t, "STOP #$2000\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	handler := assemble(t, "MOVEQ #1,D0\n")
	for i, b := range handler {
		ram.Write(Byte, handlerAddr+uint32(i), uint32(b))
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("STOP failed: %v", err)
	}
	if !cpu.stopped {
		t.Fatalf("CPU should be stopped")
	}

	if err := cpu.RequestInterrupt(2, nil); err != nil {
		t.Fatalf("failed to request interrupt: %v", err)
	}
	if err := cpu.Step(); err != nil {
		t.Fatalf("failed to resume from STOP: %v", err)
	}
	if cpu.stopped {
		t.Fatalf("CPU should resume after interrupt")
	}
	if cpu.regs.D[0] != 1 {
		t.Fatalf("expected handler to execute MOVEQ, got D0=%d", cpu.regs.D[0])
	}
	if cpu.regs.PC != handlerAddr+uint32(Word) {
		t.Fatalf("expected PC to advance past handler, got %04x", cpu.regs.PC)
	}
}

func TestRtrRestoresPcAndCcr(t *testing.T) {
	cpu, ram := newEnvironment(t)

	cpu.regs.SR = srSupervisor | 0x0700
	stackBase := cpu.regs.A[7]

	returnCCR := uint16(0x0033)
	returnPC := uint32(0x004000)

	ram.Write(Word, stackBase, uint32(returnCCR))
	ram.Write(Long, stackBase+uint32(Word), returnPC)

	code := assemble(t, "RTR")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("RTR failed: %v", err)
	}
	if cpu.regs.PC != returnPC {
		t.Fatalf("RTR should restore PC, got %08x want %08x", cpu.regs.PC, returnPC)
	}
	expectedSR := uint16(0x2733)
	if cpu.regs.SR != expectedSR {
		t.Fatalf("RTR should restore CCR and preserve upper SR bits, got %04x want %04x", cpu.regs.SR, expectedSR)
	}
	if cpu.regs.A[7] != stackBase+uint32(Word+Long) {
		t.Fatalf("RTR should advance SP, got %04x want %04x", cpu.regs.A[7], stackBase+uint32(Word+Long))
	}
}

func TestTas(t *testing.T) {
	cpu, ram := newEnvironment(t)

	cpu.regs.A[0] = 0x4000
	if err := ram.Write(Byte, cpu.regs.A[0], 0x00); err != nil {
		t.Fatalf("failed to seed memory: %v", err)
	}

	cpu.regs.SR = srExtend

	code := assemble(t, "TAS (A0)")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program: %v", err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("TAS execution failed: %v", err)
	}

	value, err := ram.Read(Byte, cpu.regs.A[0])
	if err != nil {
		t.Fatalf("failed to read TAS result: %v", err)
	}

	if value != 0x80 {
		t.Fatalf("TAS did not set high bit, got %02x", value)
	}
	if cpu.regs.SR&srZero == 0 || cpu.regs.SR&srNegative != 0 || cpu.regs.SR&srExtend == 0 {
		t.Fatalf("TAS flags incorrect: SR=%04x", cpu.regs.SR)
	}
}

func TestIllegalInstructionTriggersException(t *testing.T) {
	cpu, ram := newEnvironment(t)

	handler := uint32(0xbeef)
	if err := ram.Write(Long, uint32(XIllegal<<2), handler); err != nil {
		t.Fatalf("vector write: %v", err)
	}

	code := assemble(t, "ILLEGAL\n")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	opcode, err := cpu.fetchOpcode()
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("exec: %v", err)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("expected PC to jump to handler %08x got %08x", handler, cpu.regs.PC)
	}
}

func TestLogicalImmediateCcrAndSr(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.SR = 0x0000

	addr := cpu.regs.PC
	ram.Write(Word, addr, 0x003c)
	ram.Write(Word, addr+uint32(Word), 0x0012)
	ram.Write(Word, addr+uint32(2*Word), 0x023c)
	ram.Write(Word, addr+uint32(3*Word), 0x007f)
	ram.Write(Word, addr+uint32(4*Word), 0x0a3c)
	ram.Write(Word, addr+uint32(5*Word), 0x003f)
	cpu.regs.PC = addr

	for i := range 3 {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d failed: %v", i, err)
		}
	}

	expected := uint16(0x002d)
	if cpu.regs.SR != expected {
		t.Fatalf("unexpected CCR value: got %04x want %04x", cpu.regs.SR, expected)
	}

	cpu.regs.PC = 0x3000
	cpu.regs.SR = 0
	ram.Write(Long, XPrivViolation<<2, 0x4000)

	privileged := assemble(t, "ORI #$700,SR")
	for i, b := range privileged {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("privileged ORI failed: %v", err)
	}
	if cpu.regs.PC != 0x4000 {
		t.Fatalf("privilege violation should vector to handler, PC=%04x", cpu.regs.PC)
	}
}

func TestMoveToSrAndCcr(t *testing.T) {
	cpu, ram := newEnvironment(t)

	cpu.regs.SR = srSupervisor
	originalSSP := cpu.regs.A[7]
	cpu.regs.SSP = originalSSP
	cpu.regs.USP = 0x5000

	addr := cpu.regs.PC
	ram.Write(Word, addr, 0x46fc) // MOVE #<data>,SR
	ram.Write(Word, addr+uint32(Word), 0x0000)
	ram.Write(Word, addr+uint32(2*Word), 0x44fc) // MOVE #<data>,CCR
	ram.Write(Word, addr+uint32(3*Word), 0x000f)
	cpu.regs.PC = addr

	if err := cpu.Step(); err != nil {
		t.Fatalf("MOVE to SR failed: %v", err)
	}
	if cpu.regs.SR != 0x0000 {
		t.Fatalf("MOVE to SR should clear SR, got %04x", cpu.regs.SR)
	}
	if cpu.regs.A[7] != cpu.regs.USP {
		t.Fatalf("MOVE to SR should switch to USP, SP=%04x USP=%04x", cpu.regs.A[7], cpu.regs.USP)
	}
	if cpu.regs.SSP != originalSSP {
		t.Fatalf("MOVE to SR should preserve SSP, got %04x want %04x", cpu.regs.SSP, originalSSP)
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("MOVE to CCR failed: %v", err)
	}
	expected := uint16(0x000f)
	if cpu.regs.SR != expected {
		t.Fatalf("MOVE to CCR should update low byte only, got %04x want %04x", cpu.regs.SR, expected)
	}
}

func TestSrInstructionsSwitchToUspWhenSupervisorBitClears(t *testing.T) {
	tests := []struct {
		name       string
		program    string
		expectedSR uint16
		expectStop bool
	}{
		{name: "AndiToSr", program: "ANDI #$DFFF,SR", expectedSR: 0x0000},
		{name: "EoriToSr", program: "EORI #$2000,SR", expectedSR: 0x0000},
		{name: "Stop", program: "STOP #$0000", expectedSR: 0x0000, expectStop: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)

			originalSSP := cpu.regs.A[7]
			cpu.regs.SR = srSupervisor
			cpu.regs.SSP = originalSSP
			cpu.regs.USP = 0x5000

			code := assemble(t, tt.program)
			for i, b := range code {
				if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
					t.Fatalf("write program: %v", err)
				}
			}

			if err := cpu.Step(); err != nil {
				t.Fatalf("%s failed: %v", tt.name, err)
			}

			if cpu.regs.SR != tt.expectedSR {
				t.Fatalf("SR=%04x, want %04x", cpu.regs.SR, tt.expectedSR)
			}
			if cpu.regs.A[7] != cpu.regs.USP {
				t.Fatalf("SP=%04x, want USP=%04x after %s", cpu.regs.A[7], cpu.regs.USP, tt.name)
			}
			if cpu.regs.SSP != originalSSP {
				t.Fatalf("SSP=%04x, want %04x after %s", cpu.regs.SSP, originalSSP, tt.name)
			}
			if cpu.stopped != tt.expectStop {
				t.Fatalf("stopped=%v, want %v", cpu.stopped, tt.expectStop)
			}
		})
	}
}

func TestMoveFromSr(t *testing.T) {
	cpu, ram := newEnvironment(t)

	cpu.regs.SR = 0xa7f0
	code := assemble(t, "MOVE SR,D2")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("MOVE SR,D2 failed: %v", err)
	}
	if cpu.regs.D[2]&0xffff != 0xa7f0 {
		t.Fatalf("MOVE SR,D2 should copy SR, got %08x", cpu.regs.D[2])
	}
}

func TestRteRestoresExceptionFrame(t *testing.T) {
	cpu, ram := newEnvironment(t)

	cpu.regs.SR = srSupervisor | 0x0700
	cpu.regs.SSP = cpu.regs.A[7]
	cpu.regs.USP = 0x6000
	stackBase := uint32(0x3000)
	cpu.regs.A[7] = stackBase

	returnSR := uint16(0x0000)
	returnPC := uint32(0x002000)

	ram.Write(Word, stackBase, uint32(returnSR))
	ram.Write(Long, stackBase+uint32(Word), returnPC)

	code := assemble(t, "RTE")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("RTE failed: %v", err)
	}
	if cpu.regs.PC != returnPC {
		t.Fatalf("RTE should restore PC, got %08x want %08x", cpu.regs.PC, returnPC)
	}
	if cpu.regs.SR != returnSR {
		t.Fatalf("RTE should restore SR, got %04x want %04x", cpu.regs.SR, returnSR)
	}
	if cpu.regs.SSP != stackBase+exceptionFrameSize {
		t.Fatalf("RTE should advance SSP, got %04x want %04x", cpu.regs.SSP, stackBase+exceptionFrameSize)
	}
	if cpu.regs.A[7] != cpu.regs.USP {
		t.Fatalf("RTE should switch to USP after clearing S bit, SP=%04x USP=%04x", cpu.regs.A[7], cpu.regs.USP)
	}
}

func TestMovecProbesTrapAsIllegalOn68000(t *testing.T) {
	tests := []struct {
		name   string
		opcode uint16
	}{
		{name: "MoveFromControl", opcode: 0x4e7a},
		{name: "MoveToControl", opcode: 0x4e7b},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			illegalHandler := uint32(0x4000)
			privHandler := uint32(0x5000)
			startPC := cpu.regs.PC

			cpu.regs.SR &^= srSupervisor

			if err := ram.Write(Long, uint32(XIllegal<<2), illegalHandler); err != nil {
				t.Fatalf("failed to seed illegal vector: %v", err)
			}
			if err := ram.Write(Long, uint32(XPrivViolation<<2), privHandler); err != nil {
				t.Fatalf("failed to seed privilege vector: %v", err)
			}

			if err := ram.Write(Word, startPC, uint32(tt.opcode)); err != nil {
				t.Fatalf("failed to write MOVEC opcode: %v", err)
			}
			if err := ram.Write(Word, startPC+uint32(Word), 0x0001); err != nil {
				t.Fatalf("failed to write MOVEC extension: %v", err)
			}

			if err := cpu.Step(); err != nil {
				t.Fatalf("MOVEC probe failed: %v", err)
			}

			if cpu.regs.PC != illegalHandler {
				t.Fatalf("MOVEC probe should trap to illegal handler, PC=%08x want %08x", cpu.regs.PC, illegalHandler)
			}

			expectedSP := cpu.regs.SSP - exceptionFrameSize
			if cpu.regs.A[7] != expectedSP {
				t.Fatalf("unexpected SP after MOVEC probe: got %08x want %08x", cpu.regs.A[7], expectedSP)
			}

			stackedPC, err := ram.Read(Long, expectedSP+uint32(Word))
			if err != nil {
				t.Fatalf("failed to read stacked PC: %v", err)
			}
			if stackedPC != startPC+uint32(Word) {
				t.Fatalf("MOVEC probe should fault before extension word: stacked PC=%08x want %08x", stackedPC, startPC+uint32(Word))
			}

			if cpu.Cycles() != uint64(exceptionCyclesIllegal) {
				t.Fatalf("unexpected cycles for MOVEC probe: got %d want %d", cpu.Cycles(), exceptionCyclesIllegal)
			}

			if cpu.regs.PC == privHandler {
				t.Fatalf("MOVEC probe should not raise privilege violation")
			}
		})
	}
}
