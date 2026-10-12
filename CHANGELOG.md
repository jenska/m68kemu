# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/SemVer).

## [Unreleased]

### Added

- CALLM and RTM on the MC68020 for type $00 module descriptors; type $01
  descriptors and frames, which need access control hardware, and other
  types take the format error exception. See `doc/mc68020.md`

## [1.7.0] - 2026-10-11

### Added

- `Model` type with the constants `M68000` to `M68040`, the `WithModel`
  option and a `Model()` method on `CPU`. `M68000` and `M68010` are
  implemented; `NewCPU` returns `ErrModelUnsupported` for the others
- MC68010 model (`WithModel(M68010)`): the VBR, SFC and DFC registers
  (new fields in `Registers`); MOVEC, MOVES, RTD and MOVE CCR,<ea>;
  privileged MOVE SR,<ea>; format $0 and $8 exception stack frames; RTE
  with format checking and the format-error exception (`XFormatError`).
  RTE from a bus or address error frame reruns the faulted instruction.
  Timing reuses the MC68000 cycle counts. See `doc/mc68010.md`
- `ExceptionStackFrameFormat0` and `ExceptionStackFrameFormat8`, and the
  `ExceptionStackFrame` fields `VectorOffset`, `DataOutput`, `DataInput`
  and `InstructionInput`; `ReadExceptionStackFrame` decodes the new formats
- `BusAccessInfo.FunctionCode`
- MC68020 and MC68030 models (`WithModel(M68020)`, `WithModel(M68030)`):
  32-bit addresses, scaled-index and memory indirect addressing, MSP/ISP and
  the M bit, CACR and CAAR, the 68020 instructions, misaligned operands,
  format $1, $2 and $B frames with instruction restart after bus faults, and
  the coprocessor interface (`Coprocessor`, `WithCoprocessor`,
  `ErrCoprocessorUnsupported`). Timing is approximate; CALLM/RTM and the
  68030 MMU are not implemented. See `doc/mc68020.md`
- 32-bit buses: `NewCPU` sets the bus to the address width of the model,
  32 bits from the MC68020 on and 24 bits before, and `Bus.AddressMask`
  reports it; `Registers.MSP`, `CACR` and `CAAR`; `ExceptionStackFrameFormat1`, `2`,
  `9`, `A` and `B` with the frame fields `InstructionAddress` and
  `StageBAddress`
- `fpu.FPU.Coprocessor` attaches the FPU to an MC68020 or MC68030, which
  then executes the F-line FPU instructions
- FPU execution timing: `fpu.FPU.Cycles` with a table of typical
  MC68881/MC68882 execution times; `m68kemu.CoprocessorTiming`, through
  which a coprocessor runs concurrently with the 68020/68030 and stalls its
  next instruction until done; `fpu.CIR.Clock`, which makes the
  memory-mapped FPU report busy while a command runs. See `doc/fpu.md`
- `BenchmarkFFT`: a 1024-point FFT in Q15, Q31, SFP004 and native FPU
  variants across the CPU models. See `doc/benchmark_report.md`
- Package `fpu`: an MC68881/MC68882 FPU built on `github.com/jenska/float`,
  with all general instructions, operand formats including packed decimal,
  FMOVECR, condition predicates, exceptions and FSAVE/FRESTORE frames, and
  `fpu.CIR`, its memory-mapped coprocessor interface for 68000 machines
  (Atari Mega ST/STE at `$FFFA40`). See `doc/fpu.md`
- `MemoryDevice`: a device that is plain memory hands the CPU its backing
  store, and the CPU reads, writes and fetches from it directly. `RAM` and
  the new `ROM` (`NewROM`; writes are bus errors) implement it. This works
  for every memory device on the bus, also beside other devices, under an
  overlay and with wait states, which the CPU charges as the bus does
- `Bus.Remap` rebuilds the address decoding after a device changes its
  range or backing store, for example on a bank switch
- `RAM.Load` and `RAM.Bytes` for loading and inspecting memory
- `CPU.SetRegisters` and `CPU.SetPC`
- `ErrBusInUse`: `NewCPU` rejects a bus that already serves a CPU, which
  used to break the first CPU's wait-state accounting silently

### Changed

- **Breaking:** `NewCPU` returns the concrete type `*CPU` instead of the
  `CPU` interface. Code that only calls methods keeps working; code that
  names the type writes `*m68kemu.CPU`, and code that mocked the interface
  defines its own
- **Breaking:** `Registers.D` is `[8]uint32`, like `Registers.A`, instead of
  `[8]int32`. Convert with `int32(regs.D[n])` where a signed value is meant
