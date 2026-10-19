# Where xo's code size goes (2026-10-18)

Owner request through the lead: Go 1.26.1's `compile` has 10.3 MB of
`__text`; xo has 40.3 MB from about 171k lines of compiler/ Xo, against
about 97k lines and 7.9 MB for the old Go implementation. Measured on
the development Mac (darwin/arm64), the xo of main fbaae84c (native
backend, dead function removal on), with `scripts/codesize.py BINARY`
(symbol table plus one objdump pass; output kept below) and builds with
passes turned off.

## Summary

The size is in how the native backend handles large values, not in
generics or inlining:

| Where | MB | Share of 40.18 MB |
|---|---|---|
| Whole value word copies through the x16 scratch (`ldr x16, [a]; str x16, [b]`), runs of 16 words or more | 6.45 | 16.1% |
| Release calls in runs of 32 or more (a large value dropped field by field), with their `ldr x0` | about 4.7 | 11.8% |
| `adrp`+`add` of `_xo_empty_list` | 3.63 | 9.0% |
| Inlining, all of it (`XO_PASSES=-inline`: 38.77 MB) | 1.41 | 3.5% |
| Generic instantiations, all 542 copies | 0.60 | 1.5% |

## What the code is made of

By instruction kind (10,046,095 instructions):

| Kind | MB | Share |
|---|---|---|
| stack load/store | 13.60 | 33.9% |
| other load/store | 8.61 | 21.4% |
| constant address (adrp+add) | 4.30 | 10.7% |
| branch | 3.47 | 8.6% |
| release call | 3.45 | 8.6% |
| mov | 2.90 | 7.2% |
| other | 1.96 | 4.9% |
| other call | 0.67 | 1.7% |
| compare | 0.58 | 1.5% |
| atomic refcount | 0.50 | 1.3% |
| fault call | 0.14 | 0.3% |

By module, with the bytes per source line (non test .xo lines):

| Module | MB | Lines | Bytes a line | Word copies |
|---|---|---|---|---|
| ssa | 8.92 | 22,548 | 396 | 20.8% |
| check | 5.91 | 19,581 | 302 | 19.6% |
| gen | 5.13 | 13,936 | 368 | 21.4% |
| arm64 | 4.99 | 25,429 | 196 | 13.4% |
| cli/modules | 1.58 | 4,733 | 334 | 17.7% |
| parser | 1.33 | 7,794 | 171 | 16.0% |
| llvm | 1.15 | 7,172 | 160 | 12.4% |
| cli/semantic | 1.01 | 4,821 | 210 | 20.3% |
| wasm | 0.99 | 21,962 | 45 | 8.8% |
| macho | 0.77 | 11,824 | 65 | 3.0% |

The modules that walk the AST and the compiler's large state structs
(ssa, check, gen) cost 300 to 400 bytes a line; wasm and macho, which
work on integers and byte buffers, 45 to 65.

Concentration: the top 100 of 9,121 functions are 22.0%, the top 500
52.2%. The largest are `build_native` 645 KB, `ssa.Builder.runner` 283
KB, `check.Checker.habit_call` 213 KB, `gen.Gen.gen_func_with` 193 KB.

## The three causes

### 1. Large values copied a word at a time (6.66 MB)

Xo values are copied by value, and an enum is as large as its largest
variant with payloads inline. The types the compiler copies most
(DWARF of the build, in 8 byte words): `gen.Gen` 611, `Lowered` 537,
`ssa.Builder` 534, `arm64.G` 407, `MatchArm` 315, `EditSession` 305,
`Checker` 288, `SelectArm` 273, `IfExpr` 245, `Comprehension` 242,
`Stmt` 167, every `Node` 168.

The backend splits a loaded aggregate into one virtual register per
word; most are spilled, so each word becomes `ldr x16, [src, #o]` then
`str x16, [sp, #slot]`, and the store back the reverse. `for s in
b.stmts` in check.children copies 167 words per statement (334
instructions) before reading one field. Runs by length:

