# LLVM backend prototype (P3): `xo build --backend=llvm` (2026-10-08)

Question (decision 0040, "own backend (P3, for example LLVM): full
optimizer"): can an LLVM backend run Xo's compute code at Go's speed or
better while keeping Xo's semantics (checked Int arithmetic that faults
with values and position, value semantics, faults with exit status 70)?

Answer: on the six compute workloads below, yes: the prototype runs at
0.47x to 1.13x of hand written Go with the same algorithm, with every
overflow check kept. It ties Go on recursion, the sieve, and Collatz
(within 3 percent), is 13 percent slower on nbody, 13 percent faster on
struct sorting, and 2.1x faster on the enum interpreter (but a tuned Go
interpreter is 2.2x faster than the Xo one; see below). Against Xo's own
Go backend it ties on fib (0.97x) and is 1.2x (collatz), 8.7x (interp),
and 102x (nbody) faster; on sieve and sort the Go backend does not
finish within 60 s because its list element writes copy the whole list. Memory is the price: the
prototype never frees, so list heavy runs use 1.8x to 2.9x Go's peak RSS.

Host: darwin/arm64 (Apple M4 Pro), Go 1.26.1, Apple clang 21.0.0
(`clang -O2 -flto`).

## What was built

| Part | Path | Size |
|---|---|---|
| IR emitter | `internal/llvm/` (llvm.go, types.go, fn.go, expr.go, call.go, pattern.go) | about 4,000 lines of Go |
| C runtime | `std/llrt/xo_rt.c` (embedded as `std.LLRuntime`) | 470 lines |
| Build | `internal/build/llvm.go`; `--backend=llvm` on `xo build` and `xo run` | |
| Range analysis export | `internal/gen/rangesapi.go` (`gen.SafeIntOps`) | |
| Tests | `internal/llvm/llvm_test.go`, 27 golden and 8 unsupported programs; `cmd/xo/llvm_test.go` | |
| Benchmarks | `bench/perf/llvm/` (`run.sh`, `xo/<workload>/`, `go/`) | |

The compiler stays Go standard library only; clang is invoked as an
external tool, like `go build` for the Go backend.

### Supported subset

- Int (64 bit, checked `+ - * / %`, unary minus, `<< >>` with range and
  overflow faults, `& | ^ ~`), Bool, Float (arithmetic, `%`, comparisons,
  `sqrt floor ceil trunc round abs is_nan`), `Float(i)`, `Int(f)` (faults
  on NaN and out of range), `min`, `max`, `abs`.
- Str literals (escapes, raw, interpolation of Int, Float, Bool, Str,
  nested), `+`, `==`, ordering, `len`, `is_empty`.
- `os.stdio.print`, `println`, `eprint`, `eprintln`; `fn main(os: Os)`
  or `fn main()`.
- Structs (defaults, `with`, field assignment through nested places,
  methods, `var` receivers), tuples (any arity), enums with payloads,
  recursive enums and structs (boxed fields), Option, Result, `==` on all
  of them.
- `match` with `_`, bindings, literals (Int, negative Int, Float, Bool,
  Str), ranges, `|` (bindings across alternatives), tuple, variant
  (positional, named, `..`), struct patterns, nested, guards; `if let`
  (and the `if let x = opt` shorthand); `is`.
- `if`/`else` as expressions, `while`, `while { }` with `break`, labeled
  `break` and `continue`, `for` over `a..b`, `a..=b`, a List, and
  `xs.indexed()`; blocks as expressions; `return`.
- Functions, recursion (1 GiB stack, like Go's goroutine maximum),
  generic functions and generic types (monomorphized per type
  arguments), named and default arguments, `var` parameters.
- Closures (capture by value, nested, returned, stored in lists),
  declared functions as values, failing closures.
- `List[T]` of any supported T: literals, `push`, `pop`, `get`, `set`,
  `first`, `last`, `len`, `is_empty`, `xs[i]`, `xs[i] = v`,
  `xs[i].f = v`, `xs[i][j] = v`, `==`, iteration; value semantics.
- `?` on Result (errors converted through `from` variants) and on
  Option, `??`, `Err(e)` and `Ok(x)` returns, `ok()`, `err()`,
  `is_ok`, `is_some`, ..., `fault(msg)`, `or_fault` on Option, `const`
  (global and local).

Everything else is rejected at build time with an error that names the
construct and its position, for example
`main.xo:2:11: the LLVM backend does not support Map` or
`main.xo:7:28: the LLVM backend does not support interpolating a value of
type `P` (only Int, Float, Bool, Str)`. Only functions reachable from
`main` are compiled, so unsupported code elsewhere in a module does not
block a build.

## Design

**Front end reuse.** The emitter walks the same typed AST internal/gen
uses, through the same `check.Info` accessors (`TypeOf`, `NaturalType`,
`VarOf`, `Ref`, `Ctor`, `CallOf`, `PatVariant`, `PatType`, `ParamVars`,
`RecvVar`, `ClosureParams`, `FromVariant`). Implicit conversions (Some
wrapping, `Ok` wrapping of a failing function's tail, `from` conversion
at `?`) are read off the difference between the natural type and the
type in context, as in gen. Functions are instantiated on demand from
`main` (a worklist keyed by function and concrete type arguments), so
generics are monomorphized for free.

**Values.** Int i64, Bool i1, Float double, Str `{ptr, i64}`, `()`,
Never, and capabilities `{}`, a struct a named LLVM struct, a tuple a
literal struct, an enum `{i64 tag, [N x i64] payload}` whose payload is
written and read through memory as the variant's own struct (zeroed
first, so no undefined bytes are ever loaded), a List a pointer to a
header `{len, cap, shared}` followed by the elements, a function value
`{code, env}`. A field whose type contains its own type by value is
boxed: a pointer to an immutable heap copy, copied on write. Every
binding is an alloca; mem2reg and SROA turn them into registers. A
function that can fail returns its Result enum by value.

