package m68kemu

import "testing"

// TestTrapRteRoundTripPreservesStackPointer reproduces the gost TOS 1.06 boot
// symptom in isolation: repeatedly execute TRAP #1 against a handler that
// immediately executes RTE, and assert the supervisor stack pointer is
// bit-for-bit restored after every round trip.
func TestTrapRteRoundTripPreservesStackPointer(t *testing.T) {
	cpu, ram := newEnvironment(t)

	cpu.regs.SR = srSupervisor | 0x0700
	vector := uint32(1)
	vectorNumber := XTrap + vector
	vectorAddress := vectorNumber << 2

	handler := uint32(0x3000)
	if err := ram.Write(Long, vectorAddress, handler); err != nil {
		t.Fatalf("failed to write vector: %v", err)
	}

	trapCode := assemble(t, "TRAP #1\n")
	for i, b := range trapCode {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write trap opcode: %v", err)
		}
	}

	rteCode := assemble(t, "RTE\n")
	for i, b := range rteCode {
		if err := ram.Write(Byte, handler+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write rte opcode: %v", err)
		}
	}

	start := cpu.regs.PC
	initialSP := cpu.regs.A[7]

	for i := 0; i < 100; i++ {
		cpu.regs.PC = start

		opcode, err := cpu.fetchOpcode()
		if err != nil {
			t.Fatalf("iteration %d: failed to fetch TRAP opcode: %v", i, err)
		}
		if err := cpu.executeInstruction(opcode); err != nil {
			t.Fatalf("iteration %d: TRAP execution failed: %v", i, err)
		}
		if cpu.regs.PC != handler {
			t.Fatalf("iteration %d: expected PC at handler %08x, got %08x", i, handler, cpu.regs.PC)
		}

		opcode, err = cpu.fetchOpcode()
		if err != nil {
			t.Fatalf("iteration %d: failed to fetch RTE opcode: %v", i, err)
		}
		if err := cpu.executeInstruction(opcode); err != nil {
			t.Fatalf("iteration %d: RTE execution failed: %v", i, err)
		}

		if cpu.regs.A[7] != initialSP {
			t.Fatalf("iteration %d: SP not restored after TRAP/RTE round trip: got %08x want %08x (delta %d)",
				i, cpu.regs.A[7], initialSP, int32(cpu.regs.A[7])-int32(initialSP))
		}
	}
}

// TestTrapRteRoundTripPreservesStackPointerAcrossSRStates parameterizes the
// same round trip over interrupt-mask and user/supervisor starting states, in
// case the frame push/pop is only asymmetric under some SR condition.
func TestTrapRteRoundTripPreservesStackPointerAcrossSRStates(t *testing.T) {
	srStates := []uint16{
		srSupervisor,
		srSupervisor | 0x0100,
		srSupervisor | 0x0700,
		0x0000, // user mode; trap must switch to supervisor and back
		0x0700 &^ srSupervisor,
	}

	for _, initialSR := range srStates {
		t.Run("", func(t *testing.T) {
			cpu, ram := newEnvironment(t)
			cpu.setSR(initialSR)

			vector := uint32(1)
			vectorNumber := XTrap + vector
			vectorAddress := vectorNumber << 2
			handler := uint32(0x3000)
			if err := ram.Write(Long, vectorAddress, handler); err != nil {
				t.Fatalf("failed to write vector: %v", err)
			}

			trapCode := assemble(t, "TRAP #1\n")
			for i, b := range trapCode {
				if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
					t.Fatalf("failed to write trap opcode: %v", err)
				}
			}
			rteCode := assemble(t, "RTE\n")
			for i, b := range rteCode {
				if err := ram.Write(Byte, handler+uint32(i), uint32(b)); err != nil {
					t.Fatalf("failed to write rte opcode: %v", err)
				}
			}

			start := cpu.regs.PC
			initialSSP := cpu.regs.SSP
			initialUSP := cpu.regs.USP

			for i := 0; i < 50; i++ {
				cpu.regs.PC = start

				opcode, err := cpu.fetchOpcode()
				if err != nil {
					t.Fatalf("iteration %d: fetch TRAP failed: %v", i, err)
				}
				if err := cpu.executeInstruction(opcode); err != nil {
					t.Fatalf("iteration %d: TRAP execution failed: %v", i, err)
				}

				opcode, err = cpu.fetchOpcode()
				if err != nil {
					t.Fatalf("iteration %d: fetch RTE failed: %v", i, err)
				}
				if err := cpu.executeInstruction(opcode); err != nil {
					t.Fatalf("iteration %d: RTE execution failed: %v", i, err)
				}

				if cpu.regs.SSP != initialSSP {
					t.Fatalf("iteration %d: SSP drifted: got %08x want %08x (delta %d)",
						i, cpu.regs.SSP, initialSSP, int32(cpu.regs.SSP)-int32(initialSSP))
				}
				if cpu.regs.USP != initialUSP {
					t.Fatalf("iteration %d: USP drifted: got %08x want %08x (delta %d)",
						i, cpu.regs.USP, initialUSP, int32(cpu.regs.USP)-int32(initialUSP))
				}
			}
		})
	}
}

