# Fibonacci benchmark profiling report

## Benchmark
- Command: `go test -bench=RecursiveFibonacci -benchmem -run=^$ -cpuprofile cpu.out`
- Result: **~33.6µs/op**, **64 B/op**, **1 alloc/op** (BenchmarkRecursiveFibonacci).

## CPU profile highlights
`go tool pprof -top cpu.out` shows the following hot spots:

- `Bus.wait` accounts for ~14.7% of sampled CPU time.
- The instruction dispatch path (`cpu.executeInstruction`) accumulates ~55% of total time, with repeated `Bus.Read` (~6%) and `Bus.findDevice` (~6%) calls dominating memory access overhead.
- Effective address resolution (`cpu.ResolveSrcEA`) and RAM boundary checks (`RAM.Contains`) each consume ~4–6% of samples.

## Optimization ideas
1. **Skip wait-state bookkeeping when no wait states are possible.**
   - In this benchmark the RAM device returns zero wait states, yet `Bus.wait` still runs the hook/type assertion on every access. Consider tracking whether any attached device can contribute wait states (e.g., flag set in `AddDevice` when a `WaitStateDevice` is registered) and bypass `wait` entirely when `waitStates == 0` and the flag is false. This would reduce the top hotspot without changing semantics.
2. **Fast-path device lookup for uniform memory.**
   - With only a single RAM device, `Bus.findDevice` still performs a `Contains` check and slice iteration per access. Adding a cached direct device pointer or a code path for `len(devices)==1` would avoid repeated range checks and shrink the `Bus.Read`/`findDevice` cost.
3. **Reduce fetch overhead in the execution loop.**
   - `cpu.fetchOpcode` and `executeInstruction` repeatedly call `bus.read` even when no breakpoints are configured. Introducing a "no-breakpoints" fast path (e.g., struct field that short-circuits `checkAccessBreakpoint`/`checkExecuteBreakpoint`) or inlining the common RAM fetch when the bus is known to be RAM-backed could trim the cumulative time spent in `cpu.executeInstruction`.
4. **Flatten effective-address setup.**
   - The EA helpers (`ResolveSrcEA`/`ResolveDstEA` and the `ea*` init methods) show up in the profile. Caching pre-decoded EA handler structs per opcode or reusing allocated EA structs instead of reinitializing them every instruction could reduce this ~5–10% slice.
