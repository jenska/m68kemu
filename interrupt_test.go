package m68kemu

import "testing"

func assertStandardExceptionFrame(t *testing.T, ram *RAM, sp uint32, wantSR uint16, wantPC uint32, label string) {
	t.Helper()

	gotSR, err := ram.Read(Word, sp)
	if err != nil {
		t.Fatalf("read %s stacked SR: %v", label, err)
	}
	gotPC, err := ram.Read(Long, sp+uint32(Word))
	if err != nil {
		t.Fatalf("read %s stacked PC: %v", label, err)
	}

	if uint16(gotSR) != wantSR || gotPC != wantPC {
		t.Fatalf("%s frame mismatch: got SR=%04x PC=%08x want SR=%04x PC=%08x", label, uint16(gotSR), gotPC, wantSR, wantPC)
	}
}

func TestInterruptRespectsMaskAndTriggersWhenUnmasked(t *testing.T) {
	cpu, ram := newEnvironment(t)

	handler := uint32(0x3000)
	vector := uint32(autoVectorBase + 2)
	vectorOffset := vector << 2

	if err := ram.Write(Long, vectorOffset, handler); err != nil {
		t.Fatalf("failed to install autovector handler: %v", err)
	}

	first := assemble(t, "MOVE.B #1,D0\n")
	second := assemble(t, "MOVE.B #2,D0\n")

	startPC := cpu.regs.PC
	for i, b := range first {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write first instruction: %v", err)
		}
	}

	secondPC := startPC + uint32(len(first))
	for i, b := range second {
		if err := ram.Write(Byte, secondPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write second instruction: %v", err)
		}
	}

	cpu.setSR(srSupervisor | (3 << 8))

	if err := cpu.RequestInterrupt(2, nil); err != nil {
		t.Fatalf("failed to request interrupt: %v", err)
	}

	initialSP := cpu.regs.A[7]

	if err := cpu.Step(); err != nil {
		t.Fatalf("first step failed: %v", err)
	}

	if cpu.regs.A[7] != initialSP {
		t.Fatalf("stack pointer changed while interrupt was masked: got %08x want %08x", cpu.regs.A[7], initialSP)
	}

	if cpu.regs.PC != secondPC {
		t.Fatalf("unexpected PC after masked interrupt cycle: got %08x want %08x", cpu.regs.PC, secondPC)
	}

	cpu.setSR(srSupervisor)

	if err := cpu.Step(); err != nil {
		t.Fatalf("second step failed: %v", err)
	}

	expectedSP := initialSP - exceptionFrameSize
	if cpu.regs.A[7] != expectedSP {
		t.Fatalf("unexpected SP after unmasked interrupt: got %08x want %08x", cpu.regs.A[7], expectedSP)
	}

	stackedPC, err := ram.Read(Long, expectedSP+uint32(Word))
	if err != nil {
		t.Fatalf("failed to read stacked PC: %v", err)
	}

	expectedStackedPC := startPC + uint32(len(first)) + uint32(len(second))
	if stackedPC != expectedStackedPC {
		t.Fatalf("stacked PC mismatch: got %08x want %08x", stackedPC, expectedStackedPC)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("interrupt did not jump to handler: got %08x want %08x", cpu.regs.PC, handler)
	}

	if cpu.regs.SR&srInterruptMask != uint16(2<<8) {
		t.Fatalf("interrupt mask not raised to level: SR=%04x", cpu.regs.SR)
	}
}