| Run | Runs | MB |
|---|---|---|
| 1 to 15 words | 10,234 | 0.20 |
| 16 to 63 words | 4,227 | 1.08 |
| 64 words or more | 3,411 | 5.37 |

The most common long runs are 321 and 324 words (`MatchArm` with its
list header: 465 runs, 1.2 MB), 286 to 289 (321 runs, 0.74 MB), and 247
to 248 (264 runs, 0.52 MB).

### 2. Large values released field by field (about 4.7 MB)

Reference counting is field sensitive (compiler/ssa rc.xo: every heap
leaf of a value is tracked), so a dying `Checker` gets one release call
per heap field, each `ldr x0, [sp, #slot]; bl drop_<type>`. Release
calls in runs:

| Run | Runs | Calls |
|---|---|---|
| 1 | 46,622 | 46,622 |
| 2 to 7 | 32,148 | 113,168 |
| 8 to 31 | 7,792 | 112,180 |
| 32 or more | 3,427 | 591,116 |

`build_native` alone has 57,000 of its 68,000 calls as releases.

### 3. The empty list's address (3.63 MB)

453,180 `adrp x, _xo_empty_list; add x, x, #0`: every `[]` (struct
defaults, a field moved out and replaced, a new list) builds the
address again. The rest of the `adrp` targets are small (`_write` page
string constants 75,194).

### Not the cause

- Inlining: 1.41 MB in all (`XO_PASSES=-inline` builds 38.77 MB).
- Generics: 542 instantiations, 0.60 MB (`xo.debug` 83 copies 212 KB,
  `map.put` 66 copies 108 KB, `map.rebuild` 86 copies 68 KB,
  `list_sort_by` 78 copies 47 KB).
- Fault paths: 0.14 MB of calls, 0.12 MB of traps.
- Landing checks (`ctx_unwinding`): 6,721 calls, 0.05 MB.

## Ranked fixes

Estimates are of `__text` removed; each needs its own measurement.

1. **Read fields in place instead of copying the value** (about 5 MB of
   cause 1, and the matching releases of cause 2). A list element, a
   `for` element, or an enum payload whose only uses are field reads and
   passing it on gets its fields loaded from the source memory, as
   compiler/arm64 byref.xo already does for parameters of 256 bytes or
   more (`mem_of`). The general form keeps any aggregate of 16 words or
   more in a stack home instead of words, with copies as a loop. Effort:
   large (isel of Field, Payload, list index, and the regalloc of
   aggregates). Speeds the compiler too.
2. **One release for a value that dies whole** (about 4 MB). When every
   heap leaf of a root is released at the same point, emit one release
   of the root, lowered to a drop helper that takes the value's address
   (the existing aggregate drop helper takes every word as an argument,
   so it would trade 170 releases for 300 argument moves). Needs the
   value in memory, which fix 1 or the by-reference homes give. Effort:
   medium after fix 1, large before.
3. **Box large fields and payloads automatically** (about 2.5 MB for the
   AST types alone): `MatchArm` (315 words: a `Pattern`, an `Expr?`, a
   `Stmt?` inline), `SelectArm` 273, `IfExpr` 245, `Comprehension` 242,
   `IfLevel` 291. Source code cannot write `Box`; the SSA layout boxes
   only recursive types (the build's `Box[IfExpr]`, `Box[Stmt]`).
   Extending that rule to a field or variant payload of 32 words or more
   makes a copy one word and a retain. A layout change in compiler/ssa,
   checked by the fixed point; no language change.
4. **The empty list in a register or as zero** (up to 3.6 MB): either a
   reserved callee saved register holding `_xo_empty_list` (one less
   allocatable register; C callbacks must set it), or the empty list as
   the null pointer (every list operation tests for it; an ABI change).
5. **Pair the word copies that remain** (0.45 MB in ldp/stp range, 2.8
   MB with a base address add): a peephole on `ldr x16`/`str x16` pairs
   at consecutive offsets. Safe, small; worth it only after 1.
6. **Inline budget by callee size**: at most the 1.41 MB inlining costs
   in all; not worth tuning before 1 to 3.

None of the top four is clearly safe to land in one step, so none is
implemented here. The next experiment for each:

- Fix 3: in the SSA layout's boxing decision (where recursive types get
  `Box[T]`), box a field of 32 words or more too, rebuild xo, and run
  `scripts/codesize.py`; the 321/324 word runs (1.2 MB) should go.