**Checked arithmetic.** `+ - *` use `llvm.s{add,sub,mul}.with.overflow`;
`/` and `%` test for zero and `MIN / -1`, and `x % -1` is 0 (LLVM's
`srem` of the minimum by -1 is undefined, Xo's is 0). The fault branch is
marked unlikely and calls a `noreturn cold` runtime function with both
operands and a `file:line:col` string, producing exactly the Go
backend's message. Operations that internal/gen's range analysis proves
safe (exported as `gen.SafeIntOps`, the same analysis, run per function
instance) become plain `add nsw` and so on. Measured effect under LLVM:
none within the 10 ms timer resolution on these workloads (without the
analysis: fib 0.22 s, collatz 0.47 s, sieve 0.11 s, sort 0.23 s; with
it, in the table below: 0.23, 0.49, 0.11, 0.23); LLVM folds most
provable checks itself and the remaining ones cost one well predicted
branch.

**Memory.** A bump allocator over 4 MiB chunks (larger blocks from
malloc), never freed. Lists keep value semantics without reference
counts through a sticky `shared` flag: a value copied out of a binding,
field, element, or pattern into another binding, field, list, closure,
or return ("own") marks the list buffers it holds as shared (generated
`retain:<type>` functions walk structs, tuples, and enums; boxes are
immutable and need no marking). A write through a binding whose buffer
is shared copies the buffer first and stores the copy back. Arguments to
non `var` parameters are borrows and mark nothing, because the callee
cannot mutate them and marks whatever it keeps; the exception is a call
that also has a `var` parameter or receiver, whose by value arguments
are owned (found in review: `append_all(xs, xs)` with one `var`
parameter let the borrowed copy see the pushes, now golden test
`var_alias`). Since nothing is ever unmarked, the first write after every
copy clones, even when the other copy is dead.

**List write cost.** The first version called the runtime for every
element write and push and was 2x to 5x slower than Go on list loops.
Three changes brought it to parity (`gotchas/llvm-list-write-cost.md`):
the uniqueness and capacity checks are inline IR and only the copy is a
cold call that returns the new list (so bindings stay in registers);
list header and element accesses carry distinct TBAA tags so header loads
move past element stores; and a per block cache remembers a binding that
was just made unshared, so the later writes of one straight line region
skip the check.

**Faults and output.** The runtime prints the same fault messages as
std/rt (`integer overflow: 9223372036854775807 + 1 does not fit Int at
main.xo:4:5`, `index 7 out of range for List of length 5 at ...`,
`division by zero: ...`, `1.5e+24 is out of range for Int at ...`,
user `fault(msg)`, `or_fault`, `abs(...)`) and exits with status 70
after flushing the buffered stdout. Floats print as Go's
`strconv.FormatFloat(f, 'g', -1, 64)` plus `.0` for whole numbers: the
runtime finds the shortest round tripping digits with `%.*e` and
`strtod` and lays them out with Go's exponent rule. There are no Go
frame lines and no crash bundle; the source line is the `file:line:col`
string compiled into the binary (no DWARF).

**Build.** `prog.ll` and `xo_rt.c` go to `<build dir>/llvm/` and are
compiled with `clang -O2 -flto` so the runtime's small helpers inline.
The default macOS SDK did not link on this host
(`gotchas/macos-clt-sdk-link-failure.md`); the build tries SDKs and
remembers the one that links. A clean build of nbody takes about 0.11 s
(the Go backend about 0.1 to 0.2 s with a warm Go build cache); binaries
are 34 KB against 6.6 MB for the Go backend.

## Correctness

`go test ./internal/llvm` builds each of 27 programs with the Go backend
and with the LLVM backend, the latter twice (with and without the range
analysis), runs them, and requires equal exit status, stdout, and fault
messages (stderr with Go frame lines and crash bundle paths dropped, as
internal/gen's golden test does), all also matching `want.txt`. Status:
all 27 pass. Ten of them fault: Int add overflow, mul overflow in a
loop, shift overflow, negation of the minimum, `abs` of the minimum,
division by zero, index out of range, Float to Int out of range, a user
`fault` with interpolated values, and `or_fault` on None. The others
cover arithmetic and float formatting, structs, enums and every pattern
form, Option, Result with `from` conversion, lists and value semantics
(snapshots, nested lists, lists in structs, enums, closures, and boxes),
closures, generics, recursion (a recursive expression enum, depth one
million), loops and labels, strings, tuples and constants, failing
closures with `?`, sorting through `var` parameters, an interpreter, and
the `var` aliasing case. Eight more programs check the unsupported
construct errors (Map, interfaces, `defer`, `main` with `!`, struct
interpolation, list patterns, std clock, `scope`). The tests skip when
clang is missing.

Writing them found two Go backend bugs, recorded as gotchas: a `-0.0`
literal prints `0.0` (Go constants have no negative zero,
`gotchas/go-negative-zero-literal.md`), and tuples of more than six
elements fail the Go build (`gotchas/go-backend-tuple-arity.md`). The
Go backend's float arithmetic was not fused into FMA in the case tried
(`a * b + c`), so the two backends agree bit for bit there; a Go
compiler that fused would make them differ in the last bits.

## Benchmarks

`bench/perf/llvm/run.sh`: best of 5 wall clock runs per binary, process
start included, 60 s timeout; checksums agree. "Go" is hand written Go
with the same algorithm and sizes (plain unchecked int arithmetic, the
same quicksort, the same interpreter structure).

| Workload | What it stresses | Go ms | Xo Go backend ms | Xo LLVM ms | LLVM/Go | Go backend/LLVM |
|---|---|---|---|---|---|---|
| fib | `fib(40)`: calls, checked add | 224.9 | 223.6 | 231.4 | 1.03x | 0.97x |
| sieve | 5 sieves of 10M over `List[Bool]`: push, element writes | 110.5 | >60000 | 112.8 | 1.02x | >532x |
| nbody | 5 bodies, 5M steps: Float math on `List[Body]` fields in place | 161.5 | 18759.5 | 183.0 | 1.13x | 102.5x |
| sort | 3 x quicksort of 1M structs through a `var` parameter | 266.5 | >60000 | 231.9 | 0.87x | >259x |
| interp | stack machine over an enum `List[Op]`, 39M steps | 144.0 | 582.9 | 67.3 | 0.47x | 8.66x |
| collatz | step counts for 1..3M: checked `3n+1` the analysis cannot prove | 491.2 | 600.9 | 490.0 | 1.00x | 1.23x |

A second full run an hour earlier gave the same picture (fib 0.98x,
sieve 1.04x, nbody 1.13x, sort 0.87x, interp 0.48x, collatz 1.02x).

Notes:

- **interp**: the same algorithm in Go (a slice stack, pops that default
  on empty) takes 144 ms. A Go programmer tuning it (`llperf
  interp_tuned`: a fixed array stack with an index) gets 30 ms, 2.2x
  faster than the Xo program. The Xo program's `stack.pop() ?? 0`
  through Option is what the LLVM backend runs fast, not a better
  algorithm.
- **sort** is 13 percent faster than Go. Both compilers inline `less`
  and bounds check the accesses (Go's `-d=ssa/check_bce` lists them); the
  source of the difference was not investigated (next step: compare the
  two inner loops' assembly).
- **nbody** is 13 percent slower: each field write still checks the
  shared flag once per loop iteration and the bounds of `bs[i]` (the
  list is a `var` parameter, which the range analysis does not track).
- **Go backend**: `xs[i] = x` copies the whole list
  (`gotchas/go-backend-list-set-quadratic.md`), so sieve and sort do not
  finish in 60 s (sieve ran for over 10 minutes before it was stopped)
  and nbody (five element list) takes 18.8 s. fib and collatz show the Go
  backend's checked arithmetic is already cheap (0.99x and 1.22x of Go).
- **Range analysis** under LLVM: no measurable difference (see Design).

Peak RSS (`/usr/bin/time -l`, MB):

| Workload | Go | Xo LLVM | Xo Go backend |
|---|---|---|---|
| fib | 4.3 | 1.8 | 9.9 |
| sieve | 46.8 | 136.3 | (did not finish) |
| nbody | 4.2 | 1.8 | |
| sort | 85.3 | 149.7 | (did not finish) |
| interp | 4.4 | 1.8 | 17.6 |
| collatz | 4.3 | 1.8 | 9.9 |

The LLVM binaries start smaller (no Go runtime) but never free: every
list growth abandons the old buffer, so the sieve holds all five 10M
lists plus their growth copies (2.9x Go) and sort holds three
generations of 1M records (1.8x).

### Caveats

- The workloads were chosen inside the subset: no strings in hot loops,
  no maps, no allocation churn beyond lists, no concurrency. Allocation
  heavy request handling, where decision 0040 also hopes to beat Go,
  is not measured; with a never freeing allocator it cannot be yet.
- Timing is whole process wall clock with 10 ms resolution in the
  spot checks; laptop drift is up to 10 percent
  (`gotchas/bench-host-timing-drift.md`). The two full runs agree within
  4 percent per workload.
- One host, one compiler version, arm64 only.

## What a full backend needs

1. **Memory reclamation.** Reference counting with reuse of unique values
   (Perceus, as in Koka and Lean 4) fits Xo best: values never alias
   mutably, so counts replace the sticky shared flag (a write in place
   whenever the count is 1, including after the other copy dies), and
   frees are deterministic. Needs: increments at the "own" points this
   prototype already finds, decrements at last use (a liveness pass over
   the typed AST), drop functions per type (the `retain:` helpers
   generalized), and borrow inference for parameters (this prototype's
   borrow rule is the start). Cycles cannot form without mutable
   references, except through `Mutex`/`Atomic` handles. The alternative,
   a tracing GC, needs stack maps (`gc.statepoint` or a shadow stack)
   and would give up the deterministic frees arenas want. Request arenas
   (decision 0040) become an allocator choice per scope.
2. **The rest of the language**: interfaces (vtables or monomorphized
   dispatch), Map and Set (port the insertion ordered entry log of
   std/rt), Bytes, Rune, sized integers and Float32, Duration and Time,
   derived Display/Debug/Eq/Hash/Ord/Json, `defer` with cancellation
   shielding, contracts, error traces (frames recorded at `?`), `xo test`
   with property tests and fuzzing, holes.
3. **Concurrency.** Structured tasks, channels, `select`, cancellation,
   the deterministic scheduler, and live inspection need a runtime of
   their own: green threads (LLVM coroutines or a stack switching
   scheduler) plus an I/O poller, or a thread pool with the
   deterministic mode as a single threaded scheduler. This is the largest
   item and where Go's runtime is hardest to match.
4. **The standard library**: http, json, fs, net, log, time, path. Today
   std/rt is Go; a native backend needs these in C, Rust, or Xo itself,
   and `use go` (decision 0041) cannot work without a Go runtime in the
   process.
5. **Diagnostics and debugging**: DWARF line tables (`!DILocation`) for
   debuggers and profilers, stack traces with Xo names for faults, crash
   bundles, SIGQUIT task dumps.
6. **Targets**: cross compilation needs target triples, a sysroot per
   target, and a linker (`ld.lld`); static binaries and wasm need their
   own runtime ports. The Go backend gets all of this from `go build`.
7. **Build speed at scale**: one LTO module per program is fine at this
   size (0.1 s); larger programs need per module object files, caching,
   and probably `-O1` for development builds.

## Next steps (ranked by likelihood of paying off)

1. **Reference counting for lists** (closes the memory gap, makes value
   semantics in place after copies die): add a count to the list header,
   emit `xo_list_dec` at the last use of an owned list binding
   (liveness over `fn.vars`), and turn `retain` into an increment. First
   experiment: the sieve's peak RSS (136 MB) should fall to about Go's
   47 MB; run `REPS=5 WORKLOADS="sieve sort" bench/perf/llvm/run.sh`
   and `/usr/bin/time -l` on the binaries.
2. **Port the Go backend's list fix**: the LLVM backend's sticky flag
   shows unique ownership works; the same idea in `std/rt/list.go`
   (`SetAt` writes in place when the slice is not shared) would make the
   Go backend finish sieve, sort, and nbody. Experiment: add a `shared`
   bit to `rt.List`, set it on copies in gen's `val` of a place, run the
   same harness.
3. **Close nbody's 13 percent**: let the range analysis track the length
   of a `var` list parameter that is never pushed or popped in the
   function (so `bs[i]` with `i in 0..bs.len()` is proven), and hoist
   the shared flag check out of loops whose body neither retains nor
   calls (loop unswitching by hand in the emitter). Measure with
   `WORKLOADS=nbody`.
4. **Strings and Map**: add Map (the entry log) and Str methods, then a
   word count and a JSON like tokenizer workload, where allocation and
   hashing dominate and reference counting will matter.
5. **Interfaces**: vtable lowering, then port the `iface` workload of
   `bench/perf` (where the Go backend reaches 0.80x of Go after
   monomorphization and check elimination) to see whether LLVM
   devirtualizes as well.
