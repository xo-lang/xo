# 0307: A user value converted to Fs on the native backends

Status: Accepted (lead, relaying the owner, 2026-10-21: lift XO1201 for
user values converted to capability interfaces; layout agreed with Xo
Lang 2 the same day). Complete 2026-10-21 (decision 7: Stdio, Clock,
Rand, and Os literals; the user field's layout corrected).

## Context

Spec core 2.x (conversions, rule 2) lets any type that implements an
interface be used where the interface is expected, the capability
interfaces included, and the Go backend allows it. The native backends
(arm64, amd64, LLVM, wasm) did not: decision 0100 made user interfaces
closure records but left the capabilities as the runtime's own values
(`xo.Fs` is `{root, mode}`, Proc, Env and Net `{lists}`, Os, Stdio, Clock
and Rand void), with each method a static call of the native prelude. A
user struct converted to `Fs` was XO1201 ("converting `T` to `xo.Fs`"),
which stopped a test's own Fs wrapper (gotcha
test-fs-wrapper-not-native) and an Fs view of `embed.Dir` (decision
0304).

## Decision

1. **Fs first.** The native `xo.Fs` gains a third field, `user: UserFs?`,
   where `UserFs` (std/native/llrt/userfs.xo) declares the same methods as
   the prelude's `Fs`. The runtime's own Fs has `None`; converting a value
   of another type to `Fs` builds its interface record (decision 0100's
   `to_iface`) and stores `Some(record)`.
2. **Dispatch in std, not in each call.** Each of the native prelude's
   `fs_<method>` functions and std/testing's `memfs_<method>` functions
   starts with `if let Some(u) = userfs.user(f) { return u.<method>(...) }`.
   A call through an Fs costs one load and one branch more; nothing else
   changes at call sites. `File` and `FileLock` results are cast to the
   native prelude's own types (`userfs.open_as`, `lock_as`,
   `try_lock_as`, decision 0083's `rt.cast`).
3. **Seed compatible.** `userfs.user(f)` is an ordinary function whose
   body returns `None`; the compiler replaces a call of it with a read of
   the user field (compiler/ssa call.xo fs_user_call). A seed compiler
   that predates this decision builds the same std with the body, so no
   new intrinsic and no seed cut are needed.
4. **Fail closed.** A user Fs carries root `"denied"` and mode 7 (as
   `fs_cwd`'s fallback), so native code that reads root and mode instead
   of calling a method (`lock_shared`) denies rather than treating the user
   value as the unrestricted Fs.
5. **Effects** are unchanged: the checker already checks a user
   implementation's `uses` against the interface's.
6. **Proc, Env, and Net** (2026-10-21, the second step): their native
   struct `{lists}` gains the same `user` field (`UserProc`, `UserEnv`,
   `UserNet` in native/llrt/userfs.xo). Several of their methods are
   lowered by the compiler itself (Env.args, Proc.exit, Proc.run,
   Net.listen and dial), so the dispatch is at the call: user_cap_call
   (compiler/ssa/call.xo) evaluates the receiver once, branches on the
   user field, calls the record's method or lowers the call as before
   with that receiver (Builder.pinned). Fail closed: a user value's lists
   are `"\u{1}"`, one empty allow-list, which admits nothing. An
   interface method's default argument that is an empty collection
   (`Proc.cmd`'s `args = []`) lowers to the zero value, since such
   defaults are not checked as bodies.
7. **Stdio, Clock, Rand, and Os** (2026-10-21, the last step; the lead
   chose the uniform layout, to be made conditional only if it cost):
   - Stdio, Clock, and Rand, void before, become `xo.Stdio` (and so on)
     with the one user field; their record interfaces are UserStdio,
     UserClock, and UserRand, and the call dispatches at the call site as
     Proc's does (user_cap_call). Os becomes `xo.Os` with a user field
     holding a UserOs record: an `Os{...}` literal (XO1201 before) builds
     a native/llrt/userfs OsFields (field NAME as NAME_v, since a field
     and a method may not share a name) and converts it, and a field
     read of an Os (`o.fs`) calls the record's method of that name, else
     makes the runtime's own capability as before.
   - `Rand.shuffle` is generic, so a record cannot hold it. It needs no
     dispatch: on every backend it is derived from the value's `int`
     (std/rt's Rand_shuffle and the native prelude's rand_shuffle,
     Fisher-Yates from the end, one `int` per step; std.md 3.4); a
     `shuffle` method the user's type declares is not called on the Go
     backend either, so native code gives the same order.
   - **The user field's layout, corrected for every capability.** Steps
     one and two typed the user field `Option[UserX]`, which holds the
     interface record inline: two words per method plus its equality
     and type id, so the native Fs was about 66 words, not the three
     this decision said (three words before the field), and Proc about 25; every copy of a capability
     (a field, a capture, a store) moved that much, and arm64's entry,
     which passes main's parameters in at most eight registers, failed
     for an Os of that shape. The user field is now `xo.user[R]`, an
     enum of None and Some(Box[R]): two words (a tag and a pointer),
     all-zero words are None, so the native entries' zeroed parameters
     are the runtime's own Os. A List holder (one word, empty =
     rt.rl_empty) was tried first: the arm64 inliner's verify rejected
     the element read (list.index) the dispatch makes after inlining.
   - wasm: main's Os is two zero words in the start function and the
     task runtime's main_entry (wasm gen.xo, rt3.xo); `$prog_group` gets
     its fault place from the call, which made `os.tasks` compile on
     wasm at all (the module failed validation before).

## Consequences

- A user type implementing `Fs`, `Proc`, `Env`, or `Net` (a fake, a
  wrapper that fails one read or refuses one host, a read-only view) works
  on every backend, in tests and programs; goldens user_caps (Proc, Env,
  Net; wasm has no processes or sockets) and user_env (all five
  backends), and
  `user_fs` (a user struct in a user interface and in Fs, across a task,
  no leaks under XO_LEAKCHECK) and cmd/xo testdata/nogc/userfs (a test's
  own Fs wrapper, every native backend against Go).
- The native Fs is five words (root, two; mode, one; the user field,
  two), Proc, Env, and Net four, Stdio, Clock, Rand, and Os two (they
  were zero); the runtime's own values carry None.
- Goldens user_void_caps (a user Stdio, Clock, and Rand, in a struct
  and across a task) and os_literal (an attenuated and wrapped Os
  passed to code written against Os, nested, across a task) run on
  arm64, LLVM, Go, amd64, and wasm; cmd/xo testdata/nogc/userfs has a
  test's own Stdio, Clock, Rand, and Os wrappers (TestNoGCTests), and
  TestUserRandShuffle pins the derived shuffle on every backend.
- native/llrt modules get no preemption polls (budget.xo runtime_mod), so
  userfs.xo stays loop free.

## Alternatives

- **Lower every capability as an interface record**: the runtime's own
  Fs would become closures over root and mode, a call through a closure
  everywhere and every program's Fs allocated; rejected for the cost.
- **A tagged pointer in the void capabilities** (low bit set = record):
  Os and the others have no word to tag (Xo Lang 2).
- **A new rt intrinsic for the user field**: the seed compiler does not
  know it and cannot build the new std (gotcha
  seed-older-than-intrinsics), which would need a seed cut.
- **Dispatch emitted by the compiler at each Fs method call**: every call
  site grows; the std functions are the one place all calls pass through.
