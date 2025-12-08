# m68kemu

A small Motorola 68000 emulator written in Go. The emulator exposes a CPU core with a programmable memory bus, effective-address helpers, and a few initial instructions for experimenting with 68k code.

## Project layout
- `internal/emu/` – core emulator types such as the CPU, register file, effective-address resolver, and the basic MOVEA word/long instructions. A minimal RAM implementation in this package satisfies the `AddressBus` interface for quick tests.
- `doc/M68kOpcodes.pdf` – opcode reference used while implementing and verifying instruction behavior.

## Development
1. Ensure you have Go 1.25 or newer installed (see `go.mod`).
2. Run the test suite to verify the effective-address helpers and instruction wiring:

   ```sh
   go test ./...
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
