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
minutes and was stopped for the 0.21.0 seed checks; it will be redone
without LTO.

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
