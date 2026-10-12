# 0172: The command line tool std on the native backends

Status: Accepted (2026-10-11), in progress: (1) commands, children,
pipelines, and attenuation, (2) file handles, links, globs, and locks,
and (3) `std/regexp` are implemented natively; the rest is listed under
"What is left".

## Context

Decisions 0161, 0163, and 0164 gave the Go backend what command line
tools need: commands with directories, environments, and pipes
(`Proc.cmd`, `Child`, `Proc.pipeline`), file handles, links, globs, and
locks, `std/regexp`, and `std/archive`. The native backends (arm64 and
LLVM, `--no-gc`) reported every one of them as XO1201 (planned), and
`Proc.only` and `Env.only` (decision 0156) too: natively a `Proc` or
`Env` was a capability of zero size, with nowhere to keep a list.

Constraints:

- Output must match the Go backend byte for byte (goldens run on Go,
  LLVM, and arm64; no allocation live at exit).
- The self hosted SSA builder (selfhost/ssa) mirrors internal/ssa
  function by function (TestSelfhostSSA), so every builder change is
  made twice; work done in Xo (std/native) or C is shared.
- The C runtime is prebuilt into packs keyed by its sources (decision
  0157): adding or editing C files changes the key, which is expected;
  a new file or variant would also change the pack layout, so new C
  goes into the existing files under the existing defines.

## Decision

1. **Proc and Env carry their attenuation.** Natively a `Proc` and an
   `Env` (and a test's `FakeProc` and `FakeEnv`) lower to a struct of
   one `Str` (`xo.Proc`, `xo.Env`), the lists of `only` encoded as
   `\u{1}` plus names each ended by `\u{0}`; `""` is no attenuation
   (`os.proc`, `os.env`). The intrinsics `proc_lists`, `proc_new`,
   `env_lists`, and `env_new` (std/native/rt.xo) read and make them, as
   `fs_root` and `fs_new` do for `Fs` (decision 0102). `Proc.only`,
   `Env.only`, `Env.get`, `Env.vars`, and `Env.home` are native prelude
   functions (capImpls) that check every list, as std/rt attenuate.go
   does; `Proc.run` checks too. A one field struct keeps the value small
   and needs no new SSA kind; a `List[List[Str]]` field would have made
   every capability value reference counted twice over for the rare
   attenuated case.
2. **Cmd is a value in Xo, a Child is a handle of the C runtime.** The
   native prelude declares `struct Cmd` (the builder fields of std/rt's
   procCmd, plus the lists of the Proc that made it) and `struct Child {
   h: rt.ProcChild }`; they replace the prelude's opaque `Cmd` and
   `Child` (implType), and their methods are `cmd_*` and `child_*`.
   A command crosses into C as a spec, a `List[Str]` (name, directory,
   flags, the arguments, the environment, the `set_env` entries), so the
   runtime needs no struct layout. `std/llrt/xo_proc.c` gains
   `xo_proc_exec` (mode 0 `output`, 1 `run` on the terminal, 2 a
   pipeline) and `xo_proc_spawn` plus the `xo_proc_child_*` operations.
   It follows os/exec: `PATH` lookup in the program's environment,
   `dedupEnv` (the last entry of a name wins), `PWD` set to the
   directory when the environment is inherited, `os.StartProcess`'s
   check of the directory (a missing one is `NotFound`), the error texts
   (`fork/exec ...`, `chdir ...`, `write |1: ...`), and exit status 128
   plus the signal. `posix_spawn` with file actions sets the streams and
   the directory (`posix_spawn_file_actions_addchdir_np`;
   `addinherit_np` keeps the terminal on macOS, where
   `POSIX_SPAWN_CLOEXEC_DEFAULT` closes everything else). Standard input
   data is written by a detached thread, as os/exec copies it from a
   goroutine; the collected streams and a child's pipes wait on the
   event loop's poller. The program's buffered output is flushed before a
   child that writes to the terminal starts, so the order is the Go
   backend's. In test binaries FakeProc answers from `on_run` (output,
   the fake terminal for `run`, children reading the registered output,
   pipelines with pipefail).
3. **Files.** The native prelude declares `struct File { h:
   rt.FsFile, path, write }` and `struct FileLock { h: rt.FsLock }`
   and implements every new `Fs` method as `fs_<name>` (fsCall), with
   the state errors of std/rt fsfile.go (closed, the wrong direction) in
   Xo and the system calls in `std/llrt/xo_fs.c` under `XO_FS`
   (`xo_fs_open`, `xo_fs_file_*`, `xo_fs_copy`, `xo_fs_perm`,
   `xo_fs_chmod`, `xo_fs_symlink`, `xo_fs_read_link`, `xo_fs_lock`,
   `xo_fs_unlock`; internal/build IOFlags lists them). A read handle
   buffers like Go's 64 KiB `bufio.Reader`: a read larger than the buffer
   with nothing buffered goes to the file, else one fill and a copy, so
   reads return the same sizes. `glob` is std/rt's fsGlob in Xo with Go's
   `path.Match` ported (scanChunk, matchChunk, getEsc, and its "syntax
   error in pattern"), over `read_dir` and `exists`, so sub capabilities
   and attenuation apply. `lock` polls `flock(LOCK_EX|LOCK_NB)` every 10
   ms on the event loop's timers (a cancellation point, status 8 is
   Canceled); a lock handle dropped without `unlock` closes its
   descriptor, which releases the lock (the Go backend keeps it until the
   garbage collector closes the file).
