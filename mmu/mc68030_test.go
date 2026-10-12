package mmu_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jenska/m68kemu"
	"github.com/jenska/m68kemu/mmu"

	asm "github.com/jenska/m68kasm"
)

// The tests use a two-level tree of short descriptors: TC selects 4 KB
// pages, an initial shift of 8 and two 6-bit table indexes. The A table at
// $20000 points to the B table at $20100, which maps the first 256 KB
// one-to-one except for three pages.
const (
	tableA    = 0x20000
	tableB    = 0x20100
	tcValue   = 0x80c86600 // E, PS=12, IS=8, TIA=6, TIB=6
	remapped  = 0x10000    // page mapped to physical $30000
	missing   = 0x11000    // page with an invalid descriptor
	protected = 0x12000    // write-protected page

	berrHandler   = 0x6000
	configHandler = 0x6100
	privHandler   = 0x6200
)

func pageDescriptor(page uint32) uint32 { return tableB + page*4 }

// setupTables writes the translation tree into ram.
func setupTables(ram *m68kemu.RAM) {
	ram.Write(m68kemu.Long, tableA, tableB|2) // valid, short descriptors
	for page := uint32(0); page < 64; page++ {
		ram.Write(m68kemu.Long, pageDescriptor(page), page<<12|1)
	}
	ram.Write(m68kemu.Long, pageDescriptor(remapped>>12), 0x30000|1)
	ram.Write(m68kemu.Long, pageDescriptor(missing>>12), 0)
	ram.Write(m68kemu.Long, pageDescriptor(protected>>12), protected|4|1)
}

// prologue loads CRP and TC from the data that epilogue appends.
const prologue = `
	ORG	$2000
	PMOVE	crp,CRP
	PMOVE	tc,TC
`

const epilogue = `
done:	BRA	done
	ALIGN	4
crp:	DC.L	$7FFF0002,$20000
tc:	DC.L	$80C86600
`

type machine struct {
	cpu    *m68kemu.CPU
	ram    *m68kemu.RAM
	mmu    *mmu.MC68030
	labels map[string]uint32
}

// run assembles source for an MC68030, loads it at $2000 into 256 KB of RAM
// with the translation tables, and runs it until the label done or one of
// the exception handlers.
func run(t *testing.T, source string, setup func(*machine)) *machine {
	t.Helper()
	result, err := asm.AssembleDetailedWithOptions(strings.NewReader(source),
		asm.ParseOptions{Target: asm.Target{CPU: asm.CPU68030, Features: asm.FeatPMMU}})
	if err != nil {
		t.Fatalf("assembler: %v", err)
	}
	ram := m68kemu.NewRAM(0, 0x40000)
	ram.Write(m68kemu.Long, 0, 0x8000)
	ram.Write(m68kemu.Long, 4, 0x2000)
	ram.Write(m68kemu.Long, m68kemu.XBusError<<2, berrHandler)
	ram.Write(m68kemu.Long, m68kemu.XMMUConfiguration<<2, configHandler)
	ram.Write(m68kemu.Long, m68kemu.XPrivViolation<<2, privHandler)
	for i, b := range result.Bytes {
		ram.Write(m68kemu.Byte, result.Origin+uint32(i), uint32(b))
	}
	setupTables(ram)
	m := &machine{ram: ram, labels: result.Labels}
	if handler, ok := result.Labels["berr"]; ok {
		ram.Write(m68kemu.Long, m68kemu.XBusError<<2, handler)
	}
	if setup != nil {
		setup(m)
	}
	m.cpu, err = m68kemu.NewCPU(m68kemu.NewBus(ram), m68kemu.WithModel(m68kemu.M68030))
	if err != nil {
		t.Fatal(err)
	}
	m.mmu = m.cpu.MMU()
	stops := []uint32{berrHandler, configHandler, privHandler}
	if done, ok := result.Labels["done"]; ok {
		stops = append(stops, done)
	}
	if _, err := m.cpu.RunUntil(m68kemu.RunUntilOptions{MaxInstructions: 1000, StopAtPC: stops}); err != nil {
		t.Fatal(err)
	}
	return m
}

func (m *machine) read(size m68kemu.Size, address uint32) uint32 {
	v, _ := m.ram.Read(size, address)
	return v
}

func (m *machine) expectPC(t *testing.T, label string) {
	t.Helper()
	want, ok := m.labels[label]
	if !ok {
		want = map[string]uint32{"berr": berrHandler, "config": configHandler, "priv": privHandler}[label]
	}
	if pc := m.cpu.Registers().PC; pc != want {
		t.Fatalf("PC = %x, want %s at %x", pc, label, want)
	}
}

