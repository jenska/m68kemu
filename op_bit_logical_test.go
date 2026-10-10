package m68kemu

import "testing"

// TestBitOpcodeRegistrationPCRelative checks that BTST reads PC-relative
// operands while BCHG/BCLR/BSET, which write their operand, do not exist
// with one on the 68000.
func TestBitOpcodeRegistrationPCRelative(t *testing.T) {
	for _, opcode := range []uint16{0x013a, 0x013b, 0x083a, 0x083b, 0x013c} {
		if opcodesFor(M68000).handlers[opcode] == nil {
			t.Errorf("expected BTST opcode %04x to be registered", opcode)
		}
	}
	for _, opcode := range []uint16{0x017a, 0x017b, 0x01ba, 0x01bb, 0x01fa, 0x01fb, 0x087a, 0x087b, 0x08ba, 0x08bb, 0x08fa, 0x08fb} {
		if opcodesFor(M68000).handlers[opcode] != nil {
			t.Errorf("expected opcode %04x (bit change to a PC-relative operand) to be illegal", opcode)
		}
	}
}

func TestBitOperationsDataRegister(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[1] = 0b11
	cpu.regs.D[0] = 0 // bit number 0
	cpu.regs.SR |= srExtend

	code := assemble(t, "BTST #1,D1\nBCLR D0,D1\nBCHG #1,D1\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BTST failed: %v", err)
	}
	if cpu.regs.SR&srZero != 0 || cpu.regs.SR&srExtend == 0 {
		t.Fatalf("expected zero clear and extend preserved after BTST, SR=%04x", cpu.regs.SR)
	}

	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BCLR failed: %v", err)
	}
	if cpu.regs.D[1] != 0b10 || cpu.regs.SR&srZero != 0 {
		t.Fatalf("expected bit 0 cleared with zero clear, D1=%b SR=%04x", cpu.regs.D[1], cpu.regs.SR)
	}

	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BCHG failed: %v", err)
	}
	if cpu.regs.D[1] != 0 || cpu.regs.SR&srZero != 0 {
		t.Fatalf("expected bit 1 toggled to 0 with zero clear, D1=%b SR=%04x", cpu.regs.D[1], cpu.regs.SR)
	}
}

func TestBitSetMemory(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 9 // wraps to bit 1 for memory targets

	if err := ram.Write(Byte, 0x3000, 0x00); err != nil {
		t.Fatalf("failed to seed memory: %v", err)
	}

	code := assemble(t, "BSET D0,$3000\nBTST #1,$3000\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BSET failed: %v", err)
	}
	val, _ := ram.Read(Byte, 0x3000)
	if val != 0x02 || cpu.regs.SR&srZero == 0 {
		t.Fatalf("expected bit 1 set with zero flag set, val=%02x SR=%04x", val, cpu.regs.SR)
	}

	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BTST failed: %v", err)
	}
	if cpu.regs.SR&srZero != 0 {
		t.Fatalf("expected zero clear when testing set bit, SR=%04x", cpu.regs.SR)
	}
}

func TestBitImmediateWithDisplacementUsesImmediateBeforeEAExtensions(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[0] = 0x3000

	if err := ram.Write(Byte, 0x3008, 0x00); err != nil {
		t.Fatalf("failed to seed memory: %v", err)
	}

	code := assemble(t, "BTST #0,(8,A0)\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BTST failed: %v", err)
	}
	if cpu.regs.SR&srZero == 0 {
		t.Fatalf("expected zero flag set when testing clear bit, SR=%04x", cpu.regs.SR)
	}
}

func TestBTSTAllowsPCRelativeOperands(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 0
	start := cpu.regs.PC

	code := []byte{
		0x01, 0x3a, // BTST D0,d16(PC)
		0x00, 0x04, // target at start+6, relative to PC after opcode fetch
		0x00, 0x00,
		0x01,
	}
	for i, b := range code {
		if err := ram.Write(Byte, start+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write opcode byte: %v", err)
		}
	}

	opcode, err := cpu.fetchOpcode()
	if err != nil {
		t.Fatalf("fetch opcode: %v", err)
	}
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("BTST failed: %v", err)
	}
	if cpu.regs.SR&srZero != 0 {
		t.Fatalf("expected zero clear when testing set PC-relative bit, SR=%04x", cpu.regs.SR)
	}
	if cpu.regs.PC != start+4 {
		t.Fatalf("PC after BTST = %08x, want %08x", cpu.regs.PC, start+4)
	}
}

