package m68kemu

import "testing"

func TestAddDataRegisterToDataRegister(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 2
	cpu.regs.D[1] = 1

	code := assemble(t, "ADD.L D1,D0\n")
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

	if got := cpu.regs.D[0]; got != 3 {
		t.Fatalf("expected D0=3 got %d", got)
	}
	if cpu.regs.SR&(srZero|srNegative|srCarry|srOverflow) != 0 {
		t.Fatalf("unexpected flags %04x", cpu.regs.SR)
	}
}

func TestSubqUpdatesFlags(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[2] = 1

	code := assemble(t, "SUBQ.W #1,D2\n")
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

	if got := cpu.regs.D[2] & 0xffff; got != 0 {
		t.Fatalf("expected zero, got %04x", got)
	}
	if cpu.regs.SR&srZero == 0 {
		t.Fatalf("expected zero flag set, got %04x", cpu.regs.SR)
	}
}

func TestAddqDataRegister(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 1

	code := assemble(t, "ADDQ.B #1,D0\n")
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

	if got := cpu.regs.D[0] & 0xff; got != 2 {
		t.Fatalf("expected 2 got %d", got)
	}
	if cpu.regs.SR&(srZero|srNegative|srOverflow|srCarry|srExtend) != 0 {
		t.Fatalf("unexpected flags %04x", cpu.regs.SR)
	}
}

func TestSubInstruction(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 5
	cpu.regs.D[1] = 3

	code := assemble(t, "SUB.L D1,D0\n")
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

	if got := cpu.regs.D[0]; got != 2 {
		t.Fatalf("expected 2 got %d", got)
	}
	if cpu.regs.SR&(srZero|srNegative|srOverflow|srCarry|srExtend) != 0 {
		t.Fatalf("unexpected flags %04x", cpu.regs.SR)
	}
}

func TestAddiByteToDataRegister(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 0x1234ff

	code := assemble(t, "ADDI.B #1,D0\n")
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

	if got := uint32(cpu.regs.D[0]); got != 0x00123400 {
		t.Fatalf("expected D0=0x00123400 got %08x", got)
	}
	expected := uint16(srZero | srCarry | srExtend)
	if got := cpu.regs.SR & (srZero | srNegative | srOverflow | srCarry | srExtend); got != expected {
		t.Fatalf("unexpected flags %04x", got)
	}
}

func TestSubiWordToDataRegister(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[1] = 0x12340000

	code := assemble(t, "SUBI.W #1,D1\n")
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

	if got := uint32(cpu.regs.D[1]); got != 0x1234ffff {
		t.Fatalf("expected D1=0x1234ffff got %08x", got)
	}
	expected := uint16(srNegative | srCarry | srExtend)
	if got := cpu.regs.SR & (srZero | srNegative | srOverflow | srCarry | srExtend); got != expected {
		t.Fatalf("unexpected flags %04x", got)
	}
}

func TestAddiLongToMemory(t *testing.T) {
	cpu, ram := newEnvironment(t)
	if err := ram.Write(Long, 0x3000, 1); err != nil {
		t.Fatalf("write data: %v", err)
	}

	code := assemble(t, "ADDI.L #2,$3000\n")
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

	if got, err := ram.Read(Long, 0x3000); err != nil || got != 3 {
		t.Fatalf("expected memory=3 got %08x err=%v", got, err)
	}
	if cpu.regs.SR&(srZero|srNegative|srOverflow|srCarry|srExtend) != 0 {
		t.Fatalf("unexpected flags %04x", cpu.regs.SR)
	}
}

func TestMulDivInstructions(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 10
	cpu.regs.D[1] = 2
	cpu.regs.D[2] = 4
	cpu.regs.D[3] = 3
	cpu.regs.D[4] = -2

	code := assemble(t, "DIVU D1,D0\nMULU D2,D0\nDIVS D3,D0\nMULS D4,D0\n")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	step := func(expect int32) {
		t.Helper()
		opcode, err := cpu.fetchOpcode()
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		if err := cpu.executeInstruction(opcode); err != nil {
			t.Fatalf("exec: %v", err)
		}
		if cpu.regs.D[0] != expect {
			t.Fatalf("expected %08x got %08x", expect, cpu.regs.D[0])
		}
	}

	step(0x00000005) // DIVU => quotient 5 remainder 0
	step(0x00000014) // MULU => 5*4
	step(0x00020006) // DIVS => quotient 6 remainder 2
	step(-12)        // MULS => 6 * -2 = -12
}

func TestDivByZeroTriggersExceptionVector(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 10
	cpu.regs.D[1] = 0

	handler := uint32(0x3456)
	if err := ram.Write(Long, uint32(XDivByZero<<2), handler); err != nil {
		t.Fatalf("failed to install divide-by-zero vector: %v", err)
	}

	originalPC := cpu.regs.PC
	code := assemble(t, "DIVU D1,D0\n")
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

	expectedSP := cpu.regs.SSP - exceptionFrameSize
	if cpu.regs.A[7] != expectedSP {
		t.Fatalf("SP after divide-by-zero = %08x, want %08x", cpu.regs.A[7], expectedSP)
	}

	stackedPC, err := ram.Read(Long, expectedSP+uint32(Word))
	if err != nil {
		t.Fatalf("read stacked PC: %v", err)
	}
	if want := originalPC + uint32(Word); stackedPC != want {
		t.Fatalf("stacked PC = %08x, want %08x", stackedPC, want)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("PC after divide-by-zero = %08x, want %08x", cpu.regs.PC, handler)
	}

	state := cpu.DebugState()
	if !state.HasException || state.LastException.Vector != XDivByZero {
		t.Fatalf("expected divide-by-zero exception state, got %+v", state.LastException)
	}
	if want := originalPC + uint32(Word); state.LastException.PC != want {
		t.Fatalf("exception PC = %08x, want %08x", state.LastException.PC, want)
	}
}

