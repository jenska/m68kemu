package m68kemu

import "testing"

// TestNoOpcodePanics executes every opcode word once. Invalid encodings must
// raise an exception, never crash the emulator.
func TestNoOpcodePanics(t *testing.T) {
	ram := NewRAM(0, 0x1000000)
	bus := NewBus(ram)
	cpu, err := NewCPU(bus, WithDeferredReset())
	if err != nil {
		t.Fatalf("create CPU: %v", err)
	}
	ext := []byte{0x00, 0x12, 0x00, 0x34, 0x00, 0x56, 0x00, 0x78, 0x00, 0x9A, 0x00, 0xBC}
	for op := 0; op < 0x10000; op++ {
		_ = ram.Write(Long, 0, 0x8000)
		_ = ram.Write(Long, 4, 0x1000)
		_ = ram.Write(Word, 0x1000, uint32(op))
		for i, b := range ext {
			_ = ram.Write(Byte, 0x1002+uint32(i), uint32(b))
		}
		if err := cpu.Reset(); err != nil {
			t.Fatalf("reset: %v", err)
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("opcode %04x panics: %v", op, r)
				}
			}()
			_ = cpu.Step()
		}()
	}
}
