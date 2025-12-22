package m68kemu

import "testing"

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