func TestMulDivImmediateInstructions(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.D[0] = 3
	cpu.regs.D[1] = -7

	code := []byte{
		0xC0, 0xFC, 0x00, 0x05, // MULU #5,D0
		0xC3, 0xFC, 0xFF, 0xFE, // MULS #-2,D1
	}
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	step := func(reg int, expect int32) {
		t.Helper()
		opcode, err := cpu.fetchOpcode()
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		if err := cpu.executeInstruction(opcode); err != nil {
			t.Fatalf("exec: %v", err)
		}
		if cpu.regs.D[reg] != expect {
			t.Fatalf("expected D%d=%08x got %08x", reg, expect, cpu.regs.D[reg])
		}
	}

	step(0, 15)
	step(1, 14)
}

func TestAddaSuba(t *testing.T) {
	cpu, ram := newEnvironment(t)
	code := assemble(t, "ADDA.W #-1,A0\nSUBA.L #1,A0\n")
	for i, b := range code {
		ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
	}

	opcode, _ := cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("ADDA failed: %v", err)
	}
	if cpu.regs.A[0] != 0xffffffff {
		t.Fatalf("expected sign-extended word add, got %08x", cpu.regs.A[0])
	}

	opcode, _ = cpu.fetchOpcode()
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("SUBA failed: %v", err)
	}
	if cpu.regs.A[0] != 0xfffffffe {
		t.Fatalf("expected subtraction result 0xfffffffe, got %08x", cpu.regs.A[0])
	}
	if cpu.regs.SR != 0x2700 {
		t.Fatalf("expected condition codes untouched, SR=%04x", cpu.regs.SR)
	}
}

func TestAddaSubaAddressRegisterSource(t *testing.T) {
	tests := []struct {
		name   string
		asm    string
		a0     uint32
		a1     uint32
		sr     uint16
		wantA1 uint32
	}{
		{
			name:   "SUBA.L A0,A1",
			asm:    "SUBA.L A0,A1\n",
			a0:     0x00000003,
			a1:     0x00000010,
			sr:     0x271b,
			wantA1: 0x0000000d,
		},
		{
			name:   "SUBA.W A0,A1",
			asm:    "SUBA.W A0,A1\n",
			a0:     0x1234ffff,
			a1:     0x00000005,
			sr:     0x271b,
			wantA1: 0x00000006,
		},
		{
			name:   "ADDA.L A0,A1",
			asm:    "ADDA.L A0,A1\n",
			a0:     0x00000003,
			a1:     0x00000005,
			sr:     0x271b,
			wantA1: 0x00000008,
		},
		{
			name:   "ADDA.W A0,A1",
			asm:    "ADDA.W A0,A1\n",
			a0:     0x1234ffff,
			a1:     0x00000005,
			sr:     0x271b,
			wantA1: 0x00000004,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			cpu.regs.A[0] = tt.a0
			cpu.regs.A[1] = tt.a1
			cpu.regs.SR = tt.sr

			code := assemble(t, tt.asm)
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
				t.Fatalf("execute %s: %v", tt.name, err)
			}

			if cpu.regs.A[1] != tt.wantA1 {
				t.Fatalf("A1 = %08x, want %08x", cpu.regs.A[1], tt.wantA1)
			}
			if cpu.regs.SR != tt.sr {
				t.Fatalf("expected SR unchanged, got %04x want %04x", cpu.regs.SR, tt.sr)
			}
		})
	}
}

func TestAddSubAddressRegisterSource(t *testing.T) {
	tests := []struct {
		name   string
		asm    string
		a3     uint32
		d0     int32
		sr     uint16
		wantD0 int32
	}{
		{
			name:   "ADD.L A3,D0",
			asm:    "ADD.L A3,D0\n",
			a3:     0xfffffff8,
			d0:     0x0000000e,
			sr:     0x2700,
			wantD0: 0x00000006,
		},
		{
			name:   "ADD.W A3,D0",
			asm:    "ADD.W A3,D0\n",
			a3:     0x1234ffff,
			d0:     0x00000005,
			sr:     0x2700,
			wantD0: 0x00000004,
		},
		{
			name:   "SUB.L A3,D0",
			asm:    "SUB.L A3,D0\n",
			a3:     0x00000003,
			d0:     0x00000010,
			sr:     0x2700,
			wantD0: 0x0000000d,
		},
		{
			name:   "SUB.W A3,D0",
			asm:    "SUB.W A3,D0\n",
			a3:     0x1234ffff,
			d0:     0x00000005,
			sr:     0x2700,
			wantD0: 0x00000006,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			cpu.regs.A[3] = tt.a3
			cpu.regs.D[0] = tt.d0
			cpu.regs.SR = tt.sr

			code := assemble(t, tt.asm)
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
				t.Fatalf("execute %s: %v", tt.name, err)
			}

			if cpu.regs.D[0] != tt.wantD0 {
				t.Fatalf("D0 = %08x, want %08x", uint32(cpu.regs.D[0]), uint32(tt.wantD0))
			}
		})
	}
}
