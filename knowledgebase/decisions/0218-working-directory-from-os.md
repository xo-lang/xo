# 0218: The working directory comes from the OS, not $PWD

Status: Accepted (owner request, 2026-10-13)

## Context

xo read its working directory from the PWD environment variable in
about twenty places (selfhost/cli/core/cli.xo, cli/servers/lsp.xo,
servers.xo, serve.xo, replay.xo, cli/semantic/main.xo,
cli/modules/xomod/main.xo, cli/bind/cli.xo, xo/main.xo, xo/opts.xo,
check/main.xo, check/locate.xo, check/bindmiss.xo, and the drivers of
gen, ssa, arm64, llvm, and wasm). std had `Fs.cwd()`, a capability rooted
at the working directory, but no way to get its path as a string.

A process started without a shell inherits whatever PWD its parent had.
VS Code launched from the macOS Dock passes `PWD=/` while starting
`xo lsp` in the workspace folder, so the LSP resolved std's relative
paths against `/` and Go to Definition into std answered null
(gotchas/xo-working-dir-is-pwd.md).

## Decision

1. std gets `fn (f: Fs) cwd_path() -> Str ! FsErr` (spec/std.md 3.1): the
   working directory as an absolute path with `/` separators, the
   directory `cwd()` is rooted at. It has the semantics of Go's
   `os.Getwd`: the OS's getcwd, except that `$PWD` is returned when it
   names the same directory (so the shell's path, with its links, is kept
   where it is right: macOS's `/var/folders` stays as it is rather than
   becoming `/private/var/folders`, and paths printed relative to the
   working directory do not change). A stale `$PWD` is ignored. It fails
   with `Io` when getcwd fails. No `uses` clause, like `cwd()` and `sub`.
2. Backends:
   - Go backend: std/rt `realFs.M_cwd_path` is `os.Getwd` with
     `filepath.ToSlash`.
   - Native (arm64, LLVM): std/native/prelude.xo `fs_cwd_path` calls
     the existing `rt.fs_raw_getcwd` (std/llrt/xo_fs.c: libc getcwd on
     Darwin, the getcwd system call through xo_nolibc.c on Linux,
     `GetCurrentDirectoryW` through xo_win.c on Windows, via
     std/native/llrt/fspath.xo `cwd()`, which already backed `Fs.cwd()`),
     then compares `$PWD` by following its links (`fs_real_path`, the
     algorithm of selfhost/check's `real_path`). Windows does not consult
     `$PWD`, as Go does not.
   - WASI has no working directory: `cwd_path` fails with `Io` there. The
     selfhost wasm backend keeps the host's `target.os`
     (gotchas/wasm-target-os-is-the-host.md), so today it reports the
     call as unsupported, as it does `cwd()`.
   - A test's MemFs answers `/`, its root (its `cwd()` is itself); with
     `xo test --real` the real one answers.
3. The toolchain (every selfhost file above) uses
   `os.fs.cwd_path().ok() ?? os.env.get("PWD") ?? ""`: `$PWD` only when
   getcwd fails. `serve.xo` still sets PWD for the child it starts in a
   directory, which is harmless now and keeps the logical path.

## Consequences

- `xo lsp` started with `PWD=/` in the workspace folder answers Go to
  Definition into std (examples/notes-service/main_test.xo line 70
  character 50, `http.Request`, resolves to
  `<user cache>/xo/lsp-std/std/http.xo` line 26). The editors/vscode
  workaround (setting PWD for the server) stays harmless.
- A new Fs method: a user type that implements Fs must add it.
- Golden fs_cwd_path (internal/llvm/testdata/golden and
  internal/gen/testdata/golden) runs with `PWD=/` from env.txt and checks
  the result is the program's directory on arm64, LLVM, and Go.

## Alternatives

- An `Env` method (`os.env.wd()`): the working directory is a file system
  fact and `Fs.cwd()` is already its capability; tests fake Fs, not a
  separate cwd on Env.
- getcwd only, never `$PWD`: changes paths on macOS temp directories from
  `/var/...` to `/private/var/...`, which changes relative paths in
  diagnostics; Go's xo used `os.Getwd`, which prefers a correct `$PWD`.
- A getcwd written in Xo system calls (linux.xo, sys_darwin.xo,
  sys_windows.xo): the existing C `xo_fs_raw_getcwd` already covers the
  three OSes and backs `Fs.cwd()`; porting it is a separate step of
  decisions 0206 and 0213 when the fs layer moves.

## Verification (Mac, 2026-10-14)

- Fixed point at 2117be24 (scripts/bootstrap-xo.sh with the branch's
  bin/xo): s2 and s3 assembly identical (437,224,619 bytes), binaries
  identical (87,387,360 bytes); s1 319 s, s2 317 s, s3 317 s.
- Golden fs_cwd_path (PWD=/) passes on arm64, LLVM, and Go, fs_paths_os on arm64 and LLVM;
  TestSelfhostWasm passes; TestLSPStalePWD passes.
- LSP repro (`PWD=/ xo lsp` in the repository, definition at
  examples/notes-service/main_test.xo line 70 character 50): the old
  bin/xo answers null; the new one answers
  `<user cache>/xo/lsp-std/std/http.xo` line 26, characters 7 to 14.
