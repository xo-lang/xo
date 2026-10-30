# 0409: an edit's store link reuses std's laid-out half

Status: Accepted (lead, 2026-10-29, within milestone 0.35 item 2, which the owner approved; option (a) below). Stage 2 landed in 1cc23a65e (2026-10-29), default on for darwin/arm64 store links.

## Context

A separately compiled darwin edit (decision 0388) links the program's objects against std's
store. After cfd303004 (the DWARF cut), 225fb8410 (def_of memo) and e7675ecb0 (the prepare
cache) the HTTP server's edit link is 23.7 ms (fib 9.1): prepare 8.4, resolve 4.4, finish 10.9.
The in-flight items (parallel page hashes, eh_live's index, one-lookup add_globals) are worth
about 3 ms together. A line profile of layout and synthesize shows no line above 0.5% of the
link. The 0.35 target is an HTTP edit link under 15 ms, so the remaining time has to come from
not doing std's share of the link at all on an edit.

On an edit of a function body, the prepare cache hits: std's live set, its kept atoms, their
order and sizes are the same as the previous link's. Only the program's objects change. Yet
every link rebuilds std's objects, lays out std's sections, applies std's internal
relocations, synthesizes std's stubs, GOT entries, unwind info and DWARF, and writes them.

## Decision

On a prepare cache hit, the link reuses std's laid-out half from the previous link with the same
key:

1. Layout order: std's part of every section and segment first, the program's after (option (a),
   "Layout" below), so a std address moves only by whole program-sized shifts that the half's
   re-patch list recomputes. Only the first link of a key (a miss) lays out std.
2. The cached half (in <backend cache>/stdhalf/<prep key + layout inputs>): each std output
   section's bytes with every std-to-std relocation applied, the std symbols the program may
   reference (name, address), the relocations in std that name a program symbol (main, the
   callbacks a program defines and std calls; few), std's share of the GOT, stubs, chained
   fixups, unwind entries and __XODW tables, and the layout's end addresses.
3. A hit link: parse the program's objects, place them after std's half, resolve the program's
   references against the cached symbol table, apply the program's relocations and the few
   std-to-program ones, append the program's GOT, stub, unwind and DWARF entries, and write the
   load commands, __LINKEDIT and signature.
4. Anything the cached half cannot answer falls back to the full link ("Fallback" below): the
   output is then the full link's, byte for byte.
5. The first stage reports the program's own share (its parse, resolve and relocations) on the
   HTTP server, so the 15 ms target is checked against the real total, not the ceiling's bound.

The output must not depend on whether the half came from the cache: proof by identity with the
half cache off, miss and hit, as for the prepare cache.

## Ceiling check

In a never-pushed branch, time an HTTP and a fib edit link where std's half is taken as given:
- XO_CEIL=half: the link runs as today, but skips store_prepare's std objects past the first
  link of the process (XO_STD_STORE_REPEAT keeps the first link's std ISects, layout and
  synthesized std parts in memory and reuses them), with the program's relocations still
  applied. Wrong output is fine; the timing bounds the gain.
- Interleaved n=80 against the same binary without it, plus the one-binary A/A control.
- Gate: go on only if the ceiling puts the HTTP edit link at or under 15 ms with margin for the cache's
  own read (a std half is roughly the image size: ~6.8 MB for HTTP, a read of about 1 ms warm).

### Result (2026-10-29, a never-pushed branch, quiet interleaved n=80, one binary)

| prog | A total (prepare/resolve/finish) ms | B (write only) ms | B/A | A/A control |
|---|---|---|---|---|
| fib  | 8.86 (3.82/2.30/2.77)   | 1.45 | 0.163 | 1.061 |
| http | 23.59 (8.64/4.09/10.95) | 5.10 | 0.216 | 0.982 |

Cross-segment std relocations (option a's re-patch list): fib 2589, re-target 0.157 ms a pass;
HTTP 6648, 0.61 ms a pass. HTTP bound: 5.1 + 0.6 + ~1 (half read) = ~6.7 ms plus the program's
own parse/resolve/relocations (not in B).

Counted by relocation kind (2026-10-29): HTTP 64795 std relocations, 6648 cross-segment,
17221 cross-section within one segment, 3764 absolute pointers (chained fixups); fib 14812,
2589, 4756, 1163. With the layout below the re-patch set is the cross-segment ones, the
absolute pointers and std's FDEs in the shared __eh_frame: an estimated 1 to 1.5 ms on HTTP,
which stage 1 measures. Target 15 ms: met with about 8 ms of margin before the program's own share.

### Measured: stage 2 (2026-10-29, quiet interleaved n=80)

A is the parent's xo (parse-once, a full store link each round). B is this change's xo,
reading the half from the cache file inside each timed round (the read counts in prepare).
Both link the same edited program against the same store.

| prog | A total (prepare/resolve/finish) ms | B total (prepare/resolve/finish) ms | B/A | A/A control (B both arms) |
|---|---|---|---|---|
| fib  | 9.40 (4.31/2.54/2.59)   | 3.31 (1.11/0.27/1.97) | 0.352 | 0.963 |
| http | 23.68 (8.96/4.41/10.25) | 9.89 (2.73/0.71/6.49) | 0.418 | 0.969 |

The A/A control spreads 4% (ceiling run: 6%), so both gains are far outside the noise. HTTP's
edit link is under the 15 ms target. finish is now the largest step: __LINKEDIT, the signature,
and the program's relocations plus the kept std relocations.

The half per std key is 9,883,465 bytes for HTTP and 3,461,847 for fib. It keeps 3,482 of HTTP's 57,636 std relocations (54,154 dropped) and 1,702 of fib's 12,831 (11,129 dropped). Every repeat round from the file was byte for byte the full link. A new std (a new store) is a new prepare key and so a new half file. The old one is then unused, and the cache's prune removes it after a week, or sooner past the size cap.

## Option (a) in detail

### Layout
Every store link (hit or miss, so a hit and a full link agree; LinkOpts.split_prog) gives the
program's sections output sections of their own, after std's in their segment, named `_p` plus
the name without its leading `__` (a section name has 16 bytes: `__gcc_except_tab` has no room
for a suffix): `_ptext`, `_pconst`, `_pcstring`, `_pdata`, `_pbss` ... In each segment:
std's sections in today's order, then the program's, then __stubs and __got (they grow with
the program's imports; std's slots come first), then __unwind_info and __eh_frame (rebuilt
each link: they cover the program's code), then the zerofill sections, std's then the
program's. Consequence: inside __TEXT nothing of std's moves when the program changes (the
header's size is in the half's key); the segments after __TEXT move by whole pages, so a std
PAGEOFF12 into them is unchanged and only its PAGE21 is recomputed; std's zerofill in __DATA
moves by the program's data size (any amount), so those records recompute both.

Only a separately compiled link through std's store splits its sections (ld.xo sets
split_prog when it links against a store). Whole builds, --release builds and every other
target keep today's single __text: they never reuse a half, so nothing would pay for the
second code section. A link given a layout file (`--layout`, decision 0403) does not split
either. That order puts the program's hot functions beside the std functions they call, in
one __text, which own sections cannot do. The full suite's TestLayoutStoreOrder caught this
on 2026-10-29.

A second code section is new for every tool that reads the binary. Stage 1's gate shows each
one works on an edit build with _ptext: lldb breakpoints and backtraces in program code,
dsymutil with the debug map (decision 0401), Xo's fault tracebacks, XO_PROF samples
attributed to program functions, xo dbg, `codesign --verify`, and compiler/debug's arm64
unwinder on a fault inside _ptext (it reads _ptext beside __text).

A first draft kept the program's chunks inside std's output sections; reading relocate showed
that std's later __TEXT sections (__const, __cstring, __stubs, __eh_frame) then move by the
program's text size, so std's text-to-constant references (17221 on HTTP) would change on
every edit. The ceiling's count (below) had counted only the cross-segment ones.

### What a std address depends on
addr(std atom) = segment base (moves with earlier segments) + offset in segment (moves when
an earlier section of the same segment holds program bytes). So the cached half stores std's
bytes with every std-to-std relocation applied for a reference layout, and a list of the
relocations whose value depends on a shift: (place, kind, the two (segment, section)
ends) for every relocation whose source and target are not in the same output section
(a same-section pc-relative relocation never changes; an absolute pointer always does,
through the rebase/chained fixups, which are rewritten anyway).

### Re-patch list format (in the cached half)
A table of u32 records: place (offset in std's half of its output section), section index,
kind (ARM64 BRANCH26, PAGE21, PAGEOFF12 (the low bits only change if the section's
alignment-rounded shift is not a multiple of 4096: recompute both), UNSIGNED 64-bit,
x86 SIGNED/BRANCH), target section index, target offset, addend. A hit link computes each
section's final address (std part + shift) and re-applies these records only; the ceiling
run counts them (cross-segment and cross-section) and times one pass.

### Byte identity
The hit link writes: std's cached section bytes with the re-patch records applied at the
final addresses, then the program's sections linked as today against the cached symbol table
(each std symbol's section and offset, final address = section address + offset). A full link
with the same layout produces the same bytes because every std relocation is either
same-section pc-relative (equal by construction) or in the re-patch list (recomputed with
the same formula, link.xo relocate's). Proof: identity of hit against off (XO_STD_HALF=off),
on fib, HTTP and the goldens, plus a test that edits a program between two links.

### Fallback
- the prepare cache misses (the live set changed), or the half's key differs (xo, store,
  options, layout inputs);
- the program defines a symbol std's half resolved elsewhere (a weak definition both have,
  or a program definition of a name std imports);
- the program needs a std import std's half did not bind, or a new GOT/stub kind;
- a section kind the half has no slot for (a program TLV section where std has none, a new
  output section ordered before one of std's);
- any record the re-patch pass cannot encode (an ADDEND pair across sections, x86 SUBTRACTOR
  across sections), counted at half-building time: the half is not written then.
- a link whose program defines (not weak) a name std's store defines writes no half
  (2026-10-30, StoreLink.shadows): the half keeps only std's globals and the prepare key
  names only what the program takes from the store, so a next program with the same key
  that does not define the name would find no definition of it.

## Consequences

- An edit's link becomes proportional to the program, not to std: the expected HTTP edit link
  is the program's share plus __LINKEDIT and the signature (the ceiling bounds it at about 6.7 ms
  plus the program's own share, which stage 1 measures).
- Layout changes: std's sections first. Identity against the parent changes (addresses move)
  in separately compiled builds, once; whole builds are untouched.
- A new cache kind (stdhalf) in the back-end cache, keyed by the prepare key plus the layout
  inputs (page size, section order, opts), pruned with it, classified in
  TestCodegenInputsClassified if it adds an XO_* switch.
- More code in compiler/macho: the half's encoding and the hit path, with a full-link fallback
  for anything unusual.

## Alternatives

- (b) Reserve headroom in __TEXT (std's __TEXT rounded up with a fixed slack, e.g. 1 MiB) so
  nothing after it moves until the program outgrows the slack: no cross-segment patching, but
  every binary carries the slack in virtual space and file size (zero fill in __TEXT is not
  free on disk), and outgrowing it falls back to a full link at an arbitrary program size.
  Not chosen: (a)'s re-patch pass costs 0.61 ms on HTTP and keeps the binary's size.
- (c) The program first and std after at a fixed high address: every std address is fixed,
  but the program's references to std become long absolute jumps or extra veneers, dyld's
  chained fixups still depend on the program's base, and the binary's layout differs from a
  whole build's. Not chosen: more code-generation change for no measured gain over (a).
- Keep shaving the full link (the in-flight items): about 20 ms at best, short of the target.
- Prelink std as a dylib: changes the binary's shape (a second image, dyld load cost at every
  start); decision 0388 chose a static link.
- Run the link in a resident process (xo watch) with std's half in memory: the same reuse
  without a file, but only for watch users; the cache serves every build. The two compose
  later.

## Windows (PE) port note (2026-10-30, Lang 2)

PE has no segments, so the program's contributions go to output sections of
their own after std's: .xotext (code, then the import thunks), .xordata
(read only data, then the linker's tables: IAT, import directory, lookup
tables, names, TLS and debug directories) and .xodata (data), placed by
PEOpts.prog_from (compiler/macho/pelink.xo; ld.xo sets it to the store's
object count unless XO_STD_HALF=off or XO_STD_STORE_CHECK). std's .text,
.rdata and .data then hold std alone and keep their addresses and sizes
whatever the program adds, an import included; .pdata, .tls, .rsrc and
.reloc follow and are rebuilt every link. The half itself (kept relocations:
REL32 to the program, a thunk, the IAT or an unpinned section, and every
ADDR64 for the base relocations) is the next step.

Step 2 (2026-10-30, compiler/macho pehalf.xo): after a full split link,
capture_pe_half keeps std's objects as the link had them (the store's,
already cut to the live sections and DWARF), the pinned sections' bytes
taken from the image with each kept relocation's original bytes put back
(COFF relocations are REL style: the addend is in the bytes), and only
the kept relocations (pe_half_moves); the store's maps go with it for the
PDB. It is cached beside the prepare cache in stdhalf/ (pe_half_encode;
key: the COFF prepare file, whether DWARF is kept, --race). A later link
with the same prepare key reads the half instead of std's store (the
store is not read; its size, from a stat, is in the key) and links std's
objects and the program's; the pins (std's .text, .rdata, .data and the
program's .xotext addresses) are checked after the layout, and a
mismatch links in full, whose half replaces it. No half: program thread
locals, a program .xounsafe$ record, XO_STD_HALF=off, XO_STD_STORE_CHECK.
HTTP server: 3,138 relocations kept, 42,102 dropped, the half 9.65 MB;
an edit's cross link (Mac) 57.0 ms full to 36.0 from the half, the
same exe and PDB; `xo bench --ab` of the warm HTTP cross build on the
linux/arm64 offload host (half off against on, n=30): wall 0.929, RSS
0.869.

Step 3 (2026-10-30, compiler/xo ld.xo pdb_pre, compiler/macho cvobj.xo
pre_procs_from): the PDB's std part. With the half, std's functions keep
their places, so pre_procs_store's state after std's store parts (the
functions, files and types std brings) is the same edit to edit. It is
cached beside the half (<half>.pdb: xopdbpre1, the half's id, an ObjPdb
encoding, enc_pre_procs), and a link from the half resumes from it over
the program's parts only, reading neither std.store.pdb nor std's parts.
Each half gets a random id when captured (PeHalf.id, xopehalf2); a part
whose id is not the half's is made again. HTTP edit (Mac cross link):
pe.pdb_parts 13 to 4.6 ms, pe_link 42.5 to 34.9 ms; the exe and PDB
byte for byte the full link's, also with a stale part beside a new half.

Step 4 (2026-10-30, compiler/macho pdb.xo pe_pdb_std, PdbStd): the PDB
writer's std part. CodeView types are made in two runs (cv_types_split:
std's structures, variables' types and definitions, then the
program's), so std's type indices do not depend on the program; llvm-
pdbutil reads the HTTP server's PDB with the same records as before
(dump --all, the type records' multiset), only their indices moved. The
module stream's procedure records (proc_records, from an offset) and
c13's line blocks (proc_lines) are made per run of functions, so std's
placed functions, the files and inlined functions they name first, their
records and line blocks are kept beside the half (<half>.pdbw, the half's
id, enc_pdb_std) and a link from the half makes only the program's. HTTP
edit (Mac cross link): pe.pdb 14.5 to 8.1 ms, pe_link 34.9 to 26.6 ms
(57.0 before step 2); the exe and PDB the full link's, also with stale
parts beside a new half. Then std's CodeView type run (cv_types_std:
its records, their hashes, the DWARF offsets' indices and the forward
references) goes in the PdbStd too, and the PDB's std part is read
without std's functions when the PdbStd has them placed
(dec_pre_procs_lazy), and the PdbStd keeps only std's functions' names (all the global symbols need once their records are kept): HTTP edit pe.pdb_parts 1.4 ms, pe.pdb 2.3, pe_link 18.4. `xo bench
--ab` of the warm HTTP windows/amd64 cross build on the linux/arm64
offload host (n=30, before the type run's cache): std's half off against
on 0.814 (125.1 to 101.9 ms, RSS 0.837); the xo with only step 2 against
this one 0.883 (115.9 to 102.3 ms). With every step and the string
table built with one map entry per name (strtab_build_at; the link's
largest cost in the linux profile after the names-only PdbStd), std's
half off against on is 0.766 (127.4 to 97.5 ms, RSS 0.815, 2026-10-30);
the string table change alone is within noise of the build (0.985,
0.973 to 1.001).
