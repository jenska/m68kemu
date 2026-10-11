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

### FFT Across CPU Models

`BenchmarkFFT` (in `fft_benchmark_test.go`) runs a 1024-point complex radix-2
FFT on every model that can execute each arithmetic variant. All variants share
the same bit-reversal and stage/group loops; only the butterfly differs.
`TestFFTPrograms` checks each one against a float64 DFT.

```sh
go test -run '^$' -bench BenchmarkFFT -count=3 .
```

October 11, 2026, Apple M1, Go 1.27.1, medians:

| Variant | Machine | ns/op | instrs/fft | cycles/fft | Minstr/s | SNR |
| --- | --- | --- | --- | --- | --- | --- |
| Q15 (`MULS.W`) | MC68000 | `4885000` | 181174 | 2179904 | 37.1 | 46 dB |
| Q15 | MC68010 / 020 / 030 | `~5090000` | 181174 | 2179904 | 35.6 | 46 dB |
| Q31 (`MULS.L`) | MC68020 / 030 | `~4800000` | 167798 | 2345832 | 35.0 | 141 dB |
| SFP004 (memory-mapped FPU) | MC68000+MC68881 | `41750000` | 1006327 | 13087348 | 24.1 | 297 dB |
| SFP004 | MC68010+MC68881 | `42100000` | 1006327 | 13087348 | 23.9 | 297 dB |
| FPU (F-line) | MC68020+MC68881 / MC68030+MC68882 | `~10580000` | 166646 | 7189096 | 15.8 | 297 dB |

* The 68020's `MULS.L` gives 32-bit precision (+95 dB) with fewer instructions
  than the 16-bit 68000 code, at about the same cycle count.
* The FPU runs at the CPU's clock with the typical execution times of
  `fpu/timing.go` (see [fpu.md](fpu.md)). Double precision costs about 3x the
  cycles of fixed point: a butterfly takes about 1300 FPU cycles (six loads,
  four multiplies, six adds, four stores), while the integer code around it
  overlaps the FPU.
* On the SFP004 the 68000 polls the response CIR while the FPU is busy, so
  its instruction count includes the polling loops and depends on the FPU
  timing. It needs 1.8x the cycles of the native coprocessor interface: every
  command, operand move and poll is a bus access from 68000 code.
* The MC68000 is about 4% faster on the host than the other models, which
  save the registers at every instruction so a bus or address error can
  restart it.
* Profiling the first version of these benchmarks led to the current numbers
  (before: Q15 on the MC68010 to MC68030 5.6 ms, SFP004 86 ms, FPU 11.9 ms):
  the MC68010 and later had taken the debug-aware execution path at every
  instruction; a second device on the bus (the CIR) had turned off the direct
  RAM path, so every fetch went through the bus's device lookup; and every
  coprocessor instruction decoded its command word four times and allocated
  its operands.
* Integer cycle counts from the 68020 on reuse the 68000 tables and are an
  approximation.

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
