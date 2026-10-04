# Benchmark Report

## Current Results

Benchmarks were run on October 4, 2026 on an Apple M1 (`darwin/arm64`) with Go 1.27.1:

```sh
go test -run '^$' -bench 'Benchmark(BubbleSort|PrimeSieve|RunEightMillionCycles|RecursiveFibonacci|CycleSchedulerAdvanceBurst|BusReadMappedRanges)$' -benchmem -count=5 .
```

Representative medians from the runs, compared with the previous report (June 13, 2026, Go 1.26.3, same machine):

| Benchmark | Result | Allocations | June 2026 | Change |
| --- | --- | --- | --- | --- |
| `BenchmarkBubbleSort` | `2614994 ns/op` | `0 B/op, 0 allocs/op` | `2537946 ns/op` | +3% |
| `BenchmarkPrimeSieve` | `5001071 ns/op` | `4 B/op, 1 allocs/op` | `5035525 ns/op` | -1% |
| `BenchmarkRunEightMillionCycles` | `19508466 ns/op` | `0 B/op, 0 allocs/op` | `25526229 ns/op` | -24% |
| `BenchmarkRecursiveFibonacci` | `18013311 ns/op` | `0 B/op, 0 allocs/op` | `26577540 ns/op` | -32% |
| `BenchmarkCycleSchedulerAdvanceBurst` | `2948 ns/op` | `0 B/op, 0 allocs/op` | `3291 ns/op` | -10% |
| `BenchmarkBusReadMappedRanges` | `164.4 ns/op` | `0 B/op, 0 allocs/op` | `15.53 ns/op` | +959% |

### Bus Lookup Regression

`BenchmarkBusReadMappedRanges` is about 10x slower than in June. Bisecting points to
commit `731b476` ("Make Device.Contains optional"): its parent still measures about
`15.5 ns/op`.

Since that change, `refreshTopology` page-maps only devices that implement
`AddressRangeDevice` *without* `ContainsDevice`. A device that implements both goes on
the linear scan list instead. The benchmark's `stubMappedDevice` implements both, so
it now measures a 64-entry linear scan rather than the page map.

This affects real setups too, not only the benchmark: `RAM` and the internal
`mappedDevice` both implement `Contains` and `AddressRange`, so on a bus with more than
one device none of the built-in devices use the page map. The `Device` documentation
says that for such devices "Contains decides membership and AddressRange only bounds
it"; using the range to page-map the device and then confirming with `Contains` would
restore the fast path without changing that contract.

## What Improved

Compared with the earlier benchmark notes in this repository, the current core remains materially faster and cleaner in the common execution path:

* bus access now has direct fast paths for single-device setups
* single-RAM bus fast paths are cached when no wait-state devices are attached
* fixed-range device mappings can be indexed efficiently by 24-bit address pages
* RAM no longer advertises zero wait states through the dynamic wait-state interface, which removes unnecessary bookkeeping
* reset / benchmark loops no longer allocate in the common CPU path
* opcode metadata used by EA decoding is precomputed once up front
* debug hooks now stay off the hot path unless a tracer, history buffer, or stop-condition collector is actually active
* untraced execution avoids unnecessary register snapshots and fetch trace context
* Go 1.26 benchmark loops use `testing.B.Loop`

These changes were made while also improving correctness:

* CPU `RESET` no longer clears RAM
* bus / address faults now use the richer 68000 group 0 exception frame
* several `A7` byte-sized edge cases were corrected
* `MOVEM.W` register loads now sign-extend properly

## Profiling Snapshot

Representative profiles were collected with:

```sh
go test -run '^$' -bench BenchmarkRecursiveFibonacci -cpuprofile /tmp/m68kemu_recursive_2026-10-04.cpu.out .
go test -run '^$' -bench BenchmarkBubbleSort -cpuprofile /tmp/m68kemu_bubble_2026-10-04.cpu.out .
go tool pprof -top /tmp/m68kemu_recursive_2026-10-04.cpu.out
go tool pprof -top /tmp/m68kemu_bubble_2026-10-04.cpu.out
```

### Recursive Fibonacci

The top flat costs are now:

* `movel` (about 35% cumulative, including its EA work)
* `readProgramFastWord`
* `(*cpu).checkInterrupts`
* `add`
* `fastRAMRead`
* `(*cpu).fetchOpcode`
* `ResolveSrcEA`

Fetch and dispatch overhead has dropped enough that the instruction handlers themselves
(`movel`, `add`) now show up near the top. Operand access through `fastRAMRead` and EA
resolution is the next layer down.

### Bubble Sort

The hot path is now dominated by:

* `(*cpu).RunCycles`
* `(*cpu).checkInterrupts`
* `readProgramFastWord`
* `(*cpu).executeNext`
* `branch`
* `(*cpu).fetchOpcode`
* `(*cpu).dispatchInstruction`

Bubble sort did not get the speedup Fibonacci did. It runs tight loops of short
instructions, so per-instruction overhead dominates, and `checkInterrupts` now
accounts for about 11% of its samples. Checking for pending interrupts only when the
interrupt state changes, rather than on every instruction, is the most direct
remaining win for this kind of loop.

## Current Optimization Priorities

If performance becomes the main focus again, the highest-value next steps are:

1. Restore page-mapped lookup for devices that implement both `AddressRange` and `Contains` (see "Bus Lookup Regression").
2. Avoid the per-instruction `checkInterrupts` call when no interrupt state has changed.
3. Trim hot-loop instruction fetch overhead in `fetchOpcode`, `readProgramFastWord`, and related bookkeeping.
4. Push opcode predecode further so more handlers can avoid repeated mode / register extraction.
5. Reduce EA setup overhead on common register, displacement, and simple memory forms.
6. Keep debug hooks behind cached mode flags so new observability features do not drift back into the hot path.
7. Move from generic bus timing to machine-specific ST memory / MMIO timing tables as the chipset comes online.

## Notes

The benchmark numbers above reflect the current CPU core, not a full Atari ST machine. Once video, MFP, ACIA, DMA, and MMIO timing are integrated through the scheduler, overall machine throughput will need to be re-measured under more realistic workloads.
