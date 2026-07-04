# m68kemu

A Motorola 68000 emulator written in Go.

This project provides a Motorola 68000 CPU emulator for retro-computing projects, with a current focus on becoming part of an Atari ST emulator. The core aims to be timing-aware, testable, and easy to embed in a larger machine model.

## Features

* Motorola 68000 instruction set emulation.
* Timing-aware execution with per-instruction cycle accounting.
* Supervisor and user modes.
* Interrupt handling and exception processing.
* Correct short exception frames for group 1/2 exceptions and 68000 group 0 bus/address error frames.
* 24-bit address bus with multiple memory-devices.
* Compact per-instruction tracing and cycle-budgeted execution.
* Optional cycle scheduler hooks for machine-level devices such as timers, video, DMA, and interrupt controllers.

## Current Status

The CPU core is in good shape for integration work:

* Instruction execution, stack behavior, interrupts, and most commonly used addressing modes are covered by tests.
* `RESET` now follows machine-friendly semantics for an Atari ST integration: the CPU instruction resets attached devices but does not erase RAM contents.
* Bus and address faults now use the richer 68000 group 0 stack frame, which is important for realistic system error handling.
* The bus is intentionally simple: it performs linear device lookup and common 68000 alignment checks.

Still missing for a complete Atari ST:

* Prefetch-sensitive behavior and any remaining compatibility gaps found by larger TOS / software workloads.

## Getting Started

This package is designed to be used as a library in your own projects.

The repository also includes a small example command in `cmd/qsortdemo`, which assembles and executes the `testdata/qsort.s` quicksort demo.

### Requirements

This module targets Go 1.26.

### Installation

```sh
go get github.com/jenska/m68kemu
```

If you want the demo binary, install it directly:

```sh
go install github.com/jenska/m68kemu/cmd/qsortdemo@latest
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

### Tracing

The CPU exposes one optional trace callback for instruction-level logging:

```go
cpu.SetTracer(func(info m68kemu.TraceInfo) {
 log.Printf("pc=%08x opcode=%04x cycles=%d", info.PC, info.Opcode, info.Cycles)
})
```

For normal execution control, use `Step`, `RunInstructions`, or `RunCycles`.

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

To run the core CPU and infrastructure benchmarks without test noise:

```sh
go test -run '^$' -bench 'Benchmark(BubbleSort|PrimeSieve|RunEightMillionCycles|RecursiveFibonacci|CycleSchedulerAdvanceBurst|BusReadManyDevices)$' -benchmem ./...
```

## Performance Notes

Recent profiling work focused on the interpreter hot path:

* amortized scheduler event dispatch without per-event slice shifting
* fewer allocations and less debug bookkeeping in normal benchmark loops
* predecoded opcode metadata for common decode fields
* Go 1.26 benchmark loops using `testing.B.Loop`

Representative results on June 13, 2026 on Apple M1 (`darwin/arm64`, Go 1.26.3) were:

* `BenchmarkBubbleSort`: ~2.54 ms/op
* `BenchmarkPrimeSieve`: ~4.98 ms/op
* `BenchmarkRunEightMillionCycles`: ~25.6 ms/op
* `BenchmarkRecursiveFibonacci`: ~26.5 ms/op
* `BenchmarkCycleSchedulerAdvanceBurst`: ~3.29 us/op
* `BenchmarkBusReadManyDevices`: linear lookup across devices

See [doc/benchmark_report.md](doc/benchmark_report.md) for more detail.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
