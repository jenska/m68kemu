package mmu_test

import (
	"errors"
	"testing"

	"github.com/jenska/m68kemu"
	"github.com/jenska/m68kemu/mmu"

	asm "github.com/jenska/m68kasm"
)

// run851 is run for an MC68020 with an MC68851.
func run851(t *testing.T, source string, setup func(*machine)) *machine {
	t.Helper()
	opts := []m68kemu.Option{m68kemu.WithModel(m68kemu.M68020), m68kemu.WithMC68851()}
	return runOn(t, asm.CPU68020, opts, source, setup)
}

// A second translation tree, for another task: it maps the first 256 KB
// one-to-one except page $10, which it maps to physical $31000.
const (
	tableA2 = 0x24000
	tableB2 = 0x24100
)

func setupTask2(m *machine) {
	m.ram.Write(m68kemu.Long, tableA2, tableB2|2)
	for page := uint32(0); page < 64; page++ {
		m.ram.Write(m68kemu.Long, tableB2+page*4, page<<12|1)
	}
	m.ram.Write(m68kemu.Long, tableB2+0x10*4, 0x31000|1)
	m.ram.Write(m68kemu.Long, 0x10000, 1)
	m.ram.Write(m68kemu.Long, 0x30000, 3)
	m.ram.Write(m68kemu.Long, 0x31000, 4)
}

func TestMC68851Translation(t *testing.T) {
	m := run851(t, prologue+`
	MOVE.L	#$12345678,$10004
	MOVE.L	$10004,D0
`+epilogue, nil)
	m.expectPC(t, "done")
	if got := m.read(m68kemu.Long, 0x30004); got != 0x12345678 || m.cpu.Registers().D[0] != 0x12345678 {
		t.Errorf("physical $30004 = %x, D0 = %x, want 12345678", got, m.cpu.Registers().D[0])
	}
	if d := m.read(m68kemu.Long, pageDescriptor(remapped>>12)); d != 0x30000|0x18|1 {
		t.Errorf("page descriptor = %x, want U and M set", d)
	}
}

func TestMC68851TaskAliases(t *testing.T) {
	// Loading CRP picks a task alias from the root pointer table. Entries
	// cached for one task survive while another runs: after switching back,
	// task A still sees its cached translation although its table changed.
	m := run851(t, prologue+`
	MOVE.L	$10000,D0		; task A: physical $30000
	PMOVE	PCSR,pcsrA
	PMOVE	crpB,CRP
	PMOVE	PCSR,pcsrB
	MOVE.L	$10000,D1		; task B: physical $31000
	MOVE.L	#$10001,$20140		; map task A's page one-to-one, no flush
	PMOVE	crp,CRP
	PMOVE	PCSR,pcsrA2
	MOVE.L	$10000,D2		; still the cached $30000
	PFLUSHR	crp2			; drop task A's alias, while B's is not current
	PMOVE	crpB,CRP
	PMOVE	crp,CRP
	PMOVE	PCSR,pcsrA3
	MOVE.L	$10000,D3		; searched again: physical $10000
	BRA	done
	ALIGN	4
crpB:	DC.L	$7FFF0002,$24000
crp2:	DC.L	$7FFF0002,$20000
pcsrA:	DC.W	0
pcsrB:	DC.W	0
pcsrA2:	DC.W	0
pcsrA3:	DC.W	0
`+epilogue, setupTask2)
	m.expectPC(t, "done")
	regs := m.cpu.Registers()
	if regs.D[0] != 3 || regs.D[1] != 4 || regs.D[2] != 3 || regs.D[3] != 1 {
		t.Errorf("D0-D3 = %d %d %d %d, want 3 4 3 1", regs.D[0], regs.D[1], regs.D[2], regs.D[3])
	}
	for _, tt := range []struct {
		label string
		want  uint32
	}{{"pcsrA", 0x8001}, {"pcsrB", 0x8002}, {"pcsrA2", 0x0001}, {"pcsrA3", 0x8001}} {
		if got := m.read(m68kemu.Word, m.labels[tt.label]); got != tt.want {
			t.Errorf("%s = %04x, want %04x", tt.label, got, tt.want)
		}
	}
}

