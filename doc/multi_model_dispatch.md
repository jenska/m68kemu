# Plan: Per-Model Dispatch Tables

This is phase 1 of the [roadmap](roadmap.md). It changes how the CPU looks up
opcode handlers so more than one CPU model can exist, **without changing any
behaviour**. Every existing test stays green, and the MC68000 remains the
default and the only supported model.

## Current State

| Global | Where | Written by | Model-specific? |
| --- | --- | --- | --- |
| `opcodeTable [0x10000]instruction` | `cpu.go` | about 20 `init()` functions via `registerInstruction`, plus direct writes in `registerExtendInstruction` (`op_arithmetic.go`), `registerMoveUsp` (`op_control_flow.go`) and `registerLogicalInstruction` (`op_bit_logical.go`) | yes |
| `opcodeCycleTable [0x10000]uint32` | `cpu.go` | same functions | yes |
| `pruneInvalidOpcodes` (`sync.Once`) | `opcode_validity.go` | `NewCPU`; after it runs, the global table is edited in place | yes (`valid68000`) |
| `opcodeMetaTable` | `ea.go` | `init()` | no: it only splits out opcode bit fields, so it stays global |

## Target Design

```go
type Model int

const (
	M68000 Model = iota
	M68010
	M68020
	M68030
	M68040
	M68060
)

// opcodeSet is the immutable dispatch table for one model, shared by every
// CPU of that model.
type opcodeSet struct {
	handlers [0x10000]instruction
	cycles   [0x10000]uint32
}

// tableBuilder fills an opcodeSet; registration code can branch on model.
type tableBuilder struct {
	model Model
	set   *opcodeSet
}

func (b *tableBuilder) add(ins instruction, match, mask, eaMask uint16, calc cycleCalculator)
func (b *tableBuilder) set1(opcode uint16, ins instruction, cycles uint32)

var opcodeSets [modelCount]struct {
	once sync.Once
	set  *opcodeSet
}

// opcodesFor builds the table on first use: it runs every registrar, then
// prunes the result with validFor(m, op).
func opcodesFor(m Model) *opcodeSet
```

* **CPU instances.** `cpu` gets an `ops *opcodeSet` field.
  `dispatchInstruction` reads `cpu.ops.handlers[opcode]` and
  `cpu.ops.cycles[opcode]`. The index is a `uint16` into a `[0x10000]` array,
  so Go still skips the bounds check. The only extra cost is one pointer load.
* **Registration.** Each `init()` becomes a named function, for example
  `func registerArithmetic(b *tableBuilder)`, listed in one `registrars`
  slice. This also makes the registration order explicit; today it depends on
  the order in which Go runs each file's `init()`. `registerInstruction`
  becomes `b.add`. The panic on duplicate registration stays.
* **Validity.** `valid68000(op)` stays as it is and is called through
  `validFor(model, op)`. Pruning moves into the builder, so the shared table is
  never edited after it is built.
* **Construction.** `WithModel(m Model)` is a new `Option`, defaulting to
  `M68000`. For any other model, `NewCPU` returns `ErrModelUnsupported` until
  that model is implemented. A `Model()` getter goes on the `CPU` interface.
  Strictly speaking this breaks the API, but only for code that implements
  `CPU` outside this package.
* **Memory.** Each table set is 768 KiB (64K handlers × 8 bytes plus 64K cycle
  counts × 4 bytes), built only for the models actually used.

## Commit Sequence

1. **Introduce `opcodeSet`, `tableBuilder` and `cpu.ops`.** Convert every
   `init()` registration and the three helpers that write to the table
   directly. Delete the globals; the 68000 table comes from
   `opcodesFor(M68000)`.
2. **Move pruning into the builder.** Delete `pruneOnce` and
   `pruneInvalidOpcodes`.
3. **Add `Model`, `WithModel` and `Model()`.** Only the 68000 is accepted for
   now. Document the option in the README.
4. **(Optional, separate commit) Add an address mask.** Replace the hard-coded
   `& 0xffffff` (about 55 places in non-test code) with a `cpu.addrMask` field
   set from the model: 24-bit for 68000/010, 32-bit for 020 and later.
   Behaviour stays the same with `0xffffff`. This commit is separate because it
   touches the hottest paths, so it gets its own benchmark comparison.

## Tests

The tests that use the global tables change as follows:

* Tests that only read the tables (in `op_control_flow_test.go`,
  `op_system_test.go`, `op_bit_logical_test.go` and `cpu_test.go`) switch to
  `opcodesFor(M68000)`.
* `cpu_test.go` temporarily sets the NOP cycle count to 0 in the global table.
  With a shared table that change would leak into every other CPU, so the test
  gives its CPU a private copy instead (`c.ops = cloneOpcodeSet(...)`).

New tests:

* **Snapshot equality.** Before the refactor, generate a snapshot of the
  current table (handler identity and cycle count for each of the 65536
  opcodes). After the refactor, `opcodesFor(M68000)` must match it exactly.
  This is the proof that behaviour did not change.
* Two CPUs of the same model share the same `ops` pointer.
* `opcodesFor` running concurrently under `-race` builds the table exactly
  once.

## Done When

* `go test -race ./...` passes.
* `BenchmarkRecursiveFibonacci`, `BenchmarkBubbleSort`,
  `BenchmarkRunEightMillionCycles` and the scheduler benchmarks show no
  slowdown, compared with `benchstat` over 10 runs before and after (see
  [benchmark_report.md](benchmark_report.md)).

## What This Sets Up

For the MC68010, the registrars add model checks such as
`if b.model >= M68010 { b.add(movec, ...) }`, and `MOVE from SR` gets a
model-dependent privilege check. Exception stack frames are the next design
point: from the 68010 on, every frame carries a format/vector word, so
`raiseExceptionWithPC` needs a frame builder chosen per model. That belongs to
the 68010 phase, not this one.
