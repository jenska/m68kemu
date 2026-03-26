# m68kemu

A Motorola 68000 emulator written in Go.

This project provides a Motorola 68000 CPU emulator for retro-computing projects, with a current focus on becoming part of an Atari ST emulator. The core aims to be timing-aware, testable, and easy to embed in a larger machine model.

## Features

* Motorola 68000 instruction set emulation.
* Timing-aware execution with per-instruction cycle accounting.
* Supervisor and user modes.
* Interrupt handling and exception processing.
* Correct short exception frames for group 1/2 exceptions and 68000 group 0 bus/address error frames.
* 24-bit address bus with support for multiple devices, fixed-range mappings, and Atari ST-style region layout.
* Tracing, breakpoints, and cycle-budgeted execution.
* Optional cycle scheduler hooks for machine-level devices such as timers, video, DMA, and interrupt controllers.

## Current Status

The CPU core is in good shape for integration work:

* Instruction execution, stack behavior, interrupts, and most commonly used addressing modes are covered by tests.
* `RESET` now follows machine-friendly semantics for an Atari ST integration: the CPU instruction resets attached devices but does not erase RAM contents.
* Bus and address faults now use the richer 68000 group 0 stack frame, which is important for realistic system error handling.
* The bus has fast paths for simple memory setups and fixed-range mappings, which keeps the core practical for full-machine emulation.

Still missing for a complete Atari ST:

* The rest of the ST chipset and memory-mapped I/O devices.
* A full machine-level reset / cold-boot model on top of CPU `RESET`.
* More detailed interrupt acknowledge and device-level timing behavior.
* Prefetch-sensitive behavior and any remaining compatibility gaps found by larger TOS / software workloads.

## Getting Started

This package is designed to be used as a library in your own projects.

### Installation

```sh
go get github.com/jenska/m68kemu
```

### Example Usage

Here's a simple example of how to set up the CPU, load a program, and run it:

```go
package main

import (
	"fmt"
	"log"

	"github.com/jenska/m68kemu"
)

func main() {
	// Create a 64KB RAM device at address 0.
	ram := m68kemu.NewRAM(0, 64*1024)

	// Create a bus and attach the RAM.
	bus := m68kemu.NewBus(ram)

	// Set up the initial stack pointer and program counter.
	// SSP at 0x1000, PC at 0x2000.
	ram.Write(m68kemu.Long, 0, 0x1000)
	ram.Write(m68kemu.Long, 4, 0x2000)

	// Create the CPU.
	cpu, err := m68kemu.NewCPU(bus)
	if err != nil {
		log.Fatalf("Failed to create CPU: %v", err)
	}

	// Assemble a simple program: MOVEQ #5, D0 (opcode 0x7005)
	program := []byte{0x70, 0x05}
	startPC, _ := ram.Read(m68kemu.Long, 4)

	for i, b := range program {
		if err := ram.Write(m68kemu.Byte, startPC+uint32(i), uint32(b)); err != nil {
			log.Fatalf("Failed to write program: %v", err)
		}
	}

	// Step one instruction.
	if err := cpu.Step(); err != nil {
		log.Fatalf("CPU step failed: %v", err)
	}

	// Print registers to see the result.
	regs := cpu.Registers()
	fmt.Printf("D0 = %d\n", regs.D[0]) // Should be 5
	fmt.Printf("PC = 0x%04x\n", regs.PC) // Should be 0x2002
}
```

### Atari ST-Oriented Bus Setup

The bus can also be built from explicit 24-bit address ranges, which is useful when wiring together an Atari ST memory map:

```go
ram := m68kemu.NewRAM(0x000000, 512*1024)
tos := m68kemu.NewRAM(m68kemu.STTOSStart, 192*1024)

bus := m68kemu.NewAtariSTBus(
	m68kemu.STRegionMapping{Start: 0x000000, End: 0x07ffff, Device: ram},
	m68kemu.STRegionMapping{Start: m68kemu.STTOSStart, End: m68kemu.STTOSEnd, Device: tos},
)
```

The built-in Atari ST constants map TOS ROM to `0xFC0000-0xFEFFFF` and MMIO to `0xFF8000-0xFFFFFF`.
For now this is just a convenient fixed-range decoder; the actual ST devices still need to be implemented on top of it.

### Cycle Scheduler

Machine devices can follow CPU time by attaching a scheduler:

```go
scheduler := m68kemu.NewCycleScheduler()
cpu.SetScheduler(scheduler)

scheduler.ScheduleAfter(512, func(now uint64) {
	// Run a timer tick, trigger an interrupt, advance video state, etc.
})
```

The scheduler is intentionally small at this stage. It is meant as a foundation for ST components rather than a finished machine-timing framework.

## Testing

The emulator has an extensive test suite, including instruction-level tests and small programs.

To run the tests:
```sh
go test ./...
```

To run the benchmarks:
```sh
go test -bench=. ./...
```

## Performance Notes

Recent profiling work focused on the interpreter hot path:

* bus fast paths for simple and fixed-range mappings
* reduced wait-state overhead when no device contributes extra wait states
* fewer allocations on reset / benchmark loops
* predecoded opcode metadata for common decode fields

On the current benchmark set, that work brought the project to roughly:

* `BenchmarkBubbleSort`: ~3.08 ms/op
* `BenchmarkPrimeSieve`: ~5.84 ms/op
* `BenchmarkRecursiveFibonacci`: ~28.6 ms/op

See [doc/benchmark_report.md](doc/benchmark_report.md) for more detail.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
