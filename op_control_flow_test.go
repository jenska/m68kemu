package m68kemu

import (
	"fmt"
	"testing"
)

func TestSccSetsByteOnCondition(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.SR = srZero

	code := assemble(t, "SEQ D0\n")
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

	if got := cpu.regs.D[0] & 0xff; got != 0xff {
		t.Fatalf("expected 0xff got %02x", got)
	}

	cpu.regs.SR &^= srZero
	cpu.regs.PC = 0x2000
	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write code: %v", err)
		}
	}

	opcode, err = cpu.fetchOpcode()
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := cpu.executeInstruction(opcode); err != nil {
		t.Fatalf("exec: %v", err)
	}

	if got := cpu.regs.D[0] & 0xff; got != 0x00 {
		t.Fatalf("expected 0x00 got %02x", got)
	}
}

func TestDbccLoop(t *testing.T) {
	cpu, ram := newEnvironment(t)

	code := []byte{
		0x70, 0x01, // MOVEQ #1,D0
		0x51, 0xC8, 0xFF, 0xFE, // DBRA D0,-2
		0x4E, 0x71, // NOP
	}

	for i, b := range code {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write program: %v", err)
		}
	}

	endPC := cpu.regs.PC + uint32(len(code))
	for steps := 0; steps < 10 && cpu.regs.PC < endPC; steps++ {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step failed: %v", err)
		}
	}

	if cpu.regs.PC != endPC {
		t.Fatalf("DBF loop did not fall through to NOP, PC=%04x end=%04x", cpu.regs.PC, endPC)
	}
	if cpu.regs.D[0]&0xffff != 0xffff {
		t.Fatalf("DBRA should leave low word at -1, got %04x", cpu.regs.D[0]&0xffff)
	}
}

func TestJsrRts(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC

	program := assemble(t, `
        BRA.S main
inc:    ADD.L D1,D0
        RTS
main:   MOVEQ #1,D0
        MOVEQ #1,D1
        BSR.S inc
        NOP
`)

	for i, b := range program {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	endPC := startPC + uint32(len(program))
	for cpu.regs.PC < endPC {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step: %v", err)
		}
	}

	if cpu.regs.D[0] != 2 {
		t.Fatalf("D0=%d, want 2", cpu.regs.D[0])
	}
}

func TestJsrPushesReturnAddressAndJumps(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC

	program := []byte{
		0x4E, 0xB9, 0x00, 0x00, 0x20, 0x08, // JSR $00002008
		0x4E, 0x71, // NOP (return address)
		0x4E, 0x75, // RTS
	}
	for i, b := range program {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("step jsr: %v", err)
	}

	if cpu.regs.PC != startPC+8 {
		t.Fatalf("PC after JSR = %08x, want %08x", cpu.regs.PC, startPC+8)
	}

	if cpu.regs.A[7] != 0x0ffc {
		t.Fatalf("SP after JSR = %08x, want 00000ffc", cpu.regs.A[7])
	}

	returnAddr, err := ram.Read(Long, cpu.regs.A[7])
	if err != nil {
		t.Fatalf("read return address: %v", err)
	}
	if returnAddr != startPC+6 {
		t.Fatalf("stacked return address = %08x, want %08x", returnAddr, startPC+6)
	}
}

func TestBSRWordUsesExtensionWordAddressAsBase(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC

	program := []byte{
		0x61, 0x00, 0x00, 0x06, // BSR.W to 0x2008 when based on the extension word address
		0x4E, 0x71, // NOP (return address)
		0x4E, 0x71, // filler
		0x4E, 0x75, // RTS
	}
	for i, b := range program {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("step bsr.w: %v", err)
	}

	if cpu.regs.PC != startPC+8 {
		t.Fatalf("PC=%08x, want %08x", cpu.regs.PC, startPC+8)
	}
	if cpu.regs.A[7] != 0x0ffc {
		t.Fatalf("SP=%08x, want 00000ffc", cpu.regs.A[7])
	}
	returnAddr, err := ram.Read(Long, cpu.regs.A[7])
	if err != nil {
		t.Fatalf("read return address: %v", err)
	}
	if returnAddr != startPC+4 {
		t.Fatalf("return address=%08x, want %08x", returnAddr, startPC+4)
	}
}