- **Breaking:** the cycle scheduler works by deadline: the CPU compares its
  cycle counter with the time of the next event and calls into the
  scheduler only when an event is due, instead of on every cycle charge.
  `CycleListener` and `CycleScheduler.AddListener` are removed; a device
  that changes continuously computes its state from `Now()`, which is exact
  also in the middle of an instruction. An attached scheduler used to slow
  the CPU down by 24%, plus more for every listener
- `Step` no longer rebuilds the CPU's cached run modes on every call, about
  16% faster per call. See `doc/benchmark_report.md` for the other effects
  of this rework

- The opcode dispatch tables are built per model on first use and shared by
  every CPU of that model, instead of being package-level arrays pruned in
  place by the first `NewCPU`. The MC68000 table is unchanged
- `CPU` has a new method, `Model()`, which breaks code that implements the
  interface outside this package
- The MC68010 and later run on the same fast execution path as the MC68000
  while no debug hook is active, about 9% faster
- The CPU reads and writes a RAM directly also when other devices share the
  bus, as long as no earlier device overlaps it and no wait states apply.
  The memory-mapped FPU benchmark (SFP004) runs twice as fast
- Coprocessor instructions decode the command word once and no longer
  allocate; the F-line FPU benchmark runs 11% faster
- `fpu.FPU.Execute` returns a slice that stays valid only until the next
  call, and `Coprocessor.Execute` may only use its operand slice during the
  call

### Removed

- **Breaking:** `CPU.SetFastMemory` and `FastRegion`. Implement
  `MemoryDevice` on the memory device instead, and call `Bus.Remap` where
  the fast regions used to be re-installed
- **Breaking:** `CPU.SetTracer`, `SetBusTracer` and `SetExceptionTracer`.
  Use `SetHooks`, starting from `Hooks()` to keep the other callbacks
- **Breaking:** `CPU.RunInstructions`. Use
  `RunUntil(RunUntilOptions{MaxInstructions: n})` or a loop of `Step`

### Fixed

- CPUs running in parallel goroutines corrupted each other's operands: the
  effective-address operand objects were shared package-level values. Every
  CPU now has its own, so separate CPUs (each with its own `Bus`) can run
  concurrently
- `RequestInterrupt` is safe to call from any goroutine, also while the CPU
  runs; it used to race with the CPU's interrupt check

## [1.6.2] - 2026-10-04

### Changed

- Refreshed README and benchmark documentation with Go 1.27.1 results

### Fixed

- Bus lookup on a multi-device bus is fast again: devices that implement both
  `AddressRange` and `Contains` (including `RAM` and `MapDevice` wrappers) had
  fallen back to a linear scan since `Contains` became optional, making a
  64-device lookup about 10x slower. They are page-mapped again, with
  `Contains` still filtering sparse decodes

## [1.6.1] - 2026-10-04

### Fixed

- `MOVEP` swapped its size and direction bits: opmode bit 7 selects register
  to memory and bit 6 a long transfer. A word load ran as a long store and
  so on; the tests passed only because m68kasm before v1.6.0 had the
  mirror-image encoding bug
- Opcode words the MC68000 does not have now raise the illegal-instruction
  exception instead of executing: `MOVE.B`/`CMP.B` from an address register,
  `ADDQ.B`/`SUBQ.B` to one, `LEA`/`PEA` with non-control operands, `TST`
  with PC-relative or immediate operands, `BCHG`/`BCLR`/`BSET` with a
  PC-relative operand and others. A table of the Motorola operation code map
  decides, and a test checks all 65536 opcode words against it
- `MOVE` with a nonexistent destination mode and memory shifts with an
  immediate or nonexistent operand crashed the CPU (index out of range)
- `CHK #<data>,Dn` and `BTST Dn,#<data>` are accepted, as on the 68000

### Changed

- Bumped the `github.com/jenska/m68kasm` test dependency to v1.6.1

## [1.6.0] - 2026-10-03

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
- Exceptions that replace an instruction's time with their own (divide by
  zero, CHK, TRAPV, privilege violation, illegal and line A/F opcodes, bus and
  address errors) now advance an attached `CycleScheduler` as well; before,
  only the CPU cycle counter moved, so scheduled devices fell behind. `RESET`
  charges its reset pulse only after the privilege check, so a `RESET` in user
  mode costs 34 cycles instead of 132
- `NBCD` accepts every data-alterable addressing mode, not only `Dn` and
  `-(An)`; the other modes raised an illegal-instruction exception

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
