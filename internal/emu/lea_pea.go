package emu

import "fmt"

func init() {
	const leaPeaAddressMask = eaMaskIndirect | eaMaskPostIncrement | eaMaskPreDecrement | eaMaskDisplacement | eaMaskIndex | eaMaskAbsoluteShort | eaMaskAbsoluteLong | eaMaskPCDisplacement | eaMaskPCIndex

	RegisterInstruction(lea, 0x41c0, 0xf1c0, leaPeaAddressMask, leaPeaCycleCalculator(4))
	RegisterInstruction(pea, 0x4840, 0xffc0, leaPeaAddressMask, leaPeaCycleCalculator(8))
}

func lea(cpu *CPU) error {
	mode := (cpu.regs.IR >> 3) & 0x7
	reg := cpu.regs.IR & 0x7
	if mode < 2 || (mode == 7 && reg == 4) {
		return fmt.Errorf("invalid addressing mode for LEA")
	}

	src, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return err
	}

	*ax(cpu) = src.computedAddress()
	return nil
}

func pea(cpu *CPU) error {
	mode := (cpu.regs.IR >> 3) & 0x7
	reg := cpu.regs.IR & 0x7
	if mode < 2 || (mode == 7 && reg == 4) {
		return fmt.Errorf("invalid addressing mode for PEA")
	}

	src, err := cpu.ResolveSrcEA(Long)
	if err != nil {
		return err
	}

	return cpu.Push(Long, src.computedAddress())
}

func leaPeaCycles(ir uint16, base uint32) uint32 {
	mode := (ir >> 3) & 0x7
	reg := ir & 0x7
	return base + eaAccessCycles(mode, reg, Long)
}

func leaPeaCycleCalculator(base uint32) CycleCalculator {
	return func(opcode uint16) uint32 {
		return leaPeaCycles(opcode, base)
	}
}
