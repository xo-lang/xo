# 0337: Function values borrow their arguments; no borrow syntax

Status: Accepted (owner, 2026-10-22; proposed by Xo Publish the same
day, the language poll's second theme, "borrowed iteration").
Implemented 2026-10-22 (see "Implementation" at the end). Refines decision 0046 (reference counting) and 0149
(borrowed parameters consumed on some paths) for functions called
through a function value; leaves 0186 (loop elements borrow from their
container) and the 0291 addendum (a Map or Set loop walks its table in
place) as they are.

## Context

Agents asked for read only loops and callbacks that do not count each
element up and down. Where Xo stands on origin/main 456e0bc8a (native
arm64, the probes in "Measurements" below):

- **`for` loops already borrow.** A `for x in xs` whose body does not
  consume the list reads each element as a borrowed projection (decision
  0186): the loop's SSA after insert_rc has no Retain or Release. A Set
  or Map loop walks a root log table in place (0291 addendum). The same
  1000 element walk costs 362 M instructions over a List, 403 M over a
  Set, 404 M over a Map, with no count operations in any of them.
- **Function values do not.** A function called through a value (a
  closure literal, a named function passed as a value through its
  `NAME.value` thunk, a `fn` parameter) is `indirect`, and insert_rc's
  convention for those is fixed: "callers through a function value pass
  the closure borrowed and the arguments owned" (compiler/ssa rc.xo
  init_borrows). The caller cannot know which body it calls, so the
  arguments cannot be inferred borrowed the way a named function's are.
  Each element handed to a callback is therefore counted up before the
  call (one Retain per heap field: a `User{id, name, email}` is two) and
  down inside it. The prelude's `List.any`, `all`, `find`, `filter`,
  `map`, `fold`, `sort_by`, `group_by`, and every user `fn` parameter
  pay this per element.
- **On a list shared across threads the counts are atomic**, and two
  tasks walking the same list fight over the strings' count lines.
- **`xo vet --cost` does not see it.** XO0908 is meant to note "an
  element counted up and down for a function value", but the probe's
  `for u in xs { t += f(u) }` gets no note: the Retains are on the
  element's fields (`field 1`, `field 2`) and the CallValue uses the
  element, so cost.xo elem_count_per_iter's "a CallValue uses the
  retained value" test never matches a struct element. And the common
  form, `xs.any(fn(u) { ... })`, has its loop inside the prelude's
  list_any, which cost.xo does not read (it notes the root module's
  functions only). XO0907 to XO0910 report nothing on the probe.

Where the code is:

| Callback calls, non-test `.xo` | `.map(fn` | `.filter(fn` | `.any(fn` | `.all(fn` | `.find(fn` | `.sort_by(fn` | `.fold(` |
|---|---|---|---|---|---|---|---|
| compiler/ | 65 | 18 | 26 | 6 | 0 | 91 | 3 |
| std/native/ | 38 | 2 | 4 | 3 | 4 | 3 | 0 |
| bench/perf/http/xo (CRUD) | 0 | 0 | 0 | 0 | 0 | 0 | 0 |

The compiler's hot passes already walk their lists with `for` (0186
moved the worst of them, bench/results/rc-borrow-2026-10-12.md), and
the CRUD server's GET /users is a Map loop (borrowed since the 0291
addendum) with `json.encode(u)` a direct call (which 0046's inference
borrows when the callee only reads; not measured here). So the gain is in code written in the callback style, which
is how agents write filters and searches, not in today's two hottest
programs. The figures below are what that style costs.

## Measurements

Native arm64, origin/main 456e0bc8a, the development Mac, instructions retired from
`/usr/bin/time -l` (three runs each, spread under 0.3%). A list of 1000
`User{id: Int, name: Str, email: Str}`, walked 20,000 times for a name
that is never found (20 M element visits per program), each walk in its
own function:

| Walk | Instructions | Per visit | Count operations per visit |
|---|---|---|---|
| `for u in xs { if u.name == want {...} }` | 363 M | 18 | 0 |
| `xs.any(fn(u) { u.name == want })` | 1,185 M | 59 | 2 up, 2 down |
| `xs.filter(fn(u) { u.name == want }).len()` | 1,266 M | 63 | 2 up, 2 down |
| `xs.fold(0, fn(t, u) { ... })` | 1,244 M | 62 | 2 up, 2 down |
| same, `User` of three Ints: `for` | 155 M | 8 | 0 |
| same, three Ints: `any` | 299 M | 15 | 0 |
| same, three Ints: `fold` | 358 M | 18 | 0 |