func TestTranslation(t *testing.T) {
	m := run(t, prologue+`
	MOVE.L	#$12345678,$10004
	MOVE.L	$10004,D0
	MOVE.L	$3000,D1
	MOVE.L	$4000,D2		; a read, then the first write to the page
	MOVE.L	D2,$4004
`+epilogue, func(m *machine) { m.ram.Write(m68kemu.Long, 0x3000, 0xcafe) })
	m.expectPC(t, "done")
	regs := m.cpu.Registers()
	if got := m.read(m68kemu.Long, 0x30004); got != 0x12345678 || m.read(m68kemu.Long, 0x10004) != 0 {
		t.Errorf("the write went to physical $10004, not $30004 (%x)", got)
	}
	if regs.D[0] != 0x12345678 || regs.D[1] != 0xcafe {
		t.Errorf("D0=%x D1=%x, want 12345678 and cafe", regs.D[0], regs.D[1])
	}
	if d := m.read(m68kemu.Long, pageDescriptor(remapped>>12)); d != 0x30000|0x18|1 {
		t.Errorf("page descriptor = %x, want U and M set", d)
	}
	if d := m.read(m68kemu.Long, pageDescriptor(3)); d != 0x3000|0x08|1 {
		t.Errorf("descriptor of a page only read = %x, want U set", d)
	}
	if d := m.read(m68kemu.Long, pageDescriptor(4)); d != 0x4000|0x18|1 {
		t.Errorf("descriptor of a page read, then written = %x, want U and M set", d)
	}
	if d := m.read(m68kemu.Long, tableA); d != tableB|0x08|2 {
		t.Errorf("A table descriptor = %x, want U set", d)
	}
	if m.mmu.Register(mmu.TC) != tcValue || m.mmu.Register(mmu.CRP) != 0x7fff0002_00020000 {
		t.Errorf("TC=%x CRP=%x", m.mmu.Register(mmu.TC), m.mmu.Register(mmu.CRP))
	}
}

func TestInstructionFetchTranslated(t *testing.T) {
	// The code of sub sits at physical $30000, mapped at logical $10000.
	m := run(t, prologue+`
	JSR	$10000
`+epilogue, func(m *machine) {
		code := []byte{0x70, 0x2a, 0x4e, 0x75} // MOVEQ #42,D0; RTS
		for i, b := range code {
			m.ram.Write(m68kemu.Byte, 0x30000+uint32(i), uint32(b))
		}
	})
	m.expectPC(t, "done")
	if d0 := m.cpu.Registers().D[0]; d0 != 42 {
		t.Fatalf("D0 = %d, want 42 from the code at physical $30000", d0)
	}
}

func TestPageFaultRestart(t *testing.T) {
	// The bus error handler tests the faulting address, maps the page and
	// returns; RTE runs the faulting instruction again.
	m := run(t, prologue+`
	MOVE.L	$11008,D1
	BRA	done
berr:	MOVE.L	16(A7),A0		; data fault address of the format $B frame
	PTESTR	#5,(A0),#7
	PMOVE	MMUSR,status
	MOVE.L	#$11001,$20144		; map the page
	PFLUSHA
	RTE
status:	DC.W	0
`+epilogue, func(m *machine) { m.ram.Write(m68kemu.Long, 0x11008, 0xbeef) })
	m.expectPC(t, "done")
	if d1 := m.cpu.Registers().D[1]; d1 != 0xbeef {
		t.Errorf("D1 = %x, want beef after the restart", d1)
	}
	if status := uint16(m.read(m68kemu.Word, m.labels["status"])); status != mmu.SRInvalid|2 {
		t.Errorf("MMUSR = %04x, want I and two levels", status)
	}
}

func TestWriteProtect(t *testing.T) {
	m := run(t, prologue+`
	MOVE.L	$12000,D0
	MOVE.L	D0,$12000
`+epilogue, func(m *machine) { m.ram.Write(m68kemu.Long, protected, 7) })
	m.expectPC(t, "berr")
	if d0 := m.cpu.Registers().D[0]; d0 != 7 {
		t.Fatalf("reading a write-protected page: D0 = %d, want 7", d0)
	}
	if d := m.read(m68kemu.Long, pageDescriptor(protected>>12)); d&0x10 != 0 {
		t.Errorf("descriptor %x: a refused write set M", d)
	}
}

