# 0332: lazy `const` (a value computed once per process on first use)

Status: Accepted (owner, 2026-10-21; in direction after the language poll,
the design below the same evening). Design: Xo Publish.

## Context

Std re-derives fixed data on every call: parsed time layouts, compiled
regexp patterns, lookup tables built from text. Xo has no global
variables, so the ports either recompute per call or rewrite the parse as
byte-level code (the time format work of 2026-10-21). Two of six agents
named it in the 2026-10-21 language poll.

Cost (bench/results/time-layout-parse-cost-2026-10-21.md, table at lines
23 to 27, measured): std/time `format` over 600,000 calls takes 6,166 M
instructions, of which parsing the layout string is 3,830 M (62%). The
same file (lines 29 to 35) estimates, without measuring, about 2,500 M
with a Mutex-held Map cache and about 2,400 M with a value parsed once
(Go's `time.Format`: 953 M).

What exists today (read from the code, 2026-10-21):

- `const` initializers in user modules must be compile time values:
  literals, operators, other constants, collection and struct literals
  (`check_const_expr`, compiler/check/stmt.xo; anything else is `XO0408`).
  Std modules skip that check, so a std `const` may already call
  functions.
- A constant of a heap type other than `Str` is already lazy (decision
  0081): an entry function `xo.const[M.N]` builds it, and each use calls
  `rt.const_once(f, drop, id)` (std/native/llrt/consts.xo; wasm rt4.xo),
  which publishes the value with a compare and swap and keeps it immortal
  until exit. Two threads can both build it (the loser drops its copy),
  and a fault in the builder unwinds into the reader and leaves the slot
  empty, so the next use builds again.
- A constant of a non-heap type (`Int` folding aside) is re-lowered at
  every use (`const_value` in compiler/ssa/show.xo, `foreign_value`): a
  std constant that calls a function runs the call at every read.
- The Go backend emits `var C_X = func() T {...}()`, evaluated eagerly at
  package initialization (compiler/gen/decl.xo `gen_const`).
- `task.Once` (decision 0043, std/native/task.xo) is single flight with a
  shared result and re-raises a stored fault in every caller; it needs a
  value to live in, which a module level cache does not have.

## Decision

### 1. Syntax: plain `const`, no new keyword

```
const RFC3339 = time.layout("{year}-{month}-{day}T{hour}:{minute}:{second}{offset}")
const WORD = regexp.must("[a-z]+")
const CRC_TABLE = make_crc_table(0xedb88320)
```

A `const` initializer may call functions. Its value is computed at most
once per process, on first use, and shared by every task. A constant
whose initializer the compiler can evaluate (today's `XO0408`-clean
forms) keeps its current lowering; the rule is the same for both, so a
reader does not have to know which kind a constant is.

No `lazy` marker: the initializer is effect free (section 3), so when it
runs is not observable except through its cost and a fault, and heap
constants are already computed on first use (0081).

### 2. Exactly once, safe under concurrency and preemption

Each constant's slot has four states: empty, running, a value, a fault.

- The fast path is today's: one acquire load of the slot; a value is
  returned as is (immortal, shared, decision 0058).
- On empty, the reader CASes the slot to running, records its task as the
  runner, and calls the entry function. On success it stores the value
  with release order and wakes every waiter.
- A reader that finds running parks on the runtime's wait list
  (`tasks.wl_wait_as`, as `Mutex.lock` does) until the slot leaves
  running. It never spins, so a preempted runner (decision 0318: the
  initializer is ordinary preemptible Xo code) costs the waiters nothing.
  The CAS and the publish need no no-preemption region (0318 section 4).
- If the runner's task is canceled (a sibling faulted) before it
  finishes, the slot goes back to empty and one waiter is woken to run it
  (as std/rt once.go hands the run on). Cancellation is not a failure of
  the initializer.

### 3. Effects: none

The initializer's inferred effect set must be empty: no `fs`, `net`,
`clock`, `rand`, `env`, `proc`, `stdio`, `ffi` (decisions 0004, 0033). A
call that performs one is a new error `XO0415` ("a const initializer
cannot perform effects; it runs once, at first use"). No capability is
reachable at module level anyway; the rule also keeps `clock` and `rand`
out, so a constant's value depends only on the program text.

The constant's type must not hold shared mutable state: no `Mutex`,
channel, task handle, or other handle type, at any depth (new error
`XO0416`). A constant is an immutable value, not a global variable.

### 4. A fault in the initializer: stored and raised again

A fault in the initializer (an index out of range, `fault(...)`, an
overflow) is stored in the slot, and every read of the constant, the
first and every later one, faults with the original message and location
plus a note naming the constant (`while initializing const time.RFC3339
at layout.xo:12`). This is `task.Once`'s rule (0043) and Rust's
`LazyLock` poisoning; it is deterministic and does not pay a failing
initializer's cost at every read. A fault is not retried.

### 5. Cycles

A constant that reads itself through function calls (`const A = f()`,
`fn f() -> Int { A + 1 }`) would wait on itself. The checker reports the
cycles it sees through direct references (`XO0407`, as today); the
runtime catches the rest: a read of a constant in the running state by
its own runner, or by a task started inside the initializer, faults with
new `XO-F009` ("constant used during its own initialization", both
sites) instead of deadlocking. The runner records its task; a task
started inside an initializer inherits the set of constants being
initialized.

### 6. Backends

- **Native (arm64, amd64) and LLVM**: `llrt/consts.xo`'s `once` gains the
  states above, the fault record, and the wait list; the per-use code is
  unchanged (`rt.const_once` then `list.index`). A non-heap constant whose
  initializer calls a function goes through the same one-element list
  path instead of `foreign_value`, so it is computed once too. The entry
  function runs under the fault unwinding frame of decision 0085 so the
  fault can be stored.
- **wasm**: single threaded; rt4.xo's `const_once` gets the fault record
  and the cycle check (no waiting is possible).
- **Go**: a constant whose initializer calls a function becomes lazy,
  `var C_X = rt.LazyConst[T]{...}` read through `C_X.Get(t, at)`, built
  on std/rt's once.go (fault stored and raised again, hand on under
  cancellation). Eager package initialization would fault at start for a
  constant that is never read, and would order initializers by package.
  Compile time constants stay plain `var`s.
- The 4096 constant limit (0081) stays and becomes a checker error
  instead of an `unsupported` from SSA.

### 7. Std

Std constants follow the same rules: they keep calling functions, and an
std initializer with an effect is `XO0415` too. std/time pre-parses its
named layouts; a layout given at run time keeps parsing per call (a cache
keyed by layout text is a separate decision).

## Consequences

- Std and programs compute fixed data once: std/time's named layouts skip
  the 62% layout parse (the measured share; the end figure is measured
  after implementation, by Lib1).
- The first read of a constant can take arbitrarily long and can fault;
  `xo vet --cost` can list constants whose initializer calls a function.
- Reads after the first cost one acquire load and an index, as heap
  constants do today.
- New codes: `XO0415`, `XO0416` (checker), `XO-F009` (fault); spec core
  3.1 (constants) and section 10.
- Implementation in stages, each with goldens on go, arm64, LLVM, and
  wasm: (1) checker rules and the non-heap once path; (2) the exactly
  once states, waiting, and fault storing in llrt and wasm; (3) the Go
  backend's LazyConst; (4) std/time named layouts. Tests: 64 tasks
  reading a constant first at once, all seeing one value, with a runtime
  debug counter (`XO_DEBUG=consts`, stderr) showing one build; a
  faulting initializer read twice (the same fault both times); a cycle
  through a function (`XO-F009`); errorcheck for `XO0415` and `XO0416`.

## Implementation notes (2026-10-21)

- Checker (compiler/check stmt.xo `const_type`): a top-level initializer
  is no longer limited to compile time forms; its effect set becomes an
  obligation of kind `Const` (`XO0415`, effects.xo), and `const_handle`
  finds a handle at any depth (`XO0416`). A block `const` keeps
  `check_const_expr` (`XO0408`). `is_compile_time` (info.xo) tells the
  backends which constants run code: a call that is not a constructor,
  or a reference to such a constant.
- SSA (show.xo `const_value`): such a constant goes through the 0081
  path whatever its type (a non-heap one is no longer re-lowered at each
  read). Its builder is `xo.const_body[M.N]`; the entry function
  `xo.const[M.N]` (`const_entry`) calls it under a landing and, on a
  fault (`rt.fault_take`) or a cancellation, returns the immortal empty
  list. `rt.const_once` joined `faulting_rt`.
- Native runtime (std/native/llrt/consts.xo): slot states empty, running,
  faulted, or the list; one reader builds under the table's lock and the
  others park on its wait list (`tasks.wl_wait_as`, cancelable); a
  canceled builder empties the slot and wakes the waiters, one of which
  builds; a fault's message and location move out of the context
  (`fault.move_out`) into the table and are raised again at every read.
  `rt.const_once`'s signature is unchanged, so code of an older xo (whose
  entry functions cannot fail) runs on it. A reader about to wait follows
  the chain of runners and the constants they wait for (the parked
  readers are recorded in the table); reaching itself is `XO-F009`, so
  two tasks whose constants read each other fault instead of hanging.
  The build is a release and a read an acquire for `--race`
  (`sanhook`, as tasks.xo does; the runtime is not instrumented).
  Reviewed by Xo Lang 2 (2026-10-21): the table's size, the `--race`
  annotations, and the cross-task cycle came from that review.