The Int only rows separate the call from the counts: an indirect call
costs about 7 to 10 instructions a visit; counting two strings up and
down costs about 34 (1,185 - 363 = 822 M, less the call's 144 M).

The same walk done by two tasks at once (the list captured by both, so
shared across threads and counted atomically; 40 M visits):

| Walk, two tasks | Instructions | User CPU |
|---|---|---|
| `for` | 710 M | 0.06 to 0.07 s |
| `any` | 3,883 M | 0.30 to 0.44 s |
| `fold` | 3,961 to 3,974 M | 0.29 to 0.49 s |

A callback walk of a shared list costs 5.5 times the instructions and
5 to 7 times the CPU of the `for` loop. `sort_by(fn(u) { u.id })` over
the same 1000 users, 300 sorts: 171 M against 148 M with Int fields
(+16%: the key is called once per element, then the sort compares the
keys), so sort_by is a small case.

Copies counted: the probe's functions' SSA after insert_rc
(`XO_DUMP_FN=NAME XO_PASSES=-inline xo build --backend=arm64`): the
`for` walks have no Retain, Release, or Reuse; `for u in xs { t += f(u) }`
has two Retains per iteration (the two strings) before `callv`, and the
callee releases them; list_any, list_filter, and list_fold do the same
for their `f(x)`.

## Decision

1. **No borrow syntax.** No `for &x in xs`, no `fn(&T)`. A `for` loop
   already borrows whenever the body does not consume its list (0186),
   so `&x` on a loop would assert what the compiler proves or fails
   to prove, and the error it would give ("the body writes `xs`") is a
   copy the language requires (a loop reads the list as it was). On a
   function type, `&T` would be a second kind of parameter that generics
   (`fn(T) -> U` instantiated at a reference), tasks (a borrowed value
   captured by `spawn`), and stored function values all have to reason
   about: a lifetime system in all but name, which agents would have to
   learn and apply at every callback. Cost visibility stays with `xo vet
   --cost`.

2. **Function values borrow their arguments.** The fixed convention for
   indirect functions becomes: the closure borrowed (as today) and each
   heap argument **borrowed**, except:
   - a `var` parameter (`m.update(k, init, fn(var v) ...)`,
     `Mutex.with(fn(var st) ...)`): owned, so it is updated in place;
   - a parameter whose type is the function's result type (an
     accumulator threaded through: `fold`'s `fn(A, T) -> A`, a builder
     `fn(B) -> B`): owned, so `var a = acc; a.push(x); a` keeps updating
     one list in place instead of copying it every call (the O(n
     squared) trap of 0149's first version, which made the checker 25
     times slower);
   - a move only type: moved, as today.

   The rule is a function of the function type alone, so every caller
   and every body agree without knowing each other. A body that keeps a
   borrowed argument (stores, returns, pushes, captures it) retains it
   on that path, as decision 0149 already does for enum parameters; a
   caller passing a temporary releases it after the call, as it does for
   a named function's borrowed parameter. A named function used as a
   value keeps its own inferred convention: its `NAME.value` thunk
   (compiler/ssa call.xo) retains the arguments the named function owns.

3. **`xo vet --cost`**: with (2), XO0908's case (an element counted for a
   function value) no longer occurs for a body that only reads; the code
   keeps its meaning for what remains (an element a callback keeps is a
   real copy and is not noted, as now). The two gaps found above are
   fixed only if (2) is deferred: XO0908 following a field's Retain back
   to its element, and a note at the call site of a prelude callback
   method (`xs.any(fn ...)` in a loop or over a parameter list).

## How it interacts

- **Copy on write.** A borrowed List, Str, or Bytes argument that the
  body updates (`var y = x; y.push(1)`) is copied at the update, because
  the caller still holds it. Today the same happens whenever the
  argument is an element of a collection (the collection holds it), so
  the only callbacks that lose an in place update are those passed a
  value with no other holder that update it, and whose type is not the
  result type. Implementation step 4 counts them over compiler/, std/,
  and bench/ before anything lands; if any is hot, the rule's second
  exception is widened (owned for any List, Map, Set, or Bytes
  parameter), at the cost of keeping today's counts for callbacks over
  lists of lists.
- **Moves.** Move only values are moved, unchanged. Decision 0300's
  flow sensitive moves are about bindings in the caller and are not
  affected: a borrowed argument is not a move.
