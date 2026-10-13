# Plan: PMMU

This is phase 5 of the [roadmap](roadmap.md). It adds paged memory
management: first the MC68030's on-chip MMU, then the MC68851 for the
MC68020, and the MC68040's MMU with phase 6. The models without an MMU keep
their behaviour and speed, and so does an MC68030 until TC enables
translation.

**Status: 5a and 5b done** (the MC68030 MMU and the MC68851). 5c comes with
phase 6.

## Design

The MMU is part of the MC68030, so `NewCPU` creates one for every `M68030`
CPU. The MC68851 is an optional part of an MC68020 system: `WithMC68851()`
attaches one. `cpu.MMU()` returns the MMU as an `mmu.MMU` (an
`*mmu.MC68030` or `*mmu.MC68851`), and nil without one. The emulation lives
in package `m68kemu/mmu`, which, like `float`, does not depend on the core:
the core imports it, so the MMUs add only `WithMC68851` and `MMU()` to the
core's API.

The core (`cpu_mmu.go`) drives both MMUs through the `mmu.MMU` interface:

* `Translate(address, fc, write)` translates one access, or reports a bus
  error. The core calls it only while `Enabled` reports the E bit of TC set;
  otherwise logical addresses are physical.
* `Register`, `SetRegister`, `FlushAll`, `Flush`, `Load` and `Test` back the
  instructions PMOVE, PFLUSH, PLOAD and PTEST. The core decodes them (they
  are F-line instructions with coprocessor ID 0 and take effective
  addresses, function code operands and address registers) and calls the
  MMU. `SetRegister` returns `mmu.ErrConfiguration` for an invalid TC or
  root pointer, and the CPU takes the MMU configuration exception (vector
  56).
* The MMU reads its tables through an `mmu.Memory` the core passes to
  `NewMC68030` or `NewMC68851`. Each descriptor access costs 4 cycles plus
  the bus's wait states, an approximation.

