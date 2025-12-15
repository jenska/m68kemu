package m68kemu

import "testing"

func TestJmp(t *testing.T) {
	cpu, ram := newEnvironment(t)

	jumpBytes := assemble(t, "JMP $3000\n")
	for i, b := range jumpBytes {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write jump: %v", err)
		}
	}

	payload := assemble(t, "MOVEQ #2,D0\nNOP\n")
	for i, b := range payload {
		if err := ram.Write(Byte, 0x3000+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write payload: %v", err)
		}
	}

	for i := 0; i < 3; i++ {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}

	if cpu.regs.D[0] != 2 {
		t.Fatalf("D0=%d, want 2", cpu.regs.D[0])
	}
}

func TestMoveUsp(t *testing.T) {
	cpu, ram := newEnvironment(t)

	program := assemble(t, `
        MOVE.L #$4000,A0
        MOVE.L #$0,A1
        MOVE A0,USP
        MOVE USP,A1
        NOP
`)

	for i, b := range program {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	for i := 0; i < 5; i++ {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}

	if cpu.regs.USP != 0x4000 {
		t.Fatalf("USP=%04x, want 0x4000", cpu.regs.USP)
	}
	if cpu.regs.A[1] != 0x4000 {
		t.Fatalf("A1=%04x, want 0x4000", cpu.regs.A[1])
	}
}

func TestLinkUnlk(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[7] = 0x3000
	cpu.regs.A[6] = 0x11112222

	program := assemble(t, `
        LINK A6,#-4
        UNLK A6
        NOP
`)

	for i, b := range program {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	for i := 0; i < 4; i++ {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}

	if cpu.regs.A[6] != 0x11112222 {
		t.Fatalf("A6=%04x, want 0x11112222", cpu.regs.A[6])
	}
	if cpu.regs.A[7] != 0x3000 {
		t.Fatalf("A7=%04x, want 0x3000", cpu.regs.A[7])
	}

	value, err := ram.Read(Long, 0x2ffc)
	if err != nil {
		t.Fatalf("read stack: %v", err)
	}
	if value != 0x11112222 {
		t.Fatalf("stack value=%08x, want 0x11112222", value)
	}
}

func TestChk(t *testing.T) {
	cpu, ram := newEnvironment(t)

	program := assemble(t, `
        MOVEQ #5,D0
        MOVEQ #10,D1
        CHK D1,D0
        NOP
`)

	for i, b := range program {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	for i := 0; i < 3; i++ {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}

	if cpu.regs.SR&(srNegative|srCarry|srOverflow) != 0 {
		t.Fatalf("unexpected flags set: %04x", cpu.regs.SR)
	}
	if cpu.regs.SR&srZero != 0 {
		t.Fatalf("Z flag set, want clear")
	}
}

func TestChkException(t *testing.T) {
	cpu, ram := newEnvironment(t)
	if opcodeTable[0x4181] == nil {
		t.Fatalf("CHK handler not registered")
	}
	// vector 6 handler at 0x4000
	if err := ram.Write(Long, 6<<2, 0x4000); err != nil {
		t.Fatalf("write vector: %v", err)
	}

	program := assemble(t, `
        MOVEQ #-1,D0
        MOVEQ #10,D1
        CHK D1,D0
        NOP
`)

	for i, b := range program {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	if err := cpu.Step(); err != nil { // MOVEQ
		t.Fatalf("step moveq: %v", err)
	}
	if cpu.regs.D[0] != -1 {
		t.Fatalf("D0=%08x, want ffffffff", cpu.regs.D[0])
	}
	if err := cpu.Step(); err != nil { // bound load
		t.Fatalf("step bound: %v", err)
	}
	if err := cpu.Step(); err != nil { // CHK triggers exception
		t.Fatalf("step chk: %v", err)
	}

	t.Logf("after CHK PC=%04x SR=%04x", cpu.regs.PC, cpu.regs.SR)

	if cpu.regs.PC != 0x4000 {
		t.Fatalf("PC=%04x, want 0x4000", cpu.regs.PC)
	}
	if cpu.regs.SR&srSupervisor == 0 {
		t.Fatalf("SR supervisor bit not set after CHK exception")
	}
}
