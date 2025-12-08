package emu

import "testing"

func TestEARegisterDirectReadWrite(t *testing.T) {
	cpu, _ := newEnvironment(t)
	cpu.ir = 0x0001 // y=1 selects D1
	cpu.regs.D[1] = -0x10000

	ea, err := (&eaRegister{areg: dy}).init(cpu, Word)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	if got, err := ea.read(); err != nil || got != 0x0000 {
		t.Fatalf("unexpected read got=%04x err=%v", got, err)
	}

	if err := ea.write(0x1234); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if cpu.regs.D[1] != -0xEDCC {
		t.Fatalf("write masked upper bits incorrectly: %08x", uint32(cpu.regs.D[1]))
	}
}

func TestEAPostIncrementUpdatesAddressAndRegister(t *testing.T) {
	cpu, _ := newEnvironment(t)
	cpu.ir = 0x0000 // y=0 selects A0
	cpu.regs.A[0] = 0x2000

	ea, err := (&eaPostIncrement{eaRegisterIndirect{eaRegister{areg: ay}, 0}}).init(cpu, Word)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	if addr := ea.computedAddress(); addr != 0x2000 {
		t.Fatalf("address mismatch: got %04x", addr)
	}
	if cpu.regs.A[0] != 0x2002 {
		t.Fatalf("post-increment did not advance register: %04x", cpu.regs.A[0])
	}
}

func TestEAPreDecrementUsesUpdatedAddress(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.ir = 0x0000 // y=0 selects A0
	cpu.regs.A[0] = 0x2000

	ea, err := (&eaPreDecrement{eaRegisterIndirect{eaRegister{areg: ay}, 0}}).init(cpu, Byte)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	if addr := ea.computedAddress(); addr != 0x1FFF {
		t.Fatalf("address mismatch: got %04x", addr)
	}
	if cpu.regs.A[0] != 0x1FFF {
		t.Fatalf("pre-decrement did not update register: %04x", cpu.regs.A[0])
	}

	if err := ea.write(0xAA); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if got, err := ram.ReadByteFrom(0x1FFF); err != nil || got != 0xAA {
		t.Fatalf("unexpected memory value %02x err=%v", got, err)
	}
}

func TestEADisplacementUsesOffset(t *testing.T) {
	cpu, _ := newEnvironment(t)
	cpu.ir = 0x0000 // y=0 selects A0
	cpu.regs.A[0] = 0x1000

	if err := cpu.bus.WriteWordTo(cpu.regs.PC, 0xFFFE); err != nil {
		t.Fatalf("failed to write displacement: %v", err)
	}

	ea, err := (&eaDisplacement{eaRegisterIndirect{eaRegister{areg: ay}, 0}}).init(cpu, Word)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	if addr := ea.computedAddress(); addr != 0x0FFE {
		t.Fatalf("computed address mismatch: got %04x", addr)
	}
	if cpu.regs.PC != 0x2002 {
		t.Fatalf("PC not advanced after displacement read: %04x", cpu.regs.PC)
	}
}

func TestEAPCDisplacementUsesProgramCounter(t *testing.T) {
	cpu, _ := newEnvironment(t)

	if err := cpu.bus.WriteWordTo(cpu.regs.PC, 0x0004); err != nil {
		t.Fatalf("failed to write displacement: %v", err)
	}

	ea, err := (&eaPCDisplacement{eaDisplacement{eaRegisterIndirect{eaRegister{areg: nil}, 0}}}).init(cpu, Word)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	if addr := ea.computedAddress(); addr != 0x2006 {
		t.Fatalf("computed PC-relative address mismatch: got %04x", addr)
	}
	if cpu.regs.PC != 0x2002 {
		t.Fatalf("PC not advanced after displacement read: %04x", cpu.regs.PC)
	}
}

func TestEAAbsoluteWordAndLong(t *testing.T) {
	cpu, _ := newEnvironment(t)

	if err := cpu.bus.WriteWordTo(cpu.regs.PC, 0x1234); err != nil {
		t.Fatalf("write absolute word failed: %v", err)
	}
	eaWord, err := (&eaAbsolute{eaSize: Word}).init(cpu, Word)
	if err != nil {
		t.Fatalf("init word failed: %v", err)
	}
	if addr := eaWord.computedAddress(); addr != 0x00001234 {
		t.Fatalf("absolute word address mismatch: %08x", addr)
	}

	if err := cpu.bus.WriteLongTo(cpu.regs.PC, 0xAABBCCDD); err != nil {
		t.Fatalf("write absolute long failed: %v", err)
	}
	eaLong, err := (&eaAbsolute{eaSize: Long}).init(cpu, Long)
	if err != nil {
		t.Fatalf("init long failed: %v", err)
	}
	if addr := eaLong.computedAddress(); addr != 0xAABBCCDD {
		t.Fatalf("absolute long address mismatch: %08x", addr)
	}
}

func TestEAImmediateReadsFromPC(t *testing.T) {
	cpu, _ := newEnvironment(t)

	if err := cpu.bus.WriteWordTo(cpu.regs.PC, 0x00FF); err != nil {
		t.Fatalf("write immediate failed: %v", err)
	}

	ea, err := (&eaImmediate{}).init(cpu, Word)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	if val, err := ea.read(); err != nil || val != 0x00FF {
		t.Fatalf("unexpected immediate value %04x err=%v", val, err)
	}
	if cpu.regs.PC != 0x2002 {
		t.Fatalf("PC not advanced after immediate read: %04x", cpu.regs.PC)
	}
}

func TestEAStatusRegisterReadWrite(t *testing.T) {
	cpu, _ := newEnvironment(t)
	cpu.regs.SR = 0xAAAA

	eaByte, err := (&eaStatusRegister{size: Byte}).init(cpu, Byte)
	if err != nil {
		t.Fatalf("init byte failed: %v", err)
	}
	if val, err := eaByte.read(); err != nil || val != 0xAA {
		t.Fatalf("byte read mismatch val=%04x err=%v", val, err)
	}
	if err := eaByte.write(0x55); err != nil {
		t.Fatalf("byte write failed: %v", err)
	}
	if cpu.regs.SR != 0xAA55 {
		t.Fatalf("byte write corrupted SR: %04x", cpu.regs.SR)
	}

	eaWord, err := (&eaStatusRegister{size: Word}).init(cpu, Word)
	if err != nil {
		t.Fatalf("init word failed: %v", err)
	}
	if err := eaWord.write(0x1234); err != nil {
		t.Fatalf("word write failed: %v", err)
	}
	if cpu.regs.SR != 0x1234 {
		t.Fatalf("word write mismatch: %04x", cpu.regs.SR)
	}
}

func TestEAIndirectIndexUsesIndexAndDisplacement(t *testing.T) {
	cpu, _ := newEnvironment(t)
	cpu.ir = 0x0000 // y=0 selects A0
	cpu.regs.A[0] = 0x3000
	cpu.regs.D[0] = 0x10

	if err := cpu.bus.WriteWordTo(cpu.regs.PC, 0x0801); err != nil {
		t.Fatalf("failed to write index extension: %v", err)
	}

	ea, err := (&eaIndirectIndex{eaRegisterIndirect{eaRegister{areg: ay}, 0}, ix68000}).init(cpu, Word)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	if addr := ea.computedAddress(); addr != 0x3011 {
		t.Fatalf("indexed address mismatch: %04x", addr)
	}
	if cpu.regs.PC != 0x2002 {
		t.Fatalf("PC not advanced after index extension: %04x", cpu.regs.PC)
	}
}