- Fix 1: count in compiler/arm64 isel how many Field reads come from a
  list index or loop element value with no other use (an `XO_DEBUG`
  counter), to size the in place read before writing it.
- Fix 4: rebuild with the empty list address in x28 for one function
  class and compare spills.

The LLVM comparison (bytes per IR op against LLVM's build of the same
module) is still to run: a full LTO link of xo with LLVM took over 40
minutes and was stopped for the 0.21.0 seed checks; it was redone
without LTO on 2026-10-19 (below, "LLVM without LTO").

## Fix 1, first step (2026-10-18)

compiler/arm64 inplace.xo: `list.index` of an element of 128 bytes or
more is a stable value at its address (byref.xo `stable`, so field,
payload, and tag reads and the release load from the list) when its
uses read it, at most one takes it whole, and nothing between the read
and the uses can change the list: for a list reached through fields of
a read only parameter, no release, move (call argument), or update of
that parameter; otherwise no call but in line reads. Optional pass
`inplace_elems`.

xo built by itself with the same compiler, the pass on against
`XO_PASSES=-inplace_elems`: 40,491,972 to 39,915,536 bytes of
`__text` (-576 KB, -1.4%). The first form (no calls at all between the
read and the uses, no whole use) changed nothing measurable: the
compiler's loops call `out.push` and wrap the element whole
(`Node.NStmt(s)`). What remains is elements of lists held in locals and
`var` parameters, and loop elements passed on as call arguments.

## Fix 2, landing pads chained (2026-10-18)

Where the releases of whole values come from: a count in compiler/arm64
isel.xo's release of a whole struct held in registers (each heap leaf
released apart; one held in memory already calls the type's `mem_drop`
helper once), over xo built by itself: 327,438 leaf releases, 292,333
of them (89%) on landing pads. Each call's pad released every leaf the
function held at the call, so a value live across a run of calls was
released leaf by leaf again on every one of their pads; `build_native`
alone had 64,379. By root: block parameters 119k, entry parameters 52k,
call results 137k (the `S` that `ssa.Builder.begin` returns, live until
`end`, 68k).

compiler/ssa chainpads.xo (`chain_pads`, in the reference counting
after the landings are placed): a pad (field reads and releases, then
the unwind exit) whose released leaves include all of a nearby pad's
(the 32 nearest by block number; the same leaves count for the later
of two) releases only the rest and goes to that pad, when each of its
releases is all inside or all outside the other pad's leaves and the
other pad reads from outside only values it reads too. Optional pass
`chain_pads`.

