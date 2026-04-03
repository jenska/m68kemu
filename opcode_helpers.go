package m68kemu

const (
	statusMaskNZVC  uint16 = srNegative | srZero | srOverflow | srCarry
	statusMaskNZVCX uint16 = statusMaskNZVC | srExtend
)

func replaceStatusFlags(cpu *cpu, mask, flags uint16) {
	cpu.regs.SR = (cpu.regs.SR &^ mask) | (flags & mask)
}

func nzFlags(result uint32, size Size) uint16 {
	var flags uint16
	if size.isZero(result) {
		flags |= srZero
	} else if size.isNegative(result) {
		flags |= srNegative
	}
	return flags
}

func updateNZClearVC(cpu *cpu, result uint32, size Size) {
	replaceStatusFlags(cpu, statusMaskNZVC, nzFlags(result, size))
}
