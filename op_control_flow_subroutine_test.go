package m68kemu

import "testing"

func TestJsrRts(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC

	program := assemble(t, `
        BRA.S main
inc:    ADD.L D1,D0
        RTS
main:   MOVEQ #1,D0
        MOVEQ #1,D1
        BSR.S inc
        NOP
`)

	for i, b := range program {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	endPC := startPC + uint32(len(program))
	for cpu.regs.PC < endPC {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step: %v", err)
		}
	}

	if cpu.regs.D[0] != 2 {
		t.Fatalf("D0=%d, want 2", cpu.regs.D[0])
	}
}

func TestJsrPushesReturnAddressAndJumps(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC

	program := []byte{
		0x4E, 0xB9, 0x00, 0x00, 0x20, 0x08, // JSR $00002008
		0x4E, 0x71, // NOP (return address)
		0x4E, 0x75, // RTS
	}
	for i, b := range program {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("step jsr: %v", err)
	}

	if cpu.regs.PC != startPC+8 {
		t.Fatalf("PC after JSR = %08x, want %08x", cpu.regs.PC, startPC+8)
	}

	if cpu.regs.A[7] != 0x0ffc {
		t.Fatalf("SP after JSR = %08x, want 00000ffc", cpu.regs.A[7])
	}

	returnAddr, err := ram.Read(Long, cpu.regs.A[7])
	if err != nil {
		t.Fatalf("read return address: %v", err)
	}
	if returnAddr != startPC+6 {
		t.Fatalf("stacked return address = %08x, want %08x", returnAddr, startPC+6)
	}
}

func TestBSRWordUsesExtensionWordAddressAsBase(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC

	program := []byte{
		0x61, 0x00, 0x00, 0x06, // BSR.W to 0x2008 when based on the extension word address
		0x4E, 0x71, // NOP (return address)
		0x4E, 0x71, // filler
		0x4E, 0x75, // RTS
	}
	for i, b := range program {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("step bsr.w: %v", err)
	}

	if cpu.regs.PC != startPC+8 {
		t.Fatalf("PC=%08x, want %08x", cpu.regs.PC, startPC+8)
	}
	if cpu.regs.A[7] != 0x0ffc {
		t.Fatalf("SP=%08x, want 00000ffc", cpu.regs.A[7])
	}
	returnAddr, err := ram.Read(Long, cpu.regs.A[7])
	if err != nil {
		t.Fatalf("read return address: %v", err)
	}
	if returnAddr != startPC+4 {
		t.Fatalf("return address=%08x, want %08x", returnAddr, startPC+4)
	}
}

func TestDBRAWordUsesExtensionWordAddressAsBase(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC
	cpu.regs.D[0] = 1

	program := []byte{
		0x51, 0xC8, 0xFF, 0xFE, // DBRA D0,-2 -> branch back to the opcode when based on the extension word address
		0x4E, 0x71, // NOP
	}
	for i, b := range program {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("step dbra 1: %v", err)
	}
	if cpu.regs.PC != startPC {
		t.Fatalf("PC after first DBRA=%08x, want %08x", cpu.regs.PC, startPC)
	}
	if cpu.regs.D[0]&0xffff != 0 {
		t.Fatalf("D0 low word after first DBRA=%04x, want 0000", cpu.regs.D[0]&0xffff)
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("step dbra 2: %v", err)
	}
	if cpu.regs.PC != startPC+4 {
		t.Fatalf("PC after second DBRA=%08x, want %08x", cpu.regs.PC, startPC+4)
	}
	if cpu.regs.D[0]&0xffff != 0xffff {
		t.Fatalf("D0 low word after second DBRA=%04x, want ffff", cpu.regs.D[0]&0xffff)
	}
}
