package m68kemu

import (
	"testing"

	asm "github.com/jenska/m68kasm"
)

func newEnvironment010(tb testing.TB) (*cpu, *RAM) {
	tb.Helper()

	memory := NewRAM(0, 1024*64)
	bus := NewBus(memory)
	memory.Write(Long, 0, 0x1000)
	memory.Write(Long, 4, 0x2000)
	processor, err := NewCPU(bus, WithModel(M68010))
	if err != nil {
		tb.Fatalf("Failed to create CPU: %v", err)
	}
	return processor.(*cpu), memory
}

// load010 assembles source for the 68010 and stores it at the current PC.
func load010(tb testing.TB, cpu *cpu, ram *RAM, source string) {
	tb.Helper()
	code, err := asm.AssembleStringWithOptions(source, asm.ParseOptions{Target: asm.Target{CPU: asm.CPU68010}})
	if err != nil {
		tb.Fatalf("Assembler failed: %v", err)
	}
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			tb.Fatalf("failed to store program byte: %v", err)
		}
	}
}

func step(tb testing.TB, cpu *cpu) {
	tb.Helper()
	if err := cpu.Step(); err != nil {
		tb.Fatalf("Step failed: %v", err)
	}
}

func TestVectorsRelativeToVBR(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	cpu.regs.VBR = 0x8000
	ram.Write(Long, 0x8000+XTrap<<2, 0x5000)
	ram.Write(Long, XTrap<<2, 0x6000) // must not be used
	load010(t, cpu, ram, "TRAP #0")

	step(t, cpu)
	if cpu.regs.PC != 0x5000 {
		t.Fatalf("PC = %04x, want the TRAP #0 vector from the VBR table at 5000", cpu.regs.PC)
	}
}

func TestResetClearsControlRegisters(t *testing.T) {
	cpu, _ := newEnvironment010(t)
	cpu.regs.VBR, cpu.regs.SFC, cpu.regs.DFC = 0x8000, 5, 6
	if err := cpu.Reset(); err != nil {
		t.Fatal(err)
	}
	if cpu.regs.VBR != 0 || cpu.regs.SFC != 0 || cpu.regs.DFC != 0 {
		t.Fatalf("after reset VBR=%x SFC=%d DFC=%d, want all zero", cpu.regs.VBR, cpu.regs.SFC, cpu.regs.DFC)
	}
}

const (
	illegalHandler = 0x4000
	privHandler    = 0x5000
)

// setTrapVectors points the illegal-instruction and privilege-violation
// vectors of the table at address 0 to their handlers.
func setTrapVectors(ram *RAM) {
	ram.Write(Long, XIllegal<<2, illegalHandler)
	ram.Write(Long, XPrivViolation<<2, privHandler)
}

func enterUserMode(cpu *cpu) {
	cpu.regs.USP = 0x0e00
	cpu.setSR(cpu.regs.SR &^ srSupervisor)
}

func TestMovecControlRegisters(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	load010(t, cpu, ram, `
	MOVE.L #$8000,D0
	MOVEC D0,VBR
	MOVEC VBR,A1
	MOVEQ #-1,D1
	MOVEC D1,SFC
	MOVEC SFC,D2
	MOVE.L #$3000,A2
	MOVEC A2,USP
	MOVEC USP,D3
	MOVEQ #3,D4
	MOVEC D4,DFC
	MOVEC DFC,D5
`)
	for range 12 {
		step(t, cpu)
	}
	r := cpu.regs
	switch {
	case r.VBR != 0x8000 || r.A[1] != 0x8000:
		t.Fatalf("VBR=%x A1=%x, want 8000", r.VBR, r.A[1])
	case r.SFC != 7 || r.D[2] != 7:
		t.Fatalf("SFC=%d D2=%d, want 7 (3 bits of -1)", r.SFC, r.D[2])
	case r.USP != 0x3000 || r.D[3] != 0x3000:
		t.Fatalf("USP=%x D3=%x, want 3000", r.USP, r.D[3])
	case r.DFC != 3 || r.D[5] != 3:
		t.Fatalf("DFC=%d D5=%d, want 3", r.DFC, r.D[5])
	}
}

func TestMovecInUserModeIsPrivileged(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	setTrapVectors(ram)
	load010(t, cpu, ram, "MOVEC VBR,D0")
	enterUserMode(cpu)
	step(t, cpu)
	if cpu.regs.PC != privHandler {
		t.Fatalf("PC = %x, want the privilege-violation handler", cpu.regs.PC)
	}
}

