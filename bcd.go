package m68kemu

func init() {
	RegisterInstruction(abcd, 0xc100, 0xf1f8, 0, abcdCycleCalculator)
	RegisterInstruction(abcd, 0xc108, 0xf1f8, 0, abcdCycleCalculator)

	RegisterInstruction(sbcd, 0x8100, 0xf1f8, 0, sbcdCycleCalculator)
	RegisterInstruction(sbcd, 0x8108, 0xf1f8, 0, sbcdCycleCalculator)

	RegisterInstruction(nbcd, 0x4800, 0xfff8, 0, nbcdCycleCalculator)
	RegisterInstruction(nbcd, 0x4820, 0xfff8, 0, nbcdCycleCalculator)
}

func abcd(cpu *cpu) error {
	src, dst, err := bcdOperands(cpu)
	if err != nil {
		return err
	}

	prevZero := cpu.regs.SR&srZero != 0
	result, carry := bcdAdd(src.value, dst.value, cpu.regs.SR&srExtend != 0)
	if err := dst.write(result); err != nil {
		return err
	}

	updateBCDFlags(cpu, result, carry, prevZero, true)

	return nil
}

func sbcd(cpu *cpu) error {
	src, dst, err := bcdOperands(cpu)
	if err != nil {
		return err
	}

	prevZero := cpu.regs.SR&srZero != 0
	result, borrow := bcdSub(src.value, dst.value, cpu.regs.SR&srExtend != 0)
	if err := dst.write(result); err != nil {
		return err
	}

	updateBCDFlags(cpu, result, borrow, prevZero, true)

	return nil
}

func nbcd(cpu *cpu) error {
	operand, err := bcdDestination(cpu)
	if err != nil {
		return err
	}

	prevZero := cpu.regs.SR&srZero != 0
	result, borrow := bcdSub(operand.value, 0, cpu.regs.SR&srExtend != 0)
	if err := operand.write(result); err != nil {
		return err
	}

	updateBCDFlags(cpu, result, borrow, prevZero, false)

	return nil
}

func updateBCDFlags(cpu *cpu, result byte, carry bool, prevZero bool, propagateZero bool) {
	cpu.regs.SR &^= srCarry | srOverflow | srNegative
	if carry {
		cpu.regs.SR |= srCarry | srExtend
	} else {
		cpu.regs.SR &^= srExtend
	}

	zero := result == 0
	if propagateZero {
		zero = zero && prevZero
	}
	if zero {
		cpu.regs.SR |= srZero
	} else {
		cpu.regs.SR &^= srZero
	}
}

func bcdAdd(src, dst byte, extend bool) (byte, bool) {
	sum := int(src) + int(dst)
	if extend {
		sum++
	}

	if (sum & 0x0f) > 9 {
		sum += 0x06
	}

	carry := sum > 0x99
	if carry {
		sum += 0x60
	}

	return byte(sum & 0xff), carry
}

func bcdSub(src, dst byte, extend bool) (byte, bool) {
	e := 0
	if extend {
		e = 1
	}

	low := (int(dst) & 0x0f) - (int(src) & 0x0f) - e
	diff := int(dst) - int(src) - e

	if low < 0 {
		diff -= 0x06
	}

	borrow := diff < 0
	if borrow {
		diff -= 0x60
	}

	return byte(diff & 0xff), borrow
}

type bcdOperand struct {
	value byte
	write func(byte) error
}

type bcdSourceDest struct {
	value byte
	write func(byte) error
}

func bcdOperands(cpu *cpu) (bcdOperand, bcdOperand, error) {
	if (cpu.regs.IR>>3)&0x1 == 0 {
		srcReg := dy(cpu)
		dstReg := udx(cpu)
		return bcdOperand{value: byte(*srcReg & 0xff)}, bcdOperand{
			value: byte(*dstReg & 0xff),
			write: func(v byte) error {
				*dstReg = (*dstReg & 0xffffff00) | uint32(v)
				return nil
			},
		}, nil
	}

	sourceAddr := *ay(cpu) - 1
	destAddr := *ax(cpu) - 1
	*ay(cpu) = sourceAddr
	*ax(cpu) = destAddr

	srcValue, err := cpu.Read(Byte, sourceAddr)
	if err != nil {
		return bcdOperand{}, bcdOperand{}, err
	}
	dstValue, err := cpu.Read(Byte, destAddr)
	if err != nil {
		return bcdOperand{}, bcdOperand{}, err
	}

	return bcdOperand{value: byte(srcValue)}, bcdOperand{
		value: byte(dstValue),
		write: func(v byte) error {
			return cpu.Write(Byte, destAddr, uint32(v))
		},
	}, nil
}

func bcdDestination(cpu *cpu) (bcdSourceDest, error) {
	mode := (cpu.regs.IR >> 3) & 0x1
	reg := y(cpu.regs.IR)

	if mode == 0 {
		dstReg := dy(cpu)
		return bcdSourceDest{
			value: byte(*dstReg & 0xff),
			write: func(v byte) error {
				*dstReg = (*dstReg & 0xffffff00) | uint32(v)
				return nil
			},
		}, nil
	}

	addr := cpu.regs.A[reg] - 1
	cpu.regs.A[reg] = addr

	value, err := cpu.Read(Byte, addr)
	if err != nil {
		return bcdSourceDest{}, err
	}

	return bcdSourceDest{
		value: byte(value),
		write: func(v byte) error { return cpu.Write(Byte, addr, uint32(v)) },
	}, nil
}

func abcdCycleCalculator(opcode uint16) uint32 {
	if (opcode>>3)&0x1 == 0 {
		return 6
	}
	return 18
}

func sbcdCycleCalculator(opcode uint16) uint32 {
	if (opcode>>3)&0x1 == 0 {
		return 6
	}
	return 18
}

func nbcdCycleCalculator(opcode uint16) uint32 {
	if (opcode>>3)&0x1 == 0 {
		return 6
	}
	return 8
}
