# m68kemu

A Motorola 680x0 emulator written in Go.

This project provides a Motorola 68000-family CPU emulator for retro-computing projects, with a current focus on becoming part of an Atari ST emulator. The core aims to be timing-aware, testable, and easy to embed in a larger machine model.

## Features

* The MC68000, MC68010, MC68020 and MC68030 (integer units), selected per CPU with `WithModel`.
* The MC68881/MC68882 FPU, memory-mapped on a 68000 or attached as the coprocessor of a 68020/030, with typical execution times.
* Timing-aware execution with per-instruction cycle accounting.
* Supervisor and user modes.
* Interrupt handling and exception processing.
* Correct short exception frames for group 1/2 exceptions and 68000 group 0 bus/address error frames; format $0–$B frames and instruction restart on the 68010 and later.
* 24-bit or 32-bit address bus with support for multiple devices, fixed-range mappings, and Atari ST-style region layout.
* Tracing, breakpoints, cycle-budgeted execution, and verbose logging helpers with instruction-range disassembly.
* Rich debug hooks for per-instruction trace, pre-instruction snapshots, exceptions, bus accesses, and accepted interrupts.
* `RunUntil` stop conditions for instruction budgets, exact PC stops, PC ranges, exceptions, bus-access matches, and custom predicates.
* Optional rolling debug history plus helpers to inspect the last exception stack frame.
* Optional cycle scheduler hooks for machine-level devices such as timers, video, DMA, and interrupt controllers.

## Current Status

The CPU core is in good shape for integration work:

* Instruction execution, stack behavior, interrupts, and most commonly used addressing modes are covered by tests.
* `RESET` now follows machine-friendly semantics for an Atari ST integration: the CPU instruction resets attached devices but does not erase RAM contents.
* Bus and address faults now use the richer 68000 group 0 stack frame, which is important for realistic system error handling.
* The bus has fast paths for simple memory setups and fixed-range mappings, which keeps the core practical for full-machine emulation.

Still missing for a complete Atari ST:

* Prefetch-sensitive behavior and any remaining compatibility gaps found by larger TOS / software workloads.