func TestMC68851SharedEntries(t *testing.T) {
	// The root pointer is globally shared, so its entries survive PFLUSH and
	// go only with PFLUSHS.
	m := run851(t, `
	ORG	$2000
	PMOVE	crpSG,CRP
	PMOVE	tc,TC
	LEA	$10000,A0
	MOVE.L	(A0),D0
	MOVE.L	#$10001,$20140
	PFLUSH	#5,#7,(A0)
	MOVE.L	(A0),D1			; still cached
	PFLUSHS	#5,#7,(A0)
	MOVE.L	(A0),D2
	BRA	done
	ALIGN	4
crpSG:	DC.L	$7FFF0202,$20000
`+epilogue, setupTask2)
	m.expectPC(t, "done")
	if regs := m.cpu.Registers(); regs.D[0] != 3 || regs.D[1] != 3 || regs.D[2] != 1 {
		t.Errorf("D0-D2 = %d %d %d, want 3 3 1", regs.D[0], regs.D[1], regs.D[2])
	}
}

func TestMC68851Conditions(t *testing.T) {
	m := run851(t, prologue+`
	LEA	$11000,A0
	PTESTR	#5,(A0),#7,A1
	PMOVE	PSR,psr
	PSIS	D0
	PSIC	D1
	MOVEQ	#2,D2
loop:	PDBIS	D2,loop			; I set: no loop
	MOVEQ	#2,D3
loop2:	PDBIC	D3,loop2		; I clear is false: loops until D3 = -1
	PBIC	done			; not taken
	PTRAPIS
	BRA	done
	ALIGN	2
psr:	DC.W	0
`+epilogue, nil)
	m.expectPC(t, "trap")
	regs := m.cpu.Registers()
	if regs.D[0]&0xff != 0xff || regs.D[1]&0xff != 0 {
		t.Errorf("PSIS/PSIC: D0=%x D1=%x, want ff and 00", regs.D[0]&0xff, regs.D[1]&0xff)
	}
	if regs.D[2] != 2 || regs.D[3]&0xffff != 0xffff {
		t.Errorf("PDBIS left D2 = %d, PDBIC D3 = %x; want 2 and ffff", regs.D[2], regs.D[3]&0xffff)
	}
	if psr := uint16(m.read(m68kemu.Word, m.labels["psr"])); psr != mmu.SRInvalid|2 {
		t.Errorf("PSR = %04x, want I and two levels", psr)
	}
	if regs.A[1] != pageDescriptor(missing>>12) {
		t.Errorf("A1 = %x, want the invalid descriptor at %x", regs.A[1], pageDescriptor(missing>>12))
	}
}

func TestMC68851IllegalOperation(t *testing.T) {
	// PTEST and PLOAD need translation enabled on the MC68851.
	for _, source := range []string{"PTESTR #5,(A0),#7", "PLOADR #5,(A0)"} {
		m := run851(t, "\tORG\t$2000\n\t"+source+"\ndone:\tBRA\tdone\n", nil)
		m.expectPC(t, "illop")
	}
}

func TestMC68851SaveRestore(t *testing.T) {
	m := run851(t, prologue+`
	PMOVE	#$40,CAL
	PSAVE	-(A7)
	MOVE.L	A7,A1
	PMOVE	#0,CAL
	PRESTORE (A7)+
	PMOVE	CAL,D0
`+epilogue, nil)
	m.expectPC(t, "done")
	regs := m.cpu.Registers()
	if regs.A[7] != 0x8000 || regs.A[1] != 0x8000-36 {
		t.Errorf("A7=%x A1=%x, want a 36-byte idle frame below 8000", regs.A[7], regs.A[1])
	}
	frame := regs.A[1]
	if got := m.read(m68kemu.Long, frame); got != 0xc0200000 {
		t.Errorf("format word = %08x, want c0200000", got)
	}
	if got := m.read(m68kemu.Long, frame+16); got != 0x20000 {
		t.Errorf("frame CRP low = %x, want 20000", got)
	}
	if regs.D[0]&0xff != 0x40 {
		t.Errorf("CAL after PRESTORE = %x, want 40", regs.D[0]&0xff)
	}
}

