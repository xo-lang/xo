# Self hosted SSA builder, stage 1 (core subset), 2026-10-10

Decision 0150. The Xo SSA builder (`selfhost/ssa`, 7,145 lines of Xo
with its CLI; plus 98 lines of `selfhost/check/info.xo` and about 200
lines of checker records and native mode) against internal/ssa's builder
(Go), compared through the build stage of the new `xo ssadump`: the
module the builder makes before any pass, function by function.

## What stage 1 covers

Functions and methods with `var` parameters and receivers, generic
functions (an instance per list of type arguments, `#N` names), named
and default arguments (defaults lowered in their declaring file), Int,
Float, Float32, Bool, Str, Bytes, the sized integers and Rune
(conversions and checked arithmetic through the runtime), structs
(defaults, `with`, nested field and element assignment), tuples, enums
and recursive types (boxes), Option and Result, `?` with the error frames
of decision 0104 (`xo.frameargs(...)` closures), `match`, `if let`,
`while let`, every pattern form, `while`, `for` over ranges, Lists,
`indexed()`, and Bytes, labeled `break` and `continue`, closures and
named functions as values, string interpolation, Display and Debug text
(`xo.debug[T]` for structs, enums, tuples, lists, newtypes), slicing,
`+` on lists, std calls through std/native (the prelude's Str, Fs, Int
helpers), the prelude operations lowered inline (List, Option, Result,
Int, Float, Str, Stdio, StrBuilder, Env, Proc, Rand, Clock, Duration,
Time), `target`, the main wrapper of `main ! E`, the checks after calls
that faults land on (`rt.landing`), and the rebuild with unwinding checks
(`rt.unwinding`) of programs that use concurrency (the concurrency
constructs themselves are stage 5, so the functions using them are
skipped, the others compared).

Outside stage 1 (skipped, counted): Map and Set (stage 3), interface
values and dynamic values, method values, `derive Ord` order (stage 2),
`defer`, contracts, loop bounds, real-time sections (stage 4), scopes,
tasks, channels, `select`, waits (stage 5), the generic intrinsics of
JSON, SQL, HTTP (stage 6), test binaries (stage 7), and the passes
(stage 8).

## Agreement

`TestSelfhostSSA` on all 630 directories of the repository holding .xo
files (std/, runs/, output/, dot directories excluded), each a run of
its own, with the Xo builder built by the go, arm64, and LLVM backends:

| runs (632) | count |
|---|---|
| built by both and compared function by function | 300 |
| rejected by both checkers | 249 |
| without `main` (built by neither) | 65 |
| not built by the Xo builder: `use go` or `use c` (17), holes (1, XO0900, which the Xo checker does not report) | 18 |

The result is the same with the go, arm64, and LLVM builds of the Xo
builder: **0 differences**. Of the Go builder's 10,990 functions in the
300 runs both build:

| functions | count | share |
|---|---|---|
| identical | 5,844 | 53% |
| skipped by the Xo builder (a construct of a later stage) | 1,642 | 15% |
| never declared by the Xo builder (callees of skipped code) | 3,504 | 32% |

On the compiler's own sources (selfhost/lexer, parser, check, ssa) and
the notes service of bench/xo/v0.4, 2,372 of 3,038 functions (78%) are
identical; maps are most of the rest.

The functions the Xo builder never declares are callees of skipped
code: the helpers of maps (`xo.map.find`, `put`, `rebuild`, `xo.hash`),
JSON encoders and decoders, interface adapters, and the std/native
functions only skipped code calls. The most frequent reasons a function
is skipped (go build):

| reason | functions |
|---|---|
| Map and Set: methods, literals, `m[k] = v`, `for` over them, their Debug text | 693 |
| in a function skipped (closures of it) | 180 |
| concurrency constructs (in the rebuild with unwinding checks) | 171 |
| JSON encoders and decoders (generic intrinsics) | 134 |
| calls of interface methods | 60 |
| `defer` | 39 |
| other generic intrinsics (`status_of`, `launder`, `any_of`, `cast`, SQL) | 120 |
| an error converted to the Error interface | 29 |
| `requires`, `ensures`, invariants | 49 |

`TestSelfhostSSAMutations` (400 mutations of the single directory
programs with `main`: operators swapped, integer and Bool literals
changed, statement lines dropped or doubled, `?` removed):

400 mutations: 285 built by both and 115 rejected by both; of the Go
builder's 7,263 functions, 3,156 identical, 1,021 skipped, 3,086 never
declared, 0 differences (go build; 150 more with the same result during
development).

## Bugs and gaps found

- **arm64: `add xN, sp, #4808`** (gotchas/arm64-add-immediate-range.md):
  the address of a frame's $OUT slot was one `add`, whose immediate stops
  at 4095; a call passing a struct of 4.7 KiB by value (the builder's
  `Builder`, by value receivers at the time) put the slot past it and
  the assembler rejected the arm64 build. Fixed with a shifted add,
  golden `internal/arm64/testdata/run/far_out_slot`, `TestFarOutSlot`.
- **By value receivers are read before the arguments**
  (gotchas/receiver-read-before-args.md): `b.type_name(b.impl_named(t))`
  looked up a type `impl_named` had just added in a copy of the builder
  made before it. Consistent on the three backends and left to right like
  any operand, but the opposite of `var` receivers (decision 0142); spec
  5.2 now says so, golden `var_recv_after_args` covers it.
- **Go backend: `==` on enums boxed both values** (rt.Eq with a type
  switch). An enum of a user module whose variants have no fields is now
  compared by tag; 9% of the Go build's time.