The MC68010, MC68020 and MC68030 are supported (see [CPU Models](#cpu-models)), and so is the MC68881/MC68882 FPU, memory-mapped on a 68000 or as the coprocessor of a 68020/030 (see [FPU](#fpu-mc68881mc68882)). The 68030 MMU (next, phase 5) and the MC68040 is planned. See [doc/roadmap.md](doc/roadmap.md).

## Getting Started

This package is designed to be used as a library in your own projects.

The repository also includes a runnable example, `Example_quicksort` in `example_test.go`, which assembles and executes the `testdata/qsort.s` quicksort demo. Run it with `go test -run Example_quicksort -v`.

### Requirements

This module targets Go 1.27.

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

 // The reset vectors: SSP at 0x1000, PC at 0x2000.
 ram.Write(m68kemu.Long, 0, 0x1000)
 ram.Write(m68kemu.Long, 4, 0x2000)

 // Load a simple program: MOVEQ #5, D0 (opcode 0x7005).
 if err := ram.Load(0x2000, []byte{0x70, 0x05}); err != nil {
  log.Fatalf("Failed to load program: %v", err)
 }

 // Create the CPU on a bus with the RAM. NewCPU resets it from the vectors.
 cpu, err := m68kemu.NewCPU(m68kemu.NewBus(ram))
 if err != nil {
  log.Fatalf("Failed to create CPU: %v", err)
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

### CPU Models

`NewCPU` emulates an MC68000 by default. `WithModel` selects the model
explicitly, and `Model()` reports it:

```go
cpu, err := m68kemu.NewCPU(bus, m68kemu.WithModel(m68kemu.M68010))
```

The MC68010 adds the VBR, SFC and DFC registers, the instructions MOVEC,
MOVES, RTD and MOVE CCR,<ea>, a privileged MOVE SR,<ea>, and exception stack
frames with a format/vector word. After a bus or address error, RTE runs the
faulted instruction again. The emulator restarts the instruction where the
real CPU would continue it from saved internal state. The 68010 currently
uses the MC68000 cycle counts. [doc/mc68010.md](doc/mc68010.md) has the
details.

The MC68020 and MC68030 add 32-bit addresses, the scaled-index and memory
indirect addressing modes, the master and interrupt stack pointers, CACR and
CAAR, the 68020 instructions (32-bit branches, MULx.L/DIVx.L, bit fields,
CAS/CAS2, CHK2/CMP2, PACK/UNPK, TRAPcc and others), misaligned operands, the
format $1, $2 and $B stack frames, and the coprocessor interface. `NewCPU`
sets the bus to decode 32 address bits for them (and 24 for the MC68000 and
MC68010):

```go
cpu, err := m68kemu.NewCPU(m68kemu.NewBus(ram), m68kemu.WithModel(m68kemu.M68030))
```

Their timing is approximate, CALLM/RTM support only type $00 module
descriptors and the caches are not emulated. [doc/mc68020.md](doc/mc68020.md)
has the details.

### MMU (MC68030)

An MC68030 has its paged memory management unit built in, emulated by
package `github.com/jenska/m68kemu/mmu`. The CPU executes PMOVE, PFLUSH,
PLOAD and PTEST, translates every access through the translation tables in
memory once TC enables it, and takes a restartable bus error on a page
fault. `cpu.MMU()` returns the MMU, for example to read its registers:

```go
tc := cpu.MMU().Register(mmu.TC)
```

An MC68020 can have an MC68851 PMMU, attached with `WithMC68851`. It
translates in the same way and adds the DMA root pointer, task aliases,
access levels, PVALID, the PMMU conditionals, PSAVE/PRESTORE and the
breakpoint registers that answer BKPT:

```go
cpu, err := m68kemu.NewCPU(bus, m68kemu.WithModel(m68kemu.M68020), m68kemu.WithMC68851())
```

[doc/mmu.md](doc/mmu.md) has the details.

The constant `M68040` exist for the model on the
[roadmap](doc/roadmap.md). Until a model is implemented, `NewCPU` returns
an error that matches `m68kemu.ErrModelUnsupported` with `errors.Is`.

### FPU (MC68881/MC68882)

Package `github.com/jenska/m68kemu/fpu` emulates the MC68881 and MC68882 on
top of [github.com/jenska/float](https://github.com/jenska/float). On a 68000
machine the FPU is a memory-mapped peripheral; `fpu.NewCIR` puts its
coprocessor interface registers on the bus, for example at `$FFFA40` as in
the Atari Mega ST (SFP004) and Mega STE:

```go
ram := m68kemu.NewRAM(0, 4*1024*1024)
fpu68881 := fpu.New(fpu.MC68881)
bus := m68kemu.NewBus(ram, fpu.NewCIR(fpu68881, 0xfffa40))
cpu, err := m68kemu.NewCPU(bus)
```

Software then drives it like the real chip: it writes command words to the
command CIR, polls the response CIR and moves operands through the operand
CIR. The FPU supports every 68881 general instruction, all operand formats
including packed decimal, FMOVECR, the condition predicates, exceptions and
FSAVE/FRESTORE frames.

On an MC68020 or MC68030 the FPU is a coprocessor, and the CPU executes the
F-line instructions (`FADD`, `FMOVEM`, `FBcc`, `FSAVE` and the rest):

```go
cpu, err := m68kemu.NewCPU(bus, m68kemu.WithModel(m68kemu.M68020),
	m68kemu.WithCoprocessor(1, fpu.New(fpu.MC68882).Coprocessor()))
```

Both paths can model the FPU's execution time from a table of typical
MC68881/MC68882 times (`FPU.Cycles`). On a 68020/030 the FPU runs concurrently
with the CPU, which waits only at the next FPU instruction or for a result.
On a 68000, set `CIR.Clock` and the response CIR reports busy while a command
runs, so polling loops behave as on the real card:

```go
cir := fpu.NewCIR(fpu68881, 0xfffa40)
cir.Clock = cpu.Cycles
```

[doc/fpu.md](doc/fpu.md) has the details and the simplifications.

### Memory and Devices

A `Bus` routes every access to the device whose address range holds it.
`RAM` and `ROM` are plain memory; any other peripheral implements `Device`
plus `AddressRangeDevice` (or `ContainsDevice` for a sparse decode):

```go
ram := m68kemu.NewRAM(0, 4*1024*1024)
rom := m68kemu.NewROM(0xfc0000, tosImage) // writes to it are bus errors
bus := m68kemu.NewBus(ram, rom, mfp, glue)
bus.SetWaitStates(1)
```

The CPU reads, writes and fetches instructions from a `MemoryDevice`, such
as `RAM` and `ROM`, directly instead of calling the device, and charges the
bus wait states as the bus would. Implement `MemoryDevice` for your own
side-effect-free memory to get the same speed, and call `bus.Remap()` after
such a device switches banks or changes its range. Devices listed earlier on
the bus win where ranges overlap, so a ROM overlay at address 0 in front of
RAM works as on the Atari ST.

### Cycle Scheduler

Machine devices can follow CPU time by attaching a scheduler:

```go
scheduler := m68kemu.NewCycleScheduler()
cpu.SetScheduler(scheduler)

scheduler.ScheduleAfter(512, func(now uint64) {
 // Run a timer tick, trigger an interrupt, etc.
})
```

The CPU compares its cycle counter with the time of the next event and only
calls into the scheduler when an event is due, so an attached scheduler costs
next to nothing between events. `Now()` is exact at any point, also in the
middle of an instruction, so a device whose state changes continuously (a
video counter, a free-running timer) computes it from `Now()` when it is
read. `SetClockRatio` makes scheduler time run at a device clock instead of
the CPU clock.

### Verbose Logging And Range Disassembly

The emulator includes helpers for both one-off disassembly and trace logging:

```go
logger := m68kemu.NewVerboseLogger(cpu, bus, os.Stdout, m68kemu.VerboseLoggerOptions{
  IncludeRegisters: true,
  IncludeCycles:    true,
  MemoryRanges: []m68kemu.MemoryRange{
    {Start: 0x2000, Length: 0x10, Label: "program"},
  },
})
cpu.SetHooks(m68kemu.Hooks{Trace: logger.Trace})

lines, err := m68kemu.DisassembleMemoryRange(bus, 0x2000, 0x10)
if err != nil {
 log.Fatalf("disassembly failed: %v", err)
}
for _, line := range lines {
 fmt.Println(line)
}
```

Verbose trace lines include the current PC, decoded assembly, and optionally the total cycle count. When the tracer has access to the fetched instruction bytes, the logger also includes the raw opcode and per-instruction cycle delta, for example:

```text
TRACE PC 00002000 OPCODE 7005 DELTA 4 CYCLES 4 MOVEQ #5, D0
```

These helpers use the bus `Peek` path when available so debug output does not trigger device side effects, and the verbose logger prefers `TraceInfo.Bytes` for disassembly so the trace remains accurate even when fetch-side effects would make a second bus read misleading.

### Debug Hooks

For emulator bring-up and TOS failure analysis, the CPU exposes several
debugger-oriented callbacks. `SetHooks` installs them all at once; a nil
field means no callback:

```go
cpu.SetHooks(m68kemu.Hooks{
 PreTrace: func(info m68kemu.PreTraceInfo) {
  // Inspect registers before the instruction executes.
 },
 Trace: func(info m68kemu.TraceInfo) {
  // Instruction address, opcode bytes, mnemonic, before/after registers,
  // per-instruction cycle delta, and total cycle count.
 },
 Exception: func(info m68kemu.ExceptionInfo) {
  // Vector, trapping opcode address, stacked/reported PC, SR before/after,
  // new handler PC, and decoded stack-frame details.
 },
 Bus: func(info m68kemu.BusAccessInfo) {
  // Address, size, read/write, value, instruction-fetch flag, and current instruction PC.
 },
 Interrupt: func(info m68kemu.InterruptInfo) {
  // Accepted IRQ level, vector, autovector/explicit, and PC/SR at acceptance.
 },
})
```

To change one callback and keep the others, start from `cpu.Hooks()`.
`SetRegisters` and `SetPC` set the CPU's state directly, for example to start
a loaded program without a reset vector.

`RunUntil` can also stop on richer conditions:

```go
result, err := cpu.RunUntil(m68kemu.RunUntilOptions{
 MaxInstructions: 1000,
 StopAtPC:        []uint32{0x00fc1234},
 StopOnException: true,
 StopOnBusAccess: func(info m68kemu.BusAccessInfo) bool {
  return !info.InstructionFetch && info.Address == 0x00ff8209
 },
 StopPredicate: func(info m68kemu.RunPredicateInfo) bool {
  return info.Registers.D[0] == 0xdeadbeef
 },
})
```

If you want a rolling "what just happened?" buffer without always logging, call `cpu.SetHistoryLimit(n)` and inspect `cpu.History()`. After an exception, `cpu.CurrentExceptionFrame()` and `m68kemu.ReadExceptionStackFrame(...)` can decode the pushed 68000 frame directly from memory.

### Concurrency

Separate CPUs can run in parallel goroutines, including CPUs of different
models. A CPU, its `Bus`, the devices on that bus and its scheduler belong
together: drive them from one goroutine at a time, and give every CPU its own
`Bus`: `NewCPU` returns `ErrBusInUse` for a bus that already serves a CPU. The methods of a single CPU are not
safe to call from several goroutines at once, with one exception:
`RequestInterrupt` may be called from any goroutine, also while the CPU runs,
so a device in its own goroutine can raise interrupts and wake a CPU waiting
in `STOP`. Such a request is taken at the next instruction boundary after the
CPU sees it, so its point in emulated time depends on the host's scheduling.
For cycle-exact timing, request interrupts from `CycleScheduler` events,
which run on the CPU's goroutine. An `IRQSource` is always called on the
CPU's goroutine.

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
go test -run '^$' -bench 'Benchmark(BubbleSort|PrimeSieve|RunEightMillionCycles|RecursiveFibonacci|CycleSchedulerAdvanceBurst|BusReadMappedRanges)$' -benchmem ./...
```

## Performance Notes

Recent profiling work focused on the interpreter hot path:

* bus fast paths for simple and fixed-range mappings
* cached single-RAM fast path when the bus has no wait-state devices
* precomputed page-range lookup for mapped devices
* amortized scheduler event dispatch without per-event slice shifting
* reduced wait-state overhead when no device contributes extra wait states
* fewer allocations and less debug bookkeeping in normal benchmark loops
* predecoded opcode metadata for common decode fields
* Go 1.26 benchmark loops using `testing.B.Loop`

Representative results on October 4, 2026 on Apple M1 (`darwin/arm64`, Go 1.27.1) were:

* `BenchmarkBubbleSort`: ~2.67 ms/op
* `BenchmarkPrimeSieve`: ~5.13 ms/op
* `BenchmarkRunEightMillionCycles`: ~20.0 ms/op
* `BenchmarkRecursiveFibonacci`: ~18.3 ms/op
* `BenchmarkCycleSchedulerAdvanceBurst`: ~2.97 us/op
* `BenchmarkBusReadMappedRanges`: ~15.9 ns/op

`BenchmarkFFT` runs a 1024-point FFT in fixed-point, SFP004 and native FPU
variants on every model that supports them.

See [doc/benchmark_report.md](doc/benchmark_report.md) for more detail.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
