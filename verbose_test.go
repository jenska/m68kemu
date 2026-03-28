package m68kemu

import (
	"bytes"
	"strings"
	"testing"
)

func TestDisassembleInstructionUsesPeek(t *testing.T) {
	bus := newDebugPeekBus()
	if err := bus.Write(Word, 0x2000, 0x7005); err != nil {
		t.Fatalf("seed opcode: %v", err)
	}

	line, err := DisassembleInstruction(bus, 0x2000)
	if err != nil {
		t.Fatalf("disassemble instruction: %v", err)
	}

	if bus.readCount != 0 {
		t.Fatalf("DisassembleInstruction used Read: got %d", bus.readCount)
	}
	if bus.peekCount == 0 {
		t.Fatalf("DisassembleInstruction did not use Peek")
	}
	if line.Assembly != "MOVEQ #5, D0" {
		t.Fatalf("unexpected assembly: got %q", line.Assembly)
	}
	if len(line.Bytes) != 2 || line.Bytes[0] != 0x70 || line.Bytes[1] != 0x05 {
		t.Fatalf("unexpected instruction bytes: % x", line.Bytes)
	}
}

func TestDisassembleMemoryRangeDecodesSequentialInstructions(t *testing.T) {
	cpu, ram := newEnvironment(t)

	code := assemble(t, "MOVEQ #5,D0\nNOP\n")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program byte: %v", err)
		}
	}

	lines, err := DisassembleMemoryRange(cpu.bus, cpu.regs.PC, uint32(len(code)))
	if err != nil {
		t.Fatalf("disassemble range: %v", err)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 disassembly lines, got %d", len(lines))
	}
	if lines[0].Assembly != "MOVEQ #5, D0" {
		t.Fatalf("unexpected first line: %q", lines[0].Assembly)
	}
	if lines[1].Assembly != "NOP" {
		t.Fatalf("unexpected second line: %q", lines[1].Assembly)
	}
}

func TestVerboseLoggerIncludesDisassemblyAndRanges(t *testing.T) {
	cpu, ram := newEnvironment(t)

	code := assemble(t, "MOVEQ #5,D0\nNOP\n")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program byte: %v", err)
		}
	}

	var output bytes.Buffer
	logger := NewVerboseLogger(cpu, cpu.bus, &output, VerboseLoggerOptions{
		IncludeRegisters: true,
		IncludeCycles:    true,
		MemoryRanges: []MemoryRange{
			{Start: cpu.regs.PC, Length: uint32(len(code)), Label: "program"},
		},
	})
	cpu.SetTracer(logger.Trace)

	if err := cpu.Step(); err != nil {
		t.Fatalf("step failed: %v", err)
	}

	text := output.String()
	if !strings.Contains(text, "TRACE PC 00002000 OPCODE 7005 DELTA 4 CYCLES 4 MOVEQ #5, D0") {
		t.Fatalf("verbose log missing trace header, got:\n%s", text)
	}
	if !strings.Contains(text, "SR 2700 PC 00002002") {
		t.Fatalf("verbose log missing register dump, got:\n%s", text)
	}
	if !strings.Contains(text, "DISASM RANGE program") {
		t.Fatalf("verbose log missing range label, got:\n%s", text)
	}
	if !strings.Contains(text, "00002000: 70 05                   MOVEQ #5, D0") {
		t.Fatalf("verbose log missing first disassembly line, got:\n%s", text)
	}
	if !strings.Contains(text, "00002002: 4e 71                   NOP") {
		t.Fatalf("verbose log missing second disassembly line, got:\n%s", text)
	}
}
