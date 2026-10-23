# Scale suite

Last verified: 2026-10-23

A standing sweep of program size along one axis at a time, so a pass
that grows faster than linearly, or a count field that overflows, shows
up before a user's program finds it. Asked for by the owner on
2026-10-23 after a run of bugs that only large inputs hit:

| Bug | Axis that finds it |
| --- | --- |
| merge_jumps renumbered everything per merge (4b695bc56) | fnlen |
| dv_field scanned every fault stub per variable (4b695bc56) | fnlen, vars |
| read_var quadratic (ea5a9c087) | fnlen, vars |
| Win64 prologue past 255 bytes (33f465cc7, 9a5b29998) | frame |
| COFF relocation count modulo 65,536, the 0.25.1 xo.exe crash (3cd6e6d3e) | funcs, relocs |
| 2 GiB views, 4 GiB join truncation | strbytes |
| debug information growth | fnlen, vars, funcs |

## Pieces

- `internal/progen/scale.go`: `Scale(knob, n)` writes a program that is
  large along one knob only, and `ScaleWant(knob, n)` the output it
  must print, computed in Go, so every backend is checked against one
  value (the Go column of the cross-check is both the Go backend and
  this computation). Programs read `os.env.args().len() + 1`, so no
  backend folds the work away.
- `scripts/scale` (Go): sweeps sizes `-from` to `-to` doubling, plus
  `-sizes` (the count field edges below), for `-knobs`, with
  `-backends` (arm64, llvm, go on the host) and `-amd64` (built for
  linux/amd64, all binaries run in one Docker container at the end).
  Per build it records wall seconds, peak memory of the xo process
  (`ru_maxrss`; children such as clang or the Go toolchain are not
  counted) and the binary's size, and runs the binary against
  ScaleWant. Writes `results.tsv` and `flags.txt`; exit 1 on a failed
  build or a wrong output, 3 on a growth flag only.

## Knobs

Done in the first step (2026-10-23):