- wasm (rt4.xo `$const_once`): the same states (single threaded, no
  waiting); `rt.const_once` is followed by a fault check (call.xo), as
  wasm faults are a flag the caller tests.
- Go (gen decl.xo, std/rt once.go `LazyConst`): the constant's builder is
  set in an `init` function (as a variable initializer, a builder that
  reads the constant through a function is a Go initialization cycle)
  and read through `Get(tk)`: an atomic flag, then `Task_Once`'s rules,
  with the same wait-chain check (a package-level record of waiting
  tasks).
- Not yet: `XO-F009` reports the location `?` (rt.const_once takes no
  location); a task started inside an initializer that reads the
  constant being built waits forever if the initializer waits for that
  task (the wait chain follows constants, not scope joins); wasm, single
  threaded, faults at once on a read of a constant another task is
  building (possible only if the initializer yields); std/time's named
  layouts (Lib1); `xo vet --cost` listing.
- Tests: errorcheck lazy_const (`XO0415`, `XO0416`, `XO0408` for a block
  const), goldens lazy_const (64 tasks read a constant built once, `dbg`
  in the initializer shows each build; a faulting constant read twice)
  lazy_const_cycle (`XO-F009`), and lazy_const_cycle_tasks (two tasks,
  two constants that read each other) on go, arm64, LLVM, and wasm;
  race_sync's lazy_const case (TestRaceSyncTableArm64: no race reported).

