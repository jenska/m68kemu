package m68kemu

import "testing"

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

	for i := 0; i < 3; i++ {
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
