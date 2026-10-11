package m68kemu

import (
	"fmt"
	"sort"
	"testing"
)

// TestOpcodeValidityMatchesMotorola executes every opcode word and expects an
// illegal-instruction (or line A/F) exception exactly for the encodings the
// MC68000 does not have.
func TestOpcodeValidityMatchesMotorola(t *testing.T) {
	ram := NewRAM(0, 0x1000000)
	bus := NewBus(ram)
	processor, err := NewCPU(bus, WithDeferredReset())
	if err != nil {
		t.Fatalf("create CPU: %v", err)
	}
	illegal := false
	processor.SetHooks(Hooks{Exception: func(info ExceptionInfo) {
		switch info.Vector {
		case XIllegal, XLineA, XLineF:
			illegal = true
		}
	}})
	ext := []byte{0x00, 0x12, 0x00, 0x34, 0x00, 0x56, 0x00, 0x78, 0x00, 0x9A, 0x00, 0xBC}
	groups := map[string][]string{}
	for op := 0; op < 0x10000; op++ {
		_ = ram.Write(Long, 0, 0x8000)
		_ = ram.Write(Long, 4, 0x1000)
		_ = ram.Write(Word, 0x1000, uint32(op))
		for i, b := range ext {
			_ = ram.Write(Byte, 0x1002+uint32(i), uint32(b))
		}
		if err := processor.Reset(); err != nil {
			t.Fatalf("reset: %v", err)
		}
		illegal = false
		_ = processor.Step()
		valid := valid68000(uint16(op))
		if op == 0x4AFC { // ILLEGAL is a valid instruction that raises the exception
			valid = false
		}
		if valid == !illegal {
			continue
		}
		kind := "executes invalid"
		if valid {
			kind = "rejects valid"
		}
		key := fmt.Sprintf("%s %X%02o", kind, op>>12, (op>>6)&0x3F)
		groups[key] = append(groups[key], fmt.Sprintf("%04x", op))
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ops := groups[k]
		shown := ops
		if len(shown) > 8 {
			shown = shown[:8]
		}
		t.Errorf("%s (line, bits 11-6 in octal): %d opcodes, e.g. %v", k, len(ops), shown)
	}
}