func TestMC68851Breakpoints(t *testing.T) {
	// BAD0 holds ADDQ.L #1,D1 and BAC0 allows it twice; the third BKPT #0
	// takes the illegal instruction exception.
	m := run851(t, `
	ORG	$2000
	PMOVE	#$5281,BAD0
	PMOVE	#$8002,BAC0
	PMOVE	BAD0,D2
	MOVEQ	#0,D1
loop:	BKPT	#0
	BRA	loop
done:	BRA	done
`, nil)
	m.expectPC(t, "illegal")
	if d1 := m.cpu.Registers().D[1]; d1 != 2 {
		t.Errorf("D1 = %d, want 2 replacements", d1)
	}
	if d2 := m.cpu.Registers().D[2]; d2&0xffff != 0x5281 {
		t.Errorf("PMOVE BAD0,D2 = %x, want 5281", d2&0xffff)
	}
	if bac := m.mmu.Register(mmu.BAC0); bac != 0x8000 {
		t.Errorf("BAC0 = %04x, want the skip count used up", bac)
	}
}

func TestMC68851PValid(t *testing.T) {
	// PVALID is the one MC68851 instruction a user program may run. With
	// module operations disabled (AC clear) it always fails.
	m := run851(t, `
	ORG	$2000
	MOVE	#0,SR
	LEA	$1000,A0
	PVALID	VAL,(A0)
done:	BRA	done
`, nil)
	m.expectPC(t, "access")

	// With access levels on (AC: MC, one level bit) a pointer at level 0
	// is more privileged than VAL's level 1.
	for _, tt := range []struct {
		pointer uint32
		want    string
	}{{0x00001000, "access"}, {0x80001000, "done"}} {
		m := run851(t, `
	ORG	$2000
	PMOVE	#$0090,AC
	PMOVE	#$80,VAL
	PVALID	VAL,(A0)
done:	BRA	done
`, func(m *machine) { m.cpu.SetRegisters(withA0(m.cpu.Registers(), tt.pointer)) })
		m.expectPC(t, tt.want)
	}
}

func withA0(regs m68kemu.Registers, a0 uint32) m68kemu.Registers {
	regs.A[0] = a0
	return regs
}

func TestMC68851Privileged(t *testing.T) {
	m := run851(t, `
	ORG	$2000
	MOVE	#0,SR
	PMOVE	TC,D0
done:	BRA	done
`, nil)
	m.expectPC(t, "priv")
}

func TestMC68851Configuration(t *testing.T) {
	m := run851(t, `
	ORG	$2000
	PMOVE	bad,CRP
done:	BRA	done
	ALIGN	4
bad:	DC.L	$7FFF0000,$20000
`, nil)
	m.expectPC(t, "config")
	if crp := m.mmu.Register(mmu.CRP); crp != 0 {
		t.Errorf("CRP = %x, want the invalid root pointer refused", crp)
	}
}

func TestMC68851Attach(t *testing.T) {
	bus := func() *m68kemu.Bus { return m68kemu.NewBus(m68kemu.NewRAM(0, 0x10000)) }
	cpu, err := m68kemu.NewCPU(bus(), m68kemu.WithModel(m68kemu.M68020), m68kemu.WithMC68851())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cpu.MMU().(*mmu.MC68851); !ok {
		t.Errorf("MMU() = %T, want *mmu.MC68851", cpu.MMU())
	}
	for _, opts := range [][]m68kemu.Option{
		{m68kemu.WithModel(m68kemu.M68030), m68kemu.WithMC68851()},
		{m68kemu.WithModel(m68kemu.M68010), m68kemu.WithMC68851()},
		{m68kemu.WithModel(m68kemu.M68020), m68kemu.WithMC68851(), m68kemu.WithCoprocessor(0, nil)},
	} {
		if _, err := m68kemu.NewCPU(bus(), opts...); !errors.Is(err, m68kemu.ErrCoprocessorUnsupported) {
			t.Errorf("%d options: %v, want ErrCoprocessorUnsupported", len(opts), err)
		}
	}
}
