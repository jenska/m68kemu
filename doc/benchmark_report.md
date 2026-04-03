# Benchmark Report

## Current Results

Benchmarks were run on April 3, 2026 on an Apple M1 (`darwin/arm64`) with:

```sh
go test -run '^$' -bench 'BenchmarkBubbleSort|BenchmarkPrimeSieve|BenchmarkRunEightMillionCycles|BenchmarkRecursiveFibonacci' -benchmem -count=3
```

Representative medians from the three runs:

| Benchmark | Result | Allocations |
| --- | --- | --- |
| `BenchmarkBubbleSort` | `2745358 ns/op` | `0 B/op, 0 allocs/op` |
| `BenchmarkPrimeSieve` | `5104709 ns/op` | `4 B/op, 1 allocs/op` |
| `BenchmarkRunEightMillionCycles` | `26798891 ns/op` | `0 B/op, 0 allocs/op` |
| `BenchmarkRecursiveFibonacci` | `27085294 ns/op` | `0 B/op, 0 allocs/op` |

## What Improved

Compared with the earlier benchmark notes in this repository, the current core remains materially faster and cleaner in the common execution path:

* bus access now has direct fast paths for single-device setups
* fixed-range device mappings can be indexed efficiently by 24-bit address pages
* RAM no longer advertises zero wait states through the dynamic wait-state interface, which removes unnecessary bookkeeping
* reset / benchmark loops no longer allocate in the common CPU path
* opcode metadata used by EA decoding is precomputed once up front
* debug hooks now stay off the hot path unless a tracer, history buffer, or stop-condition collector is actually active

These changes were made while also improving correctness:

* CPU `RESET` no longer clears RAM
* bus / address faults now use the richer 68000 group 0 exception frame
* several `A7` byte-sized edge cases were corrected
* `MOVEM.W` register loads now sign-extend properly

## Profiling Snapshot

Representative profiles were collected with:

```sh
go test -run '^$' -bench BenchmarkRecursiveFibonacci -cpuprofile /tmp/m68kemu_recursive_2026-04-02.cpu.out
go test -run '^$' -bench BenchmarkBubbleSort -cpuprofile /tmp/m68kemu_bubble_2026-04-02.cpu.out
go tool pprof -top /tmp/m68kemu_recursive_2026-04-02.cpu.out
go tool pprof -top /tmp/m68kemu_bubble_2026-04-02.cpu.out
```

### Recursive Fibonacci

Top remaining costs are still concentrated in the interpreter core:

* `ResolveSrcEA`
* `(*cpu).fetchOpcode`
* `readProgramFastWord`
* `executeInstruction`
* `movel`

This means the project has already harvested the easy debug-path wins, and future speed work is likely to come from deeper fetch / decode specialization rather than small local cleanup.

### Bubble Sort

The hot path is now dominated by:

* `(*cpu).executeNext`
* `(*cpu).fetchOpcode`
* `readProgramFastWord`
* `(*cpu).executeInstruction`
* `ResolveSrcEA`
* `(*cpu).RunCycles`

Notably, the remaining time is concentrated in instruction fetch / dispatch and simple memory lookup rather than broad bus indirection, heap allocation, or always-on debug plumbing.

## Current Optimization Priorities

If performance becomes the main focus again, the highest-value next steps are:

1. Trim hot-loop instruction fetch overhead in `fetchOpcode`, `readProgramFastWord`, and related bookkeeping.
2. Push opcode predecode further so more handlers can avoid repeated mode / register extraction.
3. Reduce EA setup overhead on common register, displacement, and simple memory forms.
4. Keep debug hooks behind cached mode flags so new observability features do not drift back into the hot path.
5. Move from generic bus timing to machine-specific ST memory / MMIO timing tables as the chipset comes online.

## Notes

The benchmark numbers above reflect the current CPU core, not a full Atari ST machine. Once video, MFP, ACIA, DMA, and MMIO timing are integrated through the scheduler, overall machine throughput will need to be re-measured under more realistic workloads.
