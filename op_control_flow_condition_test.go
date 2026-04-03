package m68kemu

import "testing"

func TestSccSetsByteOnCondition(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.SR = srZero

	code := assemble(t, "SEQ D0\n")
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

	if got := cpu.regs.D[0] & 0xff; got != 0xff {
		t.Fatalf("expected 0xff got %02x", got)
	}

	cpu.regs.SR &^= srZero
	cpu.regs.PC = 0x2000
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	opcode, err = cpu.fetchOpcode()
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("exec: %v", err)
	}

	if got := cpu.regs.D[0] & 0xff; got != 0x00 {
		t.Fatalf("expected 0x00 got %02x", got)
	}
}

func TestDbccLoop(t *testing.T) {
	cpu, ram := newEnvironment(t)

	code := []byte{
		0x70, 0x01, // MOVEQ #1,D0
		0x51, 0xC8, 0xFF, 0xFE, // DBRA D0,-2
		0x4E, 0x71, // NOP
	}

	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program: %v", err)
		}
	}

	endPC := cpu.regs.PC + uint32(len(code))
	for steps := 0; steps < 10 && cpu.regs.PC < endPC; steps++ {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step failed: %v", err)
		}
	}

	if cpu.regs.PC != endPC {
		t.Fatalf("DBF loop did not fall through to NOP, PC=%04x end=%04x", cpu.regs.PC, endPC)
	}
	if cpu.regs.D[0]&0xffff != 0xffff {
		t.Fatalf("DBRA should leave low word at -1, got %04x", cpu.regs.D[0]&0xffff)
	}
}