func TestLogicalInstructionAcceptsImmediateSourceEA(t *testing.T) {
	tests := []struct {
		name   string
		opcode uint16
		ext    uint16
		setup  func(*cpu)
		check  func(*testing.T, *cpu)
	}{
		{
			name:   "ANDImmediateToDataRegister",
			opcode: 0xce7c,
			ext:    0x0003,
			setup: func(c *cpu) {
				c.regs.D[7] = 0x0007
				c.regs.SR = srExtend | srCarry
			},
			check: func(t *testing.T, c *cpu) {
				if got := c.regs.D[7] & 0xffff; got != 0x0003 {
					t.Fatalf("unexpected D7 after immediate-source AND: %04x", got)
				}
				if c.regs.SR&(srCarry|srOverflow) != 0 {
					t.Fatalf("carry/overflow not cleared after immediate-source AND: SR=%04x", c.regs.SR)
				}
				if c.regs.SR&srExtend == 0 {
					t.Fatalf("extend flag should be preserved after immediate-source AND: SR=%04x", c.regs.SR)
				}
			},
		},
		{
			name:   "ORImmediateToDataRegister",
			opcode: 0x8e7c,
			ext:    0x0003,
			setup: func(c *cpu) {
				c.regs.D[7] = 0x0004
			},
			check: func(t *testing.T, c *cpu) {
				if got := c.regs.D[7] & 0xffff; got != 0x0007 {
					t.Fatalf("unexpected D7 after immediate-source OR: %04x", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			tt.setup(cpu)

			code := []uint16{tt.opcode, tt.ext}
			for i, word := range code {
				if err := ram.Write(Word, cpu.regs.PC+uint32(i*2), uint32(word)); err != nil {
					t.Fatalf("failed to write word %d: %v", i, err)
				}
			}

			opcode, err := cpu.fetchOpcode()
			if err != nil {
				t.Fatalf("failed to fetch opcode: %v", err)
			}
			if err := cpu.executeInstruction(opcode); err != nil {
				t.Fatalf("failed to execute opcode %04x: %v", opcode, err)
			}

			tt.check(t, cpu)
		})
	}
}

func TestShiftRotateFlags(t *testing.T) {
	tests := []struct {
		name        string
		asm         string
		dstReg      int
		setupSR     uint16
		setupD      map[int]int32
		mask        uint32
		wantValue   uint32
		wantSRFlags uint16
	}{
		{
			"ASRRightSetsCarryAndExtend",
			"ASR.B #1,D0",
			0,
			0,
			map[int]int32{0: 0x81},
			0xff,
			0xc0,
			srCarry | srExtend | srNegative,
		},
		{
			"LSLWordClearsExtend",
			"LSL.W #1,D0",
			0,
			srExtend,
			map[int]int32{0: 0xc000},
			0xffff,
			0x8000,
			srNegative | srCarry | srExtend,
		},
		{
			"ROXRUsesExtend",
			"ROXR.B #1,D1",
			1,
			srExtend,
			map[int]int32{1: 0x01},
			0xff,
			0x80,
			srCarry | srExtend | srNegative,
		},
		{
			"ROXLUsesExtend",
			"ROXL.B #1,D5",
			5,
			srExtend,
			map[int]int32{5: 0x80},
			0xff,
			0x01,
			srCarry | srExtend,
		},
		{
			"ROLThroughZero",
			"ROL.B #2,D2",
			2,
			0,
			map[int]int32{2: 0x81},
			0xff,
			0x06,
			0,
		},
		{
			"ASLWordOverflow",
			"ASL.W #1,D3",
			3,
			0,
			map[int]int32{3: 0x4000},
			0xffff,
			0x8000,
			srNegative | srOverflow,
		},
		{
			"ROLDoesNotChangeExtend",
			"ROL.B #1,D4",
			4,
			srExtend,
			map[int]int32{4: 0x81},
			0xff,
			0x03,
			srCarry | srExtend,
		},
		{
			"RORPreservesExtend",
			"ROR.B #1,D6",
			6,
			srExtend,
			map[int]int32{6: 0x01},
			0xff,
			0x80,
			srCarry | srExtend | srNegative,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			cpu.regs.SR = (cpu.regs.SR &^ (srCarry | srExtend | srZero | srNegative | srOverflow)) | tt.setupSR
			for reg, val := range tt.setupD {
				cpu.regs.D[reg] = val
			}

			runSingleInstruction(t, cpu, ram, tt.asm)

			if got := uint32(cpu.regs.D[tt.dstReg]) & tt.mask; got != tt.wantValue {
				t.Fatalf("expected result %x, got %x", tt.wantValue, got)
			}

			mask := uint16(srCarry | srExtend | srNegative | srZero | srOverflow)
			if got := cpu.regs.SR & mask; got != tt.wantSRFlags {
				t.Fatalf("expected SR flags %04x got %04x", tt.wantSRFlags, got)
			}
		})
	}
}

func TestShiftZeroCountPreservesExtend(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.SR |= srExtend
	cpu.regs.D[0] = 0x12
	cpu.regs.D[1] = 0

	runSingleInstruction(t, cpu, ram, "LSR.B D1,D0")

	if cpu.regs.SR&srExtend == 0 {
		t.Fatalf("extend bit should remain set when count is zero")
	}
}

func TestRotateWordByFullWidthPreservesValueAndCarry(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 0x8000
	cpu.regs.D[1] = 16
	cpu.regs.SR &^= srCarry | srZero | srNegative | srOverflow

	runSingleInstruction(t, cpu, ram, "ROL.W D1,D0")

	if got := uint32(cpu.regs.D[0]) & 0xffff; got != 0x8000 {
		t.Fatalf("expected value 0x8000, got %04x", got)
	}
	if cpu.regs.SR&srCarry != 0 {
		t.Fatalf("expected carry clear for ROL.W D1,D0 with count 16, got SR=%04x", cpu.regs.SR)
	}
}

func TestRotateRightWordByFullWidthPreservesValueAndCarry(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 0x7fff
	cpu.regs.D[1] = 16
	cpu.regs.SR &^= srCarry | srZero | srNegative | srOverflow

	runSingleInstruction(t, cpu, ram, "ROR.W D1,D0")

	if got := uint32(cpu.regs.D[0]) & 0xffff; got != 0x7fff {
		t.Fatalf("expected value 0x7fff, got %04x", got)
	}
	if cpu.regs.SR&srCarry != 0 {
		t.Fatalf("expected carry clear for ROR.W D1,D0 with count 16, got SR=%04x", cpu.regs.SR)
	}
}

func TestShiftRotateMemoryLogicalRight(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[0] = 0x200

	if err := ram.Write(Word, 0x200, 0x8001); err != nil {
		t.Fatalf("failed to seed memory: %v", err)
	}

	runSingleInstruction(t, cpu, ram, "LSR.W (A0)")

	if val, _ := ram.Read(Word, 0x200); val != 0x4000 {
		t.Fatalf("expected logical shift right result 0x4000, got %04x", val)
	}
	expectedFlags := uint16(srCarry | srExtend)
	if got := cpu.regs.SR & (srCarry | srExtend | srZero | srNegative | srOverflow); got != expectedFlags {
		t.Fatalf("unexpected flags for memory LSR: got %04x want %04x", got, expectedFlags)
	}
}

func TestShiftRotateMemoryRotateLeft(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[0] = 0x200

	if err := ram.Write(Word, 0x200, 0x1111); err != nil {
		t.Fatalf("failed to seed memory: %v", err)
	}

	runSingleInstruction(t, cpu, ram, "ROL.W (A0)")

	if val, _ := ram.Read(Word, 0x200); val != 0x2222 {
		t.Fatalf("expected memory ROL result 0x2222, got %04x", val)
	}
	if got := cpu.regs.SR & (srCarry | srExtend | srZero | srNegative | srOverflow); got != 0 {
		t.Fatalf("unexpected flags for memory ROL: got %04x want 0000", got)
	}
}

func TestShiftRotateMemoryRotateLeftWrapsBit(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[0] = 0x200

	if err := ram.Write(Word, 0x200, 0x8888); err != nil {
		t.Fatalf("failed to seed memory: %v", err)
	}

	runSingleInstruction(t, cpu, ram, "ROL.W (A0)")

	if val, _ := ram.Read(Word, 0x200); val != 0x1111 {
		t.Fatalf("expected memory ROL wrap result 0x1111, got %04x", val)
	}
	expectedFlags := uint16(srCarry)
	if got := cpu.regs.SR & (srCarry | srExtend | srZero | srNegative | srOverflow); got != expectedFlags {
		t.Fatalf("unexpected flags for wrapped memory ROL: got %04x want %04x", got, expectedFlags)
	}
}