The first form, without the restriction below, saved 4.11 MB (40,077,208 to
35,963,580 bytes). But the Mac sweep found httpparse h2_get and
h2_varied 1.6% slower (reproduced), and callgrind of the HTTP server's
h2 scenario +4.6% (84,668 to 88,544 instructions a request), nearly all
in tlssrv.recv_some, feed, and send. Those take `var c: Conn` by
reference. A pad after a call that writes a memory home starts with the
home's contents unknown (byref.xo compute_held). A second way into a pad
from such a pad left the home unknown at the pad's reads of the value,
so the value was no longer held at every use and was loaded whole at
entry (recv_some's fast path copied dozens of Conn words). Now a pad that
reads a value of a type that may have a home is never a target: a struct
or enum of 256 bytes or more that the function takes as a `var`
parameter or passes to a callee's.

With that rule, xo built by itself with the same compiler, the pass on
against `XO_PASSES=-chain_pads`: 40,102,192 to 37,140,368 bytes of
`__text` (-2.96 MB, -7.4%). Mac sweep, 114 workloads: 1 slower,
fault/none 1.012 against a spread of 1.011, which a 5 round rerun put
within its spread (1.008, spread 1.010). 1 faster: httpparse h1_chunked
0.969. h2 callgrind 84,674 to 84,823 (+0.18%, within MODE noise). The
golden chain_pads faults at each call of a run in turn, and the leak
check finds every allocation released.

## Reproduce

```
go run ./scripts/xobuild
python3 scripts/codesize.py bin/xo
XO_PASSES=-inline bin/xo build --backend=arm64 --std=$PWD/std -o /tmp/xo-noinline compiler/xo
```

Toolchain: macOS 27.0 arm64, Apple clang 21.0.0 objdump and nm, xo of
main fbaae84c. Last verified: 2026-10-18.

## Fix 4, landed (2026-10-18, Xo lang 4)

The emitter-only form: compiler/arm64 emit.xo local_empty rewrites each
`adrp`+`add` of `xo_empty_list` in a function as one `adr` of the
function's own read only empty list (`{IMMORTAL, 0, 0}` as `.long` words
after it, like a literal's count never written), for functions up to
200,000 instructions (adr reaches 1 MB; build_native is 653 KB). A `[]`
is then not one address, so llrt's two address tests (tasks.list_release,
chan.release) test the immortal count instead (obj.is_immortal).
xo's `__text` 40.05 to 38.36 MB (-1.69 MB, 4.2%; binary 44.09 to 42.07
MB); 987 adrp loads of the empty list remain (not yet examined).
The Mac sweep (bench/perf/sweep.py, 114 workloads, 3 rounds): 0 slower,
2 faster beyond spread; httpparse h1 rows at 1.02 to 1.08 inside spreads
of 1.06 to 1.09 read 0.96 to 0.97 on a 5 round rerun (noise).

The empty list as zero was not taken: llrt uses 0 for "no list" apart
from the empty list (list_release, the channel results), and generated
code reads a list's length from its header without a null test, so it
is an ABI change needing a seed; a pinned register would cost one
allocatable register and C callbacks' entry.


## Release helpers: one instruction less (2026-10-18, Xo lang 4)

Not a size fix but the same helpers: compiler/arm64 rc.xo dec_text tests
`subs n, t, #1; b.le` instead of `cmp t, #1; b.le; sub n, t, #1` (an
immortal count overflows on the subtract, which sets V, so le still
sends it to the slow path), and the `dropp` element callback of a
List[Str] or List[List] carries the drop body instead of branching to
the drop helper. On Xo lang 3's plain HTTP request profile (155 Str or
List releases and 31 dropp calls a request) that is about 190
instructions a request less, at no size cost. Inlining the fast path at
the call sites was not taken: 16,959 drop Str sites in the HTTP server
binary make it about 270 KB (17%) larger. The Mac sweep (114 workloads,
3 rounds): httpparse/h1_get 0.940 (faster beyond spread), ctx/none 1.005
over a 1.002 spread (an empty program's startup), the rest inside
spreads.

## Fix 5, landed (2026-10-18, Xo lang 4)

compiler/arm64 lsr.xo copy_runs, a text pass before reload_reuse:
each run of four or more word copies through x16 or x17 (`ldr x16,
[A, #a]` then `str x16, [B, #b]`, the next words after) becomes x16 =
A + a, x17 = B + b, then `ldp`/`stp` of q30 and q31 (32 bytes each),
a q pair and a d pair for the tail, up to 128 words a base. Most of
these runs have offsets past the 504 bytes ldp of x registers reaches
(4000 and more), so pairing x16 with x17 would have taken 241 of 4,177
pairs in the HTTP server. Taken when one base is sp and the other not
(or both sp with the ranges apart) and neither x16 nor x17 is read
after the run before it is written (gotchas/arm64-text-pass-scratch-live.md).
The x17 runs are a call's stack arguments (Xo lang 3's head_of(11 args)
in http1.parse_head).
xo's `__text` 38.43 to 35.37 MB (-3.06 MB, 8.0%; binary 42.15 to 39.07
MB). The Mac sweep (114 workloads, 3 rounds): 0 slower, 5 faster
beyond spread (httpparse h2_get 0.952, h2_varied 0.953, h1_get 0.988,
tls aes128 0.962, text/none 0.994). The base xo was built before a
rebase onto main 29ae8c4a, so the faster rows may include main's
changes from that range.