| Knob | Program | What grows |
| --- | --- | --- |
| fnlen | one function of n statements, every third an if | blocks, jumps, SSA values, line rows, branch distances |
| vars | n let bindings all live to a sum at the end | live ranges, spills, frame size, DWARF variables |
| funcs | n functions in call chains of 64 | symbols, call relocations, per function tables (unwind, pdata, DWARF subprograms) |
| switch | one match of n Int arms | jump tables or compare trees |
| fns | n independent self recursive functions (never inlined), called in groups of 64 by self recursive group functions | the cost per function alone (2026-10-23; funcs' chains are mostly inlining) |

Next, in order:

| Knob | Program | Aims at |
| --- | --- | --- |
| frame | one function with a local array or struct of n words | stack probes, Win64 UNWIND_INFO (prologue 255 bytes, UWOP_ALLOC_LARGE), arm64 frame offset immediates |
| nest | n nested ifs and loops | recursion depth in the checker, SSA builder, dominator tree |
| strbytes | a Str and a Bytes of n bytes built at run time, joined, sliced, viewed (to past 2 and 4 GiB) | 32 bit lengths, view offsets, join truncation (a run time knob: build cost flat, run cost and memory linear) |
| list | a List of n elements, sorted, grown | element count fields, growth policy |
| modules | n modules of k parts each | per module front end, cache keys, link inputs, split parts |
| relocs | one function with n distinct data and call references | per section relocation counts (COFF 16 bit), GOT and literal pools |
| tasks | n spawned tasks | scheduler queues, stack pool |
| consts | n string literals | string table, literal pools, adrp range |

## Count field edges

Sizes that put a count just under and just over a field's limit, run
for the knob that drives that count (`-sizes`):

| Format | Field | Bits | Driven by |
| --- | --- | --- | --- |
| PE/COFF | section NumberOfRelocations (overflow flag past 0xffff) | 16 | funcs, relocs |
| PE/COFF | NumberOfSections | 16 | funcs with function sections, modules |
| PE/COFF | UNWIND_INFO SizeOfProlog, code offsets, CountOfCodes | 8 | frame |
| PE/COFF | UWOP_ALLOC_LARGE size | 16 (x8) / 32 | frame |
| ELF | e_shnum, st_shndx (SHN_LORESERVE 0xff00) | 16 | funcs, modules |
| Mach-O | nlist n_sect | 8 | sections per object |
| Mach-O | section nreloc | 32 | relocs |
| DWARF | DW_FORM_data2 / ref2 users, line program advances | 16 | fnlen, vars |
| DWARF | 32 bit offsets (DWARF32) | 32 | fnlen x funcs (4 GiB debug sections) |
| PDB | module, section and symbol indices | 16 | funcs, modules |
| arm64 | tbz/tbnz range (32 KiB), b.cond, cbz, adr, ldr literal (1 MiB), b and bl (128 MiB) | 14, 19, 26 | fnlen (one function past 32 KiB and 1 MiB of code), funcs (past 128 MiB of text) |
| arm64 | a jump table's adr (1 MiB) | 21 | switch at 65535 arms (`-sizes 65535`; failed until 2026-10-23) |
| x86 | rel32 (2 GiB) | 32 | not reachable by size alone |
| runtime | Str and Bytes lengths and view offsets | 31, 32 | strbytes |

## Thresholds

Between two sizes n1 < n2 of a knob and backend (n2 at least 1.5 n1),
a measure grows as (n2/n1)^e; e above the limit is flagged when the
larger measure is past a floor, so small, noisy builds do not flag:

| Measure | Flag when e > | and the larger value is at least |
| --- | --- | --- |
| build wall time | 1.4 | 2 s |
| peak memory | 1.3 | 512 MB |
| binary size | 1.15 | 4 MB |

Constant costs (the std every program links, about 0.5 s and 800 MB of
xo on arm64, 5 s with LLVM) hide growth at small n, which is why the
sweep runs to 2^17 and the floors exist. A wrong output or a failed
build is always a failure.

## Runtime budget

- Per build: `-budget` seconds (600 by default; the build is stopped
  at 1.5 times that). A knob's sweep stops at the first size over the
  budget, keeping the row that shows the blowup.
- Lead's 8 hourly full tier: every knob, 2^10 to 2^17 doubling plus the
  edges, backends arm64, llvm, go and amd64, under heavy.sh's memory cap
  as one HEAVY_CLASS=bulk job. Target under 60 minutes; the first full
  sweep's measured time goes here.
- release-verify: arm64 only, 2^10 and 2^14 plus the edge sizes of the
  formats that release (COFF and ELF counts), target under 5 minutes.

## Running it

    go run ./scripts/scale -xo bin/xo -out /tmp/scale -knobs fnlen,vars,funcs,switch \
        -from 1024 -to 131072 -sizes 65535,65536 -backends arm64,llvm,go -amd64

Smoke run 2026-10-23 (n = 64 to 1024, every knob, arm64, llvm, go,
amd64): every output right, no flags (sizes this small sit on the
constant costs).

## First sweep (2026-10-23)

arm64 only, xo of 459134929, 2^10 to 2^17 plus 65535, on the Mac
beside other jobs (wall times are noisy; the caches were still on, so
the second 65536 row of each knob was served from them, which is why
the driver now turns them off and drops repeated sizes). 45 minutes.

| Knob | Finding |
| --- | --- |
| fnlen | build time n^1.6 from 16k to 32k statements (17 s to 52 s); peak memory 1.3 GB at 32k, 6.4 GB at 64k (n^2.3); 131072 exceeds XO_MAX_MEM (18 GB) in emit, after fn_keys |
| vars | build time n^2.2 to n^2.6 from 4k to 16k bindings (1.7 s, 7.9 s, 47 s), 726 s at 131072 |
| funcs | time and memory linear, but about 140 KB of xo memory a function: 8.9 GB at 65535 one line functions, and 131072 exceeds 18 GB after inline.read |
| switch | a match of 65535 arms fails to assemble: `adr out of range` (a correctness bug, not a cost; fixed the same day, jump tables of long functions after their br, regression internal/arm64 TestSwitch65535Arms); time n^1.9 from 2k to 4k arms |

Every program that built printed the expected output.

### Fixes from the first sweep (2026-10-23)

Measured on an idle Mac (the sweep's own times ran beside other jobs
and were inflated up to 4x; profile before reading a sweep's times):

| Cost | Fix | Result |
| --- | --- | --- |
| switch 65535 `adr out of range` | 98896f323: jump tables of long functions after their br | builds and runs (TestSwitch65535Arms) |
| merge_jumps copied the head's list per merged block | 592459457 | vars 32k insert_rc 19.9 s to 0.4 s |
| checker cand_events Map.update push | b4d83b49d | 98 GB allocated to none; check 3.2 s to 0.5 s (fnlen 32k) |
| slp_f64 copy-modify-store of use lists | b4d83b49d | 54 GB allocated to none |
| natural_loops per block body and dominator walks | b4d83b49d | 1.5 s to 0.2 s (fnlen 32k) |
| Map.update copied the value it updates | a9436992c | 40,003 allocations to 86 for 20,000 updates |
| emit memos: each block's first fact copied its dominator's maps, all kept | memo cap (MEMO_INHERIT_MAX) | fnlen 64k emit peak 10 GB to 3.3 GB, build 17.3 s to 9.2 s |

Since: the spill slot scan bounded (0546a6365: vars 64k emit 13.5 s to 7.6 s, still about n^1.5), and decision 0352 (the checker and the SSA copies freed after lowering: xo's self build peak heap 12.4 to 9.7 GB, fns-16k 2.8 to 2.55 GB, its peak now in ssa and cleanup). Still open: vars emit grows about n^1.7 (register allocation with every
binding live at once: 1.3 s, 4.1 s, 13.5 s at 16k, 32k, 64k; spill
slots are found by a first fit scan over all slots). The funcs knob's
memory (about 75 KB a function) is mostly inlining: each function calls
the one before it in chains of 64, so the inliner copies up to 63 bodies
into each. The fns knob measures a function alone: about 115 KB of xo's
memory per six line function (1,778 MB at 8,192, 2,742 MB at 16,384;
280 bytes of binary each), held from the front end on: at 16,384 the
heap grows about 800 MB by the end of check, 470 MB in ssa, 250 MB in
add_landings, 270 MB in cleanup, and 50 MB in emit, so the AST, the
checker's tables and earlier SSA copies stay alive through the build.

