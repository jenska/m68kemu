# Recursive Fibonacci Profile

## Benchmark

Profiled with:

```sh
go test -run '^$' -bench BenchmarkRecursiveFibonacci -cpuprofile /tmp/m68kemu_after.cpu.out
go tool pprof -top /tmp/m68kemu_after.cpu.out
```

Observed benchmark result on March 25, 2026:

* `BenchmarkRecursiveFibonacci`: about `28.6 ms/op`
* `0 B/op, 0 allocs/op`

## Profile Highlights

The recursive Fibonacci workload is still useful because it stresses:

* frequent opcode fetches
* stack activity through subroutines
* branches and returns
* repeated simple EA resolution

Recent profiling showed these as the main remaining costs:

* `(*Bus).wait`
* `(*Bus).Read`
* `(*cpu).read`
* `(*cpu).fetchOpcode`
* `movel`
* `ResolveSrcEA` / `ResolveDstEA`

## What Changed Since Earlier Profiles

Several previously identified bottlenecks have already been addressed:

* single-device and fixed-range bus fast paths reduced repeated device-lookup overhead
* unnecessary dynamic wait-state work for plain RAM was removed
* reset-time allocations in the benchmark path were eliminated
* some decode metadata is now precomputed instead of rebuilt on every instruction

That means the profile has shifted from "obvious structural overhead" toward the remaining core interpreter work.

## Interpretation

At this point the main opportunities are less about generic Go cleanup and more about emulator-specific specialization:

1. Faster linear program fetch for RAM / ROM-backed regions.
2. More aggressive opcode predecode so handlers can avoid repeated bit extraction.
3. Lower-overhead EA resolution for the common register, displacement, and postincrement forms.
4. Later, ST-specific memory timing so the bus path can stay fast while still modeling machine behavior.

## Why This Matters For Atari ST Work

Recursive Fibonacci is not a realistic ST workload, but it does approximate the kind of control-flow-heavy execution that makes interpreter overhead visible. It is a good regression benchmark for CPU-core changes before full machine devices are added and start contributing their own timing costs.