func TestPTestAndPFlush(t *testing.T) {
	m := run(t, prologue+`
	LEA	$10000,A0
	TST.L	(A0)
	PTESTR	#5,(A0),#0		; in the ATC
	PMOVE	MMUSR,st0
	PTESTR	#1,(A0),#0		; another function code: not in the ATC
	PMOVE	MMUSR,st1
	PTESTR	#5,(A0),#7,A1
	PMOVE	MMUSR,st2
	MOVE.L	#$10001,$20140		; map it one-to-one again
	MOVE.L	(A0),D0			; still the cached translation
	PFLUSH	#5,#7,(A0)
	MOVE.L	(A0),D1
	BRA	done
	ALIGN	2
st0:	DC.W	0
st1:	DC.W	0
st2:	DC.W	0
`+epilogue, func(m *machine) {
		m.ram.Write(m68kemu.Long, 0x30000, 0x3)
		m.ram.Write(m68kemu.Long, 0x10000, 0x1)
	})
	m.expectPC(t, "done")
	if st := uint16(m.read(m68kemu.Word, m.labels["st0"])); st != 0 {
		t.Errorf("PTEST level 0 of a cached page: MMUSR %04x, want 0", st)
	}
	if st := uint16(m.read(m68kemu.Word, m.labels["st1"])); st != mmu.SRInvalid {
		t.Errorf("PTEST level 0 of an uncached page: MMUSR %04x, want I", st)
	}
	if st := uint16(m.read(m68kemu.Word, m.labels["st2"])); st != 2 {
		t.Errorf("PTEST level 7: MMUSR %04x, want 2 levels", st)
	}
	regs := m.cpu.Registers()
	if regs.A[1] != pageDescriptor(remapped>>12) {
		t.Errorf("PTEST A1 = %x, want the page descriptor at %x", regs.A[1], pageDescriptor(remapped>>12))
	}
	if regs.D[0] != 3 || regs.D[1] != 1 {
		t.Errorf("D0=%d D1=%d, want 3 from the cached translation, then 1 after PFLUSH", regs.D[0], regs.D[1])
	}
}

func TestPLoad(t *testing.T) {
	m := run(t, prologue+`
	LEA	$5000,A0
	PLOADW	#5,(A0)
	PTESTR	#5,(A0),#0
	PMOVE	MMUSR,st
	BRA	done
	ALIGN	2
st:	DC.W	0
`+epilogue, nil)
	m.expectPC(t, "done")
	if d := m.read(m68kemu.Long, pageDescriptor(5)); d != 0x5000|0x18|1 {
		t.Errorf("descriptor after PLOADW = %x, want U and M", d)
	}
	if st := uint16(m.read(m68kemu.Word, m.labels["st"])); st != mmu.SRModified {
		t.Errorf("MMUSR %04x, want M from the loaded entry", st)
	}
}

func TestTransparentTranslation(t *testing.T) {
	// TT0 maps $00xxxxxx for every function code, so $10000 is physical.
	m := run(t, prologue+`
	PMOVE	tt,TT0
	MOVE.L	$10000,D0
	MOVE.L	$11000,D1		; invalid in the tables
	BRA	done
	ALIGN	4
tt:	DC.L	$00008107
`+epilogue, func(m *machine) {
		m.ram.Write(m68kemu.Long, 0x10000, 5)
		m.ram.Write(m68kemu.Long, 0x11000, 6)
	})
	m.expectPC(t, "done")
	if regs := m.cpu.Registers(); regs.D[0] != 5 || regs.D[1] != 6 {
		t.Fatalf("D0=%d D1=%d, want 5 and 6 from untranslated addresses", regs.D[0], regs.D[1])
	}
}

func TestEarlyTermination(t *testing.T) {
	// A[1] is a page descriptor that maps the 256 KB from $40000 to
	// physical $00000.
	m := run(t, prologue+`
	MOVE.L	$45004,D0
`+epilogue, func(m *machine) {
		m.ram.Write(m68kemu.Long, tableA+4, 0x00000|1)
		m.ram.Write(m68kemu.Long, 0x5004, 9)
	})
	m.expectPC(t, "done")
	if d0 := m.cpu.Registers().D[0]; d0 != 9 {
		t.Fatalf("D0 = %d, want 9 from physical $5004", d0)
	}
}

func TestLimitAndSupervisorOnly(t *testing.T) {
	// CRP limits the A index to 0, so $40000 is out of bounds.
	m := run(t, prologue+`
	PMOVE	crp2,CRP
	MOVE.L	$40000,D0
	BRA	done
	ALIGN	4
crp2:	DC.L	$00000002,$20000
`+epilogue, nil)
	m.expectPC(t, "berr")

	// A long-format A table whose only entry is supervisor-only: user
	// accesses take a bus error.
	m = run(t, prologue+`
	PMOVE	crp3,CRP
	MOVE.L	$5000,D0
	MOVE	#0,SR
	MOVE.L	$5000,D1
	BRA	done
	ALIGN	4
crp3:	DC.L	$7FFF0003,$21000
`+epilogue, func(m *machine) {
		m.ram.Write(m68kemu.Long, 0x21000, 0x7fff0100|2) // S, valid short
		m.ram.Write(m68kemu.Long, 0x21004, tableB)
		m.ram.Write(m68kemu.Long, 0x5000, 1)
	})
	m.expectPC(t, "berr")
	if d0 := m.cpu.Registers().D[0]; d0 != 1 {
		t.Errorf("supervisor read: D0 = %d, want 1", d0)
	}
}

