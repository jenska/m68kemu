package m68kemu

import "testing"

func TestMovemStoreAndLoad(t *testing.T) {
	cpu, ram := newEnvironment(t)

	cpu.regs.D[0] = 0x11111111
	cpu.regs.D[1] = 0x22222222
	cpu.regs.A[1] = 0x33333333
	cpu.regs.A[2] = 0x3000

	code := assemble(t, `
                MOVEM.L D0-D1/A1,-(A2)
                MOVEM.L (A2)+,D2-D3/A3
        `)

	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program: %v", err)
		}
	}

	for i := 0; i < 2; i++ {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d failed: %v", i, err)
		}
	}

	if cpu.regs.A[2] != 0x3000 {
		t.Fatalf("A2 should be restored by MOVEM pair, got %04x", cpu.regs.A[2])
	}
	if cpu.regs.D[2] != cpu.regs.D[0] || cpu.regs.D[3] != cpu.regs.D[1] || cpu.regs.A[3] != cpu.regs.A[1] {
		t.Fatalf("MOVEM load mismatch: D2=%08x D3=%08x A3=%08x", cpu.regs.D[2], cpu.regs.D[3], cpu.regs.A[3])
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

func TestMovemWordLoadsSignExtendRegisters(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[2] = 0x3000

	if err := ram.Write(Word, 0x3000, 0x8001); err != nil {
		t.Fatalf("seed D0 source: %v", err)
	}
	if err := ram.Write(Word, 0x3002, 0xfffe); err != nil {
		t.Fatalf("seed A1 source: %v", err)
	}

	code := assemble(t, "MOVEM.W (A2)+,D0/A1")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program: %v", err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("step failed: %v", err)
	}

	if cpu.regs.D[0] != -32767 {
		t.Fatalf("D0 should be sign-extended, got %08x", uint32(cpu.regs.D[0]))
	}
	if cpu.regs.A[1] != 0xfffffffe {
		t.Fatalf("A1 should be sign-extended, got %08x", cpu.regs.A[1])
	}
	if cpu.regs.A[2] != 0x3004 {
		t.Fatalf("A2 should postincrement by 4, got %08x", cpu.regs.A[2])
	}
}
