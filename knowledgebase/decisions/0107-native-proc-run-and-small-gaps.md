# 0107: Native Proc.run, FakeProc.run, base64, Range.to_list, and target

Status: Accepted (2026-10-09). Implemented in `std/llrt/xo_proc.c`
(compiled only into programs that call it, `XO_PROC`, internal/build
`IOFlags`), `std/native/prelude.xo` (`proc_run`, `bytes_to_base64`,
`bytes_from_base64`, `int_range_list`, `rand_shuffle`), `std/llrt/xo_std.c`
(`xo_b64_encode`), `internal/ssa/build_io.go` (`procRun`),
`build_testmode.go` (`FakeProc.on_run`), `build_target.go`, and
`build_call.go` (`Range.to_list`). Goldens: internal/gen
`audit_second_pass` and `std06_helpers` in TestGenGoldensNative,
internal/llvm `method_values` (base64 edge cases, ranges, `target`),
cmd/xo `testdata/nogc/same/fakes_test.xo` (FakeProc).

## Context

The third pass of the spec audit left these native rows at XO1201: the
real `Proc.run`, a test's `FakeProc.run` and `on_run`, `Bytes.to_base64`
and `Bytes.from_base64`, `Range.to_list`, and the predeclared `target`.

## Decision

1. **`Proc.run`** is std/rt's realProc.run (os/exec): a name without a
   slash is looked up in `PATH` (none executable: `NotFound(cmd)`),
   standard input is `/dev/null`, standard output and error are read whole
   through nonblocking pipes registered with the event loop's poller, so
   a waiting run gives its worker back. `posix_spawn` starts the child
   with an empty signal mask and default SIGINT, SIGTERM, and SIGPIPE
   (`POSIX_SPAWN_CLOEXEC_DEFAULT` where it exists). A start error is
   `Io("fork/exec <cmd>: <errno text>")`, a signal death exit code -1,
   and cancelling the task kills the child (`SIGKILL`) and gives
   `Canceled`. The outcome comes back in the List[Str] the call returns
   (status, code or message, stdout, stderr): no thread local status,
   since a task's fiber may resume on another worker.
2. **FakeProc** in the test world: `on_run(cmd, out)` keeps the outputs
   per command in the C runtime (the last one wins); `run` answers with
   them, and a command without any is `NotFound(cmd)`.
3. **base64** is padded standard base64 (`base64.StdEncoding`) in C;
   decoding reuses the JSON decoder's `n_b64_decode`.
4. **`Range.to_list`** of a range literal (`(lo..hi)`, `(lo..=hi)`) calls
   a native prelude function; ranges are not values natively otherwise.
5. **`target`** is a constant of the build's target (`ssa.Options.GOOS`,
   `GOARCH`, the host's unless arm64 names its OS), so a `match
   target.os` folds to one arm; std/rt's names (an unknown OS is Linux,
   an unknown architecture Amd64). Go mode assembly (hybrid builds), whose
   target the builder does not know, leaves it to Go.
6. **`Rand.shuffle`** is std/rt's Fisher-Yates, one `int` draw per step,
   so seeded tests shuffle alike.

## Consequences

- `xo build --no-gc` programs can run commands; the C file is not linked
  into programs that do not.
- The audit's native rows for std 2.2 base64, std 3.4 shuffle, std 3.7
  `Proc.run`, std 8 FakeProc, and 12.2 `target` are closed.

## Alternatives

- **`fork` and `exec` by hand**: `posix_spawn` avoids copying a large
  multithreaded process and is safe with the runtime's threads.
- **Blocking `waitpid` and blocking reads on a worker**: simpler, but a
  long command would hold a worker and could not be cancelled.
- **`target` as a runtime call into C**: correct too, but no arm is
  removed at compile time (core 12.2 says only the chosen arm is code
  generated).