4. **Regexp in Xo** (`std/native/regexp.xo`, about 1200 lines): Go's
   regexp/syntax parser (the parse loop, `parsePerlFlags`, `parseClass`,
   `parseEscape`, `parseUnicodeClass`, `parseRepeat`, `repeatIsValid`,
   nesting depth) with its error texts and expressions, `Simplify`, the
   compiler (patch lists, `(f1+)?` for a nullable star), and the Pike VM
   of exec.go (threads in priority order, the first match cutting off
   the rest), plus `FindAll`'s empty match rule, `Split`,
   `ReplaceAll` with `expand`/`extract`, and `QuoteMeta`. Case folding
   is `unicode.SimpleFold`: ToLower or ToUpper from the runtime's tables
   except 38 runes of longer orbits, listed in the module (generated from
   Go's unicode package, Unicode 15). Written in Xo, not C, so a self
   hosted compiler gets it for free and the runtime packs stay as they
   were; the cost is speed (a Pike VM over lists of group positions).
   Not ported because they change speed, not results: the parser's
   factoring and literal merging, the onepass and backtracking engines,
   prefix scans. Also not ported: `ErrLarge` (the size estimate), and
   Unicode tables beyond letters and decimal digits: `\pL` (and
   `Letter`), `\p{Nd}` (`Decimal_Number`, `Digit`), `\p{Any}`, and
   `\p{ASCII}` work, from the C runtime's tables through the new
   intrinsic `unicode_ranges`; any other name Go knows (`\p{Greek}`,
   `\p{Lu}`) is a `RegexErr` "Unicode class not supported by native
   builds yet (decision 0172)", and a name Go does not know is Go's
   "invalid character class range". The golden `regexp_table` runs 86
   patterns (60 valid, 26 invalid with 11 of the error codes) on 18 inputs
   with every operation, then replacements with every template form,
   splits, quote, and invalid UTF-8: identical on Go, LLVM, and arm64.
5. **Waiting operations** (`Cmd.output`, `run`, `spawn`, `Child.write`,
   `read`, `read_err`, `read_line`, `wait`, `Proc.pipeline`, `Fs.lock`) make the
   program build with unwinding checks (nativeWaits, as `Proc.run`).
6. **Cancellation** natively kills a command when the cancelled task
   waits on it, and that call is `Canceled`; the Go backend kills it at
   once (a goroutine per child) and its `wait` gives 137. A Child that is
   dropped without `wait` is reaped if it has exited, else left to the
   operating system. Recorded in std.md 3.8.

Shared tests: the golden `proc_cmd` (internal/llvm/testdata/golden,
also run by internal/arm64's TestGoldenRun) covers outputs, statuses,
signals, directories, environments, `Env.only`, NotFound and Denied,
`run` after the program's output, every `Child` operation, and
pipelines; the golden `fs_files` covers line and size reads across the
buffer, handle state errors, create and append, copy with modes,
links (and escapes refused), globs with classes, `**`, hidden names, and
bad patterns, and locks; `internal/gen/testdata/golden/test_proc_cmd` runs in native
test binaries (cmd/xo TestNoGCTests) with the Go backend's report.

## What is left

- The decision 0163 methods on a native test binary's MemFs
  (`memfs_open` and the rest in std/native/testing.xo, std/rt
  fsfile_mem.go's semantics; the glob can call the same Xo code).
- The Unicode classes of regexp beyond `L`, `Nd`, `Any`, and `ASCII`:
  extend internal/llvm's TestUnicodeTables generator with Go's category
  and script tables (and FoldCategory, FoldScript) and return them
  through `unicode_ranges`.
- (4) `std/archive` natively (deflate, gzip, tar, zip).

## Consequences

- Programs that run commands build natively; `xo test --no-gc` runs
  FakeProc commands.
- Every `Proc` and `Env` value is now a struct of one string natively
  (a pointer and a length), not zero sized; passing one costs two
  words.
- Packs (decision 0157) get a new key from the edited runtime sources;
  their variants are unchanged.

## Alternatives

- **Spawn and pipes in Xo over thin system call intrinsics** (pipe,
  posix_spawn, read, waitpid): more code shared with self hosting, but
  `posix_spawn`'s file actions and attribute objects are C structures,
  and the stdin feeder needs a thread; the C side would still need most
  of this code.
- **A `KCap` with a side table of lists** (keyed by an id in the
  capability): keeps the zero size, but needs a registry with a
  lifetime and makes `only` allocate in C.
- **kqueue `EVFILT_PROC` or `pidfd` for waiting**: no polling of
  `waitpid`, but a second mechanism per OS; the backoff from 1 ms to
  50 ms is what `Proc.run` already does.