func TestDBRAWordUsesExtensionWordAddressAsBase(t *testing.T) {
	cpu, ram := newEnvironment(t)
	startPC := cpu.regs.PC
	cpu.regs.D[0] = 1

	program := []byte{
		0x51, 0xC8, 0xFF, 0xFE, // DBRA D0,-2 -> branch back to the opcode when based on the extension word address
		0x4E, 0x71, // NOP
	}
	for i, b := range program {
		if err := ram.Write(Byte, startPC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("step dbra 1: %v", err)
	}
	if cpu.regs.PC != startPC {
		t.Fatalf("PC after first DBRA=%08x, want %08x", cpu.regs.PC, startPC)
	}
	if cpu.regs.D[0]&0xffff != 0 {
		t.Fatalf("D0 low word after first DBRA=%04x, want 0000", cpu.regs.D[0]&0xffff)
	}

	if err := cpu.Step(); err != nil {
		t.Fatalf("step dbra 2: %v", err)
	}
	if cpu.regs.PC != startPC+4 {
		t.Fatalf("PC after second DBRA=%08x, want %08x", cpu.regs.PC, startPC+4)
	}
	if cpu.regs.D[0]&0xffff != 0xffff {
		t.Fatalf("D0 low word after second DBRA=%04x, want ffff", cpu.regs.D[0]&0xffff)
	}
}

func TestJmp(t *testing.T) {
	cpu, ram := newEnvironment(t)

	jumpBytes := assemble(t, "JMP $3000\n")
	for i, b := range jumpBytes {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write jump: %v", err)
		}
	}

	payload := assemble(t, "MOVEQ #2,D0\nNOP\n")
	for i, b := range payload {
		if err := ram.Write(Byte, 0x3000+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write payload: %v", err)
		}
	}

	for i := range 3 {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}

	if cpu.regs.D[0] != 2 {
		t.Fatalf("D0=%d, want 2", cpu.regs.D[0])
	}
}

func TestMoveUsp(t *testing.T) {
	cpu, ram := newEnvironment(t)

	program := assemble(t, `
        MOVE.L #$4000,A0
        MOVE.L #$0,A1
        MOVE A0,USP
        MOVE USP,A1
        NOP
`)

	for i, b := range program {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	for i := range 5 {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}

	if cpu.regs.USP != 0x4000 {
		t.Fatalf("USP=%04x, want 0x4000", cpu.regs.USP)
	}
	if cpu.regs.A[1] != 0x4000 {
		t.Fatalf("A1=%04x, want 0x4000", cpu.regs.A[1])
	}
}

func TestMoveUspAllRegisterForms(t *testing.T) {
	for reg := range uint16(8) {
		t.Run(fmt.Sprintf("A%d", reg), func(t *testing.T) {
			cpu, _ := newEnvironment(t)
			cpu.regs.SR = srSupervisor

			source := uint32(0x1000 + reg*0x100)
			cpu.regs.A[reg] = source
			if reg == 7 {
				cpu.regs.SSP = source
				cpu.regs.A[7] = source
			}

			toUSP := uint16(0x4e60) | reg
			if opcodeTable[toUSP] == nil {
				t.Fatalf("MOVE A%d,USP opcode %04x not registered", reg, toUSP)
			}
			if opcodeCycleTable[toUSP] != 4 {
				t.Fatalf("MOVE A%d,USP cycles=%d, want 4", reg, opcodeCycleTable[toUSP])
			}
			if err := cpu.executeInstruction(toUSP); err != nil {
				t.Fatalf("MOVE A%d,USP failed: %v", reg, err)
			}
			if cpu.regs.USP != source {
				t.Fatalf("USP=%08x, want %08x after MOVE A%d,USP", cpu.regs.USP, source, reg)
			}

			destination := uint32(0x8000 + reg*0x100)
			cpu.regs.USP = destination

			fromUSP := uint16(0x4e68) | reg
			if opcodeTable[fromUSP] == nil {
				t.Fatalf("MOVE USP,A%d opcode %04x not registered", reg, fromUSP)
			}
			if opcodeCycleTable[fromUSP] != 4 {
				t.Fatalf("MOVE USP,A%d cycles=%d, want 4", reg, opcodeCycleTable[fromUSP])
			}
			if err := cpu.executeInstruction(fromUSP); err != nil {
				t.Fatalf("MOVE USP,A%d failed: %v", reg, err)
			}
			if cpu.regs.A[reg] != destination {
				t.Fatalf("A%d=%08x, want %08x after MOVE USP,A%d", reg, cpu.regs.A[reg], destination, reg)
			}

			if cpu.Cycles() != 8 {
				t.Fatalf("cycles=%d, want 8 after both MOVE USP forms", cpu.Cycles())
			}
		})
	}
}

func TestMoveUspPrivilegeViolationAllRegisterForms(t *testing.T) {
	tests := make([]struct {
		name   string
		opcode uint16
		reg    uint16
	}, 0, 16)

	for reg := range uint16(8) {
		tests = append(tests,
			struct {
				name   string
				opcode uint16
				reg    uint16
			}{name: fmt.Sprintf("MoveA%dToUSP", reg), opcode: 0x4e60 | reg, reg: reg},
			struct {
				name   string
				opcode uint16
				reg    uint16
			}{name: fmt.Sprintf("MoveUSPToA%d", reg), opcode: 0x4e68 | reg, reg: reg},
		)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			startPC := cpu.regs.PC
			supervisorSP := cpu.regs.A[7]
			userSP := uint32(0x4000)
			handler := uint32(0x5000)

			cpu.regs.SSP = supervisorSP
			cpu.regs.USP = userSP
			cpu.regs.A[7] = userSP
			cpu.regs.SR &^= srSupervisor
			cpu.regs.A[tt.reg] = 0x12345678
			expectedUSP := cpu.regs.A[7]

			if err := ram.Write(Long, uint32(XPrivViolation<<2), handler); err != nil {
				t.Fatalf("failed to seed privilege vector: %v", err)
			}
			if err := ram.Write(Word, startPC, uint32(tt.opcode)); err != nil {
				t.Fatalf("failed to write opcode %04x: %v", tt.opcode, err)
			}

			if err := cpu.Step(); err != nil {
				t.Fatalf("step failed: %v", err)
			}

			if cpu.regs.PC != handler {
				t.Fatalf("PC=%08x, want %08x", cpu.regs.PC, handler)
			}
			if cpu.regs.USP != expectedUSP {
				t.Fatalf("USP=%08x, want %08x after privilege violation", cpu.regs.USP, expectedUSP)
			}
			if tt.reg != 7 && cpu.regs.A[tt.reg] != 0x12345678 {
				t.Fatalf("A%d changed on privilege violation: got %08x want %08x", tt.reg, cpu.regs.A[tt.reg], uint32(0x12345678))
			}
			expectedSP := supervisorSP - exceptionFrameSize
			if cpu.regs.A[7] != expectedSP {
				t.Fatalf("SP=%08x, want %08x after privilege violation", cpu.regs.A[7], expectedSP)
			}
			stackedPC, err := ram.Read(Long, expectedSP+uint32(Word))
			if err != nil {
				t.Fatalf("failed to read stacked PC: %v", err)
			}
			if stackedPC != startPC+uint32(Word) {
				t.Fatalf("stacked PC=%08x, want %08x", stackedPC, startPC+uint32(Word))
			}
			if cpu.Cycles() != uint64(exceptionCyclesPrivilege) {
				t.Fatalf("cycles=%d, want %d", cpu.Cycles(), exceptionCyclesPrivilege)
			}
		})
	}
}

