# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/SemVer).

## [Unreleased]

### Added
- `NewCPU` accepts functional options; `WithDeferredReset` skips the implicit
  reset so callers can finish wiring the bus before the reset vector is read
- `CPU.SetHooks(Hooks)` / `CPU.Hooks()` install or read every observation
  callback in one call
- `CPU.SetFastMemory(...FastRegion)` maps caller-owned flat memory slices that
  the interpreter reads, writes, and fetches from directly, bypassing the bus
  for addresses inside a region; accesses outside every region fall through to
  the bus, writes to a `ReadOnly` region (ROM) also fall through, `WaitStates`
  is charged per access, and regions are bypassed automatically while a
  breakpoint or tracer is active
- `AutoVector` constant for `CPU.RequestInterrupt` / device interrupt requests
- `CPU.SetIRQSource(IRQSource)` — a level-sensitive interrupt line the core
  samples every instruction boundary (`PendingIRQ` / `AckIRQ`), for machines
  whose peripherals hold a line rather than posting one-shot requests
- `CycleScheduler.SetClockRatio(deviceHz, cpuHz)` — run scheduler time and
  listener deltas in a device clock that differs from the CPU clock, carrying
  the sub-cycle remainder (default stays 1:1)
- `ContainsDevice` optional interface for non-contiguous decode

### Changed
- `Device` no longer requires `Contains`. A device is located by implementing
  `AddressRangeDevice` (contiguous, and then page-mapped) or `ContainsDevice`
  (non-contiguous, linear scan); implementing neither now panics at bus
  construction. The bus resolves each device's containment test once instead of
  per access.
- `CPU.RequestInterrupt(level, vector uint8)` — the vector is a plain value now;
  pass `AutoVector` (the zero value) to auto-vector, instead of a `*uint8`
- `NewCPU` takes `*Bus` directly rather than an `AddressBus` interface, so the
  wait-state and fast-RAM paths can no longer be silently bypassed by a custom
  bus type
- `CPU.SetPreTracer` and `CPU.SetInterruptTracer` are removed; set those two
  callbacks through `SetHooks`. `SetTracer`, `SetBusTracer`, and
  `SetExceptionTracer` remain as single-callback convenience setters

### Removed
- Unexported internals that were never part of the intended surface:
  `InterruptController`, `MappedDevice`, `ScheduledEvent`, `WaitHook`,
  `Bus.SetWaitHook` (`Bus.SetWaitStates` is unchanged)

## [1.3.0] - 2026-06-13

### Changed
- Updated the module to target Go 1.26 and applied Go 1.26 modernizers
- Updated m68kasm to v1.3.1
- Converted benchmarks to `testing.B.Loop`
- Refreshed README and benchmark documentation with current Go 1.26.3 results

### Performance
- Cached the single-RAM fast path for buses without wait-state devices
- Reduced normal interpreter hot-path work when tracing and debug history are disabled
- Avoided unnecessary register snapshots and bus-trace bookkeeping in untraced execution

## [1.2.3] - 2026-04-02

### Fixed
- Corrected 68000 Line-A and Line-F exception frames to stack the trapping opcode address
- Expanded validation around USP moves, privilege traps, and supervisor/user stack bank switching

## [1.2.1] - 2026-03-28

### Fixed
- Fixed MOVEM control-mode sequencing
- Added support for address-register sources in ADD and SUB instructions

## [1.2.2] - 2026-03-29

### Fixed
- Enhanced arithmetic and bit operation handling with immediate values
- Added new tests for instruction behavior

## [1.2.0] - 2026-03-28

### Changed
- Updated m68kasm to v1.3.0, adding support for $ in expressions and .w/.l label suffixes
- Updated m68kdasm to v1.0.1, fixing MOVEM register decoding and SWAP instruction handling
- Fixed PC-relative addressing calculation in PEA instruction test

### Dependencies
- github.com/jenska/m68kasm v1.3.0
- github.com/jenska/m68kdasm v1.0.1

## [1.1.0] - 2024-12-01

### Added
- Initial release

## [1.0.1] - 2024-11-15

### Fixed
- Bug fixes

## [1.0.0] - 2024-11-01

### Added
- Initial release
