package fpu

import "github.com/jenska/m68kemu"

// Coprocessor returns f as an m68kemu.Coprocessor, for an MC68020 or later:
//
//	cpu, err := m68kemu.NewCPU(bus, m68kemu.WithModel(m68kemu.M68020),
//		m68kemu.WithCoprocessor(1, f.Coprocessor()))
//
// The CPU then executes the F-line instructions with coprocessor ID 1 (FADD,
// FMOVE, FBcc, FSAVE and the rest) on f.
func (f *FPU) Coprocessor() m68kemu.Coprocessor {
	return &coprocessor{FPU: f}
}

type coprocessor struct {
	*FPU
	// The CPU asks Decode, Operands, Execute and Cycles about the same
	// command word in turn; decode keeps the last result for them.
	decoded bool
	op      Operation
	err     error
}

// decode returns Decode(cmd), which depends on cmd alone.
func (c *coprocessor) decode(cmd uint16) (Operation, error) {
	if !c.decoded || c.op.Command != cmd {
		c.op, c.err = Decode(cmd)
		c.decoded = true
	}
	return c.op, c.err
}

func (c *coprocessor) Decode(cmd uint16) (int, error) {
	op, err := c.decode(cmd)
	return op.Register, err
}

func (c *coprocessor) Operands(cmd uint16, dynamic uint32) (int, int) {
	op, _ := c.decode(cmd)
	op.SetDynamic(dynamic)
	return op.In, op.Out
}

// Cycles implements m68kemu.CoprocessorTiming, with the FPU clocked like
// the CPU.
func (c *coprocessor) Cycles(cmd uint16, dynamic uint32) uint32 {
	op, _ := c.decode(cmd)
	return uint32(c.FPU.Cycles(op, dynamic))
}

func (c *coprocessor) Execute(cmd uint16, dynamic uint32, in []byte) ([]byte, error) {
	op, err := c.decode(cmd)
	if err != nil {
		return nil, err
	}
	op.SetDynamic(dynamic)
	out, err := c.FPU.Execute(op, dynamic, in)
	if err == nil && cmd>>13 == classStoreRegisters && cmd&0x1000 == 0 {
		// Predecrement FMOVEM sends FP7 first, as the CIR protocol does;
		// the CPU writes the operand upwards from the decremented address,
		// so put the registers in memory order, FP0 lowest.
		for i, j := 0, len(out)-12; i < j; i, j = i+12, j-12 {
			var tmp [12]byte
			copy(tmp[:], out[i:i+12])
			copy(out[i:i+12], out[j:j+12])
			copy(out[j:j+12], tmp[:])
		}
	}
	return out, err
}