func TestSupervisorRootPointer(t *testing.T) {
	// With SRE, supervisor accesses use SRP, whose tree maps $5000 to
	// physical $31000; user accesses keep using CRP.
	m := run(t, prologue+`
	PMOVE	srp,SRP
	PMOVE	tcs,TC
	MOVE.L	$5000,D0
	MOVEQ	#1,D2
	MOVEC	D2,SFC
	MOVES.L	$5000,D3		; user data space: through CRP
	MOVE.L	#$7000,A0
	MOVE	A0,USP
	MOVE	#0,SR
	MOVE.L	$5000,D1
	BRA	done
	ALIGN	4
srp:	DC.L	$7FFF0002,$22000
tcs:	DC.L	$82C86600
`+epilogue, func(m *machine) {
		m.ram.Write(m68kemu.Long, 0x22000, 0x22100|2)
		for page := uint32(0); page < 64; page++ {
			m.ram.Write(m68kemu.Long, 0x22100+page*4, page<<12|1)
		}
		m.ram.Write(m68kemu.Long, 0x22100+5*4, 0x31000|1)
		m.ram.Write(m68kemu.Long, 0x31000, 1)
		m.ram.Write(m68kemu.Long, 0x5000, 2)
	})
	m.expectPC(t, "done")
	if regs := m.cpu.Registers(); regs.D[0] != 1 || regs.D[1] != 2 || regs.D[3] != 2 {
		t.Fatalf("D0=%d D1=%d D3=%d, want 1 through SRP and 2 through CRP", regs.D[0], regs.D[1], regs.D[3])
	}
}

func TestPageCrossingAccess(t *testing.T) {
	// A long at $FFFE straddles page 0 and page $10, which TIB maps apart:
	// the first word comes from physical $FFFE, the second from $30000.
	m := run(t, prologue+`
	MOVE.L	$FFFE,D0
`+epilogue, func(m *machine) {
		m.ram.Write(m68kemu.Word, 0xfffe, 0x1234)
		m.ram.Write(m68kemu.Word, 0x30000, 0x5678)
	})
	m.expectPC(t, "done")
	if d0 := m.cpu.Registers().D[0]; d0 != 0x12345678 {
		t.Fatalf("D0 = %x, want 12345678", d0)
	}
}

func TestConfigurationError(t *testing.T) {
	m := run(t, `
	ORG	$2000
	PMOVE	bad,TC
	BRA	done
	ALIGN	4
bad:	DC.L	$80C86000		; TIA+TIB+PS+IS = 26
done:	BRA	done
`, nil)
	m.expectPC(t, "config")
	if tc := m.mmu.Register(mmu.TC); tc != 0x00c86000 {
		t.Errorf("TC = %x, want the value with E clear", tc)
	}
	m = run(t, `
	ORG	$2000
	PMOVE	bad,CRP
	BRA	done
	ALIGN	4
bad:	DC.L	$7FFF0000,$20000
done:	BRA	done
`, nil)
	m.expectPC(t, "config")
}

func TestPrivileged(t *testing.T) {
	m := run(t, `
	ORG	$2000
	MOVE	#0,SR
	PFLUSHA
done:	BRA	done
`, nil)
	m.expectPC(t, "priv")
}

func TestMMUOnlyOnMC68030(t *testing.T) {
	for _, tt := range []struct {
		model m68kemu.Model
		mmu   bool
	}{{m68kemu.M68020, false}, {m68kemu.M68030, true}} {
		cpu, err := m68kemu.NewCPU(m68kemu.NewBus(m68kemu.NewRAM(0, 0x10000)), m68kemu.WithModel(tt.model))
		if err != nil {
			t.Fatal(err)
		}
		if (cpu.MMU() != nil) != tt.mmu {
			t.Errorf("%v: MMU %v, want %v", tt.model, cpu.MMU() != nil, tt.mmu)
		}
	}
	_, err := m68kemu.NewCPU(m68kemu.NewBus(m68kemu.NewRAM(0, 0x10000)), m68kemu.WithModel(m68kemu.M68030),
		m68kemu.WithCoprocessor(0, nil))
	if !errors.Is(err, m68kemu.ErrCoprocessorUnsupported) {
		t.Errorf("coprocessor 0 on an MC68030: %v, want ErrCoprocessorUnsupported", err)
	}
}
