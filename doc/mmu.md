# Plan: PMMU

This is phase 5 of the [roadmap](roadmap.md). It adds paged memory
management: first the MC68030's on-chip MMU, then the MC68851 for the
MC68020, and the MC68040's MMU with phase 6. The models without an MMU keep
their behaviour and speed, and so does an MC68030 until TC enables
translation.

**Status: 5a done** (the MC68030 MMU). 5b and 5c are open.

## Design

The MMU is part of the MC68030, so `NewCPU` creates one for every `M68030`
CPU; `cpu.MMU()` returns it, and nil for the other models. Its emulation
lives in package `m68kemu/mmu`, which, like `float`, does not depend on the
core: the core imports it, so the MMU adds no interfaces or options to the
core's API.

The core (`cpu_mmu.go`) uses `mmu.MC68030` like this:

* `Translate(address, fc, write)` translates one access, or reports a bus
  error. The core calls it only while `Enabled` reports the E bit of TC set;
  otherwise logical addresses are physical.
* `Register`, `SetRegister`, `Flush`, `Load` and `Test` back the
  instructions PMOVE, PFLUSH, PLOAD and PTEST. The core decodes them (they
  are F-line instructions with coprocessor ID 0 and take effective
  addresses, function code operands and address registers) and calls the
  MMU. `SetRegister` returns `mmu.ErrConfiguration` for an invalid TC, CRP
  or SRP, and the CPU takes the MMU configuration exception (vector 56).
* The MMU reads its tables through an `mmu.Memory` the core passes to
  `NewMC68030`. Each descriptor access costs 4 cycles plus the bus's wait
  states, an approximation.

An MC68030's coprocessor ID 0 belongs to the MMU, so `WithCoprocessor`
rejects it. The MC68EC030, which has no MMU, is not modelled.

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

### 5b. MC68851

The external PMMU for the MC68020. As an optional part it will need an
option to attach it, and the core an internal interface over the MMU
types. It
adds the access levels, the breakpoint registers, the root pointer table and
DRP, task aliases, lockable and shared ATC entries, a 64-entry ATC, and the
instructions the MC68030 dropped (PVALID, PFLUSHR, PFLUSHS, PBcc, PDBcc,
PScc, PTRAPcc, PSAVE, PRESTORE, and PMOVE for CAL, VAL, SCC, AC, BAD, BAC).

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
errors and privilege. The core's opcode snapshots are unchanged.

## Sources

The MC68030 Enhanced 32-Bit Microprocessor User's Manual (second edition,
1989), sections 3 and 9. Encodings are checked against `m68kasm`, which
assembles them with `FeatPMMU`.