## Addendum (2026-10-22): the two estimates, measured

bench/results/time-format-cache-variants-2026-10-22.md measures what the
Context section estimated (program bench/perf/timeformat-cache, `xo bench
--ab`): 600,000 formats take 5,344 M instructions with the layout text each
call, 1,995 M with a Mutex held `Map[Str, Layout]` (0.373x instructions,
0.263x wall) and 1,710 M with a lazy `const` Layout (a further 0.856x
instructions, 0.822x wall); Go's time.Format 983 M. Both estimates were
high (2,500 M and 2,400 M). Choice: std callers with a fixed layout use a
lazy const `time.Layout`; the Mutex Map cache stays the pattern only for
layout text known at run time. The HTTP date (native/http.xo
HTTP_DATE_LAYOUT) is the first std caller moved: Set-Cookie with an Expires
3,079 M to 1,320 M instructions, wall 0.427x.

## Alternatives

- **Compile time evaluation of any effect-free initializer**: no runtime
  cost at all, but the compiler needs an interpreter for all of Xo (or to
  run a built program at build time), and build time grows with every
  table. The initializers are pure under this decision, so it stays
  possible later as an optimization with the same semantics.
- **A `lazy const` keyword**: marks the runtime evaluation; rejected
  because heap constants are already lazy and the effect rule makes the
  timing unobservable.
- **An explicit `Once[T]` std type**: needs a place to live, which only a
  global gives; threading it through parameters changes every std API
  that would use it.
- **Keep today's racy build**: harmless for literal constants (building
  twice costs only time), but an expensive initializer (a compiled
  pattern) would run once per contending thread, and faults would be
  retried at every read.
- **Retry a faulting initializer** (Rust's `OnceLock::get_or_init`
  leaves itself empty after a panic; Go's `sync.Once` marks itself done
  and leaves the zero value): a retry hides a deterministic bug and pays
  the failing cost again; the stored fault is the same answer every time.