// TestNestedTrapRteRoundTripPreservesStackPointer covers a trap handler that
// itself executes a nested TRAP (a different vector) before returning, in
// case frame symmetry only breaks once exceptions nest.
func TestNestedTrapRteRoundTripPreservesStackPointer(t *testing.T) {
	cpu, ram := newEnvironment(t)
	cpu.regs.SR = srSupervisor | 0x0700

	outerVector := uint32(XTrap + 1)
	innerVector := uint32(XTrap + 2)
	outerHandler := uint32(0x3000)
	innerHandler := uint32(0x4000)

	if err := ram.Write(Long, outerVector<<2, outerHandler); err != nil {
		t.Fatalf("failed to write outer vector: %v", err)
	}
	if err := ram.Write(Long, innerVector<<2, innerHandler); err != nil {
		t.Fatalf("failed to write inner vector: %v", err)
	}

	trapOuter := assemble(t, "TRAP #1\n")
	for i, b := range trapOuter {
		if err := ram.Write(Byte, cpu.regs.PC+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write outer trap opcode: %v", err)
		}
	}
	// Outer handler: TRAP #2 then RTE.
	trapInner := assemble(t, "TRAP #2\nRTE\n")
	for i, b := range trapInner {
		if err := ram.Write(Byte, outerHandler+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write outer handler code: %v", err)
		}
	}
	// Inner handler: RTE immediately.
	rteCode := assemble(t, "RTE\n")
	for i, b := range rteCode {
		if err := ram.Write(Byte, innerHandler+uint32(i), uint32(b)); err != nil {
			t.Fatalf("failed to write inner handler code: %v", err)
		}
	}

	start := cpu.regs.PC
	initialSP := cpu.regs.A[7]

	for i := 0; i < 50; i++ {
		cpu.regs.PC = start

		// TRAP #1 -> outer handler.
		opcode, err := cpu.fetchOpcode()
		if err != nil {
			t.Fatalf("iteration %d: fetch outer TRAP failed: %v", i, err)
		}
		if err := cpu.executeInstruction(opcode); err != nil {
			t.Fatalf("iteration %d: outer TRAP failed: %v", i, err)
		}
		if cpu.regs.PC != outerHandler {
			t.Fatalf("iteration %d: expected outer handler, got %08x", i, cpu.regs.PC)
		}

		// TRAP #2 -> inner handler.
		opcode, err = cpu.fetchOpcode()
		if err != nil {
			t.Fatalf("iteration %d: fetch inner TRAP failed: %v", i, err)
		}
		if err := cpu.executeInstruction(opcode); err != nil {
			t.Fatalf("iteration %d: inner TRAP failed: %v", i, err)
		}
		if cpu.regs.PC != innerHandler {
			t.Fatalf("iteration %d: expected inner handler, got %08x", i, cpu.regs.PC)
		}

		// RTE from inner handler -> back into outer handler's RTE.
		opcode, err = cpu.fetchOpcode()
		if err != nil {
			t.Fatalf("iteration %d: fetch inner RTE failed: %v", i, err)
		}
		if err := cpu.executeInstruction(opcode); err != nil {
			t.Fatalf("iteration %d: inner RTE failed: %v", i, err)
		}

		// RTE from outer handler -> back to start.
		opcode, err = cpu.fetchOpcode()
		if err != nil {
			t.Fatalf("iteration %d: fetch outer RTE failed: %v", i, err)
		}
		if err := cpu.executeInstruction(opcode); err != nil {
			t.Fatalf("iteration %d: outer RTE failed: %v", i, err)
		}

		if cpu.regs.A[7] != initialSP {
			t.Fatalf("iteration %d: SP not restored after nested TRAP/RTE round trip: got %08x want %08x (delta %d)",
				i, cpu.regs.A[7], initialSP, int32(cpu.regs.A[7])-int32(initialSP))
		}
	}
}
