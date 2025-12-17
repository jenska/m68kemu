package m68kemu

func init() {
	registerInstruction(movep, 0x0108, 0xf1f8, 0, movepCycleCalculator)
	registerInstruction(movep, 0x0148, 0xf1f8, 0, movepCycleCalculator)
	registerInstruction(movep, 0x0188, 0xf1f8, 0, movepCycleCalculator)
	registerInstruction(movep, 0x01c8, 0xf1f8, 0, movepCycleCalculator)
}

func movep(cpu *cpu) error {
	opcode := cpu.regs.IR
	size := Word
	if opcode&0x0080 != 0 {
		size = Long
	}
	toRegister := opcode&0x0040 != 0

	disp, err := cpu.popPc(Word)
	if err != nil {
		return err
	}

	addr := uint32(int32(cpu.regs.A[opcode&0x7]) + int32(int16(disp)))
	reg := (opcode >> 9) & 0x7
	mask := size.mask()

	var value uint32
	if toRegister {
		if size == Word {
			high, err := cpu.read(Byte, addr)
			if err != nil {
				return err
			}
			low, err := cpu.read(Byte, addr+2)
			if err != nil {
				return err
			}
			value = (high << 8) | low
		} else {
			b3, err := cpu.read(Byte, addr)
			if err != nil {
				return err
			}
			b2, err := cpu.read(Byte, addr+2)
			if err != nil {
				return err
			}
			b1, err := cpu.read(Byte, addr+4)
			if err != nil {
				return err
			}
			b0, err := cpu.read(Byte, addr+6)
			if err != nil {
				return err
			}
			value = (b3 << 24) | (b2 << 16) | (b1 << 8) | b0
		}
		cpu.regs.D[reg] = (cpu.regs.D[reg] & ^int32(mask)) | int32(value&mask)
	} else {
		value = uint32(cpu.regs.D[reg]) & mask
		if size == Word {
			if err := cpu.write(Byte, addr, (value>>8)&0xff); err != nil {
				return err
			}
			if err := cpu.write(Byte, addr+2, value&0xff); err != nil {
				return err
			}
		} else {
			if err := cpu.write(Byte, addr, (value>>24)&0xff); err != nil {
				return err
			}
			if err := cpu.write(Byte, addr+2, (value>>16)&0xff); err != nil {
				return err
			}
			if err := cpu.write(Byte, addr+4, (value>>8)&0xff); err != nil {
				return err
			}
			if err := cpu.write(Byte, addr+6, value&0xff); err != nil {
				return err
			}
		}
	}

	cpu.regs.SR &^= srNegative | srZero | srOverflow | srCarry
	if size.isZero(value) {
		cpu.regs.SR |= srZero
	}
	if size.isNegative(value) {
		cpu.regs.SR |= srNegative
	}

	return nil
}

func movepCycleCalculator(opcode uint16) uint32 {
	if opcode&0x0080 != 0 {
		return 24
	}
	return 16
}
