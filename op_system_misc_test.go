package m68kemu

import (
	"reflect"
	"testing"
)

func TestNopAdvancesPCAndLeavesState(t *testing.T) {
	cpu, ram := newEnvironment(t)

	opcode := uint16(0x4e71)
	if err := ram.Write(Word, cpu.regs.PC, uint32(opcode)); err != nil {
		t.Fatalf("failed to write NOP opcode: %v", err)
	}

	initialPC := cpu.regs.PC
	initialSR := cpu.regs.SR
	initialD := cpu.regs.D
	initialA := cpu.regs.A

	if err := cpu.Step(); err != nil {
		t.Fatalf("failed to execute NOP: %v", err)
	}

	if cpu.regs.PC != initialPC+uint32(Word) {
		t.Fatalf("PC not advanced after NOP: got %08x want %08x", cpu.regs.PC, initialPC+uint32(Word))
	}

	if cpu.regs.SR != initialSR {
		t.Fatalf("SR changed after NOP: got %04x want %04x", cpu.regs.SR, initialSR)
	}

	if cpu.regs.D != initialD {
		t.Fatalf("data registers changed after NOP: got %+v want %+v", cpu.regs.D, initialD)
	}

	if cpu.regs.A != initialA {
		t.Fatalf("address registers changed after NOP: got %+v want %+v", cpu.regs.A, initialA)
	}
}

func TestNegExtSwapExg(t *testing.T) {
	cpu, ram := newEnvironment(t)

	// Prepare registers.
	cpu.regs.D[0] = 1
	cpu.regs.D[1] = 0x00000080
	cpu.regs.D[2] = 0x12345678
	cpu.regs.D[3] = -0x55555556
	cpu.regs.D[4] = -0x44444445
	cpu.regs.A[0] = 0x11111111

	// Program: NEG.B D0; EXT.W D1; EXT.L D1; SWAP D2; EXG D3,D4; EXG D0,A0
	program := []byte{
		0x44, 0x00, // NEG.B D0
		0x48, 0x81, // EXT.W D1
		0x48, 0xC1, // EXT.L D1
		0x48, 0x42, // SWAP D2
		0xC9, 0x43, // EXG D3,D4
		0xC1, 0x88, // EXG D0,A0
	}

	for i, b := range program {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to load program: %v", err)
		}
	}

	step := func() uint16 {
		opcode, err := cpu.fetchOpcode()
		if err != nil {
			t.Fatalf("failed to fetch opcode: %v", err)
		}
		if err := cpu.executeInstruction(opcode); err != nil {
			t.Fatalf("execution failed for %04x: %v", opcode, err)
		}
		return opcode
	}

	step()
	if cpu.regs.D[0]&0xff != 0xff {
		t.Fatalf("NEG.B did not produce expected result, got %02x", cpu.regs.D[0]&0xff)
	}
	if cpu.regs.SR&(srCarry|srExtend) == 0 {
		t.Fatalf("NEG.B should set carry/extend flags")
	}

	step()
	if cpu.regs.D[1]&0xffff != 0xff80 {
		t.Fatalf("EXT.W did not sign-extend byte, got %04x", cpu.regs.D[1]&0xffff)
	}
	if cpu.regs.SR&srNegative == 0 || cpu.regs.SR&(srOverflow|srCarry) != 0 {
		t.Fatalf("EXT.W flags incorrect: %04x", cpu.regs.SR)
	}

	step()
	if cpu.regs.D[1] != -0x80 {
		t.Fatalf("EXT.L did not sign-extend word, got %08x", cpu.regs.D[1])
	}

	step()
	if cpu.regs.D[2] != 0x56781234 {
		t.Fatalf("SWAP did not swap words, got %08x", cpu.regs.D[2])
	}
	if cpu.regs.SR&(srOverflow|srCarry) != 0 {
		t.Fatalf("SWAP should clear V and C, SR=%04x", cpu.regs.SR)
	}
	if cpu.regs.SR&srExtend == 0 {
		t.Fatalf("SWAP should preserve extend flag")
	}

	handler := opcodeTable[0xc943]
	if handler == nil {
		t.Fatalf("EXG Dx,Dy opcode not registered")
	}
	if reflect.ValueOf(handler).Pointer() != reflect.ValueOf(exgInstruction).Pointer() {
		t.Fatalf("EXG opcode mapped to unexpected handler")
	}
	if opcode := step(); opcode != 0xc943 {
		t.Fatalf("expected EXG Dx,Dy opcode, got %04x", opcode)
	}
	if cpu.regs.D[3] != -0x44444445 || cpu.regs.D[4] != -0x55555556 {
		t.Fatalf("EXG Dx,Dy failed: D3=%08x D4=%08x", cpu.regs.D[3], cpu.regs.D[4])
	}

	step()
	if cpu.regs.D[0] != 0x11111111 || cpu.regs.A[0] != 0x000000ff {
		t.Fatalf("EXG Dx,Ay failed: D0=%08x A0=%08x", cpu.regs.D[0], cpu.regs.A[0])
	}
}
