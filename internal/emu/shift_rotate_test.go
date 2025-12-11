package emu

import "testing"

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
			"ROLThroughZero",
			"ROL.B #2,D2",
			2,
			0,
			map[int]int32{2: 0x81},
			0xff,
			0x06,
			0,
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
