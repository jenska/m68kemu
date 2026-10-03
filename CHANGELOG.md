# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/SemVer).

## [Unreleased]

### Added

- `WithCycleRounding(n)` rounds every instruction's cycles (including the
  exception or interrupt processing it triggers) up to a multiple of `n`, for
  machines such as the Atari ST whose bus only grants the CPU 4-cycle slots

### Fixed

- Instruction timing now matches the M68000 User's Manual (section 8), checked
  by a reference test of about 200 instruction forms. Previously long operands
  in memory, read-modify-write instructions, `LEA`/`PEA`/`JMP`/`JSR`,
  `MOVEM`, `CMP`/`CMPA`/`EOR`, `ADDX.L`, the bit instructions, register shifts
  of longs and `MOVE SR,Dn` were off by 2-12 cycles, and memory shifts to the
  right were timed as register shifts
- Run-time dependent timing: `Bcc` taken/not taken and byte/word
  displacement, `DBcc` condition true/loop/expired, `Scc` true/false,
  `BCHG`/`BCLR`/`BSET` on bits 16-31 of a data register, `MOVEM` per
  addressing mode, and `MULU`/`MULS`/`DIVU`/`DIVS` by operand value (the
  divisions follow Jorge Cwik's exact algorithm) instead of their maxima

### Changed

- Bumped `github.com/jenska/m68kdasm` to v1.3.0 and `github.com/jenska/m68kasm`
  to v1.5.0
- Added regression tests covering `BRA`/`BSR.W` and `DBcc` disassembly branch
  targets, guarding against a m68kdasm bug (fixed in v1.3.0) where these were
  computed relative to the wrong base address

## [1.5.0] - 2026-09-08

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