- **Checker records**: a constructor `Kind.Newline` was recorded on its
  receiver `Kind`, not the selector (the Go checker records the
  selector), and methods had no declaration file. Both fixed in
  selfhost/check; neither changed the checker's dump.
- **Own sites in the port** (no compiler change): a by value read of
  `b.m` in the arguments of a `var` call of `b` copies the whole module
  (every function list) when the module is next written, and binding a
  list read out of the function under construction (`let ins =
  ...instrs`) copies it at the next push. The port reads in place.

## Speed

Apple M4 Pro, 10 builds after one check (the builder alone: `xo ssadump
--bench=10`, `ssa.BuildRaw`, against `selfhost/ssa --bench=10`), ms for
the 10 builds. The Xo builder makes fewer functions (the callees of
skipped code are never declared), shown in parentheses.

| input | Go builder | Xo, Go backend | Xo, arm64 | Xo, LLVM |
|---|---|---|---|---|
| selfhost/check (1,015 functions; Xo 927) | 745 | 1,247 (1.67x) | 7,655 (10.3x) | 1,076 (1.44x) |
| selfhost/parser (377; 364) | 244 | 437 (1.79x) | 2,502 (10.3x) | 356 (1.46x) |
| selfhost/ssa (1,326; 1,233) | 1,029 | 1,730 (1.68x) | 11,599 (11.3x) | 1,509 (1.47x) |
| bench/xo/v0.4/08-notes-service (192; 152) | 27 | 40 (1.48x) | 270 (10x) | 32 (1.19x) |

The LLVM build is 1.2 to 1.5 times the Go builder, the Go backend build
1.5 to 1.8 times (2.2 times before tag comparison of enums and the port
fixes above). The arm64 build is about 10 times slower: the arm64
backend passes a `var` receiver by value in and out (the Builder, which
holds the checker, is about 300 words: every method call copies 2.4 KiB
each way through the stack), where the LLVM backend keeps large `var`
parameters in the caller's memory (decisions 0145, 0148).

## Next experiments

1. **arm64: large `var` parameters and structs by reference**, the port
   of decisions 0145 and 0148 to internal/arm64 (abi.go, emit.go): pass
   a pointer for a struct of 256 bytes or more and write `var` ones back
   in place. Expected: the arm64 build of selfhost/ssa near the LLVM
   build (it is 7 times slower now); measure with `.tools/xossa
   --bench=10 selfhost/check` built with `--backend=arm64`. The arm64
   build of the checker pays the same.
2. **Stage 2, interfaces and dynamic values** (build_iface.go,
   build_show.go toDyn, ifaceCall): 60 functions call interface methods
   and 29 convert an error to `Error`; `main ! Error` programs need it.
3. **Stage 3, Map and Set** (build_map.go): 421 functions skipped for
   maps and the largest share of the functions never declared.
4. **Go backend: field reads of list elements** go through
   `rt.List_AtM` in some generated code (`b.c.objs[o].module` in
   `Builder.obj_mod`) and `List_RefM` in other; the profile's memmove
   (7%) suggests whole elements are copied. Check whether `List_AtM` of a
   large struct copies it, and read fields through `List_RefM` when it
   does (internal/gen).

# Stage 2: interfaces and dynamic values, 2026-10-11

`selfhost/ssa/iface.xo` (about 1,000 lines): `xo.iface[I]` values (the
conversion with its method, show, and equality functions, the type
numbers, an interface value converted to another), calls of interface
methods (on interface values, on values of a type parameter with an
interface constraint, on Error, Display, and Debug values), the
adaptation of a method's result to the interface's (`Ok` around a
method that cannot fail, `from` conversion of its error), `xo.dyn` (a
value converted to Error, Display, or Debug, `main ! Error`), method
values (the checker's literal, decision 0109), `==` on values holding
interface values and frames (`xo.eqc`), `derive Ord` order (`xo.cmp`),
`==` on `http.Request`, generic named functions as values, and
`a.to_lower() == b.to_lower()`. The rules added for the stage are in
decision 0150 (type numbers given after skipped code are not compared).

`TestSelfhostSSA`, go, arm64, and LLVM builds of the Xo builder, 0
differences on all three:

| functions (of the Go builder's 11,064 in the 305 runs both build) | stage 1 | stage 2 |
|---|---|---|
| identical | 5,866 | 6,380 |
| skipped (a later stage) | 1,642 | 1,520 |
| never declared (callees of skipped code) | 3,504 | 3,164 |

Every stage 2 reason for skipping is gone but the Debug text of an
Atomic (stage 5) and 13 functions whose interface type number was given
after skipped code (all in programs with a Map, stage 3).
`TestSelfhostSSAMutations` (400 mutations, go build): 298 built by both,
101 rejected by both, 3,362 of 6,700 functions identical, 0
differences.

Speed, go build of the Xo builder (`--bench=10`, ms for 10 builds):
selfhost/check 710 (Go builder) against 1,168 (1.65x); selfhost/ssa
1,017 against 1,720 (1.69x).

Found: the arm64 backend's conditional branches to fault stubs could
not reach past 1 MiB of code (`b.hs Lf852_fault1: fixup value out of
range`): the arm64 build of the Xo builder has a 280,000 instruction
`Builder.std_call` (the large `var` receiver is copied in and out at
every call). `farTests` now rewrites `b.cond`, `cbz`, and `cbnz` of such
functions as it rewrote `tbz` (gotchas/arm64-tbz-range.md,
`TestFarBranches`). A by value receiver read before its arguments
(`b.c.show(b.impl_named(ct))`) faulted in the port, as in stage 1
(gotchas/receiver-read-before-args.md). The test's poison detection
matched a string constant holding the marker text (two functions of
selfhost/ssa itself were skipped for it); it now looks at types and
callee names.
