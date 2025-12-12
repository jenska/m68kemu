package m68kemu

import "testing"

const exceptionFrameSize = uint32(Word + Long + Word)

func TestExceptionSwitchesToSupervisorStackFromUser(t *testing.T) {
	cpu, ram := newEnvironment(t)

	vector := uint32(XTrap + 1)
	vectorOffset := vector << 2
	handler := uint32(0x123456)
	initialSupervisorSP := cpu.regs.A[7]
	originalPC := cpu.regs.PC
	userSP := uint32(0x4000)

	cpu.regs.SR = 0x0700
	cpu.regs.USP = userSP
	cpu.regs.A[7] = cpu.regs.USP
	cpu.regs.SSP = initialSupervisorSP

	if err := ram.Write(Long, vectorOffset, handler); err != nil {
		t.Fatalf("failed to write vector: %v", err)
	}

	if err := cpu.exception(vector); err != nil {
		t.Fatalf("exception failed: %v", err)
	}

	expectedSP := initialSupervisorSP - exceptionFrameSize
	if cpu.regs.A[7] != expectedSP {
		t.Fatalf("unexpected supervisor SP: got %08x want %08x", cpu.regs.A[7], expectedSP)
	}
	if cpu.regs.USP != userSP {
		t.Fatalf("user stack pointer modified: got %08x want %08x", cpu.regs.USP, userSP)
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
	if stackedPC != originalPC {
		t.Fatalf("stacked PC mismatch: got %08x want %08x", stackedPC, originalPC)
	}

	stackedVector, err := ram.Read(Word, expectedSP+uint32(Word+Long))
	if err != nil {
		t.Fatalf("failed reading stacked vector: %v", err)
	}
	if stackedVector != uint32(vectorOffset) {
		t.Fatalf("stacked vector mismatch: got %04x want %04x", stackedVector, vectorOffset)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("PC did not jump to handler: got %08x want %08x", cpu.regs.PC, handler)
	}
	if cpu.regs.SR&srSupervisor == 0 {
		t.Fatalf("exception did not set supervisor bit; SR=%04x", cpu.regs.SR)
	}
}

func TestExceptionUsesCurrentSupervisorStack(t *testing.T) {
	cpu, ram := newEnvironment(t)

	vector := uint32(XTrap)
	vectorOffset := vector << 2
	handler := uint32(0x00abcd)
	supervisorSP := uint32(0x1800)
	originalPC := cpu.regs.PC
	userSP := uint32(0x4000)

	cpu.regs.SR = 0x2700
	cpu.regs.A[7] = supervisorSP
	cpu.regs.SSP = supervisorSP
	cpu.regs.USP = userSP

	if err := ram.Write(Long, vectorOffset, handler); err != nil {
		t.Fatalf("failed to write vector: %v", err)
	}

	if err := cpu.exception(vector); err != nil {
		t.Fatalf("exception failed: %v", err)
	}

	expectedSP := supervisorSP - exceptionFrameSize
	if cpu.regs.A[7] != expectedSP {
		t.Fatalf("unexpected supervisor SP: got %08x want %08x", cpu.regs.A[7], expectedSP)
	}
	if cpu.regs.USP != userSP {
		t.Fatalf("user stack pointer changed in supervisor exception: got %08x want %08x", cpu.regs.USP, userSP)
	}

	stackedSR, err := ram.Read(Word, expectedSP)
	if err != nil {
		t.Fatalf("failed reading stacked SR: %v", err)
	}
	if stackedSR != uint32(cpu.regs.SR) {
		t.Fatalf("stacked SR mismatch: got %04x want %04x", stackedSR, cpu.regs.SR)
	}

	stackedPC, err := ram.Read(Long, expectedSP+uint32(Word))
	if err != nil {
		t.Fatalf("failed reading stacked PC: %v", err)
	}
	if stackedPC != originalPC {
		t.Fatalf("stacked PC mismatch: got %08x want %08x", stackedPC, originalPC)
	}

	stackedVector, err := ram.Read(Word, expectedSP+uint32(Word+Long))
	if err != nil {
		t.Fatalf("failed reading stacked vector: %v", err)
	}
	if stackedVector != uint32(vectorOffset) {
		t.Fatalf("stacked vector mismatch: got %04x want %04x", stackedVector, vectorOffset)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("PC did not jump to handler: got %08x want %08x", cpu.regs.PC, handler)
	}
}

func TestExecuteInstructionTriggersIllegalVector(t *testing.T) {
	cpu, ram := newEnvironment(t)

	illegalVectorOffset := uint32(XIllegal << 2)
	handler := uint32(0x00beef)
	originalPC := cpu.regs.PC

	if err := ram.Write(Long, illegalVectorOffset, handler); err != nil {
		t.Fatalf("failed to write illegal instruction vector: %v", err)
	}
	if err := ram.Write(Word, cpu.regs.PC, 0xffff); err != nil {
		t.Fatalf("failed to write illegal opcode: %v", err)
	}

	opcode, err := cpu.fetchOpcode()
	if err != nil {
		t.Fatalf("failed to fetch opcode: %v", err)
	}

	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("illegal instruction handler returned error: %v", err)
	}

	expectedSP := cpu.regs.SSP - exceptionFrameSize
	if cpu.regs.A[7] != expectedSP {
		t.Fatalf("unexpected SP after illegal instruction: got %08x want %08x", cpu.regs.A[7], expectedSP)
	}

	stackedSR, err := ram.Read(Word, expectedSP)
	if err != nil {
		t.Fatalf("failed reading stacked SR: %v", err)
	}
	if stackedSR != 0x2700 {
		t.Fatalf("stacked SR mismatch: got %04x want %04x", stackedSR, 0x2700)
	}

	stackedPC, err := ram.Read(Long, expectedSP+uint32(Word))
	if err != nil {
		t.Fatalf("failed reading stacked PC: %v", err)
	}
	if stackedPC != originalPC+uint32(Word) {
		t.Fatalf("stacked PC mismatch: got %08x want %08x", stackedPC, originalPC+uint32(Word))
	}

	stackedVector, err := ram.Read(Word, expectedSP+uint32(Word+Long))
	if err != nil {
		t.Fatalf("failed reading stacked vector: %v", err)
	}
	if stackedVector != illegalVectorOffset {
		t.Fatalf("stacked vector mismatch: got %04x want %04x", stackedVector, illegalVectorOffset)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("PC did not jump to illegal instruction handler: got %08x want %08x", cpu.regs.PC, handler)
	}
}