The MC68851's other instructions use methods of `*mmu.MC68851`: PVALID
(`Validate`), PFLUSHR (`FlushRoot`), the conditionals PBcc, PDBcc, PScc
and PTRAPcc (`Condition`), PSAVE and PRESTORE (`Save`, `Restore`, through
the core's coprocessor interface code), and BKPT (`Breakpoint`).

Coprocessor ID 0 belongs to the MMU, so `WithCoprocessor` rejects it on an
MC68030 and beside an MC68851. The MC68EC030, which has no MMU, is not
modelled. The table search and the ATC are shared between the two MMUs
(`mmu/search.go`, `mmu/atc.go`).

### Speed

The CPU keeps one directly accessible memory page for data and one for code
(`bus_fastmem.go`). While translation is on, these pages hold logical
addresses: after a translated access the CPU points them at the part of the
translated page that is plain memory. Further accesses to the page cost no
more than without an MMU, so the pages act as a one-entry TLB in front of
the MMU's address translation cache (ATC). A page is read-only for the fast
path until its translation allows writes (the M bit is set and the page is
not write-protected), so the first write still reaches the MMU and sets M.

The pages serve the current program and data function codes. The CPU drops
them when the S bit changes, when the registers are set or rolled back for
an instruction restart, on reset, and after every MMU instruction. Accesses
with another function code (MOVES, PC-relative program reads, vector reads)
go to the MMU directly.

A loop that sums 64 KB through 4 KB pages runs as fast with translation as
without (`BenchmarkTranslation` in package `mmu`). The MC68000 benchmarks
are unchanged.

Accesses that cross a page boundary while translating are split into parts
that each get their own translation, as the real CPU splits them into bus
cycles.

## Steps

### 5a. MC68030 MMU (done)

Following section 9 of the MC68030 User's Manual:

* TC, CRP, SRP, TT0, TT1 and MMUSR, loaded and stored by PMOVE and PMOVEFD.
  Loading CRP, SRP, TC, TT0 or TT1 flushes the ATC unless FD is set. TC
  with E set is checked (page size, IS + PS + TIA... = 32); an invalid TC
  is loaded with E clear and takes the configuration exception, as does a
  root pointer with descriptor type 0.
* Transparent translation through TT0 and TT1 (address base and mask,
  function code base and mask, R/W and RWM), also while E is clear.
* The table search: CRP, or SRP for supervisor accesses with SRE; function
  code lookup; initial shift; up to four address levels; short and long
  descriptors; early termination page descriptors, including a root pointer
  of type page; indirect descriptors; limits on long descriptors and the
  root pointer; write protection accumulated over the levels;
  supervisor-only long descriptors; U bits on every descriptor fetched and
  the M bit on the first write to a page.
* A 22-entry ATC with the B, WP and M bits and pseudo-LRU replacement. An
  invalid descriptor, a limit or supervisor violation, or a bus error during
  the search makes an entry with B set; accesses through it, and writes
  through a write-protected entry, end in a bus error.
* MMU faults are bus errors with the logical address in the format $B
  frame, and RTE restarts the instruction, so a handler can page in and
  return.
* PFLUSHA, PFLUSH by function code and mask, with or without an address;
  PLOADR and PLOADW; PTESTR and PTESTW at level 0 (ATC, transparent
  translation) and levels 1-7 (table search without updating U and M), with
  the descriptor address in An.
* The instructions are privileged; in user mode they take the privilege
  violation exception. Encodings the MC68030 does not have take the line-F
  exception.

Not modelled: the cache inhibit bit has no effect, as there are no caches;
the MMUDIS signal; locked read-modify-write cycles (TAS and CAS are
translated as a read and a write). Disassembly in instruction traces reads
physical memory.

### 5b. MC68851 (done)

The external PMMU for the MC68020, attached with `WithMC68851()`, following
the MC68851 User's Manual:

* TC, CRP, SRP, DRP, PCSR, PSR, CAL, VAL, SCC, AC and BAD0-7/BAC0-7, moved
  by PMOVE with any addressing mode the manual allows (byte, word, long and
  quad registers). Loading TC with E clear flushes the ATC, SRP and DRP
  flush the entries formed with them; an invalid root pointer is refused.
  There are no transparent translation registers.
* The table search of 5a, plus: DRP for DMA function codes (FC3 set) with
  function code lookup; the SG, G and L bits; access levels. With AC.ALC
  set, the top 1-3 bits of a user address are its access level: a level
  more privileged than CAL ends in a bus error, and the RAL and WAL fields
  of long descriptors deny reads and writes. Supervisor accesses are not
  checked.
* A 64-entry ATC tagged with task aliases. Loading CRP looks the root
  pointer up in an eight-entry root pointer table; a new one takes over an
  entry and flushes its alias, and PCSR reports the alias and the flush.
  Entries of other tasks survive a task switch; globally shared entries
  match every alias; locked entries are not replaced, up to 63, after which
  PCSR's lock warning is set.
* PFLUSHA, PFLUSH and PFLUSHS (by function code with a 4-bit mask, with or
  without an address; only PFLUSHS flushes shared entries), PFLUSHR,
  PLOADR/W and PTESTR/W (which take the PMMU illegal operation exception,
  vector 57, while translation is disabled), PSR with the A, G and C bits,
  and PVALID, the one unprivileged MC68851 instruction (the access level
  exception, vector 58).
* PBcc, PDBcc, PScc and PTRAPcc on the PSR conditions (B, L, S, A, W, I, G,
  C, set or clear).
* PSAVE and PRESTORE with the idle (36 bytes) and breakpoint (76 bytes)
  frames, and the null frame. PRESTORE also accepts the mid-instruction
  frame (44 bytes). The format word's version byte is $C0, a guess: the
  manual's figure is not legible in the available copy.
* BKPT: the MC68020 decodes BKPT #n and runs the breakpoint acknowledge
  cycle. While BACn is enabled and its skip count is not zero, the
  MC68851 counts down and supplies BADn, which the CPU executes in place of
  the BKPT; otherwise the CPU takes the illegal instruction exception, as
  without an MC68851.

Not modelled: CALLM and RTM with type $01 descriptors, which change the
access level through the MC68851's access level control registers in CPU
space (they still take the format error exception), and the gate checks of
module descriptors; the bus error the MC68851 signals for a
read-modify-write cycle (TAS, CAS, CAS2) that misses the ATC; the
coprocessor protocol's timing.

### 5c. MC68040 MMU

With [phase 6](roadmap.md#6-mc68040): 4 KB or 8 KB pages, URP and SRP,
separate instruction and data ATCs and transparent translation registers
(ITT0/1, DTT0/1, through MOVEC), and the 68040's own PFLUSH and PTEST.

## Tests

Package `mmu` runs programs on an MC68030 with tables in RAM: translation of
data and instruction fetches, the U and M bits, a page fault whose handler
maps the page and returns with RTE, write protection, limits,
supervisor-only descriptors, SRE, early termination, transparent
translation, accesses across pages, PTEST, PFLUSH, PLOAD, configuration
errors and privilege.

On an MC68020 with an MC68851 it runs translation, task switches between
two translation trees (task aliases, PCSR, cached entries surviving the
switch, PFLUSHR), shared entries with PFLUSH and PFLUSHS, PTEST with the
conditionals, the illegal operation exception, PSAVE and PRESTORE, BKPT
replacement opcodes with skip counts, PVALID, privilege, configuration
errors and the attach rules. Unit tests cover access levels, DMA
translation, the breakpoint frame and locked entries.

The MC68020 opcode snapshot gains BKPT; the others are unchanged.

## Sources

The MC68030 Enhanced 32-Bit Microprocessor User's Manual (second edition,
1989), sections 3 and 9, and the MC68851 Paged Memory Management Unit
User's Manual (first edition, 1986). Encodings are checked against
`m68kasm`, which assembles them with `FeatPMMU`.

