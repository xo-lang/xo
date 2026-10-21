# A test's own Fs wrapper passed xo check and failed to build natively

Status: Observed (2026-10-21, compiler/check check_test.xo)

Symptom: a test that wrapped the test's file system in its own struct
implementing `Fs` (to fail one read) passed `xo check`, then `xo test
compiler/check` failed to build: XO1201 "converting `FlakyFs` to
`xo.Fs`" at each place the struct was passed as an `Fs`.

Cause: a user type converted to a capability interface such as `Fs` is
not supported without the garbage collector yet (XO1201, support
planned), and plain `xo check` does not run the native check; `xo test`
builds natively by default.

Fix: check code a native build compiles, tests included, with `xo check
--no-gc`: plain `xo check` accepts what only the native backends reject
(XO1201 and the other no-GC limits). To test error handling
without a fake `Fs`, make the decision a function of the `Result` (a
`listing(r, dir)` that maps each `FsErr` to skip or fail) and test it
with constructed errors (`Err(FsErr.Io(path: p, msg: m))`).
