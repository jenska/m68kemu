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

func TestMovemLongStoresSequentialWordsForControlMode(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[0] = 0x3000
	cpu.regs.A[1] = 0x4000

	values := []uint32{
		0x000000ff,
		0x000d000b,
		0x00080002,
		0x00020007,
		0x00080001,
		0x00070001,
		0x00015555,
		0x5555000d,
	}
	for i, value := range values {
		if err := ram.Write(Long, cpu.regs.A[0]+uint32(i*4), value); err != nil {
			t.Fatalf("seed memory %d: %v", i, err)
		}
	}

	code := assemble(t, `
		MOVEM.L (A0)+,D2-D7/A4-A5
		MOVEM.L D2-D7/A4-A5,(A1)
	`)
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program: %v", err)
		}
	}

	for step := 0; step < 2; step++ {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d failed: %v", step, err)
		}
	}

	for i, want := range values {
		got, err := ram.Read(Long, cpu.regs.A[1]+uint32(i*4))
		if err != nil {
			t.Fatalf("read back %d: %v", i, err)
		}
		if got != want {
			t.Fatalf("destination long %d = %08x, want %08x", i, got, want)
		}
	}
	if cpu.regs.A[1] != 0x4000 {
		t.Fatalf("A1 should remain unchanged for MOVEM to (A1), got %08x", cpu.regs.A[1])
	}
}

func TestMovemLongLoadsSequentialWordsForControlMode(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[6] = 0x3050

	values := []uint32{
		0x11111111,
		0x22222222,
		0x33333333,
		0x44444444,
	}
	base := cpu.regs.A[6] - 56
	for i, value := range values {
		if err := ram.Write(Long, base+uint32(i*4), value); err != nil {
			t.Fatalf("seed memory %d: %v", i, err)
		}
	}

	code := assemble(t, "MOVEM.L -56(A6),D2/D3/A2/A3")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program: %v", err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("step failed: %v", err)
	}

	if uint32(cpu.regs.D[2]) != values[0] {
		t.Fatalf("D2 = %08x, want %08x", uint32(cpu.regs.D[2]), values[0])
	}
	if uint32(cpu.regs.D[3]) != values[1] {
		t.Fatalf("D3 = %08x, want %08x", uint32(cpu.regs.D[3]), values[1])
	}
	if cpu.regs.A[2] != values[2] {
		t.Fatalf("A2 = %08x, want %08x", cpu.regs.A[2], values[2])
	}
	if cpu.regs.A[3] != values[3] {
		t.Fatalf("A3 = %08x, want %08x", cpu.regs.A[3], values[3])
	}
	if cpu.regs.A[6] != 0x3050 {
		t.Fatalf("A6 should remain unchanged for control-mode MOVEM load, got %08x", cpu.regs.A[6])
	}
}
