package m68kemu

import "testing"

// TestExceptionCyclesReachScheduler checks that instructions raising an
// exception with its own cycle total advance an attached scheduler by exactly
// the cycles the CPU counts.
func TestExceptionCyclesReachScheduler(t *testing.T) {
	for _, tc := range []struct {
		name string
		asm  string
		sr   uint16
		want uint64
	}{
		{"DIVU by zero", "DIVU.W D0,D1", 0x2700, 38},
		{"CHK out of range", "CHK.W D0,D1", 0x2700, 40},
		{"RESET in user mode", "RESET", 0x0700, 34},
		{"RESET in supervisor mode", "RESET", 0x2700, 132},
		{"TRAPV with V set", "TRAPV", 0x2702, 34},
		{"ILLEGAL", "ILLEGAL", 0x2700, 34},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			_ = ram.Write(Long, 6*4, 0x2400) // CHK vector
			_ = ram.Write(Long, 5*4, 0x2400) // divide by zero
			_ = ram.Write(Long, 8*4, 0x2400) // privilege violation
			_ = ram.Write(Long, 7*4, 0x2400) // TRAPV
			_ = ram.Write(Long, 4*4, 0x2400) // illegal
			cpu.regs.SR = tc.sr
			cpu.regs.D[0] = 0
			cpu.regs.D[1] = -1
			cpu.regs.USP = 0x1800
			for i, b := range assemble(t, tc.asm) {
				_ = ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b))
			}
			scheduler := NewCycleScheduler()
			cpu.SetScheduler(scheduler)
			start := cpu.Cycles()
			if err := cpu.Step(); err != nil {
				t.Fatalf("step: %v", err)
			}
			if got := cpu.Cycles() - start; got != tc.want {
				t.Errorf("CPU cycles = %d, want %d", got, tc.want)
			}
			if scheduler.Now() != cpu.Cycles() {
				t.Errorf("scheduler at %d, CPU at %d", scheduler.Now(), cpu.Cycles())
			}
		})
	}
}
