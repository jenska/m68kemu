# m68kemu

A small Motorola 68000 emulator written in Go. The emulator exposes a CPU core with a programmable memory bus, effective-address helpers, a level-aware interrupt controller (including autovectors), and a growing set of instructions for experimenting with 68k code.

## Roadmap for m68kemu as a computer-emulator core
The steps below focus on improving m68kemu itself so it can serve as a reliable 68000 subsystem inside a broader computer emulator.

1. **Complete and verify the CPU**
   - Implement the full 68000 instruction set with accurate condition codes, status register transitions, privilege enforcement, and all addressing modes.
   - Add precise exception handling (reset, bus/address errors, traps, and interrupt stacking) and per-instruction cycle timing.
   - Provide trace-friendly hooks (per-instruction callbacks, disassembly output) so host emulators can integrate debugging and profiling tools.

2. **Strengthen bus and memory interfaces**
   - Evolve the `AddressBus` abstraction to cover byte/word/long access sizes, alignment rules, and bus faults so host machines can plug in their own memory maps and devices.
   - Support configurable wait-state insertion and optional cycle callbacks to let the host model contention or clock stretching.
   - Maintain a reference RAM/ROM implementation for quick testing while keeping the bus API decoupled from specific hardware layouts.

3. **Interrupt and timing model**
   - Expose interrupt level handling with clear APIs for asserting/deasserting lines, acknowledging vectors, and modeling autovectors.
   - Add cycle accounting and optional timeslice stepping so host emulators can coordinate the CPU with video, audio, and I/O timing.

4. **Testing and validation**
   - Integrate public 68000 conformance suites and add unit tests that cover edge cases in addressing modes, exceptions, and timing.
   - Provide sample harnesses that show how to embed m68kemu within a larger emulator, including minimal bus/device stubs.

5. **Developer experience**
   - Offer structured logs, watchpoints, and state snapshots to make host-level debugging straightforward.
   - Document integration patterns (e.g., how to drive the CPU in lockstep with other chips) to reduce friction for emulator authors.

## Project layout
- `internal/emu/` – core emulator types such as the CPU, register file, effective-address resolver, and the basic MOVEA word/long instructions. A minimal RAM implementation in this package satisfies the `AddressBus` interface for quick tests.
- `doc/M68kOpcodes.pdf` – opcode reference used while implementing and verifying instruction behavior.

## Supported instructions
The emulator currently covers a small but growing subset of 68000 opcodes:

- Data movement: `MOVE.{B/W/L}`, `MOVEA.{W/L}`, `MOVEQ`
- Address calculation/stack: `LEA`, `PEA`, `TRAP #n`, `NOP`
- BCD arithmetic: `ABCD`, `SBCD`, `NBCD` (register and predecrement memory forms)
- Shifts and rotates: `ASL/ASR`, `LSL/LSR`, `ROL/ROR`, `ROXL/ROXR` (register and memory forms with full condition code updates)

## Development
1. Ensure you have Go 1.25 or newer installed (see `go.mod`).
2. Use the provided Makefile for common tasks:

   ```sh
   make fmt         # format Go code
   make check       # formatting check, go vet, staticcheck, and tests
   make test        # run unit tests
   ```

## Example
The CPU can be stepped manually once you provide a memory implementation. The snippet below shows how to create a CPU with the built-in RAM helper and execute a single MOVEA instruction:

```go
ram := emu.NewRAM(0, 0x10000) // 64 KiB starting at address 0
cpu, _ := emu.NewCPU(&ram, 0x1000, 0x0200)

// Encode MOVEA.L (long) from absolute long address 0x00000400 into A0
_ = ram.WriteWordTo(0x0200, 0x2040) // opcode for MOVEA.L with absolute long source
_ = ram.WriteLongTo(0x0202, 0x00000400)
_ = ram.WriteLongTo(0x0400, 0xDEADBEEF) // data at the source address

_ = cpu.Step() // executes the single instruction
// cpu.Registers().A[0] now contains 0xDEADBEEF
```

This repository currently focuses on correctness of addressing calculations and MOVEA handling; additional instructions can be registered using the exposed `RegisterInstruction` helper.

### Tracing and breakpoints
Tracing can be enabled via the exported API by installing a tracer callback on the CPU. Breakpoints and watchpoints halt execution (or invoke callbacks) when an instruction is executed or when a specific address is read or written:

```go
cpu.SetTracer(func(info emu.TraceInfo) {
        fmt.Printf("PC=%04x SR=%04x\n", info.PC, info.SR)
})

cpu.AddBreakpoint(emu.Breakpoint{Address: 0x2000, OnExecute: true, Halt: true})
cpu.AddBreakpoint(emu.Breakpoint{Address: 0x4000, OnWrite: true, Halt: true})
```
