package m68kemu

import (
	"strings"
	"testing"
)

type debugPeekBus struct {
	mem       map[uint32]uint8
	readCount int
	peekCount int
}

func newDebugPeekBus() *debugPeekBus {
	return &debugPeekBus{mem: make(map[uint32]uint8)}
}

func (b *debugPeekBus) Read(size Size, address uint32) (uint32, error) {
	b.readCount++
	return b.readMemory(size, address)
}

func (b *debugPeekBus) Peek(size Size, address uint32) (uint32, error) {
	b.peekCount++
	return b.readMemory(size, address)
}

func (b *debugPeekBus) Write(size Size, address uint32, value uint32) error {
	switch size {
	case Byte:
		b.mem[address] = uint8(value)
	case Word:
		b.mem[address] = uint8(value >> 8)
		b.mem[address+1] = uint8(value)
	case Long:
		b.mem[address] = uint8(value >> 24)
		b.mem[address+1] = uint8(value >> 16)
		b.mem[address+2] = uint8(value >> 8)
		b.mem[address+3] = uint8(value)
	}
	return nil
}

func (b *debugPeekBus) Reset() {}

func (b *debugPeekBus) readMemory(size Size, address uint32) (uint32, error) {
	switch size {
	case Byte:
		return uint32(b.mem[address]), nil
	case Word:
		return uint32(b.mem[address])<<8 | uint32(b.mem[address+1]), nil
	case Long:
		return uint32(b.mem[address])<<24 | uint32(b.mem[address+1])<<16 | uint32(b.mem[address+2])<<8 | uint32(b.mem[address+3]), nil
	default:
		return 0, nil
	}
}

func TestCPUStringIncludesCurrentDisassembly(t *testing.T) {
	cpu, ram := newEnvironment(t)

	code := assemble(t, "MOVEQ #5,D0\n")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program byte: %v", err)
		}
	}

	text := cpu.String()

	if !strings.Contains(text, "DISASM 00002000: MOVEQ #5, D0") {
		t.Fatalf("CPU.String missing disassembly, got:\n%s", text)
	}
	if !strings.Contains(text, "SR 2700 PC 00002000") {
		t.Fatalf("CPU.String missing register dump, got:\n%s", text)
	}
}

func TestCPUStringUsesPeekInsteadOfLiveReads(t *testing.T) {
	bus := newDebugPeekBus()
	if err := bus.Write(Long, 0, 0x1000); err != nil {
		t.Fatalf("seed SSP: %v", err)
	}
	if err := bus.Write(Long, 4, 0x2000); err != nil {
		t.Fatalf("seed PC: %v", err)
	}
	if err := bus.Write(Word, 0x2000, 0x7005); err != nil {
		t.Fatalf("seed opcode: %v", err)
	}

	processor, err := NewCPU(bus)
	if err != nil {
		t.Fatalf("create CPU: %v", err)
	}

	impl, ok := processor.(*cpu)
	if !ok {
		t.Fatalf("unexpected CPU implementation %T", processor)
	}

	readsAfterReset := bus.readCount
	text := impl.String()

	if bus.readCount != readsAfterReset {
		t.Fatalf("CPU.String used Read: got %d reads after reset, want %d", bus.readCount, readsAfterReset)
	}
	if bus.peekCount == 0 {
		t.Fatalf("CPU.String did not use Peek")
	}
	if !strings.Contains(text, "DISASM 00002000: MOVEQ #5, D0") {
		t.Fatalf("CPU.String missing disassembly, got:\n%s", text)
	}
}
