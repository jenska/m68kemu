# Benchmark Report

## Current Results

Benchmarks were run on October 4, 2026 on an Apple M1 (`darwin/arm64`) with Go 1.27.1:

```sh
go test -run '^$' -bench 'Benchmark(BubbleSort|PrimeSieve|RunEightMillionCycles|RecursiveFibonacci|CycleSchedulerAdvanceBurst|BusReadMappedRanges)$' -benchmem -count=5 .
```

Representative medians from the runs, compared with the previous report (June 13, 2026, Go 1.26.3, same machine):

| Benchmark | Result | Allocations | June 2026 | Change |
| --- | --- | --- | --- | --- |
| `BenchmarkBubbleSort` | `2668031 ns/op` | `0 B/op, 0 allocs/op` | `2537946 ns/op` | +5% |
| `BenchmarkPrimeSieve` | `5125765 ns/op` | `4 B/op, 1 allocs/op` | `5035525 ns/op` | +2% |
| `BenchmarkRunEightMillionCycles` | `20025338 ns/op` | `0 B/op, 0 allocs/op` | `25526229 ns/op` | -22% |
| `BenchmarkRecursiveFibonacci` | `18294825 ns/op` | `0 B/op, 0 allocs/op` | `26577540 ns/op` | -31% |
| `BenchmarkCycleSchedulerAdvanceBurst` | `2967 ns/op` | `0 B/op, 0 allocs/op` | `3291 ns/op` | -10% |
| `BenchmarkBusReadMappedRanges` | `15.90 ns/op` | `0 B/op, 0 allocs/op` | `15.53 ns/op` | +2% |

### Bus Lookup Regression (Fixed)

Commit `731b476` ("Make Device.Contains optional") page-mapped only devices that
implement `AddressRangeDevice` *without* `ContainsDevice`; anything with a `Contains`
method went on a linear scan. Since `RAM` and the internal `mappedDevice` implement
both, no built-in device used the page map on a multi-device bus, and
`BenchmarkBusReadMappedRanges` (64 devices) slowed from about `15.5 ns/op` to
`164 ns/op`.

Every device with a valid `AddressRange` is now page-mapped again, in bus order. A
device that also implements `Contains` keeps that check on its page entry; when it
rejects an address (a hole in a sparse decode), the lookup falls back to an ordered
scan so a later overlapping device can still answer.

The CPU benchmarks above run on a single-RAM bus, which goes through the `fastRAM`
path and never calls the bus lookup (a CPU profile shows no bus functions). They still
measured 2-4% slower than the commit before the fix in interleaved A/B runs, which
points to a code-layout effect rather than extra work.

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

1. Avoid the per-instruction `checkInterrupts` call when no interrupt state has changed.
2. Trim hot-loop instruction fetch overhead in `fetchOpcode`, `readProgramFastWord`, and related bookkeeping.
3. Push opcode predecode further so more handlers can avoid repeated mode / register extraction.
4. Reduce EA setup overhead on common register, displacement, and simple memory forms.
5. Keep debug hooks behind cached mode flags so new observability features do not drift back into the hot path.
6. Move from generic bus timing to machine-specific ST memory / MMIO timing tables as the chipset comes online.

## Notes

The benchmark numbers above reflect the current CPU core, not a full Atari ST machine. Once video, MFP, ACIA, DMA, and MMIO timing are integrated through the scheduler, overall machine throughput will need to be re-measured under more realistic workloads.
