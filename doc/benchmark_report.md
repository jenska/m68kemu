# Benchmark Report

## Current Results

Benchmarks were run on June 13, 2026 on an Apple M1 (`darwin/arm64`) with Go 1.26.3:

```sh
go test -run '^$' -bench 'Benchmark(BubbleSort|PrimeSieve|RunEightMillionCycles|RecursiveFibonacci|CycleSchedulerAdvanceBurst|BusReadManyDevices)$' -benchmem -count=5 .
```

Representative medians from the runs:

| Benchmark | Result | Allocations |
| --- | --- | --- |
| `BenchmarkBubbleSort` | `2537946 ns/op` | `0 B/op, 0 allocs/op` |
| `BenchmarkPrimeSieve` | `5035525 ns/op` | `4 B/op, 1 allocs/op` |
| `BenchmarkRunEightMillionCycles` | `25526229 ns/op` | `0 B/op, 0 allocs/op` |
| `BenchmarkRecursiveFibonacci` | `26577540 ns/op` | `0 B/op, 0 allocs/op` |
| `BenchmarkCycleSchedulerAdvanceBurst` | `3291 ns/op` | `0 B/op, 0 allocs/op` |
| `BenchmarkBusReadManyDevices` | linear device lookup | `0 B/op, 0 allocs/op` |

## What Improved

Compared with the earlier benchmark notes in this repository, the current core remains materially faster and cleaner in the common execution path:

* the bus was simplified to linear device lookup and global wait-state accounting
* reset / benchmark loops no longer allocate in the common CPU path
* opcode metadata used by EA decoding is precomputed once up front
* debug hooks were reduced to a compact instruction tracer
* Go 1.26 benchmark loops use `testing.B.Loop`

These changes were made while also improving correctness:

* CPU `RESET` no longer clears RAM
* bus / address faults now use the richer 68000 group 0 exception frame
* several `A7` byte-sized edge cases were corrected
* `MOVEM.W` register loads now sign-extend properly

## Profiling Snapshot

Representative profiles were collected with:

```sh
go test -run '^$' -bench BenchmarkRecursiveFibonacci -cpuprofile /tmp/m68kemu_recursive_2026-06-13.cpu.out .
go test -run '^$' -bench BenchmarkBubbleSort -cpuprofile /tmp/m68kemu_bubble_2026-06-13.cpu.out .
go tool pprof -top /tmp/m68kemu_recursive_2026-06-13.cpu.out
go tool pprof -top /tmp/m68kemu_bubble_2026-06-13.cpu.out
```

### Recursive Fibonacci

Top remaining costs are still concentrated in the interpreter core:

* `ResolveSrcEA`
* `(*CPU).fetchOpcode`
* `(*Bus).Read`
* `executeInstruction`
* `executeNext`
* `movel`

This means the project has already harvested the easy debug-path wins, and future speed work is likely to come from deeper fetch / decode specialization rather than small local cleanup.

### Bubble Sort

The hot path is now dominated by:

* `(*CPU).executeNext`
* `(*Bus).Read`
* `(*CPU).executeInstruction`
* `(*CPU).RunCycles`
* `branch`
* `(*CPU).fetchOpcode`

Notably, the remaining time is concentrated in instruction fetch / dispatch and simple memory lookup rather than broad bus indirection, heap allocation, or always-on debug plumbing.

## Current Optimization Priorities

If performance becomes the main focus again, the highest-value next steps are:

1. Trim hot-loop instruction fetch overhead in `fetchOpcode`, bus reads, and related bookkeeping.
2. Push opcode predecode further so more handlers can avoid repeated mode / register extraction.
3. Reduce EA setup overhead on common register, displacement, and simple memory forms.
4. Keep debug hooks behind cached mode flags so new observability features do not drift back into the hot path.
5. Move from generic bus timing to machine-specific ST memory / MMIO timing tables as the chipset comes online.

## Notes

The benchmark numbers above reflect the current CPU core, not a full Atari ST machine. Once video, MFP, ACIA, DMA, and MMIO timing are integrated through the scheduler, overall machine throughput will need to be re-measured under more realistic workloads.
