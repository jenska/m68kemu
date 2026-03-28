package m68kemu

import (
	"errors"
	"testing"

	asm "github.com/jenska/m68kasm"
)

type assembledProgram struct {
	Bytes   []byte
	Listing []asm.ListingEntry
}

type stepTestHelper struct {
	tb  testing.TB
	cpu *cpu
	ram *RAM
}

type loadedProgram struct {
	base uint32
	assembledProgram
}

func newStepTestHelper(tb testing.TB) *stepTestHelper {
	tb.Helper()
	cpu, ram := newEnvironment(tb)
	return &stepTestHelper{tb: tb, cpu: cpu, ram: ram}
}

func (h *stepTestHelper) Load(address uint32, data []byte) {
	h.tb.Helper()
	for i, value := range data {
		if err := h.ram.Write(Byte, address+uint32(i), uint32(value)); err != nil {
			h.tb.Fatalf("failed to write byte to %08x: %v", address+uint32(i), err)
		}
	}
}

func (h *stepTestHelper) LoadProgram(data []byte) {
	h.tb.Helper()
	h.Load(h.cpu.regs.PC, data)
}

func (h *stepTestHelper) LoadAssembly(src string) loadedProgram {
	h.tb.Helper()
	program := assembleProgram(h.tb, src)
	h.LoadProgram(program.Bytes)
	return loadedProgram{base: h.cpu.regs.PC, assembledProgram: program}
}

func (h *stepTestHelper) SetRegisters(update func(*Registers)) {
	h.tb.Helper()
	update(&h.cpu.regs)
}

func (h *stepTestHelper) RunInstructions(count uint64) {
	h.tb.Helper()
	if err := h.cpu.RunInstructions(count); err != nil {
		h.tb.Fatalf("run instructions failed: %v", err)
	}
}

func (h *stepTestHelper) RunCycles(cycles uint64) {
	h.tb.Helper()
	if err := h.cpu.RunCycles(cycles); err != nil {
		h.tb.Fatalf("run cycles failed: %v", err)
	}
}

func (h *stepTestHelper) AssertState(check func(*testing.T, *cpu, *RAM)) {
	h.tb.Helper()
	tester, ok := h.tb.(*testing.T)
	if !ok {
		h.tb.Fatalf("AssertState requires *testing.T, got %T", h.tb)
	}
	check(tester, h.cpu, h.ram)
}

func (p assembledProgram) EntryForLine(tb testing.TB, line int) asm.ListingEntry {
	tb.Helper()
	for _, entry := range p.Listing {
		if entry.Line == line {
			return entry
		}
	}
	tb.Fatalf("no listing entry for line %d", line)
	return asm.ListingEntry{}
}

func (p assembledProgram) BytesForLine(tb testing.TB, line int) []byte {
	tb.Helper()
	entry := p.EntryForLine(tb, line)
	return append([]byte(nil), entry.Bytes...)
}

func (p loadedProgram) PCForLine(tb testing.TB, line int) uint32 {
	tb.Helper()
	entry := p.EntryForLine(tb, line)
	return p.base + entry.PC
}

func assembleProgram(tb testing.TB, source string) assembledProgram {
	tb.Helper()
	code, listing, err := asm.AssembleStringWithListing(source)
	if err != nil {
		tb.Fatalf("Assembler failed: %v", err)
	}
	return assembledProgram{Bytes: code, Listing: listing}
}

func expectBusError(t *testing.T, err error) {
	t.Helper()
	var be BusError
	if err == nil || !errors.As(err, &be) {
		t.Fatalf("expected BusError, got %v", err)
	}
}

func expectAddressError(t *testing.T, err error) {
	t.Helper()
	var ae AddressError
	if err == nil || !errors.As(err, &ae) {
		t.Fatalf("expected BusError, got %v", err)
	}
}
