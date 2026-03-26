package m68kemu

import "testing"

const exceptionFrameSize = group12ExceptionFrameSize

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

	if cpu.regs.PC != handler {
		t.Fatalf("PC did not jump to illegal instruction handler: got %08x want %08x", cpu.regs.PC, handler)
	}
}

func TestBusErrorUsesGroup0StackFrame(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[0] = 0x900000

	handler := uint32(0x5000)
	if err := ram.Write(Long, uint32(XBusError<<2), handler); err != nil {
		t.Fatalf("failed to install bus error vector: %v", err)
	}

	code := assemble(t, "MOVE.B (A0),D0")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	startPC := cpu.regs.PC
	if err := cpu.Step(); err != nil {
		t.Fatalf("Step failed: %v", err)
	}

	expectedSP := cpu.regs.SSP - group0ExceptionFrameSize
	if cpu.regs.A[7] != expectedSP {
		t.Fatalf("unexpected SP after bus error: got %08x want %08x", cpu.regs.A[7], expectedSP)
	}

	statusWord, err := ram.Read(Word, expectedSP)
	if err != nil {
		t.Fatalf("read status word: %v", err)
	}
	if statusWord != 0x0015 {
		t.Fatalf("unexpected status word: got %04x want 0015", statusWord)
	}

	accessAddr, err := ram.Read(Long, expectedSP+2)
	if err != nil {
		t.Fatalf("read access address: %v", err)
	}
	if accessAddr != 0x900000 {
		t.Fatalf("unexpected access address: got %08x want 00900000", accessAddr)
	}

	stackedIR, err := ram.Read(Word, expectedSP+6)
	if err != nil {
		t.Fatalf("read IR: %v", err)
	}
	opcode := uint32(code[0])<<8 | uint32(code[1])
	if stackedIR != opcode {
		t.Fatalf("unexpected stacked IR: got %04x want %04x", stackedIR, opcode)
	}

	stackedSR, err := ram.Read(Word, expectedSP+8)
	if err != nil {
		t.Fatalf("read SR: %v", err)
	}
	if stackedSR != 0x2700 {
		t.Fatalf("unexpected stacked SR: got %04x want 2700", stackedSR)
	}

	stackedPC, err := ram.Read(Long, expectedSP+10)
	if err != nil {
		t.Fatalf("read PC: %v", err)
	}
	if stackedPC != startPC+uint32(Word) {
		t.Fatalf("unexpected stacked PC: got %08x want %08x", stackedPC, startPC+uint32(Word))
	}

	if cpu.regs.PC != handler {
		t.Fatalf("PC did not jump to bus error handler: got %08x want %08x", cpu.regs.PC, handler)
	}
}