## Combined, main 0b9c7143 (2026-10-18, Xo lang 4)

Fixes 1 (first step), 2 (chain_pads, b303289c), 4 and 5 together: xo
(s3 of a fixed point from main 0b9c7143) has 32.59 MB of `__text` in
9,387 functions, against the 40.18 MB baseline above (-7.59 MB, 18.9%);
the binary is 36.26 MB. Top 10 functions 5.3% of `__text`, top 100
22.7%. Go's `compile`: 10.3 MB.

## Borrowed pair loop elements (2026-10-19, 01e51108)

`for (i, x) in xs.indexed()` and `for (k, v) in m` built a tuple of the
pair before matching the pattern; the tuple consumed the element, so
each iteration retained and released its leaves where `for x in xs`
borrows them (decision 0186 addendum). The builder now binds a two
part tuple pattern's parts apart (compiler/ssa stmt.xo, map.xo).
std/native http's match_parts loop, copied into a test program: 297 to
249 instructions, 17 to 6 count operations; an indexed loop that only
reads its elements: 65 to 37 instructions, no count operations.
xo built by the base and the new compiler: `__text` 32,645,480 to
32,238,276 bytes (-1.25%). HTTP callgrind (MODE=short, Linux arm64 in
Docker, instructions per request): plain arm64 25,777 to 25,603
(-0.68%); tls arm64 43,658 to 43,730 in one run and 43,870 to 43,806 in
the next, within the scenario's run to run spread (llvm tls varied
55,410 to 55,885 between runs of compilers that differ only in this
change).

## Spill slots in stack words (2026-10-19, pass arg_slots)

Xo lang 4 counted 2,576 sp to sp word copies into return areas and
2,338 into stack arguments in the HTTP server (about 2.6% of its
instructions). Tagging each stack destination move of a call or return
in the Linux arm64 assembly of bench/perf/http/xo by its source: 7,042
call and 6,013 return moves from a spill slot, 3,241 and 3,479 from a
register, 339 constants, none from a memory home. The register
allocator now gives such a value the stack word itself as its spill
slot (decision 0146 addendum). The HTTP server's assembly 610,039 to
587,728 lines (-3.7%); xo `__text` 32,238,276 to 31,524,628 bytes
(-2.21%). HTTP callgrind (MODE=short, instructions per request): plain
arm64 25,687 to 25,649 (-0.15%), tls arm64 43,826 to 43,740 (within its
spread); llvm unchanged by construction.

## LLVM without LTO (2026-10-19, main 05a33dad)

xo (compiler/xo) built by the same xo with each backend on the Mac:
arm64 70 s, 6.1 GB peak; LLVM with `XO_LLVM_SPLIT=12 XO_LLVM_JOBS=3
XO_LLVM_LTO=none` 639 s at -O2 (the default), 1,180 s at O1, 6.0 GB peak.

| Build | `__text` | `__unwind_info` + `__eh_frame` | Binary |
|---|---|---|---|
| arm64 | 31.78 MB | 0.42 MB | 34 MB |
| LLVM -O2, no LTO | 89.82 MB | 4.10 MB | 112 MB |
| LLVM O1, no LTO | 89.89 MB | | |

LLVM's code is 2.83 times arm64's, and O1 is no smaller than -O2, so
inlining is not the cause; the IR the LLVM emitter writes is. The 5,659
functions both binaries name: 28.54 MB against 66.14 MB (2.3x); the
rest of LLVM's 23.7 MB is its 104,744 other symbols (helpers such as
`xo.drop.<type>`, map instances, outlined functions). By module: ssa
6.29 to 19.39 MB, arm64 4.51 to 17.95, gen 3.85 to 14.83, check 4.86 to
11.09.