func TestInterruptUsesProvidedVectorWhenAvailable(t *testing.T) {
	cpu, ram := newEnvironment(t)

	handler := uint32(0x4000)
	vector := uint32(50)
	vectorOffset := vector << 2

	if err := ram.Write(Long, vectorOffset, handler); err != nil {
		t.Fatalf("failed to install interrupt handler: %v", err)
	}

	code := assemble(t, "MOVE.B #3,D0\n")
	startPC := cpu.regs.PC
	for i, b := range code {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write instruction: %v", err)
		}
	}

	cpu.setSR(srSupervisor)

	vectorNumber := uint8(vector)
	if err := cpu.RequestInterrupt(5, &vectorNumber); err != nil {
		t.Fatalf("failed to request interrupt: %v", err)
	}

	initialSP := cpu.regs.A[7]

	if err := cpu.Step(); err != nil {
		t.Fatalf("step failed: %v", err)
	}

	expectedSP := initialSP - exceptionFrameSize
	if cpu.regs.A[7] != expectedSP {
		t.Fatalf("unexpected SP after interrupt: got %08x want %08x", cpu.regs.A[7], expectedSP)
	}

	stackedPC, err := ram.Read(Long, expectedSP+uint32(Word))
	if err != nil {
		t.Fatalf("failed to read stacked PC: %v", err)
	}

	expectedPC := startPC + uint32(len(code))
	if stackedPC != expectedPC {
		t.Fatalf("stacked PC mismatch: got %08x want %08x", stackedPC, expectedPC)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("PC did not jump to handler: got %08x want %08x", cpu.regs.PC, handler)
	}

	if cpu.regs.SR&srInterruptMask != uint16(5<<8) {
		t.Fatalf("interrupt mask not updated to level: SR=%04x", cpu.regs.SR)
	}
}

func TestInterruptControllerQueuesRequestsPerLevel(t *testing.T) {
	ic := NewInterruptController()

	firstVector := uint8(50)
	secondVector := uint8(60)

	if err := ic.Request(3, &firstVector); err != nil {
		t.Fatalf("failed to request first interrupt: %v", err)
	}
	if err := ic.Request(3, &secondVector); err != nil {
		t.Fatalf("failed to request second interrupt: %v", err)
	}

	level, vector, ok := ic.Pending(0)
	if !ok {
		t.Fatalf("expected pending interrupt")
	}
	if level != 3 || vector != uint32(firstVector) {
		t.Fatalf("unexpected first interrupt: level=%d vector=%d", level, vector)
	}

	level, vector, ok = ic.Pending(0)
	if !ok {
		t.Fatalf("expected second pending interrupt")
	}
	if level != 3 || vector != uint32(secondVector) {
		t.Fatalf("unexpected second interrupt: level=%d vector=%d", level, vector)
	}

	if _, _, ok = ic.Pending(0); ok {
		t.Fatalf("expected no further interrupts")
	}
}

