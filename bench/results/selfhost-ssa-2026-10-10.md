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

# Stage 3: collections, 2026-10-11

`selfhost/ssa/map.xo` (about 1,100 lines): Map and Set as structs of
lists with their generated helpers (`xo.hash[T]`, `xo.map.find`,
`put`, `rebuild`, `remove`, `keys`, `values`, `entries`), literals,
every method build_map.go lowers (`put`, `add`, `remove`, `get`,
`get_or_fault`, `contains`, `len`, `is_empty`, `keys`, `values`,
`entries`, `to_list`, `update` with its packed closure and the
`catches` unwind of decision 0085), `m[k] = v`, `for` over a Map or Set,
`m.values().any/all(...)` inlined (build_hof.go), their Debug text,
`==` on values holding them (`xo.eqc`), and constants of collection
types (`xo.const`, `xo.droplist`, `rt.const_once`). Buf was already
lowered in stage 1.

| functions (of the Go builder's 11,088 in the 305 runs both build) | stage 2 | stage 3 |
|---|---|---|
| identical | 6,380 | 8,002 |
| skipped (a later stage) | 1,520 | 758 |
| never declared (callees of skipped code) | 3,164 | 2,328 |

0 differences with the go, arm64, and LLVM builds. No reason for
skipping of stages 2 or 3 is left (one function keeps a
type number given after skipped code). `TestSelfhostSSAMutations` (go
build): 4,203 of 6,700 functions identical (was 3,362), 0 differences.

Speed, go build of the Xo builder (`--bench=10`, ms, measured while
another test ran): selfhost/check 757 (Go builder) against 1,259
(1.66x); selfhost/ssa 1,119 against 1,949 (1.74x); the notes service
27 against 45 (1.67x).

# Stage 4: contracts and deferred code, 2026-10-11

`selfhost/ssa/contract.xo` and `defer.xo` (about 900 lines): `requires`
(with and without `else`, the fault detail line), `ensures` (the
wrapper, `<name>.body`, `old(...)`, `result`), invariants (after struct
literals and `with`, after `var` methods, `xo.invariant[T]`), `defer`
(lowered at the end of each block, `return`, `?`, `break`, `continue`,
and the unwind path), the landings the builder places in blocks with
deferred code and the fault continuations of faults inside deferred
code (landing.go flushLanding, build_defer.go faultCont), loop bounds
(XO-F006), and real-time sections (`rt.realtime_enter`/`exit`). The Xo
checker now records an ensures clause's `result` binding and an
invariant's field bindings. internal/selfhost/testdata/ssa/stage4 is a
program of these constructs (defer at every way out, faults in deferred
code) that the comparison and the mutations now include.

| functions (Go builder) | stage 3 (11,088) | stage 4 (11,140, one more run) |
|---|---|---|
| identical | 8,002 | 8,230 |
| skipped (a later stage) | 758 | 689 |
| never declared (callees of skipped code) | 2,328 | 2,221 |

0 differences with the go, arm64, and LLVM builds;
`TestSelfhostSSAMutations` 0 differences on 400 mutations (seed
20261010: 3,877 of 5,997 functions identical) and 1,500 (seed 7: 15,677
of 23,056). No construct of stages 1 to 4 is skipped any more: what is
left is concurrency and waits (stage 5: 190 functions, 36 more with
scopes), the generic intrinsics (stage 6: JSON 136, `status_of`,
`launder`, `any_of`, `cast`, SQL), and their callers. On the compiler's
own sources (selfhost/lexer, parser, check, ssa: 2,922 functions) every
function is identical (78% at stage 1, maps were most of the rest).

Found in stage 4: a sentinel of the port (gotchas/selfhost-ssa-negative-
locals.md: a loop bound's counter is a made up variable with a negative
id, so `-1` for "no bound" hid every counter). No compiler bug.

Speed after stage 4, ms for 10 builds (the builder alone, `--bench=10`;
the Xo builder now declares every function the Go builder does on these
inputs but the notes service):

| input | Go builder | Xo, Go backend | Xo, arm64 | Xo, LLVM |
|---|---|---|---|---|
| selfhost/check (1,018 functions) | 716 | 1,248 (1.74x) | 9,434 (13.2x) | 1,117 (1.56x) |
| selfhost/parser (377) | 223 | 418 (1.87x) | 2,906 (13.0x) | 341 (1.53x) |
| selfhost/ssa (1,435) | 1,109 | 1,974 (1.78x) | 15,646 (14.1x) | 1,819 (1.64x) |
| bench/xo/v0.4/08-notes-service (192; Xo 174) | 25 | 45 (1.8x) | 318 (12.7x) | 31 (1.24x) |

The ratios grew from stage 1 (go 1.67x, LLVM 1.44x on selfhost/check)
because the Xo builder now builds the functions it skipped (maps,
interfaces), its slower paths (generated helpers built in the middle of
a body, each a save and restore of the function state). The arm64 build
stays about 13 times the Go builder: every call of a Builder method
copies its `var` receiver (the builder, which holds the checker) in and
out through the stack. It does not block the tests (the arm64 subtest
takes 33 s of the 1,200 s cold run, most of which is the LLVM build's
link), so decision 0148 was not ported to arm64 here.

## Next experiments (after stage 4)

1. **arm64: large `var` parameters kept in memory** (decision 0148's
   LLVM rule ported to internal/arm64 abi.go and emit.go): pass the
   address of a `var` struct parameter of 256 bytes or more and update
   it in place. Expected: the arm64 build near the LLVM build (13x to
   about 1.6x), and `Builder.std_call` far under the 280,000
   instructions that reached the 1 MiB branch limit. Measure with the
   cached binary (`~/Library/Caches/xo/build/*/arm64/bin/ssa
   --bench=10 selfhost/check`).
2. **Stage 5, concurrency and I/O** (build_conc.go, build_tasks.go,
   build_io.go waits): 190 functions skipped for concurrency and 36 for
   scopes; scopes also join the open blocks of defer.xo (the scope half
   of exitEntry: closeEarly at exits, the scopes' unwind blocks).
3. **Profile the go build of the Xo builder on selfhost/ssa** (`go tool
   pprof` on the generated Go program): the immediate builds of
   generated functions (begin and end copying the state S) are the
   suspected new cost.

# arm64: large struct parameters by reference, 2026-10-11

Decision 0151 (internal/arm64 byref.go): a struct parameter of 256
bytes or more of a direct call is passed as its address; a `var` one is
updated in that memory with decision 0148's dataflow (fields read and
written in place, no whole loads of a value held at every use), a non
`var` one is read in place and passed on by address. Before, the
`Builder` (about 300 words) went through the stack in both directions
at every call.

Builder alone, `--bench=10`, ms for 10 builds (Apple M4 Pro; the
internal/selfhost tests ran at the same time for the last three rows):

| input | Go builder | Xo, arm64 before | Xo, arm64 after | Xo, LLVM (stage 4) |
|---|---|---|---|---|
| selfhost/check | 710 | 9,434 (13.2x) | 1,630 (2.3x) | 1,117 (1.56x) |
| selfhost/parser | 223 | 2,906 (13.0x) | 543 (2.4x) | 341 (1.53x) |
| selfhost/ssa | 1,126 | 15,646 (14.1x) | 2,629 (2.3x) | 1,819 (1.64x) |
| bench/xo/v0.4/08-notes-service | 25 | 318 (12.7x) | 52 (2.1x) | 31 (1.24x) |

The output of the arm64 build is byte identical before and after on
selfhost/check and selfhost/ssa (256,436 lines), the binary is 23 MB
(was 54 MB). Making only `var` parameters references left 1,000 whole
loads of held values in selfhost/ssa, almost all to call by value
receivers (`Checker.show`, `Parser.span_from`); read only references
removed them (1,787 to 1,630 ms on selfhost/check). A `sample` profile
of the arm64 build is now flat (the map lookup `xo.map.find[Map[Int,
Int]]` first at 7%, then allocation).

Found: register allocation sorted a register's whole occupancy list at
every assignment (`mergeRanges`); the by reference convention gives the
builder's functions more, shorter live ranges and code generation of
selfhost/ssa went from 55 to 100 s. A linear merge into the list's
storage (`mergeSorted`) brings it to about 60 s. A first golden struct
of 248 bytes never reached the convention
(gotchas/byref-threshold-in-tests.md). No miscompilation was found.

Next experiments for arm64: enums of 256 bytes or more by reference
(decision 0147's rule: `Expr`, `Stmt`, `Pattern` of selfhost/parser
are that large), and a field of a field of a held value read without
loading the inner field whole (`h.a.a.a += n` loads all of `h.a`).

# Stage 5: concurrency and I/O, 2026-10-11

`selfhost/ssa/conc.xo` (about 1,100 lines): scopes (open, the close at
every way out: the end, `return`, `?`, `break`, `continue` in mode 1,
unwinding in mode 2, the failure block that returns a failed task's
error with its frame), spawn sites in check.Inspect order and the
bindings that make a spawn awaited, `spawn` with its entry function
(`xo.run[...]`) and drop lists, `await`, every method of Chan, Mutex
(`with`/`with_read`, packed closures with a `var` parameter, the unlock
on the unwind path), Atomic (and its Debug text), Scope, task.Group,
std/task (`isolate`, `with_timeout`, `is_canceled`, `stop_requested`,
`stopping`), `for` over a channel, `select` (receive, send, `after`),
`os.tasks` and main in the root scope (`main.run`, the group's failure
printed before main's own error), and the operations that wait:
`Clock.sleep`, `sleep_or_cancel`, Net, Listener, Conn (with
`xo.neterr`), and standard input reads. Scopes join the open blocks of
defer.xo (the scope half of exitEntry). The Xo builder now builds every
run the Go builder builds except `use go`/`use c` and holes.
internal/selfhost/testdata/ssa/stage5 is a program of these constructs.

| functions (Go builder) | stage 4 (11,171) | stage 5 (11,264, one more run) |
|---|---|---|
| identical | 8,252 | 10,356 |
| skipped (a later stage) | 690 | 405 |
| never declared (callees of skipped code) | 2,229 | 503 |

0 differences with the go, arm64 (built with decision 0151), and LLVM
builds of the Xo builder; `TestSelfhostSSAMutations` 0 differences on
400 mutations (seed 20261010: 5,691 of 6,266 functions identical, was
3,877 of 5,997) and 1,500 (seed 7: 21,224 of 23,454, was 15,677 of
23,056). No reason for skipping of stages 1 to 5 is left: what remains
is the generic intrinsics of stage 6 (`json_enc` 105, `any_of` 44,
`cast` 39, `json_dec` 37, `status_of` 35, `launder` 42, SQL 9) and
the closures of the functions they are in (75).

Found in stage 5: the port armed the landings of operations that may
fault only after a `defer` (`any_defer`, a shortcut of stage 4); the Go
builder arms them under any lexical cleanup, scopes included (78
differences, all `lands` on `rt.task_spawn` and the blocks after it;
gotchas/selfhost-ssa-port-shortcut-flags.md). No compiler bug.

Speed, ms for 10 builds (`--bench=10`, the builder alone; Apple M4
Pro, idle): the go build of the Xo builder against the Go builder,
before the fix of the next section.

| input | Go builder | Xo, Go backend |
|---|---|---|
| selfhost/check | 710 | 1,244 (1.75x) |
| selfhost/ssa | 1,160 | 2,080 (1.80x) |
| internal/gen/testdata/golden/http_scoped (1,140 functions; Xo 1,080) | 16 | 32 (2.0x) |
