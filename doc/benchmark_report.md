# Benchmark Report

## Benchmark Run
- Command: `go test -bench=RunEightMillionCycles -benchmem`
- Result: `BenchmarkRunEightMillionCycles-2 21 50900486 ns/op 1003 B/op 0 allocs/op`
- Duration: ~51 ms per run, ~1.23 s total for benchmark suite.

## CPU Profile Bottlenecks
CPU profile collected with `go test -bench=RunEightMillionCycles -benchmem -cpuprofile cpu.out` and analyzed via `go tool pprof -top cpu.out` highlighted:

| Function | Flat % | Cumulative % | Notes |
| --- | --- | --- | --- |
| `(*cpu).Step` | 10.91% | 94.55% | Central per-instruction driver; dominates total time. |
| `(*cpu).executeInstruction` | 7.27% | 41.82% | Core decoding/execution logic. |
| `(*cpu).fetchOpcode` | 5.45% | 31.82% | Opcode fetch path, includes bus reads. |
| `(*Bus).wait` | 11.82% | 14.55% | Wait-state handling; overhead per memory access. |
| `(*cpu).ResolveDstEA` | 5.45% | 5.45% | Effective address resolution for destination operands. |
| `(*cpu).ResolveSrcEA` | 4.55% | 8.18% | Effective address resolution for source operands. |

## Observations
- Execution hot path is the CPU stepping pipeline (`Step` → `executeInstruction` → `fetchOpcode`). Optimizing instruction dispatch (e.g., reducing branching or using table-driven decoding) could yield broad gains.
- Bus wait-state calculations consume noticeable time; consider caching device lookups or minimizing wait checks for contiguous accesses.
- Effective address resolution for both source and destination operands is a recurring cost; memoizing addressing mode metadata or streamlining `eaRegister` operations may reduce overhead.

## Next Steps
- Profile with larger instruction mixes and compare with realistic workloads to ensure hotspot consistency.
- Experiment with optimizing bus/device lookup and EA resolution paths, then rerun benchmarks to measure impact.

## Recursive Fibonacci Benchmark (call-heavy workload)
- Command: `./bench.test -test.run=^$ -test.bench=BenchmarkRecursiveFibonacci -test.benchmem -test.count=1`
- Result: `BenchmarkRecursiveFibonacci-2 42184 31050 ns/op 1 B/op 0 allocs/op`
- Duration: ~31 µs per run over ~42k iterations (≈1.3 s total for the benchmark).

### CPU Profile Bottlenecks
CPU profile collected with `./bench.test ... -test.cpuprofile=fib_cpu.out` and analyzed via `PPROF_PAGER=cat go tool pprof -top ./bench.test fib_cpu.out` highlighted:

| Function | Flat % | Cumulative % | Notes |
| --- | --- | --- | --- |
| `runtime.futex` | 23.02% | 23.02% | Kernel futex waits show runtime synchronization overhead from frequent timer start/stop boundaries inside the benchmark harness. |
| `(*cpu).Step` | 2.16% | 29.50% | Per-instruction stepping dominates cumulative time along the interpreter hot path. |
| `(*cpu).executeInstruction` | 1.68% | 17.51% | Instruction dispatch/decoding continues to be a significant portion of total cycles. |
| `(*Bus).wait` | 2.64% | 3.12% | Wait-state handling remains a visible per-memory-access cost. |
| `testing.(*B).StopTimer/StartTimer` | 0% | 69.54% | Benchmark harness timer toggling is the majority of cumulative samples because the benchmark reloads the program every iteration under a stopped timer. |

### Observations
- The hot path remains the CPU stepping pipeline (Step → executeInstruction), similar to the earlier instruction-mix benchmark.
- Synchronization and timer toggling within the benchmark harness account for a large portion of samples; reducing timer on/off transitions or reusing preloaded memory could lower runtime overhead unrelated to emulator logic.
- Bus wait-state checks and address resolution still contribute measurable cost in the interpreter loop.

### Next Steps
- Rework the benchmark to preload the Fibonacci program once (or use `b.ResetTimer` after setup) to measure emulator performance without harness synchronization noise.
- Investigate optimizing instruction dispatch and effective-address resolution to further cut per-step overhead.
