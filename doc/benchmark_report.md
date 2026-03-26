# Benchmark Report

## Current Results

Benchmarks were run on March 25, 2026 on an Apple M1 (`darwin/arm64`) with:

```sh
go test -run '^$' -bench 'BenchmarkRecursiveFibonacci|BenchmarkBubbleSort|BenchmarkPrimeSieve' -benchmem
```

Results:

| Benchmark | Result | Allocations |
| --- | --- | --- |
| `BenchmarkBubbleSort` | `3078764 ns/op` | `0 B/op, 0 allocs/op` |
| `BenchmarkPrimeSieve` | `5836703 ns/op` | `4 B/op, 1 allocs/op` |
| `BenchmarkRecursiveFibonacci` | `28598799 ns/op` | `0 B/op, 0 allocs/op` |

## What Improved

Compared with the earlier benchmark notes in this repository, the current core is materially faster and cleaner in the common execution path:

* bus access now has direct fast paths for single-device setups
* fixed-range device mappings can be indexed efficiently by 24-bit address pages
* RAM no longer advertises zero wait states through the dynamic wait-state interface, which removes unnecessary bookkeeping
* reset / benchmark loops no longer allocate in the common CPU path
* opcode metadata used by EA decoding is precomputed once up front

These changes were made while also improving correctness:

* CPU `RESET` no longer clears RAM
* bus / address faults now use the richer 68000 group 0 exception frame
* several `A7` byte-sized edge cases were corrected
* `MOVEM.W` register loads now sign-extend properly

## Profiling Snapshot

Representative profiles were collected with:

```sh
go test -run '^$' -bench BenchmarkRecursiveFibonacci -cpuprofile /tmp/m68kemu_after.cpu.out
go test -run '^$' -bench BenchmarkBubbleSort -cpuprofile /tmp/m68kemu_after.bubble.cpu.out
go tool pprof -top /tmp/m68kemu_after.cpu.out
go tool pprof -top /tmp/m68kemu_after.bubble.cpu.out
```

### Recursive Fibonacci

Top remaining costs were still concentrated in the interpreter core:

* `(*Bus).wait`
* `(*Bus).Read`
* `(*cpu).read`
* `movel`
* `(*cpu).fetchOpcode`
* `ResolveSrcEA` / `ResolveDstEA`

This means the project has already harvested some of the easy structural wins, and future speed work is likely to come from deeper fetch / decode specialization rather than small local cleanup.

### Bubble Sort

The hot path is now dominated by:

* `(*cpu).RunCycles`
* `(*cpu).fetchOpcode`
* `branch`
* `(*cpu).executeInstruction`
* `(*Bus).Read`
* `(*RAM).Read`

Notably, device lookup no longer dominates the profile the way it did before the fixed-range and single-device fast paths were added.

## Current Optimization Priorities

If performance becomes the main focus again, the highest-value next steps are:

1. Add a direct program-fetch fast path for RAM / ROM regions.
2. Push opcode predecode further so more handlers can avoid repeated mode / register extraction.
3. Reduce interface-heavy EA setup on common register and simple memory forms.
4. Move from generic bus timing to machine-specific ST memory / MMIO timing tables as the chipset comes online.

## Notes

The benchmark numbers above reflect the current CPU core, not a full Atari ST machine. Once video, MFP, ACIA, DMA, and MMIO timing are integrated through the scheduler, overall machine throughput will need to be re-measured under more realistic workloads.
