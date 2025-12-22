# m68kemu

A Motorola 68000 emulator written in Go.

This project provides a cycle-accurate Motorola 68000 CPU emulator, suitable for use in retro-computing projects, emulators for classic computers and consoles, or for educational purposes.

## Features

*   Motorola 68000 instruction set emulation.
*   Cycle-accurate execution.
*   Supervisor and user modes.
*   Exception and interrupt handling.
*   Address bus with support for multiple devices (e.g., RAM).
*   Tracing and breakpoint support for debugging.

## Getting Started

This package is designed to be used as a library in your own projects.

### Installation

```sh
go get github.com/jens/m68kemu
```

### Example Usage

Here's a simple example of how to set up the CPU, load a program, and run it:

```go
package main

import (
	"fmt"
	"log"

	"github.com/jens/m68kemu"
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
	fmt.Printf("D0 = %d\n", regs.D) // Should be 5
	fmt.Printf("PC = 0x%04x\n", regs.PC) // Should be 0x2002
}
```

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

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.