func TestMovecUnknownControlRegisterIsIllegal(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	setTrapVectors(ram)
	start := cpu.regs.PC
	ram.Write(Word, start, 0x4e7a)   // MOVEC Rc,D0
	ram.Write(Word, start+2, 0x0002) // CACR, which the 68010 lacks
	step(t, cpu)
	if cpu.regs.PC != illegalHandler {
		t.Fatalf("PC = %x, want the illegal-instruction handler", cpu.regs.PC)
	}
	// Same stacked PC as an unassigned opcode word: past the opcode, not
	// past the extension word.
	if stacked, _ := ram.Read(Long, cpu.regs.A[7]+2); stacked != start+2 {
		t.Fatalf("stacked PC = %x, want %x", stacked, start+2)
	}
}

func TestMoves(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	load010(t, cpu, ram, `
	MOVES.B (A2),D0
	MOVES.B (A2),A0
	MOVES.W D1,(A3)
`)
	cpu.regs.SFC, cpu.regs.DFC = 1, 5
	cpu.regs.A[2], cpu.regs.A[3] = 0x3000, 0x3010
	cpu.regs.D[0], cpu.regs.D[1] = 0x12345678, 0x0000abcd
	ram.Write(Byte, 0x3000, 0x80)

	var accesses []BusAccessInfo
	cpu.SetBusTracer(func(info BusAccessInfo) {
		if !info.InstructionFetch {
			accesses = append(accesses, info)
		}
	})
	for range 3 {
		step(t, cpu)
	}

	if got := uint32(cpu.regs.D[0]); got != 0x12345680 {
		t.Fatalf("D0 = %08x, want the byte merged into the low byte: 12345680", got)
	}
	if got := cpu.regs.A[0]; got != 0xffffff80 {
		t.Fatalf("A0 = %08x, want the byte sign-extended: ffffff80", got)
	}
	if got, _ := ram.Read(Word, 0x3010); got != 0xabcd {
		t.Fatalf("word at 3010 = %04x, want abcd", got)
	}
	want := []BusAccessInfo{
		{Address: 0x3000, Size: Byte, FunctionCode: 1},
		{Address: 0x3000, Size: Byte, FunctionCode: 1},
		{Address: 0x3010, Size: Word, Write: true, FunctionCode: 5},
	}
	if len(accesses) != len(want) {
		t.Fatalf("data accesses = %+v, want %d", accesses, len(want))
	}
	for i, a := range accesses {
		if a.Address != want[i].Address || a.Size != want[i].Size || a.Write != want[i].Write || a.FunctionCode != want[i].FunctionCode {
			t.Fatalf("access %d = %+v, want %+v", i, a, want[i])
		}
	}
}

func TestMovesInUserModeIsPrivileged(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	setTrapVectors(ram)
	load010(t, cpu, ram, "MOVES.L (A2),D0")
	enterUserMode(cpu)
	step(t, cpu)
	if cpu.regs.PC != privHandler {
		t.Fatalf("PC = %x, want the privilege-violation handler", cpu.regs.PC)
	}
}

func TestRtd(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	load010(t, cpu, ram, "RTD #8")
	cpu.regs.A[7] = 0x0f00
	ram.Write(Long, 0x0f00, 0x2400)
	step(t, cpu)
	if cpu.regs.PC != 0x2400 {
		t.Fatalf("PC = %x, want 2400", cpu.regs.PC)
	}
	if cpu.regs.A[7] != 0x0f00+4+8 {
		t.Fatalf("SP = %x, want %x: return address and 8 bytes of arguments released", cpu.regs.A[7], 0x0f00+4+8)
	}
}

func TestMoveFromCcr(t *testing.T) {
	cpu, ram := newEnvironment010(t)
	load010(t, cpu, ram, "MOVE CCR,D0")
	enterUserMode(cpu) // MOVE from CCR is not privileged
	cpu.regs.SR |= 0x15
	cpu.regs.D[0] = -1
	step(t, cpu)
	if got := uint32(cpu.regs.D[0]); got != 0xffff0015 {
		t.Fatalf("D0 = %08x, want the CCR as a word: ffff0015", got)
	}
}

func TestMoveFromSrPrivilege(t *testing.T) {
	for _, tt := range []struct {
		model      Model
		privileged bool
	}{
		{M68000, false},
		{M68010, true},
	} {
		t.Run(tt.model.String(), func(t *testing.T) {
			ram := NewRAM(0, 0x10000)
			ram.Write(Long, 0, 0x1000)
			ram.Write(Long, 4, 0x2000)
			setTrapVectors(ram)
			ram.Write(Word, 0x2000, 0x40c0) // MOVE SR,D0
			c, err := NewCPU(NewBus(ram), WithModel(tt.model))
			if err != nil {
				t.Fatal(err)
			}
			cpu := c.(*cpu)
			enterUserMode(cpu)
			step(t, cpu)
			if got := cpu.regs.PC == privHandler; got != tt.privileged {
				t.Fatalf("privilege violation = %v, want %v (PC=%x)", got, tt.privileged, cpu.regs.PC)
			}
		})
	}
}
