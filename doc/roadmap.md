# Roadmap: MC68010 to MC68060

m68kemu emulates the MC68000 today. The aim is to grow it into an emulator for
the whole family, from the MC68010 to the MC68060, including the paged memory
management unit (PMMU) and floating-point unit (FPU).

## Repository Layout

All CPU models stay in this repository and this Go module. The models share
most of the core (effective-address decoding, the bus, exception processing,
the scheduler and most of the instruction set), and the existing tests can
then run against every model. Splitting the models across repositories would
mean fixing the same bug in several places and releasing several modules for
every change to the core.

The coprocessors get their own packages inside the module:

```
m68kemu/        core: CPU, bus, EA, scheduler, integer instruction set of all models
m68kemu/fpu/    MC68881/MC68882 and the reduced 68040/68060 FPUs
m68kemu/mmu/    MC68851, 68030, 68040 and 68060 PMMUs
```

The core talks to them through small interfaces:

* a coprocessor interface for the F-line (`cpID`) instructions
* an address translator between the CPU and the `Bus`

Without an FPU, F-line opcodes raise the line-F exception. Without an MMU,
addresses are physical. The 68000 path therefore keeps its current speed.

`fpu` is the one package that could later move to its own module. Correct
80-bit extended-precision arithmetic is useful on its own and does not depend
on the CPU. We would only move it if other projects want to use it.

## Phases

### 1. Per-model dispatch (no behaviour change)

The opcode handler and cycle tables are package-level globals today, so one
process can only emulate one kind of CPU. This phase replaces them with
immutable tables per model, chosen with `WithModel` when the CPU is created.
The detailed plan is in [multi_model_dispatch.md](multi_model_dispatch.md).

### 2. MC68010

A small step that tests whether the phase 1 design holds up:

* the VBR, SFC and DFC registers; the `MOVEC` and `MOVES` instructions
* `RTD`
* `MOVE from SR` becomes privileged; `MOVE from CCR` is added
* stack frames carry a format/vector word (formats $0 and $8), and `RTE`
  understands them
* bus and address errors push the format $8 frame and can be resumed
* loop mode (optional, affects timing only)

The design question here is exception frames: `raiseExceptionWithPC` needs a
frame builder chosen per model.

### 3. MC68020 / MC68030

* 32-bit address bus (an address mask per model; 24-bit for the 68EC020)
* the new addressing modes: scaled index, memory indirect, full extension word
* bitfield instructions (`BFxxx`), `CAS`, `CAS2`, `CHK2`, `CMP2`
* 32-bit `MULx.L` and `DIVx.L`, `EXTB.L`, `PACK`, `UNPK`, `TRAPcc`, `BKPT`
* the MSP, ISP and M bit; the CACR and CAAR registers
* `CALLM`/`RTM` (68020 only)
* stack frame formats $1, $2, $9, $A and $B
* coprocessor interface (F-line dispatch to an attached coprocessor)

Exact cycle timing is out of reach once there are caches and a pipeline. From
the 68020 on, timing is an approximation, and the goal is correct behaviour.

### 4. FPU (`m68kemu/fpu`)

* MC68881/MC68882 programming model: FP0–FP7, FPCR, FPSR, FPIAR
* 80-bit extended precision with correct rounding modes and precision control
* the transcendental functions (`FSIN`, `FETOX`, `FLOGN`, ...)
* `FSAVE`/`FRESTORE` frames and FPU exceptions
* tests against reference values, since host `float64` is not precise enough

### 5. PMMU (`m68kemu/mmu`)

* MC68030 MMU first: TC, CRP, SRP, TT0/TT1, MMUSR; `PMOVE`, `PFLUSH`,
  `PTEST`, `PLOAD`
* table walk, address translation cache (ATC), bus errors from the MMU with
  restartable frames
* MC68851 as an external coprocessor variant for the 68020
* MC68040/MC68060 MMU afterwards; it works differently (fixed page sizes,
  separate instruction and data ATCs, `PTEST`/`PFLUSH` variants)

### 6. MC68040 / MC68060

* `MOVE16`, `CINV`, `CPUSH`
* removed instructions trap and leave their emulation to software:
  the 68060 drops `MOVEP`, `CAS2`, `CHK2`/`CMP2`, 64-bit `MULx`/`DIVx`, and
  others
* the reduced FPUs: unimplemented FPU instructions raise exceptions for the
  operating system's floating-point library to handle
* stack frame formats $3, $4 and $7
* 68060: the PCR register and superscalar timing (approximate)

## Testing Across Models

* Each model gets its own opcode-validity table (`validFor(model, op)`).
* Instruction and timing tests become table-driven over the models where the
  behaviour is shared.
* Two CPUs of different models can run side by side in one process. That makes
  differential tests possible, for example a 68000 against a 68010 on code
  that should behave the same on both.