func TestLinkUnlk(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.A[7] = 0x3000
	cpu.regs.A[6] = 0x11112222

	program := assemble(t, `
        LINK A6,#-4
        UNLK A6
        NOP
`)

	for i, b := range program {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	for i := range 4 {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}

	if cpu.regs.A[6] != 0x11112222 {
		t.Fatalf("A6=%04x, want 0x11112222", cpu.regs.A[6])
	}
	if cpu.regs.A[7] != 0x3000 {
		t.Fatalf("A7=%04x, want 0x3000", cpu.regs.A[7])
	}

	value, err := ram.Read(Long, 0x2ffc)
	if err != nil {
		t.Fatalf("read stack: %v", err)
	}
	if value != 0x11112222 {
		t.Fatalf("stack value=%08x, want 0x11112222", value)
	}
}

func TestChk(t *testing.T) {
	cpu, ram := newEnvironment(t)

	program := assemble(t, `
        MOVEQ #5,D0
        MOVEQ #10,D1
        CHK D1,D0
        NOP
`)

	for i, b := range program {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	for i := range 3 {
		if err := cpu.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}

	if cpu.regs.SR&(srNegative|srCarry|srOverflow) != 0 {
		t.Fatalf("unexpected flags set: %04x", cpu.regs.SR)
	}
	if cpu.regs.SR&srZero != 0 {
		t.Fatalf("Z flag set, want clear")
	}
}

func TestChkException(t *testing.T) {
	cpu, ram := newEnvironment(t)
	if opcodeTable[0x4181] == nil {
		t.Fatalf("CHK handler not registered")
	}
	// vector 6 handler at 0x4000
	if err := ram.Write(Long, 6<<2, 0x4000); err != nil {
		t.Fatalf("write vector: %v", err)
	}

	program := assemble(t, `
        MOVEQ #-1,D0
        MOVEQ #10,D1
        CHK D1,D0
        NOP
`)

	for i, b := range program {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("write program: %v", err)
		}
	}

	if err := cpu.Step(); err != nil { // MOVEQ
		t.Fatalf("step moveq: %v", err)
	}
	if cpu.regs.D[0] != -1 {
		t.Fatalf("D0=%08x, want ffffffff", cpu.regs.D[0])
	}
	if err := cpu.Step(); err != nil { // bound load
		t.Fatalf("step bound: %v", err)
	}
	if err := cpu.Step(); err != nil { // CHK triggers exception
		t.Fatalf("step chk: %v", err)
	}

	t.Logf("after CHK PC=%04x SR=%04x", cpu.regs.PC, cpu.regs.SR)

	if cpu.regs.PC != 0x4000 {
		t.Fatalf("PC=%04x, want 0x4000", cpu.regs.PC)
	}
	if cpu.regs.SR&srSupervisor == 0 {
		t.Fatalf("SR supervisor bit not set after CHK exception")
	}
}
