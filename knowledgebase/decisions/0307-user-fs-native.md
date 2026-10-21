# 0307: A user value converted to Fs on the native backends

Status: Accepted (lead, relaying the owner, 2026-10-21: lift XO1201 for
user values converted to capability interfaces; layout agreed with Xo
Lang 2 the same day).

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
6. **Later**: Proc, Env and Net (structs, the same `user` field), then
   Os, Stdio, Clock and Rand, which are void today (one word each for
   every program that passes one, so perhaps only in programs that convert
   to them; Xo Lang 2, 2026-10-21).

## Consequences

- A user type implementing `Fs` (a fake, a wrapper that fails one read, a
  read-only view) works on every backend, in tests and programs; goldens
  `user_fs` (a user struct in a user interface and in Fs, across a task,
  no leaks under XO_LEAKCHECK) and cmd/xo testdata/nogc/userfs (a test's
  own Fs wrapper, every native backend against Go).
- The native Fs grows from two words to three (the user field, None for
  the runtime's own).
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