func TestNestedInterruptRegressionRestoresExactSRPCAndFrames(t *testing.T) {
	cpu, ram := newEnvironment(t)

	mainProgram := assembleProgram(t, "NOP\nNOP\nNOP")
	startPC := cpu.regs.PC
	main := loadedProgram{base: startPC, assembledProgram: mainProgram}
	for i, b := range mainProgram.Bytes {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write main program: %v", err)
		}
	}

	level4Handler := uint32(0x3000)
	level6Handler := uint32(0x4000)
	if err := ram.Write(Long, uint32((autoVectorBase+4)<<2), level4Handler); err != nil {
		t.Fatalf("install level 4 handler: %v", err)
	}
	mfpVector := uint8(69)
	if err := ram.Write(Long, uint32(mfpVector)<<2, level6Handler); err != nil {
		t.Fatalf("install level 6 handler: %v", err)
	}

	level4Program := assembleProgram(t, "NOP\nNOP\nNOP\nRTE")
	level4 := loadedProgram{base: level4Handler, assembledProgram: level4Program}
	for i, b := range level4Program.Bytes {
		if err := ram.Write(Byte, level4Handler+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write level 4 handler: %v", err)
		}
	}
	level6Program := assembleProgram(t, "NOP\nRTE")
	for i, b := range level6Program.Bytes {
		if err := ram.Write(Byte, level6Handler+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write level 6 handler: %v", err)
		}
	}

	initialSP := cpu.regs.A[7]
	initialSR := uint16(srSupervisor | srExtend | srOverflow | srCarry)
	level4SR := uint16(initialSR | (4 << 8))
	level6SR := uint16(initialSR | (6 << 8))
	mainResumePC := main.PCForLine(t, 2)
	level4ResumePC := level4.PCForLine(t, 2)

	cpu.setSR(initialSR)

	if err := cpu.RequestInterrupt(4, nil); err != nil {
		t.Fatalf("request level 4 interrupt: %v", err)
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("step into level 4 handler: %v", err)
	}
	if cpu.regs.PC != level4Handler {
		t.Fatalf("expected entry into level 4 handler, got PC=%08x", cpu.regs.PC)
	}
	outerFrameSP := initialSP - exceptionFrameSize
	if cpu.regs.A[7] != outerFrameSP {
		t.Fatalf("unexpected SP after level 4 interrupt: got %08x want %08x", cpu.regs.A[7], outerFrameSP)
	}
	if cpu.regs.SR != level4SR {
		t.Fatalf("unexpected SR on level 4 entry: got %04x want %04x", cpu.regs.SR, level4SR)
	}
	assertStandardExceptionFrame(t, ram, outerFrameSP, initialSR, mainResumePC, "outer interrupt")

	if err := cpu.RequestInterrupt(6, &mfpVector); err != nil {
		t.Fatalf("request level 6 interrupt: %v", err)
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("step should execute first level 4 instruction and preempt to level 6: %v", err)
	}
	if cpu.regs.PC != level6Handler {
		t.Fatalf("expected preemption into level 6 handler, got PC=%08x", cpu.regs.PC)
	}
	innerFrameSP := initialSP - 2*exceptionFrameSize
	if cpu.regs.A[7] != innerFrameSP {
		t.Fatalf("unexpected SP after nested interrupt: got %08x want %08x", cpu.regs.A[7], innerFrameSP)
	}
	if cpu.regs.SR != level6SR {
		t.Fatalf("unexpected SR on level 6 entry: got %04x want %04x", cpu.regs.SR, level6SR)
	}
	assertStandardExceptionFrame(t, ram, outerFrameSP, initialSR, mainResumePC, "outer interrupt after nesting")
	assertStandardExceptionFrame(t, ram, innerFrameSP, level4SR, level4ResumePC, "inner interrupt")

	if err := cpu.Step(); err != nil {
		t.Fatalf("execute level 6 handler body: %v", err)
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("return from level 6 handler: %v", err)
	}
	if cpu.regs.PC != level4ResumePC {
		t.Fatalf("RTE from level 6 should resume level 4 handler, got PC=%08x want %08x", cpu.regs.PC, level4ResumePC)
	}
	if cpu.regs.SR != level4SR {
		t.Fatalf("RTE from level 6 restored wrong SR: got %04x want %04x", cpu.regs.SR, level4SR)
	}
	if cpu.regs.A[7] != outerFrameSP {
		t.Fatalf("stack pointer after level 6 RTE = %08x, want %08x", cpu.regs.A[7], outerFrameSP)
	}
	assertStandardExceptionFrame(t, ram, outerFrameSP, initialSR, mainResumePC, "outer interrupt after level 6 RTE")

	if err := cpu.Step(); err != nil {
		t.Fatalf("execute level 4 NOP 1: %v", err)
	}
	if err := cpu.Step(); err != nil {
		t.Fatalf("execute level 4 NOP 2: %v", err)
	}
	if err := cpu.Step(); err != nil {
		t.Fatalf("return from level 4 handler: %v", err)
	}

	if cpu.regs.PC != mainResumePC {
		t.Fatalf("RTE from level 4 should resume main loop, got PC=%08x want %08x", cpu.regs.PC, mainResumePC)
	}
	if cpu.regs.SR != initialSR {
		t.Fatalf("RTE from level 4 restored wrong SR: got %04x want %04x", cpu.regs.SR, initialSR)
	}
	if cpu.regs.A[7] != initialSP {
		t.Fatalf("stack pointer not restored after nested interrupts: got %08x want %08x", cpu.regs.A[7], initialSP)
	}
}
