# m68kemu

A small Motorola 68000 emulator written in Go. The emulator exposes a CPU core with a programmable memory bus, effective-address helpers, a level-aware interrupt controller (including autovectors), and a growing set of instructions for experimenting with 68k code. Everything is packaged as a Go library so it can be embedded inside a larger machine emulator or used directly in tests and small tools.

## Features
- **Accurate CPU core**
  - Programmer-visible registers (D0–D7, A0–A7/USP/SSP), full 24-bit PC, and status register bits (X, N, Z, V, C) are maintained.
  - Supports per-instruction cycle accounting; `Cycles()` exposes the running tally, and registration helpers populate an opcode-specific cycle table.
  - Interrupt controller with level-based requests and autovector handling (`RequestInterrupt`) mirrors 68000 priorities and masks.
- **Memory bus and devices**
  - Pluggable `AddressBus` that routes byte/word/long accesses to registered devices, enforces alignment, and produces bus errors when unmapped regions are accessed.
  - Optional wait-state hook for cycle modeling and device-level `WaitStateDevice` support.
  - Simple `RAM` helper that implements the bus interfaces for quick experiments; additional devices can be added for ROMs or peripherals.
- **Debugging hooks**
  - Instruction tracing via `SetTracer`, reporting PC, SR, and register snapshots before each opcode runs.
  - Breakpoints/watchpoints with execution, read, and write triggers; callbacks can halt execution or react to memory touches.
- **Execution control**
  - Single-step via `Step()` or budgeted execution via `RunCycles(budget)` for coordinating the CPU with other subsystems.
  - Reset behavior that clears memory devices and initializes stack pointers from the reset vector in memory.

## Supported instructions
The emulator currently implements the following opcode families with proper condition-code updates and addressing modes appropriate to each instruction:

- **Data movement**: `MOVE.{B/W/L}`, `MOVEA.{W/L}`, `MOVEQ`, `MOVEM.{W/L}`, `MOVE USP`
- **Address calculation / stack / flow**: `LEA`, `PEA`, `ADDA.{W/L}`, `SUBA.{W/L}`, `JSR`, `JMP`, `LINK`, `UNLK`, `RTS`, `TRAP #n`, `TRAPV`, `RESET`, `STOP`, `NOP`, `BRA`, `BSR`, `Bcc`, `DBcc`
- **Arithmetic and loops**: `ADD.{B/W/L}` (both `<ea>,Dn` and `Dn,<ea>` forms), `SUBQ.{B/W/L}`
- **BCD arithmetic**: `ABCD`, `SBCD`, `NBCD` (register and predecrement memory forms)
- **Logic and flags**: `CLR.{B/W/L}`, `TST.{B/W/L}`, `EXT.W`, `EXT.L`, `TAS`, `CHK`
- **Bit operations**: `BTST`, `BCHG`, `BCLR`, `BSET`
- **Shifts and rotates**: `ASL/ASR`, `LSL/LSR`, `ROL/ROR`, `ROXL/ROXR` (register and memory variants with full condition code updates)

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
- `doc/M68kOpcodes.pdf` – opcode reference used while implementing and verifying instruction behavior.
- Instruction handlers are grouped by opcode family in files prefixed with `op_` (for example `op_arithmetic.go` and
  `op_branch.go`), with matching `op_*_test.go` suites covering their behavior.

## Development
1. Ensure you have Go 1.25 or newer installed (see `go.mod`).
2. Use the provided Makefile for common tasks:

   ```sh
   make fmt         # format Go code
   make check       # formatting check, go vet, staticcheck, and tests
   make test        # run unit tests
   ```

## Examples
The CPU can be stepped manually once you provide a memory implementation. The snippet below shows how to create a CPU with the built-in RAM helper, attach it to a bus, and execute a single `MOVEA` instruction:

```go
ram := m68kemu.NewRAM(0, 0x10000) // 64 KiB starting at address 0
bus := m68kemu.NewBus(&ram)
cpu, _ := m68kemu.NewCPU(bus)

// Set initial SSP and PC
_ = ram.Write(m68kemu.Long, 0x0000, 0x00002000)
_ = ram.Write(m68kemu.Long, 0x0004, 0x00000200)

// Encode MOVEA.L (long) from absolute long address 0x00000400 into A0
_ = ram.Write(m68kemu.Word, 0x0200, 0x2040)      // opcode for MOVEA.L with absolute long source
_ = ram.Write(m68kemu.Long, 0x0202, 0x00000400)
_ = ram.Write(m68kemu.Long, 0x0400, 0xDEADBEEF)  // data at the source address

_ = cpu.Step() // executes the single instruction
// alternatively run for a fixed cycle budget with cpu.RunCycles(4)
// cpu.Registers().A[0] now contains 0xDEADBEEF
```

### Fibonacci demo
The following 68000 program uses `ADD`, `SUBQ`, and `BNE` to calculate ten Fibonacci numbers and stream them into memory starting at `$3000`:

```
        LEA $3000,A0
        MOVEQ #0,D0
        MOVEQ #1,D1
        MOVEQ #8,D2
        MOVE.L D0,(A0)+
        MOVE.L D1,(A0)+
loop:   MOVE.L D1,D3
        ADD.L D0,D1
        MOVE.L D1,(A0)+
        MOVE.L D3,D0
        SUBQ.W #1,D2
        BNE.S loop
        NOP
```

Running it on the emulator mirrors the MOVEA sample above—load the assembled bytes into RAM at the reset PC (here `$2000`), then step until execution falls through the `NOP`:

```go
program, _ := asm.AssembleString(fibSource) // see listing above
for i, b := range program {
        _ = ram.Write(m68kemu.Byte, 0x2000+uint32(i), uint32(b))
}
for cpu.Registers().PC < 0x2000+uint32(len(program)) {
        _ = cpu.Step()
}
// RAM[0x3000..] now holds 0, 1, 1, 2, 3, 5, 8, 13, 21, 34
```

The emulator also supports a recursive variant that exercises `JSR`/`RTS` stack control. The following routine returns `fib(7)` in `D0` and writes it to `$4000`:

```
        BRA main
fib:    MOVE.L D0,D1
        SUBQ.L #1,D1
        BLE.S base
        MOVE.L D0,-(A7)   ; save n
        SUBQ.L #1,D0
        BSR fib           ; fib(n-1)
        MOVE.L D0,-(A7)
        MOVE.L 4(A7),D0   ; reload n without disturbing fib(n-1) on stack
        SUBQ.L #2,D0
        BSR fib           ; fib(n-2)
        MOVE.L (A7)+,D2   ; restore fib(n-1)
        MOVE.L (A7)+,D1   ; discard saved n
        ADD.L D2,D0
        RTS
base:   RTS
main:   LEA $4000,A0
        MOVEQ #7,D0
        BSR fib
        MOVE.L D0,(A0)
```

### Tracing and breakpoints
Tracing can be enabled via the exported API by installing a tracer callback on the CPU. Breakpoints and watchpoints halt execution (or invoke callbacks) when an instruction is executed or when a specific address is read or written:

```go
cpu.SetTracer(func(info m68kemu.TraceInfo) {
        fmt.Printf("PC=%04x SR=%04x\n", info.PC, info.SR)
})

cpu.AddBreakpoint(m68kemu.Breakpoint{Address: 0x2000, OnExecute: true, Halt: true})
cpu.AddBreakpoint(m68kemu.Breakpoint{Address: 0x4000, OnWrite: true, Halt: true})
```
