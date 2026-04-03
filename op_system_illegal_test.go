package m68kemu

import "testing"

func TestIllegalInstructionTriggersException(t *testing.T) {
	cpu, ram := newEnvironment(t)

	handler := uint32(0xbeef)
	if err := ram.Write(Long, uint32(XIllegal<<2), handler); err != nil {
		t.Fatalf("vector write: %v", err)
	}

	code := assemble(t, "ILLEGAL\n")
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	opcode, err := cpu.fetchOpcode()
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("exec: %v", err)
	}

	if cpu.regs.PC != handler {
		t.Fatalf("expected PC to jump to handler %08x got %08x", handler, cpu.regs.PC)
	}
}
