# MC68881/MC68882 FPU (phase 4)

Package `m68kemu/fpu` emulates the MC68881 and MC68882 floating-point
coprocessors on top of [github.com/jenska/float](https://github.com/jenska/float).
This document covers the FPU core (4a), the memory-mapped coprocessor
interface of 68000 machines (4b), the F-line instructions on the 68020/030
(4c) and the execution timing. See the [roadmap](roadmap.md) for the phases.

**Status: 4a, 4b and 4c are done.** On an MC68020 or MC68030,
`FPU.Coprocessor` attaches the FPU through the CPU's coprocessor interface
([mc68020.md](mc68020.md)). The tests run 68000 programs that drive the
memory-mapped FPU with the command words and polling loops of the SFP004
library.

## 4a. FPU Core

### Programming Model

* FP0–FP7, 80-bit extended values (`float.X80`). Reset fills them with the
  non-signaling NaN `$7FFF FFFFFFFF FFFFFFFF`.
* **FPCR**: exception enable byte (bits 15–8) and mode control byte: rounding
  precision in bits 7–6 (extended, single, double) and rounding mode in bits
  5–4 (nearest, toward zero, toward minus infinity, toward plus infinity).
  The FPU keeps one `float.Env` and sets its rounding mode and precision
  from the FPCR before every operation.
* **FPSR**: condition code byte (N, Z, I, NAN in bits 27–24), quotient byte
  (sign and 7 bits, set by FMOD and FREM), exception status byte (bits
  15–8) and accrued exception byte (bits 7–3).
* **FPIAR**: the address of the last arithmetic instruction, as supplied by
  the CPU.

Exception bits, in the enable and status bytes: BSUN, SNAN, OPERR, OVFL,
UNFL, DZ, INEX2, INEX1 (bits 7–0). The `float.Env` flags map onto them:
invalid becomes SNAN when an operand is a signaling NaN and OPERR otherwise;
divide by zero becomes DZ, overflow OVFL, underflow UNFL, inexact INEX2.
INEX1 reports an inexact conversion of a packed decimal operand. The accrued
byte collects IOP (BSUN, SNAN, OPERR), OVFL, UNFL (only with INEX2), DZ and
INEX (INEX1, INEX2, OVFL).

### Commands

A general instruction is a 16-bit command word; bits 15–13 select the class:

| Class | Operation |
| --- | --- |
| 000 | FPm → FPn, opmode in bits 6–0 |
| 010 | `<ea>` → FPn in the format of bits 12–10 (L, S, X, P, W, D, B); format 111 is FMOVECR |
| 011 | FMOVE FPn → `<ea>`; for P, a static (bits 6–0) or dynamic (Dn in bits 6–4) k-factor |
| 100 / 101 | FMOVE(M) to / from the control registers selected in bits 12–10 (FPCR, FPSR, FPIAR) |
| 110 / 111 | FMOVEM to / from FP0–FP7, static or dynamic register list |

The FPU core is independent of the CPU. A command reads its source operand
as bytes in memory format and returns its result the same way; the caller
(the CIR device or the 68020's coprocessor interface) moves the bytes. A command that needs
a data register for a dynamic k-factor or register list names it, and the
caller supplies its value.

Opmodes: FMOVE, FINT, FSINH, FINTRZ, FSQRT, FLOGNP1, FETOXM1, FTANH, FATAN,
FASIN, FATANH, FSIN, FTAN, FETOX, FTWOTOX, FTENTOX, FLOGN, FLOG10, FLOG2,
FABS, FCOSH, FNEG, FACOS, FCOS, FGETEXP, FGETMAN, FDIV, FMOD, FADD, FMUL,
FSGLDIV, FREM, FSCALE, FSGLMUL, FSUB, FSINCOS, FCMP, FTST. Every result is
rounded to the FPCR precision; FSGLMUL and FSGLDIV round to single.

### Conditions, Exceptions, Frames

* The 32 condition predicates of FBcc, FScc, FDBcc and FTRAPcc are evaluated
  from N, Z and NAN; predicates $10–$1F set BSUN when NAN is set.
* An exception whose enable bit is set makes the FPU report a pending
  exception with its vector: BSUN 48, INEX 49, DZ 50, UNFL 51, OPERR 52,
  OVFL 53, SNAN 54, in the priority order BSUN, SNAN, OPERR, OVFL, UNFL, DZ,
  INEX. Results are written as with the exception disabled.
* FSAVE produces a null frame after reset and an idle frame afterwards;
  FRESTORE of a null frame resets the FPU.

## 4b. Memory-Mapped Coprocessor Interface

On a 68000 or 68010 the CPU has no coprocessor interface, and software drives
the 68881 through its coprocessor interface registers (CIRs). The Atari Mega
ST (SFP004 card) and Mega STE put them at `$FFFA40`:

| Offset | CIR | Access |
| --- | --- | --- |
| `$00` | response | read word |
| `$02` | control | write word |
| `$04` | save | read word |
| `$06` | restore | read/write word |
| `$08` | operation word | write word |
| `$0A` | command | write word |
| `$0E` | condition | write word |
| `$10` | operand | read/write, long, word or byte |
| `$14` | register select | read word |
| `$18` | instruction address | write long |
| `$1C` | operand address | read/write long |

`fpu.CIR` is a bus device with this layout at a configurable base address.
Software writes a command word, reads the response CIR, moves operands
through the operand CIR and polls the response until the command has
finished. The operand CIR is a byte stream: the 68000 bus splits a long
access into two word accesses, and byte and word operands use the first
bytes of the register.

### Sources and Confidence

The register map is from the Atari hardware register list
(`hardware.txt`). The protocol is checked against a real SFP004 library
(the GNU C math library for the SFP004 by M. Ritzert and others), whose
routines run unchanged in the tests. That library relies on these responses:

* `$8900` (null primitive, come again) while the FPU is busy; without a
  `Clock` the emulated FPU finishes every command at once and never reports
  busy (see Timing);
* a value other than `$8900` once an operand transfer is expected;
* bit 15 clear once a register-to-register command has finished.

The other primitive encodings are from memory of the MC68881/MC68882 User's
Manual and not verified: the idle response `$0802`, the transfer primitives,
take pre-instruction exception (`$1C00` + vector), the condition result in
bit 0 of the null primitive, and the FSAVE idle frame (version `$1F`, size
`$18` for the 68881 and `$38` for the 68882). An unknown command reports the
F-line vector (11).

## Timing

`FPU.Cycles` returns the execution time of a command in FPU clock cycles,
from a table of the typical times in the MC68881/MC68882 User's Manual
(`fpu/timing.go`). The values are recalled from memory and not verified
against the manual. A register-to-register operation takes its opmode's time
(FMOVE 33, FADD/FSUB 51, FMUL 71, FDIV 103, FSQRT 107, FSIN 391, FSINCOS
451, ...); a memory source adds its conversion (integer 18, single 12,
double 14, extended 6, packed 842); FMOVE to memory takes the time of its
destination format (double 77, extended 47, packed 1937). Control register
moves take 30 cycles per register, FMOVEM 33 plus 25 per register.

Both paths run the FPU concurrently with the CPU:

* **68020/68030.** `fpu.FPU.Coprocessor` implements
  `m68kemu.CoprocessorTiming`. A general instruction that returns nothing to
  the CPU leaves the FPU busy for its time while the CPU continues; the next
  FPU instruction waits for it. A store keeps the CPU waiting for its result.
  The FPU is clocked like the CPU.
* **CIR.** With `CIR.Clock` set (for example to the CPU's `Cycles`), each
  command keeps the FPU busy for its time, queued behind the previous one,
  and the response CIR reads `$8900` until it is done, so the polling loops
  of 68000 software run as on the real card. A `Clock` that scales the CPU
  cycles models an FPU on a different clock.

## Simplifications

* Execution times are typical values and do not depend on the operands. The
  MC68882 uses the MC68881's times; its faster conversions and its overlap
  of consecutive FPU instructions are not modelled. The CPU side of the
  coprocessor protocol costs only the instruction's base cycles.
* Results are written even when an enabled exception occurs.
* Packed decimal output rounds to nearest rather than in the FPCR rounding
  mode, and decides INEX2 from 25 guard digits.
* The idle FSAVE frame carries only a pending exception, not the internal
  state of the real chip.