- **Defer and faults.** A `defer` in the body runs before the call
  returns, while the caller still holds the argument: safe. When the
  body faults or returns an error, the caller's landing pad releases the
  temporaries it passed (insert_rc already releases a caller's owned
  values on its landing paths; today the callee's pad released them).
- **Tasks and threads.** The call is synchronous, so the caller's
  reference keeps the count at one or more for its whole duration, on
  any thread and under preemption (0318). A body that captures the
  argument in a `spawn` closure keeps it, so it retains it first. Task
  bodies take no arguments (their captures are in the closure, param 0,
  borrowed today).
- **Read shares.** `with_read(fn(st) ...)` already borrows the protected
  value (0334); (2) changes nothing there.
- **Map and Set loops.** A loop body with a closure still takes the
  lists' loop, not the in place walk (0291 addendum, XO0907); (2) only
  makes that closure's arguments cheaper.
- **Backends.** The convention is decided in compiler/ssa insert_rc,
  which arm64, amd64, LLVM, and wasm share, so all four change together.
  The Go backend has no counts (its runtime is garbage collected) and is
  unchanged. The per function caches that hold insert_rc's output
  (rccache, fecache, the arm64 inliner's) bump their versions.

## Plan

1. Audit every producer of `Op.CallValue` (compiler/ssa call.xo,
   conc.xo, iface.xo, map.xo's update callback, show.xo, budget.xo,
   testmode.xo) and every runtime caller of an Xo closure in std/native
   and llrt that passes arguments: each must release what it passed
   after the call instead of handing it over.
2. compiler/ssa rc.xo: one helper `fv_param_owned(fn_type, i)` (`var`,
   the result type, move only) used by init_borrows for indirect
   functions (fixed flags: borrow_scan never demotes them) and by the
   CallValue's leaf uses (Mode.Borrow for borrowed positions); the 0149
   retain at a consuming path extended from enum parameters to every
   borrowed parameter of an indirect function.
3. Tests: a golden `borrow_callbacks` (any, filter, map, fold
   accumulating a List of 10,000, a closure that keeps its argument, one
   that captures it in a spawn, one that faults, a named function with
   an owned parameter passed as a value, the same over a list shared by
   two tasks), XO_LEAKCHECK 0 live on arm64 and LLVM, XO_RC_CHECK, the
   wasm goldens, TSan on the shared walk, and the fixed point.
4. Census before landing: the callbacks over unique values that update
   their argument (the copy on write risk above), with
   `XO_DUMP_FN` on each candidate.
5. Measure: the table above again (expected, an estimate: `any` and
   `filter` near the `for` walk plus the call, about 500 M single
   threaded and about 1,000 M on two tasks), xo's own build (user CPU,
   paired cold builds), and `__text` for native and LLVM.

## Consequences

- Callbacks that only read their elements stop counting them: the
  callback style costs the call (7 to 10 instructions a visit) rather
  than 34 more for two strings, and on a shared list it stops fighting
  over count lines across cores.
- One more rule in the ownership model (written in spec/core.md where
  function values are described): an argument passed to a
  function value is borrowed unless it is `var`, of the result type, or
  move only. Programs do not change: the convention is not visible in
  the source, only in counts.
- A callback that keeps every element (`map(fn(u) { Wrapped{u} })`)
  costs the same as today: the Retain moves from the caller into the
  body.
- A temporary passed to a function value that keeps it costs one more
  count pair than today (the body retains, the caller releases) where
  today it was moved. The census in step 4 also counts these.

## Alternatives

- **`for &x in xs` and `fn(&T)` parameters**, checked by the compiler
  (Rust's shared references, Swift's `borrowing`). Rejected above: the
  loop form adds nothing the analysis does not already do, and the
  function form needs lifetimes across generics, tasks, and stored
  function values.
- **`&x` as an assertion only** (an error when the loop cannot borrow).
  Rejected: when it fails, the cause is a copy the program's meaning
  requires; `xo vet --cost` reports costs without new syntax.
- **Specialize the prelude's callback methods at a closure literal**
  (inline list_any with the literal's body, devirtualized, so the walk
  is a `for` loop and 0186 borrows it). It also removes the call, but
  only where the method, the literal, and the inliner's budget meet;
  stored function values (handlers, comparators, a `fn` parameter)
  keep paying. A complement to (2), not a replacement: proposed as a
  follow up once (2) is measured.
- **A convention bit in each closure** (the body says whether it
  borrows; the caller branches). Rejected: a branch and two code paths
  at every indirect call to save what a type level rule already saves.
- **Two entry points per indirect function** (borrowed and owned).
  Rejected: doubles closure code and thunks for the accumulator case the
  result type rule already covers.
- **Keep owned arguments and fix only `xo vet --cost`.** Rejected as the
  answer to the poll: it would report the cost at every callback without
  a way to avoid it short of rewriting each one as a `for` loop.

## Implementation (2026-10-22)

- compiler/ssa rc.xo `fv_arg_owned(types, ft, i)`: argument i of a
  function value of type ft is handed over when its type is the
  result's, or one of a tuple result's; any other is borrowed. The
  `var` exception needs no rule of its own: a `fn(var T)` closure
  (Map.update, Mutex.with; closure_with) has the SSA type
  `fn(T) -> (R, T)`, so its value is in the tuple result. The tuple case
  also hands over an element a callback returns in a tuple
  (`xs.map(fn(u) { (u.id, u) })`): the caller's count goes into the
  result instead of a retain in the body.
- init_borrows gives an indirect function the closure and the arguments
  fv_arg_owned does not own as borrowed; borrow_scan returns no
  demotion for it (the flags are its type's), so a body that consumes a
  borrowed argument on a path retains it there (insert_rc's existing
  retain of a consumed untracked leaf, as for 0149's enum parameters);
  infer_borrows_incr never gives an indirect function the last build's
  flags. The CallValue's modes use fv_arg_owned on the closure
  argument's type: types are interned by name, so the caller's and the
  body's are the same type. RC cache v9, borrow record v2.
- A mismatch is an internal compiler error, not a silent double
  release (Xo Lang 2's review): init_borrows faults on an indirect
  function whose parameter 0 is not of a function type, fv_arg_owned on
  a type that is not a function's, and a CallValue of a Closure of a
  known function faults unless the closure's type is that function's
  parameter 0 type (Rc.fparam0; also in the verifier's checker). The
  last is also what makes arm64's devirtualize safe.
- Every target of a CallValue is an SSA function marked indirect
  (closures, `NAME.value` thunks, the interface and Show thunks,
  closure_with); no C or assembly calls an Xo closure with arguments.
  The `NAME.value` thunk needs no change: its call of the named function
  uses that function's inferred modes, so a named function that owns a
  parameter gets it retained by the thunk. arm64's devirtualize (a
  CallValue of a known closure becomes a Call, after insert_rc) sees the
  callee's flags, which are the rule's.
- Not done, by the type rule: a fallible accumulator (`fn(A, T) -> A !
  E`, whose SSA result is an enum holding A) is borrowed, so its update
  copies; no prelude or compiler callback has that shape. Move only
  types are not visible in the SSA types (StrBuilder lowers to Str, Buf
  to List): a borrowed one stays correct because their writes copy a
  shared value first.
- `xo vet --cost` XO0908 now notes only the elements handed to a
  function value that takes them (fv_arg_owned): `xs.map(fn(u: User) ->
  User { ... })`, not `xs.any(fn(u) { ... })`.
- Tests: compiler/ssa fvborrow_test.xo (the rule, the fixed flags, the
  CallValue's modes); the golden borrow_callbacks (any, all, filter,
  fold, map that keeps, a List accumulator of 10,000 pushes, an owned
  accumulator every borrowed element also holds (the first push
  copies, the elements keep the original), bodies
  that capture their argument in a closure and a task, a named function
  that keeps its parameter used as a value, a failing callback,
  Map.update and Mutex.with closures, two tasks over one shared list):
  arm64, LLVM, Go parity, wasm; 0 live of 21,112 allocations under
  XO_LEAKCHECK; XO_RC_CHECK=1; cmd/xo TestVetCost*; the fixed point.
  `XO_DUMP_FN` shows a capturing body retaining the captured field
  (`retain` of `field 1` before `closure`) and fold's accumulator
  updated in place (`reuse` of the List parameter, then `list.push`).
- Measured (bench/results/callbacks-0337-2026-10-22.md,
  bench/perf/callbacks): any over 1000 users of two strings, 20 M
  visits, 1,759 M to 1,160 M instructions native (-34%), 2,066 M to
  1,507 M LLVM (-27%); filter and fold the same; two tasks over one
  shared list 4,067 M to 2,306 M (-43%) native, 5,959 M to 3,003 M
  (-50%) LLVM, wall 191 ms to 75 ms native; the for loop and the Int
  rows unchanged. The rest of the gap to the for loop is the call itself
  (about 31 instructions a visit native, 56 LLVM, more than the 7 to 10
  estimated above): specializing the prelude's callback methods at a
  closure literal (Alternatives) is the follow up.
