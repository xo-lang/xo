# 0332: lazy `const` (a value computed once per process on first use)

Status: Accepted in direction (owner, 2026-10-21, after the language poll).
Design below: Proposed (Xo Publish, 2026-10-21), for the owner to accept
before implementation.

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