func TestBusErrorOnSecondLongWriteCycleUsesFaultCycleAddress(t *testing.T) {
	system := NewRAM(0x0000, 0x4000)
	probe := NewRAM(0x800000, 0x0004)
	bus := NewBus(system, probe)

	if err := system.Write(Long, 0, 0x1000); err != nil {
		t.Fatalf("seed SSP: %v", err)
	}
	if err := system.Write(Long, 4, 0x2000); err != nil {
		t.Fatalf("seed PC: %v", err)
	}

	processor, err := NewCPU(bus)
	if err != nil {
		t.Fatalf("Failed to create CPU: %v", err)
	}
	cpu, ok := processor.(*cpu)
	if !ok {
		t.Fatalf("CPU implementation has unexpected type %T", processor)
	}

	cpu.regs.A[0] = 0x800002
	cpu.regs.D[0] = -1430532899 // 0xaabbccdd

	handler := uint32(0x3000)
	if err := system.Write(Long, uint32(XBusError<<2), handler); err != nil {
		t.Fatalf("failed to install bus error vector: %v", err)
	}

	code := assemble(t, "MOVE.L D0,(A0)")
	for i, b := range code {
		if err := system.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	startPC := cpu.regs.PC
	if err := cpu.Step(); err != nil {
		t.Fatalf("Step failed: %v", err)
	}

	expectedSP := cpu.regs.SSP - group0ExceptionFrameSize
	accessAddr, err := system.Read(Long, expectedSP+2)
	if err != nil {
		t.Fatalf("read access address: %v", err)
	}
	if accessAddr != 0x800004 {
		t.Fatalf("unexpected access address: got %08x want 00800004", accessAddr)
	}

	statusWord, err := system.Read(Word, expectedSP)
	if err != nil {
		t.Fatalf("read status word: %v", err)
	}
	if statusWord != 0x0005 {
		t.Fatalf("unexpected status word: got %04x want 0005", statusWord)
	}

	stackedPC, err := system.Read(Long, expectedSP+10)
	if err != nil {
		t.Fatalf("read PC: %v", err)
	}
	if stackedPC != startPC+uint32(Word) {
		t.Fatalf("unexpected stacked PC: got %08x want %08x", stackedPC, startPC+uint32(Word))
	}

	written, err := probe.Read(Word, 0x800002)
	if err != nil {
		t.Fatalf("read partially written probe word: %v", err)
	}
	if written != 0xaabb {
		t.Fatalf("partial write = %04x, want aabb", written)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("PC did not jump to bus error handler: got %08x want %08x", cpu.regs.PC, handler)
	}
}

func TestBusErrorHandlerCanTrimGroup0FrameBeforeRTE(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[0] = 0x900000

	handler := uint32(0x3000)
	if err := ram.Write(Long, uint32(XBusError<<2), handler); err != nil {
		t.Fatalf("failed to install bus error vector: %v", err)
	}

	program := assemble(t, "MOVE.B (A0),D0\nNOP\nNOP")
	for i, b := range program {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	handlerCode := assemble(t, "ADDQ.L #8,A7\nRTE")
	for i, b := range handlerCode {
		if err := ram.Write(Byte, handler+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write handler: %v", err)
		}
	}

	startPC := cpu.regs.PC
	initialSP := cpu.regs.SSP

	if err := cpu.Step(); err != nil {
		t.Fatalf("faulting step failed: %v", err)
	}
	if cpu.regs.PC != handler {
		t.Fatalf("PC did not jump to handler: got %08x want %08x", cpu.regs.PC, handler)
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("handler trim step failed: %v", err)
	}
	if cpu.regs.A[7] != initialSP-uint32(Word+Long) {
		t.Fatalf("handler did not trim group-0 frame to short frame: got %08x want %08x", cpu.regs.A[7], initialSP-uint32(Word+Long))
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("RTE failed: %v", err)
	}

	expectedResumePC := startPC + uint32(Word)
	if cpu.regs.PC != expectedResumePC {
		t.Fatalf("RTE resumed at %08x, want %08x", cpu.regs.PC, expectedResumePC)
	}
	if cpu.regs.A[7] != initialSP {
		t.Fatalf("stack pointer not restored after trimmed RTE: got %08x want %08x", cpu.regs.A[7], initialSP)
	}
}

func TestAddressErrorUsesGroup0StackFrame(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[0] = 0x3001

	handler := uint32(0x6000)
	if err := ram.Write(Long, uint32(XAddressError<<2), handler); err != nil {
		t.Fatalf("failed to install address error vector: %v", err)
	}

	code := assemble(t, "MOVE.W D0,(A0)")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	startPC := cpu.regs.PC
	if err := cpu.Step(); err != nil {
		t.Fatalf("Step failed: %v", err)
	}

	expectedSP := cpu.regs.SSP - group0ExceptionFrameSize
	statusWord, err := ram.Read(Word, expectedSP)
	if err != nil {
		t.Fatalf("read status word: %v", err)
	}
	if statusWord != 0x0005 {
		t.Fatalf("unexpected status word: got %04x want 0005", statusWord)
	}

	accessAddr, err := ram.Read(Long, expectedSP+2)
	if err != nil {
		t.Fatalf("read access address: %v", err)
	}
	if accessAddr != 0x3001 {
		t.Fatalf("unexpected access address: got %08x want 00003001", accessAddr)
	}

	stackedPC, err := ram.Read(Long, expectedSP+10)
	if err != nil {
		t.Fatalf("read stacked PC: %v", err)
	}
	if stackedPC != startPC+uint32(Word) {
		t.Fatalf("unexpected stacked PC: got %08x want %08x", stackedPC, startPC+uint32(Word))
	}

	if cpu.regs.PC != handler {
		t.Fatalf("PC did not jump to address error handler: got %08x want %08x", cpu.regs.PC, handler)
	}
}
