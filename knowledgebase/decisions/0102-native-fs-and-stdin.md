# 0102: Native Fs capabilities and standard input

Status: Accepted (2026-10-09). Implemented in `internal/ssa` (build.go
`typ`, build_io.go `fsType` and `fsCall`, build_native.go `fs_root`,
`fs_mode`, `fs_new`, build_call.go Stdio), `std/native/prelude.xo` (the
`fs_*` and `stdio_*` functions), `std/native/rt.xo`, `std/llrt/xo_fs.c`,
`std/llrt/xo_stdin.c`. Tests: golden
`internal/llvm/testdata/golden/fs_native`, internal/gen's `fs_rooting`
and `cancellation` in `gen_native.txt`, TestStdinNative
(`internal/llvm/stdin_test.go`).

## Context

Natively, Fs was a capability without a value (`KCap`) and only
`os.fs.read` lowered; every other Fs method, the attenuations (`sub`,
`read_only`, `write_only`, a `Denied` escape), `Stdio.write`, and the
standard input reads (`read_line`, `read_all`, `lines`, with the
cancellation and stop rules of decision 0088) were XO1201
(spec audit rows 6.3, std 3.1, std 3.6, 7.2).

## Decision

1. **An Fs is a value: its root and mode bits** (`xo.Fs {root: Str,
   mode: Int}`: 1 a sub capability, 2 read only, 4 write only), the
   fields of std/rt's `realFs`. `os.fs` is `{"", 0}`; a test's MemFs is
   the same type.
2. **Fs methods are Xo** in `std/native/prelude.xo` (`fs_<method>`, the
   Fs first), a port of std/rt caps.go: path resolution and escape
   checks (`path.Clean` rules), mode checks, `Denied`, `Canceled` (shield
   aware: deferred code may still write, as on the Go backend), FileInfo
   paths relative to the root, read_dir sorted by name, walk parents
   before children. System calls are small C functions in
   `std/llrt/xo_fs.c` returning a status (FsErr's tag plus one) with the
   message of `strerror` lowercased, as Go's errno texts read.
   `create_temp` names files prefix plus a random number, as
   `os.CreateTemp`.
3. **Standard input waits on the event loop**: `xo_stdin.c` registers
   descriptor 0 with the poller (kqueue or epoll) unless it is a regular
   file, reads what `poll` says is available, and otherwise parks the
   task until readable, canceled, or 50 ms passed (to see a stop
   request). Input read by a reader that gave up stays buffered for the
   next read; readers take turns. A line loses its `\n` and a `\r`
   before it, as std/rt's readLine.
4. **with_timeout**: a closure that finishes after the deadline fired
   without reaching a cancellation point gives `Err(TimedOut)` natively
   too (std/rt decides by which came first); before, natively it gave
   its own result.
5. In test binaries, Fs methods other than `read` stay XO1201 (a test's
   MemFs, item of decision 0083); standard input reads what
   `FakeStdio.set_input` gave (std/rt's FakeStdio: a line loses its
   `\r`, the end of the text is the end of input).

## Consequences

- `fs_rooting`, `cancellation`, and the new `fs_native` golden print the
  same on Go, LLVM, and arm64; TestStdinNative compares the four stdin
  cases.
- An Fs value costs a string and an Int where it was nothing; hello
  world is unchanged (no Fs code without Fs calls).
- An Fs operation is a few runtime calls (status, message, info) after
  the system call; the native prelude's path handling allocates.

## Alternatives

- **Fs as an interface value** (decision 0100) with a C implementation
  object: would also let user types implement Fs, but every capability
  call would become an indirect call and MemFs would need a value too.
  Kept for when user implemented capabilities are needed.
- **A reader thread for stdin** (std/rt's goroutine): the poller already
  exists and parks fibers; a thread would need its own handoff.
