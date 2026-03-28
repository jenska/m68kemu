package m68kemu

import (
	"fmt"
	"io"
	"strings"

	"github.com/jenska/m68kdasm"
)

const maxDisassemblyBytes = 16

// DisassemblyLine captures one decoded instruction plus the backing bytes.
type DisassemblyLine struct {
	Address  uint32
	Bytes    []byte
	Assembly string
}

// MemoryRange describes a region of memory to disassemble for debug output.
type MemoryRange struct {
	Start  uint32
	Length uint32
	Label  string
}

// VerboseLoggerOptions controls how much detail a VerboseLogger emits.
type VerboseLoggerOptions struct {
	IncludeRegisters bool
	IncludeCycles    bool
	MemoryRanges     []MemoryRange
}

// VerboseLogger formats trace callbacks with disassembly and optional state dumps.
type VerboseLogger struct {
	cpu     CPU
	bus     AddressBus
	writer  io.Writer
	options VerboseLoggerOptions
}

// String renders a disassembly line with its bytes for human-readable logs.
func (line DisassemblyLine) String() string {
	byteText := formatDisassemblyBytes(line.Bytes)
	if byteText == "" {
		return fmt.Sprintf("%08x: %s", line.Address, line.Assembly)
	}
	return fmt.Sprintf("%08x: %-23s %s", line.Address, byteText, line.Assembly)
}

// NewVerboseLogger builds a trace callback helper that writes detailed execution logs.
func NewVerboseLogger(cpu CPU, bus AddressBus, writer io.Writer, options VerboseLoggerOptions) *VerboseLogger {
	if writer == nil {
		writer = io.Discard
	}
	return &VerboseLogger{
		cpu:     cpu,
		bus:     bus,
		writer:  writer,
		options: options,
	}
}

// Trace implements TraceCallback for use with CPU.SetTracer.
func (logger *VerboseLogger) Trace(info TraceInfo) {
	if logger == nil {
		return
	}

	var text strings.Builder
	fmt.Fprintf(&text, "TRACE PC %08x", info.PC&0xffffff)
	if info.Opcode != 0 || len(info.Bytes) >= 2 {
		fmt.Fprintf(&text, " OPCODE %04x", info.Opcode)
	}
	if info.CycleDelta != 0 {
		fmt.Fprintf(&text, " DELTA %d", info.CycleDelta)
	}
	if logger.options.IncludeCycles && logger.cpu != nil {
		fmt.Fprintf(&text, " CYCLES %d", logger.cpu.Cycles())
	}
	if line, err := traceDisassemblyLine(logger.bus, info); err == nil {
		fmt.Fprintf(&text, " %s", line.Assembly)
	} else {
		fmt.Fprintf(&text, " <disassembly unavailable: %v>", err)
	}
	text.WriteByte('\n')

	if logger.options.IncludeRegisters {
		text.WriteString(info.Registers.String())
	}

	for _, memRange := range logger.options.MemoryRanges {
		fmt.Fprintf(&text, "DISASM RANGE %s\n", formatMemoryRangeLabel(memRange))
		lines, err := DisassembleMemoryRange(logger.bus, memRange.Start, memRange.Length)
		if err != nil {
			fmt.Fprintf(&text, "  <error: %v>\n", err)
			continue
		}
		for _, line := range lines {
			fmt.Fprintf(&text, "  %s\n", line.String())
		}
	}

	_, _ = io.WriteString(logger.writer, text.String())
}

// DisassembleInstruction decodes one instruction at the given bus address.
func DisassembleInstruction(bus AddressBus, address uint32) (DisassemblyLine, error) {
	inst, err := decodeInstruction(bus, address)
	if err != nil {
		return DisassemblyLine{}, err
	}

	return DisassemblyLine{
		Address:  inst.Address,
		Bytes:    append([]byte(nil), inst.Bytes...),
		Assembly: inst.Assembly(),
	}, nil
}

// DisassembleMemoryRange decodes instructions sequentially until the range is covered.
func DisassembleMemoryRange(bus AddressBus, start uint32, length uint32) ([]DisassemblyLine, error) {
	start &= 0xffffff
	if length == 0 {
		return nil, nil
	}

	lines := make([]DisassemblyLine, 0)
	address := start
	remaining := length
	for remaining > 0 {
		line, err := DisassembleInstruction(bus, address)
		if err != nil {
			return lines, err
		}
		lines = append(lines, line)

		size := uint32(len(line.Bytes))
		if size == 0 {
			return lines, fmt.Errorf("decoded zero-length instruction at %08x", address)
		}

		address = (address + size) & 0xffffff
		if size >= remaining {
			break
		}
		remaining -= size
	}

	return lines, nil
}

func decodeInstruction(bus AddressBus, address uint32) (*m68kdasm.Instruction, error) {
	peeker, ok := bus.(interface {
		Peek(Size, uint32) (uint32, error)
	})
	if !ok {
		return nil, fmt.Errorf("address bus does not support Peek")
	}

	address &= 0xffffff
	data := make([]byte, 0, maxDisassemblyBytes)
	for i := uint32(0); i < maxDisassemblyBytes; i++ {
		value, err := peeker.Peek(Byte, (address+i)&0xffffff)
		if err != nil {
			break
		}
		data = append(data, byte(value))
	}

	if len(data) < 2 {
		return nil, fmt.Errorf("insufficient data for opcode at address %08x", address)
	}

	return m68kdasm.Decode(data, address)
}

func traceInstructionBytes(bus AddressBus, address uint32, opcode uint16) []byte {
	if inst, err := decodeInstruction(bus, address); err == nil && len(inst.Bytes) != 0 {
		return append([]byte(nil), inst.Bytes...)
	}
	return []byte{byte(opcode >> 8), byte(opcode)}
}

func traceDisassemblyLine(bus AddressBus, info TraceInfo) (DisassemblyLine, error) {
	if len(info.Bytes) >= 2 {
		if inst, err := m68kdasm.Decode(append([]byte(nil), info.Bytes...), info.PC); err == nil {
			return DisassemblyLine{
				Address:  inst.Address,
				Bytes:    append([]byte(nil), inst.Bytes...),
				Assembly: inst.Assembly(),
			}, nil
		}
	}
	return DisassembleInstruction(bus, info.PC)
}
func formatDisassemblyBytes(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	var text strings.Builder
	for i, value := range data {
		if i > 0 {
			text.WriteByte(' ')
		}
		fmt.Fprintf(&text, "%02x", value)
	}
	return text.String()
}

func formatMemoryRangeLabel(memRange MemoryRange) string {
	if memRange.Label != "" {
		return memRange.Label
	}
	if memRange.Length == 0 {
		return fmt.Sprintf("%08x", memRange.Start&0xffffff)
	}
	end := (memRange.Start + memRange.Length - 1) & 0xffffff
	return fmt.Sprintf("%08x-%08x", memRange.Start&0xffffff, end)
}