The largest cause found: the drop helpers of large structs take the
struct by value, so each drop site copies every word to the stack
arguments first. In the whole LLVM binary, 11.15 MB (12.4% of its 22.5
million instructions) are runs of more than 40 loads and stores right
before a call, 85% of them before `xo.drop.Builder` (0.71 million
instructions), `xo.drop.Gen` (0.62 million), `xo.drop.Checker` (0.52
million) and `xo.drop.G` (0.28 million). arm64.G.regalloc is 104 KB on
arm64 and 993 KB on LLVM: 72% of its instructions are loads and stores,
34% of them in 111 runs of up to 765 words before `xo.drop.G`, and 3,262
calls of rl_free_n follow inline releases of G's lists. arm64.G.inline_rt
(72 KB against 531 KB) has no such run but the same mix (75% loads and
stores, 846 rl_free_n calls).

Next experiments (compiler/llvm): give `xo.drop.T` and `xo.share.T` of a
type of BY_REF_MIN (256) bytes or more a pointer parameter, as the
arm64 backend's mem_drop reads a value in memory, and rebuild with the
command above (expected: most of the 11.15 MB); then count where
inline_rt's loads and stores come from in its IR (`xo asm
--backend=llvm` of compiler/xo, the function's landing pads).

## LLVM: count helpers of big values by pointer (2026-10-19)

The `xo.dup.T`, `xo.drop.T` and `xo.share.T` helpers of a struct, tuple
or enum of 256 bytes or more (BY_REF_MIN) take a pointer and read each
counted field where it is (compiler/llvm gen.xo rc_by_ptr, rc_at); a
Release, Retain or Share passes the memory that holds the value (a
`var` parameter's home, decision 0148, or decision 0147's unwritten
memory), else the function's slot of the type after a store; a list
update releases the old element where it is, and the element, box and
environment callbacks pass their pointers on. xo built by LLVM (split
12, no LTO, -O2, as above): `__text` 89.82 to 50.56 MB (-43.7%), the
build 639 to 446 s. `XO_LLVM_OPT` other than O0 to O3 is now an error
(gotcha llvm-opt-level-os-ignored).

Pushed as 7724ca1e. From now on these tables carry an LLVM column next
to native whenever code generation changes (owner, 2026-10-19):

| xo `__text` | native arm64 | LLVM (split 12, no LTO, -O2) |
|---|---|---|
| main 05a33dad (before) | 31.78 MB | 89.82 MB (2.83x) |
| 7724ca1e | 31.78 MB | 50.56 MB (1.59x) |

xo.exe by the release recipe (`--targets=windows/amd64`, split 16,
ThinLTO, O2, `XO_LLVM_JOBS=4`, winxc clang 23.1.3 on the Mac): 80.3 MB
(was 160 MB), `.text` 74.7 MB; the build 341 s, of it 312 s in clang
and the link, 6.79 GB peak RSS.

HTTP callgrind (MODE=short, Linux arm64 in Docker, instructions per
request, main f33791fc against 7724ca1e): llvm plain 22,237 and 22,261
against 22,280 and 22,192 (two runs each; a first pair read 21,461
against 22,294, outside the spread of the repeats), llvm tls 52,459
against 51,933; arm64 unchanged by construction (24,442 against 24,675
plain, 40,950 against 40,396 tls: the run to run spread).

What remains of LLVM's 50.56 MB against native's 31.78 (7724ca1e),
ranked, from the symbol table and one objdump pass:

1. Releases on the rarely run paths, 8.17 MB: LLVM splits a function's
   cold blocks out (113,092 `.cold.N` pieces, 12.66 MB in all), and the
   2,062 that release values (landing pads and frees) hold 8.17 MB. The
   count helpers of Str, List, Box, Func and Handle are `alwaysinline`
   (attribute #4), so each release there is the full test, decrement
   and slow path. Next: call them out of line from the pads (a `cold`
   or `noinline` call site in a pad block).
2. The functions both binaries have, 33.30 MB against 28.58 (+4.72 MB),
   largely the inlined release fast paths of hot code.
3. 92,273 small cold pieces of 45 bytes on average, 4.16 MB: each
   inlined release's own slow path. Next: one out of line call for the
   shared count and free path, the decrement inline.
4. LLVM's own helpers, 2.8 MB: count helpers 1.42 MB (586 dup, drop,
   share; 1,227 element, box, environment), other `xo.*` 0.28, map
   instances 0.19, eq and hash 0.17, generic instances 0.16, the
   machine outliner 0.13, other functions 0.40.
5. Symbols nm prints as `_x` (unnamed), 1.86 MB: not identified yet.

## LLVM: count helpers' rare paths out of line (2026-10-19)

The two largest LLVM causes above (1, releases on the rarely run paths,
8.17 MB; 3, each inlined release's own slow path, 4.16 MB), in
compiler/llvm gen.xo and func.xo:

- The drop helper of Str, List, Box, Func, and Handle (alwaysinline)
  keeps inline the decrement of a thread local count, the last
  reference's free (one runtime call, as common as a decrement in hot
  code), and the immortal check (a literal's count: no change); a shared
  count's drop (its atomic decrement, maybe the free) is one cold
  noinline function per type, `xo.dropslow.T`. The dup is unchanged.
- A count helper called in a block that only a fault's cleanup reaches
  (func.xo cold_blocks: from the blocks a landing check sends a fault
  to, those the entry does not reach without passing through one) is a
  call with the call site attribute `noinline cold` (#12), not its
  inlined body.

xo built by LLVM (split 12, no LTO, -O2), main b48b8156's xo against the
change's, the same tree; `size -m`, and Xo Lang 2's cold.py over objdump
for the cold pieces:

| | main | the change |
|---|---|---|
| `__text` | 51.51 MB | 32.59 MB (-36.7%) |
| the LLVM build of xo | 609 s | 372 s |
| hot code | 38.42 MB | 30.84 MB |
| cold pieces that release (landing pads, frees) | 8.57 MB (2,070) | 0.09 MB (415) |
| other cold pieces (per site slow paths) | 4.17 MB (92,580) | 1.58 MB (51,955) |

Two earlier shapes, measured and dropped: everything past the decrement
out of line (dup's shared path too): 31.10 MB but HTTP about 1.5%
slower (short mode), since a literal's immortal count took the out of
line call on every dup and drop (xo.dropslow.Str 742 and the dup's 233
instructions a request); the last reference's free inline but the
immortal check out of line: 32.30 MB, speed inconclusive in short mode.

HTTP callgrind on LLVM, MODE=full (N=100, REPS=2), main and the change
interleaved, two rounds each, the median of four (repeats within about
0.2%):

| scenario | main | the change |
|---|---|---|
| plain | 22,473 | 22,215 (-1.1%) |
| plain-newconn | 29,370 | 29,169 (-0.7%) |
| tls | 52,682 | 52,370 (-0.6%) |
| tls-newconn | 1,793,211 | 1,781,670 (-0.6%) |
| h2 | 89,486 | 89,530 (+0.05%, inside main's 89,339 to 89,633) |

xo.exe by the release recipe (`--targets=windows/amd64`, split 16,
ThinLTO, O2, `XO_LLVM_JOBS=4`, winxc clang 23.1.3 on the Mac), main
b48b8156 against the change: 80.56 MB to 63.65 MB (-21.0%), `.text`
75.00 MB to 57.81 MB (-22.9%); the builds took 683 s and 238 s, the
first at a higher load (not comparable).

Pushed as c40c2514 (gate: -short, internal/llvm with
XO_TEST_LLVM_OPT=O2, the fixed point).

MODE=short's spread on LLVM is about 3% run to run (the same build read
22,087 and 23,397), too wide to judge a 1% change: use MODE=full for
LLVM speed checks.

| xo `__text` | native arm64 | LLVM (split 12, no LTO, -O2) |
|---|---|---|
| 7724ca1e | 31.78 MB | 50.56 MB (1.59x) |
| main b48b8156 | (unchanged) | 51.51 MB |
| this change | (unchanged) | 32.59 MB (1.03x) |
