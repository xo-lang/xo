# Changelog

Xo stays on 0.x until the owner declares 1.0 (decision 0089). The toolchain
minor matches the spec draft; the full list of language changes for each
release is the "Changes in 0.N" section of `spec/core.md`.

## Unreleased

- Native runtime (zero-C audit item B21, part 1): the heap's 4 MiB chunks
  and its big blocks (above 256 KiB) are mapped from Xo
  (std/native/llrt/heapos.xo: mmap, VirtualAlloc) instead of the C
  library's malloc, with a cache of given back mappings, and the central
  lists' lock is a lock word instead of a pthread mutex. bench/perf
  text, json, alloc, and nested are 0.973 to 1.029 of main's
  instructions, the self build 0.997 and 0.996
  (bench/results/heap-platform-2026-10-14.md).
- std/time on Windows: `time.local` builds the zone as Go's time.Local
  does (transitions for 100 years each side of now, no rule past them,
  Go's abbreviations), so times before 1926 and after 2125 match the Go
  backend; the C before it extended the current rule to every year.
- Native runtime (zero-C audit item B27): std/time's zone data and the
  table of loaded zones are Xo (std/native/llrt/tzhost.xo): the
  program's zone data through a `xo_tz_table` accessor the build writes
  beside it, the host's zone files, and Windows' local zone from
  GetDynamicTimeZoneInformation. std/llrt/xo_time.c (519 lines) is
  deleted.
- Native runtime (zero-C audit item B06, part 3): a fault on Windows
  unwinds through the Xo walker (std/native/llrt/unwind.xo win_raise,
  over RtlVirtualUnwind) instead of xo_rt.c's C walk, which is deleted
  with its LSDA reader. Windows goldens 157 of 157. The walk starts at
  the C fault_raise's captured context, and the unwind tables name the
  Xo personality export on Windows too; fault paths about 0.97 to 1.02
  of the C walk's wall clock time on windows/amd64.
- Native runtime (zero-C audit item B13, stage 3): the string objects'
  appends and builders are Xo (std/native/llrt/obj.xo): `a + b`, and
  the runtime's rs_append, push_byte, push_word, push_words,
  append_bytes, and with_cap. arm64 generated code and std/native call
  them with no C level, and a unique string with room is appended to
  without a call. bench/perf text, json, alloc, and nested are 0.776 to
  1.040 of main's instructions on arm64 and LLVM (json enc_small 0.776,
  text debug 0.801), the self build 0.998 and 0.999
  (bench/results/string-list-objects-2026-10-14.md). The C rs_new stays
  for the C's own string creators.
- bench/perf/sysio/windows-ci.sh runs the sysio Windows measurement on
  CI's Windows runner (`-f suite=script -f runner=xo-windows`).
- `XO_LEAKCHECK=1` on a native program that ends with `os.proc.exit` (or
  whose main returns an error) prints `xo leakcheck: skipped
  (os.proc.exit)` instead of nothing: the report runs only when main
  returns (gotchas/leakcheck-skipped-at-exit.md).
- Native code generation (decision 0228, zero-C audit item B06 part 2):
  a failed check (overflow, division by zero, an index out of range, a
  shift, a Float to Int conversion) and `fault(msg)` call the runtime's
  Xo exports `xo_x_fault_raise_*` directly; the C entry points forward
  to them until the next seed. The fault paths on the Mac are 0.962 to
  1.036 of main's instructions.
- Native runtime (zero-C audit item B16, option D): macOS on x86_64
  makes its file calls from Xo too (std/native/llrt/darwin.xo, through
  libSystem's stat64, lstat64, fstat64, and __getdirentries64, which
  both macOS architectures now use), so the C Fs operations
  (std/llrt/xo_fs.c) and the C fs.read are deleted: 831 lines net.
  macOS figures unchanged (bench/results/fs-ops-2026-10-14.md).
- Native runtime (decision 0227): the C path of the number and time text
  (`XO_LLRT_C=text`, std/native/llrt/text_c.xo and 514 lines of
  std/llrt/xo_std.c) is deleted; the owner accepted Duration text at
  1.112 of the C's instructions on arm64 with 0.925 on LLVM.
  `XO_LLRT_C=text` now fails the build.
- Native runtime (zero-C audit item B06, part 1): a context's fault
  record and the report of an uncaught fault are Xo
  (std/native/llrt/fault.xo); xo_rt.c loses 57 lines and gains 23. The
  fault paths on the Mac are 0.993 to 1.006 of main's instructions. A
  new test raises a fault through the C library's qsort, covering the
  unwinder's shared object lookup on macOS and Linux.
- Native runtime (decision 0221 point 10): a fault on macOS arm64
  unwinds through the Xo walker, which reads the compact unwind of
  `__unwind_info` itself, instead of libunwind. bench/perf/fault is
  0.083 to 0.123 of libunwind's instructions on the Mac. C frames
  between a fault and its catch are passed over on macOS arm64 too.
  darwin/amd64 keeps libunwind until stage 2.
- Native runtime (zero-C audit item B16, part 2): on Windows the Fs
  operations past read, write, and stat (directory names, rename,
  remove, set_modified, file handles, copy, modes, links, locks) call
  kernel32 from Xo (std/native/llrt/windows.xo under fsops.xo), with
  the C path's errors and messages. xo_win.c's POSIX shims for them are
  deleted (260 lines); xo_fs.c's operations stay only for darwin/amd64.
  macOS figures unchanged (bench/results/fs-ops-2026-10-14.md).
- Native runtime (zero-C audit item B02, part 2): the Bytes primitives of
  the system call layers (`bytes_addr`, `bytes_buf`, `bytes_len`) are Xo
  in std/native/llrt/raw.xo, and the per thread system call scratch is
  256 bytes of the thread's heap record (obj.xo `scratch`) in place of a
  C `__thread` buffer; std/llrt/xo_sys.c loses 48 lines net.
- Native code generation (decision 0221 point 7): the unwind tables name
  the runtime's Xo personality export `xo_x_personality` directly, except
  on Windows and x86_64. The C forwarder `xo_personality` stays for
  code built by the 0.11.0 seed and goes after the next seed.
- Native runtime (zero-C audit item B05, decision 0221 stage 3): a fault
  on glibc linux/arm64 unwinds through the Xo unwinder
  (std/native/llrt/unwind.xo) instead of libgcc's forced unwind; shared
  objects' unwind tables are found through ld.so's link_map. On macOS
  the personality routine is Xo over libunwind. C frames between a
  fault and its catch are passed over on Linux arm64 and their cleanups
  no longer run (only the runtime's own C can sit there; `use c` cannot
  call back into Xo). The TSan test builds drop function entry and exit
  instrumentation. bench/perf/fault is 1.001 to 1.009 of the C's
  instructions on the Mac and 0.405 to 0.624 of libgcc's on Linux arm64
  (bench/results/unwinder-glibc-2026-10-14.md).
  glibc linux/amd64 keeps libgcc until stage 2.
- Native runtime: the C the 0.10.0 seed still called is deleted now that
  seed 0.11.0 is out (xo_static_word, xo_ct_barrier and xo_crypto.c,
  the print, exit, env, and rune count forwards; about 200 lines), and
  `Stdio.write` writes through std/native/llrt/stdio.xo instead of the C
  `xo_print` (gotchas/runtime-c-the-seed-still-calls.md).
- Native runtime (zero-C audit item B13, stage 2; decision 0226): the
  list objects (a new list, growth, the copy of a shared list, the
  release of a list's elements) and the drop worklist are Xo
  (std/native/llrt/obj.xo). arm64 generated code calls them directly,
  with no C level: they find the thread's heap themselves, and their
  common cases make no call. The C entry points forward to them for
  LLVM and the seed. bench/perf alloc, nested, text, json and xo are
  0.883 to 1.021 of the C's instructions on arm64 and LLVM, the self
  build 1.000 (bench/results/string-list-objects-2026-10-14.md). The
  string objects stay C.
- Native runtime (zero-C audit item B14): the test world's part 1 is
  Xo (std/native/llrt/testworld.xo): `xo test --real`'s switches, seeded
  Rand, the generators of property tests and testing.gen, captured
  output, FakeEnv, FakeProc.exit_code and on_run, FakeStdio.set_input,
  log records, FakeNet's handlers, failures, and requests, MemFs, and
  the cleanup; std/llrt loses 916 lines and gains 87. Test reports are
  unchanged (TestNoGCTests, TestNoGCTestLeaks on both native backends);
  instructions in bench/results/test-world-2026-10-14.md (every test within
  5 percent of the C, rand 1.006 and gen 0.924 on arm64). The virtual
  clock, in memory listeners, and testing.run_main stay C (B15).
- Compiler: a constant Int expression (`const MIN_INT: Int =
  -9223372036854775807 - 1`, `5 - 1`) is one constant at every use, not
  its operations; testing.gen's generators no longer record their
  choices (only property tests report them).
- arm64 backend (decision 0224): shrink wrapping and tail calls. A
  function makes its frame (frame record, callee saved registers) only
  on the paths that need it, and a call whose results are returned as
  they are is a branch after the epilogue. The self build of xo runs
  0.985 of the instructions; bench/perf 0.963 to 1.004 of before; the
  Xo list objects of B13 move from 1.242 to 1.201 (alloc small) of the
  C's instructions on the Mac (bench/results/shrink-wrapping-2026-10-14.md).
- Tests: `go test -short` over the repository 2.2 times faster (35.0 s
  to 15.7 s on the shared Mac) and `./cmd/xo -short` 4.3 times (24.6 s
  to 5.7 s): every cmd/xo test is parallel (a per test environment for
  the xo processes, cmd/xo/env_test.go), internal/llvm caches its C
  compiles by content (XO_CC_CACHE), internal/gen keeps `use go`
  bindings and generated programs across runs, the harness self test
  runs per method in parallel; scripts/seed-pack.sh takes
  XO_SEED_GZIP_LEVEL (bench/results/short-tests-2026-10-14.md).
- Faster native rebuilds, reference counting per function (decision
  0220 addendum stage C): each function's reference counting is cached
  beside its assembly, keyed by the function before the pass, the types
  it reaches, and the borrowed parameters it calls with; verify checks
  it too. A one-module edit of xo now rebuilds in about 12 s (31 s
  without the cache) and 20 to 25 s of CPU (131 s) on the Mac.
- Faster arm64 rebuilds, per function (decision 0220 addendum stage B):
  each function's assembly is cached in <user cache>/xo/backend under
  the hash of its SSA, the types it reaches, the signatures of the
  functions it names, and the compiler, so a rebuild generates only the
  functions an edit changed; `XO_BACKEND_CACHE=verify` generates every
  function and fails on a cached text that differs, `off` turns the
  cache off, and all three give the same binary. After a one-module
  edit of xo the emit phase drops from 6 to 9.5 s to about 0.2 s and
  the build's CPU time from about 140 s to about 65 s on the Mac.
  --release builds do not use it.
- Faster arm64 rebuilds (decision 0220 addendum stage A): the parts of a
  large program's assembly are cut by their contents and named by their
  first function, and each part's object is cached in
  <user cache>/xo/backend (`XO_BACKEND_CACHE=off` turns it off), so an
  unchanged part is not assembled again: after a one-module edit of xo
  the assemble phase drops from about 6 s to 1.5 s on the Mac, and the
  binary is the same as without the cache
  (bench/results/fast-builds-2026-10-13.md).
- Native runtime (zero-C work item B16, part 1): on Linux and
  darwin/arm64 every Fs operation past read, write, and stat (read_dir,
  walk, rename, remove, remove_all, create_dir_all, create_temp,
  cwd_path, set_modified, file handles, copy, modes, links, and locks,
  with lock's 10 ms polled wait) makes its system calls from Xo
  (std/native/llrt/fsops.xo over linux.xo and darwin.xo); results and
  messages are unchanged, read_line costs 0.68 to 0.82 of the C's
  instructions, the rest within noise
  (bench/results/fs-ops-2026-10-14.md). Windows and darwin/amd64 still
  use the C runtime's xo_fs.c.
- Native runtime (zero-C audit item B13, stage 1): std/http's parser
  and client search text with std/native/llrt/scan.xo (SIMD) instead of
  the C `xo_str_find`, which is deleted; single byte searches scan only
  their range. The stack trace's padding counts runes in Xo. HTTP/1.1
  server 0.961 to 1.013 of the C's instructions
  (bench/results/string-list-objects-2026-10-14.md). The string and
  list objects were ported to Xo and measured, and stay C: on arm64
  they took up to 1.27 times the C's instructions (no shrink wrapping
  in the arm64 emitters); the results file ranks the next steps.
- Native runtime (zero-C audit item B11): the HTTP/2 client's header
  blocks are encoded and decoded by Xo (std/native/llrt/hpack.xo, which
  the server already used); the client's request fields are listed in
  Xo (http1.xo `h2_exchange`) and the C HPACK copy leaves the runtime
  (xo_h2.c 3,606 to 3,056 lines). HTTP/2 client requests measure 0.982
  to 1.009 times the C's instructions
  (bench/results/http2-client-hpack-2026-10-14.md; new h2len and h2big
  workloads in bench/perf/httpclient).
- Native runtime (zero-C audit item B09, decision 0222): a value barrier
  for constant time runtime code (`rt.ct_barrier`, internal: no
  instruction on arm64, an empty asm on LLVM) and constant time masks in
  std/native/llrt/ct.xo; `crypto.equal`, verify_password's comparison,
  and Ed25519 (std/native/llrt/ed25519.xo) are Xo, the same results, the
  C deleted (std/llrt/xo_crypto.c 540 lines to 16). Ed25519 takes 0.944
  to 0.971 of the C's instructions on arm64 and 0.757 to 0.768 on LLVM
  (bench/results/ed25519-consttime-2026-10-14.md) and now builds for
  wasm too. `xo dev codegen` patterns can
  name another module's function with a `// func MOD.NAME` line
  (test/codegen/consttime). Tests: internal/llvm TestConstantTimeLLVM
  (clang -O2 keeps the barriers; no branch or select in the masks) and
  TestCryptoXoDiff (crypto.equal and Ed25519 against Go on both native
  backends).
- xo prunes its caches by age and size again (decision 0223): the build
  directories and the Go backend's cache (whose trims went away with the
  Go driver), the binary cache, and the test harness's xo builds
  (`checkout/<key>`, the newest 4 kept). Entries unused for 24 hours go
  (`XO_CACHE_MAX_AGE`, e.g. `6h`, `0` off), then the least recently used
  past 20 GiB per cache (`XO_CACHE_MAX_GB`); nothing used within two
  hours is removed, and a prune runs at most once an hour.
- Native runtime (zero-C audit item B03, stage 1): standard output's
  64 KiB buffer, its flush, and standard error are Xo
  (std/native/llrt/stdio.xo, process globals in rt.static_word);
  Stdio.print and friends call it through the native prelude, and the C
  runtime flushes through cabi.xo's exports. 1,000,000 println cost
  0.788 (arm64) and 0.859 (LLVM) of the C path's instructions
  (bench/results/stdio-2026-10-14.md). os.proc.exit, env.get, env.vars,
  env.args, and env.program are Xo too (std/native/llrt/process.xo).
- Runtime process globals (zero-C work item B02, decision 0207): an
  internal `rt.static_word(k)` intrinsic (64 zeroed words) in the arm64,
  LLVM, and wasm emitters; std/log's state (log.configure's level,
  format, and output; the dependency attribution of decision 0173 as a
  thread local word; the record time) is Xo, and the 66 line logging
  section of the C runtime is gone. The wasm backend now lowers the
  runtime's thread local words and runs std/log programs.
- Native runtime (zero-C audit item B12): the HTTP/1.1 response writer
  is Xo (std/native/llrt/http1w.xo: status text, DetectContentType, the
  Date header, chunkWriter.writeHeader's rules, the error answers),
  called by the C connection code through cabi_http.xo's exports; HTTP/2
  sniffs and dates through it. 514 lines of xo_http.c deleted; the bytes
  on the wire are unchanged; http workloads 0.976 to 1.001 of the C's
  instructions on HTTP/1.1, HTTP/2 within noise
  (bench/results/http-writer-2026-10-14.md).
- Programs linked with no C library (XO_NOLIBC=1) on linux/arm64 unwind
  faults: deferred code runs and tasks and `task.isolate` catch faults,
  through an unwinder written in Xo (std/native/llrt/unwind.xo:
  .eh_frame_hdr lookup, DWARF CFI interpreter, the personality routine;
  decision 0221). The 12 fault goldens that ended at the fault now pass
  on the arm64 and LLVM backends; xo_nolibc.c loses its unwinder stubs.
- Native runtime (zero-C audit item B10): std/http's client reads
  HTTP/1.1 responses, follows redirects, and parses URLs in Xo
  (std/native/llrt/http1.xo `client_do`, `parse_response`); the C keeps
  the connection, TLS, HTTP/2, and the request's writing. The HTTP/2
  server's request URLs are parsed by Xo too. 1,629 lines of C leave the
  runtime (the C request parser stays as test data for the C fuzzers);
  the client measures 0.953 to 1.001 and the HTTP/2 server 1.006 to
  1.008 times the C's instructions (bench/results/http-client-2026-10-14.md),
  and its answers to 457 responses match the C client's byte for byte.
  Chunked bodies decode faster (5 to 6 percent fewer instructions for
  the server's chunked requests).
- Version 0.10.0 and its prebuilt seeds (decision 0216); the C Rand
  that only the 0.9.0 seed called is deleted (113 lines of xo_crypto.c
  and xo_win.c).
- Smaller native output and faster x86 self builds (decision 0220):
  reference counting releases a dead struct once instead of once per
  heap field (arm64 assembly of xo 17 percent smaller, 73 percent fewer
  calls; LLVM IR 72 percent smaller); `XO_LLVM_OPT` sets clang's level,
  and development builds of xo on x86 use -O0 (xo rebuilds itself in
  1:23 on c7i.8xlarge, was about 42 minutes); parallel jobs default to
  the physical cores and the memory.
- Native runtime (zero-C audit item W1): the scheduler's C fallback,
  `XO_LLRT_C=sched`, is deleted: 1,284 lines of std/llrt/xo_rt.c (its
  fiber switch assembly, thread locals, run queue ring, worker pool,
  parks, fiber stacks, spawn and join copies), std/native/llrt/sched_c.xo,
  `rt.sched_c`, and -DXO_SCHED_C. The scheduler is sched.xo and pool.xo
  only; `XO_LLRT_C=sched` now fails the build. The live C path names
  are net and text.
- Fixes from the Linux arm64 milestone run of 2026-10-14: `--ld=macho`
  links every object of a program whose assembly was split into parts
  (decision 0220; it took only the first and rejected the runtime's
  source); `xo bind` finds or builds the Go binder xo-gobind from the
  std beside xo instead of "std" in the working directory. Tests:
  TestDevGoldensELF builds the linux/arm64 runtime pack it links with
  (`xo dev rtpack`, cached by pack key); TestCrossCompile looks up the
  wasm binary by xo's name, wasip1/wasm (xotool.Target);
  TestSelfhostLLVMEndToEnd compares its IR with the recording only when
  dump replays are on (decision 0217) and keeps its build and output
  check. From the linux/amd64 milestone: internal/llvm's diff tests skip
  the arm64 backend off arm64 hosts (xotool.Arm64Skip), and TestHarness
  builds xo before the Python self test (its 600 s adapter timeout no
  longer includes the build).
- Native runtime (zero-C audit item B08): SHA-512 is Xo
  (std/native/llrt/digest.xo, called by the C Ed25519 through
  cabi_crypto.xo's export; 0.887 of the C's instructions on arm64, 0.641
  on LLVM, bench/results/sha512-rand-2026-10-14.md) and Rand is Xo
  (std/native/llrt/rand.xo over getrandom, getentropy, BCryptGenRandom,
  or the test world's seeded draws, the same values as before). The C
  SHA-512 is deleted; the C Rand functions stay until the seed is a
  release with this change (gotchas/runtime-c-the-seed-still-calls.md).
- The clocks are read from Xo on every OS (work item B01, decision
  0213 item 6): Clock.now on macOS (clock_gettime_nsec_np) and Windows
  (GetSystemTimePreciseAsFileTime), and the monotonic clock of the
  scheduler, timers, and budgets everywhere (the vDSO on Linux,
  libSystem on macOS, QueryPerformanceCounter on Windows), the clock of
  the build chosen once into a word so the hot path tests no target.
  The C runtime's mono_ns, xo_clock_now, and xo_tz_now call the Xo
  exports; xo_win.c's clock_gettime shim is deleted. Clock.now on macOS
  takes 0.969 (arm64) and 0.977 (LLVM) times the C path's instructions,
  on Windows (LLVM) 0.928 to 0.952 times its CPU cycles
  (bench/results/clocks-2026-10-14.md). bench/perf/sysio gains a sleep
  workload.
- Native runtime: the C with no caller is deleted (zero-C audit item
  W0): xo_utf8_valid, xo_str_contains, starts_with, ends_with, split,
  lines, xo_bytes_span, rune_count, and hex (141 lines of std/llrt,
  their std/native/rt.xo declarations), and the -D flags of deleted C
  paths that xo's io_flags still added. A build with an `XO_LLRT_C`
  name that has no C path (no `std/native/llrt/<name>_c.xo`; `all`
  aside) now fails with `XO_LLRT_C=<name>: no C path named <name>`
  instead of silently building the Xo path; the live names are net,
  sched, and text.
- Faster native builds (decision 0220 stage 1): the arm64 backend
  generates a large program's functions in parts on parallel tasks and
  assembles the parts apart before one link; reference counting and the
  inlined module's verification run on tasks too; the runtime flags come
  from one scan of the assembly. xo builds itself in 59.00 s on the
  development Mac (was 337.28 s) and 49.83 s on r8g.8xlarge with
  `XO_EMIT_JOBS=16` (was 496.82 s), the output byte identical for any
  job count. `XO_EMIT_JOBS` sets the tasks (default the cores, at most
  8); `--phase-times` prints the `assemble` phase.
- Developer tooling (decision 0209): hidden `xo dev goldens|errorcheck|
  codegen|dump|rtpack|build-each` in xo. Golden programs are built,
  run, and compared byte for byte by xo itself, in parallel workers,
  largest first, with passes cached by input hash (arm64 set: 49.74 s
  with the Python script, 16.93 s with `-j 8`, 0.21 s cached). Error
  check and codegen pattern suites start in test/ (decision 0217).
  selfhost/macho assembles `svc`, so linux/arm64 `--ld=macho` builds
  work again.
- `xo lsp` answers `textDocument/completion` and
  `textDocument/signatureHelp` (decision 0219): fields and methods
  after `x.` (prelude methods included), variants and statics after a
  type name, public members after a package name, and elsewhere the
  locals in scope, the module's names, imports, prelude names, and
  keywords, with the hover's declaration line and doc comment, ranked
  first when their type is the one expected at the cursor; signature
  help shows the callee and its active parameter. A dangling `.` or an
  open call no longer needs to parse: the server checks a copy patched
  at the cursor.
- std: `Fs.cwd_path()` is the working directory's absolute path from the
  OS (decision 0218), on the Go, arm64, and LLVM backends; a test's MemFs
  gives `/`. The toolchain uses it instead of `$PWD`, so `xo lsp` started
  without a shell (VS Code from the macOS Dock passes `PWD=/`) resolves
  std paths again and Go to Definition into std works.
- Prebuilt seeds (decision 0216 item 1): a checkout with no xo fetches
  the newest seed (release N-1) from a private S3 bucket with the aws
  CLI, checks its sha256, and builds its xo in one hop
  (internal/xotool); a failed fetch is an error naming the cause, and
  the go-compiler-final root and the seed chain run only when asked for
  (`XO_SEED_FALLBACK=chain`, `go run ./scripts/xobuild --from-source`). `scripts/seed-pack.sh XO OUTDIR` packs a built xo as a
  seed. `XO_SEED_BUCKET` and `XO_SEED_URL` choose the location.
- The Go implementation of the compiler is deleted (decision 0215): Go
  cmd/xo, the hidden comparison dumps, and the Go compiler packages
  (lexer, parser, checker, SSA builder, Go, arm64, and LLVM emitters,
  build driver, modules, and the tools), 95,011 lines of non-test Go
  and 18,837 of tests. `xo` is the self hosted toolchain alone; the
  last commit with the Go compiler is tagged `go-compiler-final`. A
  checkout's `xo` is built by an earlier xo (`go run ./scripts/xobuild`;
  with none, by the xo1 of that tag). The Go backend stays (selfhost/gen
  and std/rt), as does `use go` (xo-gobind). The playground's compiler
  half (cmd/xoplay) is gone with it: the site builds it from the tag
  until an Xo replacement exists.
- Native runtime: the C paths of the runtime pieces ported to Xo that
  passed the 2026-10-13 batch 8 full tier are deleted (decision 0198,
  second round): tz, report, show, atomic, fspath, gen, glob, digest,
  ints, rc, alloc, scan, sysio, http1, hpack, and procspec, with their
  `std/native/llrt/<name>_c.xo` modules and their `-DXO_*_C` C: std/llrt
  is 3,223 lines shorter. `XO_LLRT_C` naming one of them now selects
  nothing; only `text`, `sched`, and `net` keep a C path.
- Native backends on Linux: the network's system calls (socket, bind,
  listen, accept4, connect, reads, sends with MSG_NOSIGNAL, socket
  options, names, shutdown, close, and the poller's epoll) are made from
  Xo (std/native/llrt/sock.xo), and host names are resolved by a
  resolver written in Xo (std/native/llrt/dns.xo) instead of glibc's
  getaddrinfo: /etc/nsswitch.conf's files and dns, /etc/hosts,
  /etc/resolv.conf (nameservers, search, ndots, timeout, attempts,
  rotate, single-request, use-vc), A and AAAA over UDP with TCP on
  truncation, and RFC 6724 ordering, following Go's resolver with
  glibc's error texts (decision 0206 stage 3). Its differences from
  glibc are listed at the top of dns.xo. `XO_LLRT_C=net` builds the C
  library path. With `XO_NOLIBC=1`, programs that dial or listen now
  link and run with no C library (std/http still needs it until
  decision 0202).
- Linux: Clock.now calls the kernel's vDSO from Xo instead of the C
  library (0.864 to 0.967 of the C path's instructions,
  bench/results/linux-syscalls-2026-10-13.md); `XO_LLRT_C=sysio` keeps
  the C clock. Static programs (`XO_NOLIBC=1`, decision 0206) take their
  clock, locks, signal handlers' installation, threads, and thread local
  storage layout from Xo, and link again with the arm64 backend (they
  failed on libgcc's atomics helpers). Output is unchanged.
- Fixed: ThreadSanitizer builds on Linux ran out of address space for
  task stacks under load (`fault: out of memory` in conc_stress, not a
  leak); their task stacks are now 2 MiB (XO_TASK_STACK_MB overrides).
  `XO_RTSTATS=1` reports `fiber_live` and `fiber_peak`, and a failing
  task stack mapping says how many stacks were mapped
  (knowledgebase/gotchas/linux-tsan-fiber-stacks.md).
- Native backends on macOS (arm64) and Windows: `os.fs` read, write,
  and stat call libSystem and kernel32 directly from Xo (decision 0213)
  instead of going through the C runtime, with the same errors and
  messages byte for byte; on macOS within the C path's instructions
  (0.562 to 1.013 times, bench/results/platform-calls-2026-10-13.md).
  `XO_LLRT_C=sysio` builds the C path.
- The self-hosted toolchain's Go backend holds the shared lock on the Xo
  Go cache while go build runs, as the Go implementation does, so a
  concurrent cache trim no longer removes entries a build is reading
  (decision 0214). The lock comes from `std/internal`, a module only std
  and the toolchain can import; Xo's public `fs.lock` is unchanged.
- `xo` run with no std library to find (no $XO_STD and no release
  layout beside the binary), or with a std whose prelude lacks a core
  type, now says so (`std library not found: ...`, exit 2) instead of
  faulting in the checker with `prelude type missing: Int`.
- Native backends: the rest of the task scheduler's machinery is Xo
  (std/native/llrt/pool.xo, decision 0212 stage 2): the worker pool
  (waking, idling, stealing, the inject queue), the runtime locks' slow
  paths and their futex waits (libSystem's __ulock, WaitOnAddress,
  futex(2)), worker thread starts (pthread_create from Xo on macOS,
  CreateThread on Windows), task stacks with their guard pages and
  caches, parks, the cancellation tree, a spawn's queueing, and a
  scope's join. Within the 5 percent bar on both backends
  (bench/results/task-switch-2026-10-13.md, Stage 2); `XO_LLRT_C=sched`
  builds the C path. The C runtime's default build is 523 lines shorter.
- Native backends: the task switch is emitted by the compiler (no
  assembly left in the C runtime's default build), and the scheduler's
  core (each worker's run queue, starting, parking, and waking fibers,
  task states) is Xo (std/native/llrt/sched.xo, decision 0212), called
  from the C runtime through C ABI exports; thread local state lives in
  a table the compiler emits. Within the 5 percent bar on both backends
  (bench/results/task-switch-2026-10-13.md); `XO_LLRT_C=sched` builds the
  C path. Fixed on the way: `--as=macho` builds failed on `prfm` (since
  the heap moved to Xo) and on `sxtw` (http programs).
- Native backends: the memory allocator (size classes, free lists,
  chunks, and the magazines that hand freed blocks between threads) is
  Xo (std/native/llrt/alloc.xo, decision 0207 stage 3), called from the
  C runtime through C ABI exports; within 5 percent of the C's
  instructions on both backends and below it on LLVM
  (bench/results/raw-memory-2026-10-13.md, bench/perf/alloc).
  `XO_LLRT_C=alloc` builds the C path.
- Faster list code on the LLVM backend (decision 0211): the compiler
  tells LLVM which lists cannot share storage (a `var` list once made
  unique, a new list, the borrowed parameters, as Xo's value semantics
  guarantee), so it keeps lengths, drops index checks, and vectorizes
  code that writes one list while reading another. Argon2id and
  BLAKE2b in std/crypto are Xo again on the native backends, at 1.043
  times the C's instructions on LLVM and 0.980 on arm64
  (bench/results/llvm-noalias-2026-10-13.md); `XO_LLRT_C=digest` builds
  the C. Output is unchanged.
- Fixed: go commands that ran on the Xo Go cache outside an Xo build
  (`go vet` in tests, the Go binder's `go list`, tool builds) could lose
  cache entries to a concurrent trim and fail with `could not import
  strings (... no such file or directory)`; they now hold the cache's
  shared lock (gotchas/go-commands-outside-build-race-trim.md).
- Native backends: the C runtime's reference count operations (retain,
  release, claim, and marking a value shared between threads) are Xo
  (std/native/llrt/rc.xo, decision 0207 stage 2), called from the C
  through C ABI exports; within 5 percent of the C's instructions on
  both backends (bench/results/raw-memory-2026-10-13.md).
  `XO_LLRT_C=rc` builds the C path. Lists of zeros made by std's
  implementation (deflate's tables, regexp's queue, JSON object key
  tables) take one allocation and a memset instead of a push per
  element. Fixed: the wasm backend reported std code using the raw
  memory intrinsics as unsupported.
- Added: `./...` and `DIR/...` on every command that takes directories
  (check, test, build, vet, doc, fmt, fix, parse, audit). It means DIR,
  which those commands already walk below, so `xo check ./...` equals
  `xo check .`; a `DIR/...` with no `.xo` file under it is an error
  (exit 1) (decision 0210).
- Changed: directories whose name starts with `_` are skipped like
  `testdata` and `.` names, as the go command skips them (decision 0210).
- Fixed: `xo help` lists every command with its purpose, and every
  command takes `-h`/`--help` (usage on stdout, exit 0), also after a
  subcommand (`xo edit rename --help`); `xo` alone and `xo build -h` no
  longer print the internal xo1 usage line or its internal flags; an
  unknown flag exits 2 with the command's usage on stderr.
- The LLVM backend lowers std's internal spin loop hint to the target's
  pause instruction (`yield` on arm64, `pause` on x86-64) instead of
  nothing (decision 0200).
- Windows builds are deterministic: the link passes lld-link `/Brepro`,
  so rebuilding the same sources gives a byte identical `.exe` (the COFF
  time stamp is a hash of the output, not the link time; decision 0197).
- Native backends: more of the C runtime is Xo (decisions 0204, 0207):
  the text of every arithmetic, index, and conversion fault, Int and
  Float text, UTF-8 decoding, white space, Int parsing, Duration and Time
  text used by the runtime itself, `Net.only`'s address checks, and the
  TLS client's and server's name checks, PEM decoding, and alert texts.
  Messages are unchanged byte for byte; TLS name checks now follow Go's
  `net.ParseIP` exactly. Under the hood the C runtime can call runtime
  functions written in Xo, and std's implementation gets raw memory
  intrinsics (internal only). A program built with the arm64 backend is
  about 20 KB larger (LLVM builds are not).
- Added: on Linux, the native backends' os.fs.read, write, and stat
  (and read_dir's and walk's stats) make their system calls from Xo
  (std/native/llrt/linux.xo and sysio.xo, decision 0206) with the C
  runtime's semantics and messages: reads take 0.45 to 0.67 times the
  C path's user instructions on Graviton, the rest within 1 percent
  (bench/results/linux-syscalls-2026-10-13.md). `XO_LLRT_C=sysio`
  builds the C path. Clock.now stays on the vDSO.
- Added: `XO_NOLIBC=1` (experimental, Linux, xo1's arm64 and LLVM
  backends): a static executable with no C library at all (decisions
  0203, 0206 stage 1); a hello world and a file and clock program run in
  an empty root on linux/arm64 and linux/amd64. Faults end such a
  program without running deferred code; sockets past epoll, DNS,
  std/http's TLS, and `use c` still need the C library.
- Faster list code on arm64 (decision 0208): an element access is one
  load or store with an immediate or a scaled register offset (the
  list's data address computed once per loop when the loop cannot move
  the list), bounds checks a guard before an `a || b` proved stay
  removed, 32 by 32 bit products of masked values are `umull` and
  `umaddl`, and a loop that stores into a list it received shared makes
  it unique once before the loop instead of checking at every store
  (all native backends). Argon2id written in Xo went from 2.21 to 1.13
  times the C's instructions on arm64 (1.29 to 1.25 on LLVM); it stays
  C. Output is unchanged.

- Faster on the native backends: Str.split, Str.lines, Str.rune_count,
  Str.find and contains, Bytes.to_hex, Bytes.rune_count, and
  Bytes.to_str (UTF-8 validity) are now Xo in the runtime, scanning 16
  bytes a step with NEON on arm64 and vector compares on LLVM (decision
  0201). They retire from 0.32 to 1.01 times the instructions of the C
  they replace (rune_count about a third); `XO_LLRT_C=scan` builds the
  C path for comparison. Output is unchanged.
- Faster: `Atomic[T]` on the native backends no longer calls the C
  runtime: get, set, swap, and compare_and_swap are one inline atomic
  instruction each (get is a single `ldar` on arm64) and add and max are
  Xo code (std/native/llrt/atomic.xo), 0.60 to 0.94 times the C path's
  instructions (bench/results/atomics-2026-10-13.md; `XO_LLRT_C=atomic`
  restores the C). Under the hood, std's implementation modules get
  atomics with C11 memory orders (decision 0200, internal only).
- **Sized integers are much faster on the native backends, and more of
  the runtime is Xo** (decision 0195's portable group): Int8 to UInt64
  arithmetic, shifts, and conversions are checked inline instead of a C
  call each (5 to 20 times fewer instructions on bench/perf/portable),
  Float32 operations round inline, and the faults, UInt64 division,
  `Time.unix`, the Str Map key hash, Int's checked_ and saturating_
  methods, MemFs.fail_next's patterns, SHA-256, HMAC, PBKDF2,
  verify_password's parsing, and the values of property tests are Xo
  (std/native/llrt/ints.xo, glob.xo, digest.xo, gen.xo). Output is
  unchanged (TestIntsDiff, TestDigestDiff, TestGlobDiff, TestGenDiff
  against the Go backend). Argon2id stays C (its Xo version was 2.07
  times the C's instructions on arm64). `XO_LLRT_C=ints,glob,digest,gen`
  builds the C paths for comparison
  (bench/results/runtime-port-portable-2026-10-13.md).

- Changed: on the native backends, the HTTP/2 server's HPACK (header
  block decoding and encoding), the HTTP/1.1 server's request parsing
  (request line, header fields, request targets and queries, chunked
  bodies and trailers, and std/http's URL helpers), the Fs path walks
  (create_dir_all, remove_all, create_temp, the working directory), and
  the environments and Windows command lines of child processes are Xo
  (std/native/llrt hpack.xo, http1.xo, fspath.xo, procspec.xo; decision
  0193 step 6), as fast as the C or faster (HTTP/1.1 GETs 13 to 16
  percent fewer instructions, HTTP/2 within 2 percent;
  bench/results/runtime-port-step6-2026-10-13.md). `XO_LLRT_C=hpack`,
  `http1`, `fspath`, `procspec` build the C paths for comparison.
- Fixed: `--backend=arm64` (the Go driver and xo1) failed with
  "internal: reads virtual register N before it is written" on a loop
  that clears a flag and breaks, followed by a test of the flag and code
  that goes on from the loop's index; a block entered through a Bool
  join (`a && b`, `a || b`, such a flag) now reuses only the registers
  of the block that dominates it (gotchas/arm64-vreg-read-before-write-break-flag.md).
- Fixed: a chunked request body with a chunk size of 2^63 or more made
  the native HTTP/1.1 server copy a negative length; an IPv6 request
  host with an IPv4 part written with a leading zero is now rejected as
  the Go backend rejects it.
- **The xo1 golden scripts no longer report show_table as failing**:
  scripts/selfhost-goldens.py (and selfhost-gogoldens.py,
  elf-goldens.py) read want.txt in Python's text mode, which turned the
  raw `\r` show_table prints into `\n`, so the golden "differed" on xo1
  although xo1's output matched it byte for byte. They now read it with
  `newline=''`; internal/diag's TestScriptsReadWantBytes keeps it so
  (gotcha python-text-mode-want-cr.md). No compiler change.
- **The C paths of the ported runtime pieces are gone** (decision
  0198): the Unicode operations (`XO_UNICODE=c`, xo_unicode.c), std/json's
  text (`XO_LLRT_C=jsontext`, xo_json.c), the number text
  (`XO_LLRT_C=num`), and the Str operations (`XO_LLRT_C=strings`), 3,373
  lines of std/llrt. Those settings now select nothing; the Xo versions,
  the default since decisions 0180 and 0193, are the only ones. No pack
  object changes (packs never held these paths).
- **xo for Windows can be built by Xo** (decision 0197 stage 7): `xo
  build --backend=llvm --targets=windows/amd64` (or windows/arm64) now
  cross builds from Linux or macOS when `XO_WINSYSROOT` names a Windows
  sysroot (MSVC's C library and the Windows SDK, e.g. made by xwin)
  and an upstream clang with lld-link is used. xo built this way from
  Linux runs on Windows, passes the 145 Windows goldens, and builds
  itself on Windows to the same output. Fixed: on Windows, `xo` started
  by name from cmd.exe did not find its standard library
  (`fault: prelude type missing: Str`).
- Faster: the self hosted compiler (xo1) compiles selfhost/xo 3.6 times
  faster in 37 percent of the memory (588.86 s to 163.83 s, 26.1 GB to
  9.66 GB on the Mac; bench/results/xo1-speed-2026-10-13.md), with the
  same assembly: liveness and dce by worklists over sparse sets, fewer
  allocations per instruction, insert_rc's liveness and borrow sites
  without per pass maps. On the native backends an empty list literal
  `[]` allocates nothing until its first push (decision 0199), and the
  heap prefetches on each allocation from a free list. `XO_HEAPSTATS=1`
  prints a native program's heap by size class at exit.
- Changed: the xo1 that native builds of the development `xo` run
  (switch point A) is cached on its own inputs (selfhost/, the std
  modules it uses, the `xo` version) in `<user cache>/xo/xo1/bin/<key>`,
  so a change to another std module, the C runtime (std/llrt), or
  third_party no longer rebuilds it (5 to 12 minutes); the std beside it
  is still written from the full embedded set.
- The self hosted toolchain runs on x86-64 hosts: on darwin/amd64 and
  linux/amd64, `xo build|run|test --backend=llvm` (and `--no-gc`, whose
  default backend is LLVM there) go through xo1, which `xo` builds with
  the LLVM backend split into 12 modules without LTO (decision 0178
  stage 7, decision 0196). That xo1 passes the LLVM goldens on both
  hosts and rebuilds itself to a byte identical fixed point on both
  with no Go on PATH. scripts/bootstrap-xo.sh picks the
  LLVM backend on x86-64 hosts; scripts/remote-xo1-x86.sh and
  scripts/xo1-host-check.sh run the check on EC2 and the Intel Mac.
- **More of the native runtime is Xo** (decision 0193 step 5): std/time's
  zone rules (TZif parsing, the POSIX TZ footer, the lookup and Go's
  cache) are std/native/llrt/tz.xo, the C keeping only the zone data
  and the table of loaded zones; a Rune's Display and Debug text and a
  UInt64's text (the leaves of the Debug text of collections) are
  show.xo; the report of a property test's case to the harness is
  report.xo. Output is unchanged (TestTZDiff over every zone of Go's
  database, the new goldens show_table and tz_table, TestNoGCTests).
  `XO_LLRT_C=show,tz,report` builds the C paths for comparison
  (bench/results/runtime-port-step5-2026-10-13.md).
- Fixed: in a native test (`xo test --no-gc`, arm64 and llvm), an http
  request to an in memory server sent after a stop request waited in
  the listener's backlog instead of failing, and under load could end
  the test with `XO-F001: deadlock`; it now fails at once, as on the Go
  backend (gotchas/mem-listener-backlog-after-serve.md). Also fixed: on
  a loaded machine the virtual clock of a native test could move twice
  for one idle instant, so a task woken by the first move saw the time
  of the second (gotchas/virtual-clock-double-kick.md).
- **A native build on macOS needs no C compiler** (decision 0182 next
  steps 1 and 2): `xo build --backend=arm64` (and run, test) now
  assembles and links with Xo's own Mach-O assembler and linker when the
  release's runtime pack covers the program and no `use c` source is
  linked; otherwise it uses cc as before. `--ld=cc` keeps cc, `--ld=macho`
  insists on Xo's linker. Linux hosts keep cc for now. Release archives
  carry Go's zone database as `lib/xo/zoneinfo.zip`, which `xo` reads
  before asking `go env GOROOT`, so programs that name time zones build
  with no Go installed. Fixed on the way: a native build for several
  targets of different OSes (`--targets=darwin/arm64,linux/arm64`) gave
  every target the host's predeclared `target`; each OS now gets its
  own; `xo rtpack --status` on linux/amd64 named linux/arm64.
- **`xo` is written in Xo** (decision 0178, switch point B): the release
  archives ship the self hosted toolchain as `bin/xo`, with
  `bin/xo-gobind` (for `use go`) and `lib/xo/` (std, third_party, the
  prebuilt runtime) beside it; keep that layout, `bin/xo` alone does not
  run. The Go `cmd/xo` only builds the first one: scripts/release.sh
  builds it, it builds the darwin/arm64 `xo`, and that `xo` builds the
  runtime packs, the linux/arm64 `xo` (linked by its own ELF linker, no C
  compiler), and the linux/amd64 `xo` (Go backend).
  `scripts/release-verify.sh` checks an archive on its host; cmd/xo's
  suite passes against the archive's `xo` on darwin/arm64 and
  linux/arm64. Fixed on the way: the ELF assembler's `.file`, glibc's
  `stat` family for binaries linked without cc (glibc before 2.33 has no
  `stat` in libc.so.6), runtime flags turned on by symbol names inside
  string literals, the predeclared `target` of a cross build (it was the
  compiler's), the Go backend and `--no-gc` of the self hosted `xo` on
  an amd64 host, and paths printed by `xo serve` on Linux.
- Windows, decision 0197 stages 5 and 6: native programs wait for
  sockets with AFD polls on a completion port (a round trip stays at
  44 us with 4000 idle connections, where WSAPoll took 11.9 ms; set
  `XO_WINPOLL=wsapoll` for the old poller), read standard input from a
  thread so reads can be cancelled, take the local time zone from the
  system setting (as the Go backend), read the environment as UTF-8,
  report a stack overflow as the Go backend does (`fatal error: stack
  overflow`, status 2), and no longer close a socket when closing one of
  hundreds of open files. The LLVM runtime tests and `xo test
  --backend=llvm` run on Windows; `scripts/remote-windows.sh --batch`
  runs the Windows checks of a batch (scripts/windows-batch.sh). Fixed:
  a native test's client waiting for an in memory listener that was
  freed slept forever (test_shutdown deadlocked on Windows).
- Windows paths (decision 0197 stage 4, std.md 3.1 and 3.5): on
  Windows `Fs` takes `\` as a separator, drive paths (`C:\x`) and UNC
  paths (`\\host\share`) as absolute, and a path rooted at `/` on the
  working directory's drive; `Env.program`, `home`, `config_dir`,
  `cache_dir`, and every path an `Fs` returns use `/`. Native builds
  read the home directory from `USERPROFILE` and the user directories
  from `%AppData%` and `%LocalAppData%` on Windows, as the Go backend
  did. Other OSes are unchanged. All 145 LLVM goldens pass on Windows
  built by the self hosted `xo`.
- Windows, decision 0197 stage 2: native programs run processes
  (`Proc.run`, `Cmd`, `Child`, pipelines) on Windows through
  `CreateProcessW` (PATH and PATHEXT lookup, arguments quoted as Go's
  EscapeArg and on line ends, only the three standard handles
  inherited); XO1504 no longer stops any Windows build. On Windows
  `Child.kill`, cancellation, and `Proc.kill(pid, "KILL")` end a process
  with status 137 on both backends (the Go backend's was 1), and other
  signal names are an `Io` error. The self hosted `xo` builds Windows
  programs too (`--backend=llvm` on a windows/amd64 host, and
  `--targets=windows/arm64` cross builds): 143 of 144 LLVM goldens pass
  built by it, and all 144 build for windows/arm64.
- The LLVM backend builds Windows programs (windows/amd64 host, decision
  0197 stage 1): `xo build --backend=llvm` with LLVM's clang
  (x86_64-pc-windows-msvc, lld) and the MSVC build tools. The runtime's
  POSIX use maps onto Win32 and Winsock (std/llrt/xo_win.c): threads,
  tasks on fibers, faults with deferred code, timers, files, sockets,
  HTTP and TLS, standard input. 140 of 144 LLVM goldens pass on Windows
  Server 2025. A program using processes (Proc) stops with the new
  XO1504 (a std API the native runtime does not support on the target
  yet). A Windows host also cross builds the other Windows architecture
  (`--targets=windows/arm64`: 141 of 144 goldens build and link; none
  run yet, for lack of a Windows on Arm host). scripts/remote-windows.sh
  runs a command on, or holds, a temporary Windows EC2 instance.
- The LLVM backend builds and passes the golden suite on darwin/amd64
  (decision 0196): on an Intel Mac with macOS 26.5.2 the whole
  internal/llvm package passes, TestGolden 143 of 143, with no emitter
  or runtime change. `scripts/intel-mac-test.sh` runs a test command for
  a commit on that Mac. The LLVM tests now drop AddressSanitizer where
  it builds but cannot start, as they did for ThreadSanitizer.
- The WebAssembly backend (decision 0179) coalesces each function's
  locals by liveness and lays out reducible functions as structured
  control flow (block, loop, br) instead of the dispatch loop: the self
  hosted checker's module is 8.3 MB (8.9 MB), runs seven goldens under
  Node in about 0.55 s and 340 MB (2.3 s and 2.1 GB), and checks all 144
  LLVM goldens in a Chrome worker (63 before, the rest overflowed the
  worker's stack). `os.fs.write` works in wasm (WASI `path_open` and
  `fd_write`).
- internal/selfhost scales with cores: every native build, Xo process,
  and Go side comparison draws from one pool of `XO_SELFHOST_JOBS`
  slots (default min(GOMAXPROCS, 4); scripts/remote-full.sh passes the
  core count), builds and the heavy runs also from
  `XO_SELFHOST_HEAVY_JOBS` (default one per 8 GiB of memory, 16 GiB in
  the full tier); heavy runs get a process each, backends and
  comparisons run at once, and the full tier prebuilds every component.
  The full tier takes 3,490 s on r8g.8xlarge with 32 jobs (over 3 hours
  before); scripts/remote-full.sh defaults to r8g.8xlarge (256 GiB).
  `-short` no longer samples the heavy runs (175 s to 31 to 52 s on the
  development Mac).
- The arm64 backend (both emitters) lowers a division or remainder by
  an Int constant to a multiply high and shifts instead of `sdiv`
  (decision 0194): faster, with the same faults (only the constants 0
  and -1 can fault). A negative literal is now a constant there (no
  overflow check). Duration text in Xo is still 1.171x of the C on
  arm64 and 1.087x on LLVM in instructions, so it keeps its C path.

- The self hosted `xo` can be bootstrapped without Go:
  `scripts/bootstrap-xo.sh SEED` builds selfhost/xo with a given xo and
  then with itself twice, with no Go on PATH, and checks the fixed point
  (identical assembly and binary); it passed on a clean Linux arm64
  machine with Go deleted (decision 0178 stage 7). Fixed on the way: the
  self hosted driver run with the default relative `--std` could not
  build the TLS library on an empty cache. The selfhost differential
  tests can record their Go side and replay it
  (`XO_SELFHOST_GOLDENS=record|replay`); internal/progen now gives one
  program per seed. The recording is committed as hashes (6 MB:
  internal/selfhost/testdata/gogoldens), per unit, and per function for
  the SSA, LLVM, arm64, and Go generator comparisons, so a replayed
  mismatch names the function; the mutation generators no longer need
  internal/lexer.

- The self hosted `xo` has `xo rtpack` (decision 0178): the prebuilt
  runtime packs scripts/release.sh ships, byte identical to the Go
  command's. Switch point B's gate passes: cmd/xo's whole suite against
  the self hosted binary laid out as a release, 69 of 71 tests passing
  and the two hidden comparison dumps (ssadump, llvmdump) exempt.
- `xo bind --c` is written in Xo (decision 0178 stage 6,
  selfhost/cli/bind): xo1 binds C headers with the same output and
  cache files, byte for byte, as the Go binder. `xo bind` for Go
  packages runs the new Go helper `xo-gobind` (cmd/xo-gobind), which
  stays Go. The self hosted checker now binds a `use go` package or a
  `use c` header missing from the binding cache, and fetches a module
  version missing from the module cache, before checking again, as the
  Go checker does; xo1 runs `xo publish` and prints the API changes of
  `xo get`. `xo publish` in a module of a repository subdirectory under
  a symbolic link (macOS's /var) now finds its tags.
- Linux fixes from the first full suite run on EC2 (Amazon Linux 2023,
  arm64; bench/results/linux-2026-10-12.md): the native runtime
  unregisters a socket or pipe from epoll when it closes it (a child
  process holding the file kept the registration, and the poller then
  touched freed memory) and makes descriptors close-on-exec at creation
  (SOCK_CLOEXEC, accept4, pipe2); HTTP/2 responses with no header fields
  no longer pass NULL to memcpy; Ed25519's constants initialize once
  (pthread_once) instead of racing; ThreadSanitizer builds on Linux use
  8 MiB task stacks; the arm64 backend's binaries are reproducible with
  gcc as `cc` (a fixed `.file`); the bin cache's copy fallback no longer
  makes a parallel exec fail with "text file busy". scripts/remote-full.sh
  gains `--cmd`, `--hold`, and per-package durations at the end of its log.

- The self hosted `xo` (xo1) closes the gaps of decision 0178's switch
  point B outside stage 6: `build`, `run`, and `test` default to the Go
  backend as `xo` does; std is found beside the binary (lib/xo/std of a
  release) through `os.env.program()`, without XO_STD; `xo check
  --no-gc` reports XO1201; `build --ci`, the workspace check of
  `--release`, and native `test --real` work; `xo lsp` answers hover,
  definition, references, rename, formatting, and semantic tokens as the
  Go server does; `xo replay` reads the rebuilt program's build
  information again. The Go `xo` can build xo1 from its embedded sources
  again (the nested selfhost/cli packages were missing).
- The server commands in Xo (decision 0178 stage 5, selfhost/cli/servers):
  xo1 runs `mcp`, `serve`, `dump`, `replay`, and `lsp` (diagnostics,
  quick fixes, symbols) with the Go servers' responses byte for byte,
  and forwards `check` and `query` to a running `xo serve`. Natively,
  `os.env.program()` builds, and removing from both a `Map[Str, Str]`
  and a `Map[Str, Bytes]` in one program no longer fails the SSA
  verifier.
- `os.proc.kill(pid, signal)` sends a signal (by name: `TERM`, `QUIT`,
  ...) to another process, and `os.net.listen` and `dial` take
  `unix:PATH` addresses for Unix domain sockets, on every backend; an
  attenuated `Proc` must list `"kill"`, an attenuated `Net` lists the
  socket as `unix:PATH`; `FakeProc` records signals (`killed()`) and
  `FakeNet` listens on sockets in memory (decision 0190).
- The core commands of `xo` are written in Xo too (decision 0178 stage
  2): `check`, `parse`, `explain`, `version`, `init`, `fmt`, `fix`,
  `vet`, and `doc` run in the self hosted driver xo1 with the same flags,
  exit codes, and output; the Xo formatter writes the same bytes as the
  Go one on every file of the repository. The self hosted checker now
  reports holes (XO0900). Two types of one name in different modules no
  longer share a generated SSA helper (the native build of such a
  program failed).
- The LLVM backend drops the overflow checks that value ranges prove
  (loop counters, binary search midpoints, operations under a branch or
  after an index), tells LLVM the ranges of loop variables, and inlines
  the functions the arm64 backend inlines (decision 0192): the Unicode
  operations written in Xo are now within 5 percent of their C versions
  on LLVM too.
- std/crypto signs and verifies Ed25519 (RFC 8032, decision 0189):
  `crypto.ed25519_public_key(seed)`, `crypto.ed25519_sign(seed, msg)`,
  `crypto.ed25519_verify(public_key, msg, sig)`, with the results of Go's
  crypto/ed25519 on every backend (native builds compute them in C).
- `archive.untar` skips a PAX global header (the one `git archive`
  writes) instead of failing on it, on every backend.
- Native builds of a program whose packages declare types of the same
  name no longer fail in the SSA verifier when both are printed with
  Debug, encoded as JSON, decoded from SQL, or generated in property
  tests (gotchas/ambiguous-type-generated-function-names.md).
- The module commands (`get`, `mod tidy|accept-effects|clean|proxy`,
  `vendor`, `audit`) are written in Xo too (selfhost/cli/modules,
  decision 0178 stage 4); xo1 runs them.
- A native program that asks whether a stream is a terminal
  (`stdout_is_tty`, `width`) without reading standard input links again
  (the runtime's terminal functions were left out).
- `xo query` and `xo edit` are written in Xo too (selfhost/cli/semantic,
  decision 0178 stage 3): `xo1 query|edit` print what the Go tool
  prints (37 of 37 command lines byte identical on cmd/xo's test
  programs; TestQueryCLI and TestEditCLI pass against xo1). The
  definition hashes of `xo query hash` (decision 0125) are byte identical
  to the Go ones on 270 of 270 modules (13,662 definitions).
- Native builds run the self hosted driver (decision 0178 switch point
  A, approved 2026-10-12): `xo build|run|test --backend=arm64|llvm` (and
  `--no-gc`) execute xo1, the driver written in Xo, which `xo` builds
  once from its embedded sources into the user cache (a few minutes the
  first time). Output, diagnostics, JSON, exit codes, and paths are the
  Go driver's; `XO_SELF_DRIVER=0` restores the Go driver. xo1 gained
  `run`, `xo build`'s flags, several directories, the build directory
  and binary cache, split LLVM builds, `--release`, `--stamp`, and the
  check's diagnostics (stage 1).
- `selfhost/macho` also writes ELF: `macho asm --elf` assembles the
  linux/arm64 emitter's assembly into the object `clang -c` writes (byte
  identical on the 159 arm64 goldens and on xo1), and `macho elflink`
  links it with the linux runtime pack into a dynamically linked glibc
  executable (versioned imports, stubs, TLS relaxation,
  `.eh_frame_hdr`). `xo1 build --target=linux/arm64 --ld=macho` builds a
  Linux program from any host with no C toolchain; every golden runs on
  glibc 2.31 and 2.41 (decision 0182 stage 3).
- The native backends no longer count the element of `for x in <place>`
  or an inner list read through an outer one (`a[i][j]`) when nothing
  changes the container while it is in use (decision 0186): on
  bench/perf/nested the read workloads went from 600,041 retains to 1
  (walk) and from 201,001 to 1 (index2) on arm64, allocations unchanged.
- The arm64 backend (and its Xo twin, byte identical) inlines small
  functions that call others and larger ones with at most two call
  sites, and drops the bounds, overflow, and shift checks that value
  bounds, loop conditions (also through `&&` and `||`), and binary
  search midpoints prove (decision 0184). The Unicode runtime in Xo is
  now within 5 percent of the C path on every bench/perf/unicode
  workload on arm64 (39 percent fewer instructions on mixed script case
  mapping, 42 on Rune case mapping).
- `selfhost/macho`, a Mach-O assembler written in Xo, turns the arm64
  emitter's assembly into the same object `cc -c` writes (every section,
  function, and relocation identical on the 158 arm64 goldens and on
  xo1's own assembly; all 158 golden objects byte identical), 6.8 times
  faster; `xo1 build --as=macho` uses it. Its linker links the objects
  with a prebuilt runtime pack into a signed executable, so `xo1 build
  --ld=macho` needs no C compiler at all for programs without `use c`
  sources (decision 0182).
- The WebAssembly backend's modules are much smaller (decision 0179,
  stage 3): the self hosted checker went from 43.2 MB to 8.9 MB and
  runs under Node in 1.9 s and 2.5 GB instead of 10.7 s and 5.9 GB.
  Large `var` parameters are passed by reference and kept in memory,
  programs where nothing waits keep no suspension frames, field
  releases are one call, locals are shared and renumbered.
  `selfhost/wasm --annotate` attributes a module's bytes to SSA opcodes.
- A WebAssembly backend written only in Xo (`selfhost/wasm`, decision
  0179): `xo1 build --backend=wasm` lowers the self hosted SSA to WAT,
  assembles it with an assembler written in Xo, and links a runtime of
  its own over WASI preview1; no C compiler, linker, or Go toolchain.
  71 of the 133 LLVM goldens run under Node with the output of the
  native backends; the rest (tasks, network, files, time, strconv
  quoting, more of the std runtime) fail the build with a clear reason.
- The WebAssembly backend, stage 2 (decision 0179): the Unicode code
  in Xo (decision 0180) compiles to wasm with the program, and the
  backend's runtime adds the text primitives it calls, number and
  duration parsing, and base64; tasks, scopes, channels, select, Mutex,
  Atomic, timers, and budgets run on one thread under a deterministic
  cooperative scheduler (a waiting task's wasm stack is unwound and
  rewound); `rand` uses WASI `random_get`; files are read through WASI
  (`path_open`, `fd_read`, `fd_readdir`): 116 of the 134 LLVM goldens
  pass, and the network fails the build with a clear error. A module
  carries only the runtime functions it reaches (recursion: 7.5 KB, 16.7
  KB before). Values share wasm locals by liveness, so large programs
  build: the self hosted lexer, parser, and checker run as wasm under
  Node with the output of their go backend builds (the checker: a
  40 MB module); `selfhost/wasm --why` lists every construct a program
  needs that the backend lacks.
- The self hosted driver xo1 (`selfhost/xo`, not the released `xo`)
  builds with the LLVM backend (`--backend=llvm`), runs Go backend tests
  (`xo1 test --backend=go`), and builds `use go` programs on the Go
  backend; every golden suite passes through it (decision 0178, which
  proposes the plan for retiring the Go implementation). The Go code
  generator no longer imports a module's package that the generated
  text does not name (go build failed with "imported and not used" on
  selfhost/xo).
- Native builds (arm64, LLVM, `--no-gc`): the Unicode operations of the
  runtime are written in Xo (decision 0180, the runtime pilot):
  `std/native/llrt/unicode.xo` with generated tables
  (`unicode_tables.xo`) does Str and Rune case mapping, `is_letter`,
  `is_digit`, `is_print`, strconv quoting, and std/regexp's `\pL` and
  `\p{Nd}` ranges; the C runtime keeps two bulk byte primitives. Output
  is unchanged on every backend. ASCII case mapping and quoting are
  faster than before; per rune work on arm64 is 1.45 to 1.8 times the
  C's instructions, so `XO_UNICODE=c` at build time still selects the C
  implementation (compiled only then, never in the prebuilt packs).
- More of the native runtime is Xo (decision 0193, which also orders the
  rest of the port): `std/native/llrt/text.xo` does Int.parse, a
  Duration's text and Duration.parse, a Time's RFC 3339 text and
  std/json's Time parse, and the Debug quoting of Str and Bytes;
  `strings.xo` does Str.replace and the trims. Output is unchanged on
  every backend, and the wasm backend runs the Xo versions. Int.parse,
  Duration.parse, Time text, Debug quoting, and replace retire fewer
  instructions than the C did (Time text about a quarter); Duration text
  does not yet meet the 5 percent bar, so `XO_LLRT_C=text,strings` at
  build time selects the C implementations (`XO_UNICODE=c` still works).
- Int and Float text, Float.parse, Bytes.from_hex, and base64 on the
  native backends are Xo (`std/native/llrt/num.xo`, decision 0193 step
  3), checked line by line against Go's strconv on 336,386 inputs. Float
  text retires under 1/50 of the instructions it did (the C ran snprintf
  up to 17 times a Float), Float.parse about 0.28x, Int text 0.73x to
  0.80x, base64 0.31x to 0.39x. Float.parse on the native backends now
  takes what the Go backend takes: hex floats (`0x1p-2`), underscores
  between digits (`1_000`), and any length. `XO_LLRT_C=num` at build time
  selects the C implementations.
- std/json on the native backends is Xo (`std/native/llrt/jsontext.xo`,
  decision 0193 step 4): the parser, the readers the generated decoders
  use, and the text of strings, Floats, and Bytes, checked against the Go
  backend on about 28,000 inputs. Decoding takes 0.41x to 0.96x of the
  instructions it did, encoding strings about 0.9x, structs about 0.12x,
  and Floats about 1/50 (the C ran snprintf up to 17 times a Float).
  std/json now also works on the wasm backend. `XO_LLRT_C=jsontext` at
  build time selects the C implementation.
- A Go backend build with no go command (`$XO_GO` not executable, or no
  `go` on PATH) now fails with XO1503 naming the fix, instead of a
  "code generator bug" message with no detail (decision 0176).
- `xo1 --phase-times` and `XO_PHASE_TIMES=1 xo asmdump` print the wall
  clock of each compiler phase. The self-hosted arm64 emitter's dead code
  elimination and register allocation liveness no longer copy every MIR
  instruction and bit set per pass: xo1 compiles selfhost/xo with 32
  percent fewer instructions and 45 percent fewer cycles, output byte
  identical (bench/results/xo1-perf-2026-10-12.md).
- The self hosted checker (`selfhost/check`) checks `use go` programs
  from the binding the Go toolchain cached, and reports what the Go
  checker reports for escaping scope handles and task groups (XO0802),
  skipped Go names (XO1102), `edition 2026` (XO1315), unvouched escape
  hatches of dependencies (XO1316), and effects beyond the effect lock
  (XO1317) (decision 0177). `xo check` reports XO1102 with the same text
  whether the binding was just made or read from the cache.
- Native builds (arm64, LLVM, `--no-gc`) have `Net.only` and
  `task.with_budget` (decision 0175): a `Net` carries its attenuation
  lists, and dial, listen, each HTTP request and redirect of a client
  made from it are checked (`sql.open` refuses an attenuated `Net`);
  `with_budget` bounds steps, running time, and allocation (counted
  exactly per task) through polls at loop iterations and at the entry of
  the program's functions, in programs that call it. Output is the Go
  backend's (goldens `net_only`, `task_budget`, `test_net_only`); the
  supply-chain replication's run time rows also block natively.
- Go versions (decision 0176): the Go backend needs Go 1.24 or newer
  (std/rt uses crypto/pbkdf2, maphash.Comparable, http.Protocols);
  `xo build`, `xo run`, and `xo test` read `go env GOVERSION` once per
  process and report the new XO1502 for an older go command. The
  generated go.mod and the `use go` binding workspace get
  `go <min(installed, 1.26)>` instead of a fixed `go 1.26`, so an older
  Go is never made to download a newer one. New
  `scripts/go-version-check.sh` runs the version sensitive tests with
  `GOTOOLCHAIN=goX.Y` (no download without `--download`). Every go
  command the tests run uses Xo's capped Go cache (`build.GoEnv`), held
  by the new TestGoCommandsUseXoCache; `go build` in reducediff_test and
  `go tool objdump` in internal/arm64 used the default cache.
- Native backends: an update of a collection inside a list element
  (`g.blocks[i].insts.push(x)`, `m[i][j] = v`, `rows[i].cells[j].g = v`,
  `rows[i].m[k] = v`, `rows[i].text += "x"`) is in place when the outer
  list is unique, instead of copying the inner collection every time
  (decision 0169, `list.take`): 20,000 pushes 63.6 ms to 0.1 ms on
  arm64. A shared outer list still copies (value semantics).
- The Go code generator in Xo (`selfhost/gen`, decision 0167), compared
  with internal/gen through the new hidden `xo gendump`
  (TestSelfhostGen, TestSelfhostGenMutations): every declaration and
  every whole file identical on the repository runs the Xo checker
  checks. The Xo driver `selfhost/xo` gains `--backend=go`: it compiles
  selfhost/check to the same Go as `xo build` and compiles itself to a
  fixed point. The released `xo` is unchanged.
- LLVM builds can split the IR into modules compiled by parallel clang
  processes: `XO_LLVM_SPLIT=n`, `XO_LLVM_JOBS` (default min(cores, 6)),
  `XO_LLVM_LTO=thin|none` (decision 0168). Builds without the variable
  are unchanged (one module, full LTO). The self hosted compiler's
  tests split (12 modules, no LTO): its LLVM build of selfhost/ssa
  takes 512 s instead of 2,394 s.
- The LLVM emitter in Xo (decision 0166, stages 1 to 3):
  `selfhost/llvm` lowers the self hosted SSA module to the LLVM IR
  internal/llvm prints, compared function by function and module by
  module through the new hidden `xo llvmdump` (TestSelfhostLLVM,
  TestSelfhostLLVMMutations); every module both lower is byte
  identical, and the self hosted pipeline lowers selfhost/ssa to IR
  byte identical to internal/llvm's, from which clang builds a working
  binary (TestSelfhostLLVMEndToEnd).
- Spec audit, fifth pass (bench/results/spec-audit-2026-10-11.md): core
  10.1's `xo build` row lists `--stamp`; core 14 no longer calls
  `std/time` Go backend only. New tests: `xo replay` keeps `--stamp`
  values (cmd/xo `TestRecordReplayStamp`); test_cli and test_term in
  `TestNoGCTests`; formats_toml in `TestGenGoldensNative`.
- Native builds (arm64, LLVM, `--no-gc`) run commands like the Go
  backend (decision 0172): `Proc.cmd` with `dir`, `env`, `set_env`,
  `stdin`, `inherit`, `merge_stderr`; `output`, `run`, `spawn` and every
  `Child` method; `Proc.pipeline` with pipefail; and `Proc.only` and
  `Env.only` are enforced natively (`Env.vars` and `Env.home` too).
  File handles (`open`, `create`, `open_append`), `append`, `copy`,
  `mode`/`set_mode`, `symlink`/`read_link`, `glob` with `**`, and
  `lock`/`try_lock` build natively as well, and so does `std/regexp`
  (Go's RE2 parser and Pike VM ported to Xo: the same matches, groups,
  replacements, and errors; Unicode classes other than `\pL`, `\p{Nd}`,
  `\p{Any}`, `\p{ASCII}` are an error natively for now), and
  `std/archive` (Go's compress/flate, gzip, tar, and zip ported to Xo:
  the same bytes and errors; zip64 is an error natively).
- Log attribution and resource budgets (decision 0173). A log record
  written by a dependency's code names it: `[<module>@<version>]` after
  the level in text, `"module"` in JSON (a field of that name cannot
  forge it), `log.Record.module` in tests; `xo audit` lists each
  dependency's log call sites. `task.with_budget(clock, b, f)` with
  `task.budget(cpu:, alloc:, steps:)` runs f under ceilings on running
  time, allocation, and steps and returns `Err(Exceeded(resource))` when
  one is passed; in a program that calls it, generated Go polls at every
  loop iteration and at the entry of functions that call user code
  (Go backend; native XO1201 planned; programs without a budget are
  unchanged). New fault `XO-F008`. The supply-chain replication blocks 12
  of 17 scenarios (was 10 of 15).

- xo1 (`selfhost/xo`, decision 0174) does what internal/build adds for
  native builds, with the same cache keys (shared caches): the TLS
  library of std/http, the zone data of std/time, prebuilt runtime packs
  and the runtime object cache, `use c` inputs, `--target=linux/arm64`
  (assembly), and `xo1 test`. The self compiled compiler passes all 147
  arm64 goldens; `scripts/selfhost-samebin.py` finds its binaries byte
  identical to `xo build --backend=arm64`'s (147 of 147).
  `scripts/selfhost-goldens.py` passes args.txt, env.txt, and stdin.txt.
- `List[T].with_capacity(n)` (decision 0170, every backend): an empty
  list with room for n elements before it grows.
- The `xo <edition>` line of xo.work is validated as xo.mod's `edition`
  line is (decision 0155): `xo 2026` gets the fixable note `XO1315` and
  `xo fix` rewrites it to `xo preview`; any other unknown name is
  `XO1301`.
- The arm64 emitter in Xo (`selfhost/arm64`, decision 0158), compared
  with internal/arm64 piece by piece through the new hidden `xo
  asmdump` (TestSelfhostArm64, TestSelfhostArm64Mutations): every
  function and every whole file identical on the repository runs both
  SSA builders build completely. `selfhost/xo`, a compiler driver all
  in Xo for the switch-over experiment (`scripts/selfhost-switchover.sh`,
  `scripts/selfhost-goldens.py`); the released `xo` is unchanged.
- `xo get` and `xo mod accept-effects` refuse a version with an
  escape hatch (`use c`, impure `use go`) the main module does not
  vouch for (XO1316) before writing xo.mod or xo.lock; `--accept-effects`
  accepts effects, not escape hatches (decision 0156).
- Shell completions (std.md 14.5, decision 0165): `cli.completion(cmd,
  shell)` returns a bash, zsh, or fish script that completes
  subcommands, flags, and choice values from the declaration, on every
  backend. Command names are now limited to letters, digits, `.`, `_`,
  and `-`.
- Version stamping (core 10.1, decision 0165): `xo build --stamp
  NAME=VALUE` (repeatable; also `xo run`) sets `const NAME: Str = "..."`
  of the main module for that build, on every backend, so `--version`
  can print a version set by the build script. A name that is not such
  a constant is the new error XO1501. Stamps are part of the build
  information, so `xo replay` rebuilds the same program.
- Formats (std.md 16 to 19, decision 0164): `std/csv` (`parse`,
  `parse_maps`, `format`; RFC 4180, any separator) on every backend;
  `std/regexp` (`compile`, `must`, `quote`, `is_match`, `find`,
  `find_index`, `find_all`, `captures`, `named`, `replace_all`,
  `replace_all_literal`, `split`; RE2), `std/archive` (`gzip`,
  `gunzip`, `tar`, `untar`, `zip`, `unzip` in memory; entries leaving the
  root are refused), and `std/toml` (`decode[T: Json]`, `to_json`; TOML
  1.0) on the Go backend, XO1201 (planned) on native builds. A std
  parameter's default may be a rune literal.
- `Env.config_dir()` and `cache_dir()` build natively too (native
  prelude; both SSA builders route them there).
- `std/toml` builds natively too (`std/native/toml.xo`, the same JSON
  text and errors as the Go backend).
- Files (std.md 3.1, 3.5, 8.3, decision 0163), Go backend and MemFs:
  streaming handles `os.fs.open(path)`, `create`, `open_append` (a
  `File` with `read`, `read_line`, `write`, `close`); `append`, `copy`
  (keeps permission bits), `mode` and `set_mode`, `symlink` (a sub
  capability refuses a target that escapes it) and `read_link`, `glob`
  (`*`, `?`, `[...]`, `**`), advisory locks `lock` and `try_lock` (a
  `FileLock`, `flock`/`LockFileEx`); `Env.config_dir()` and
  `cache_dir()` (XDG variables, else ~/.config, ~/Library/..., or
  %AppData%). Native builds report them as XO1201 (planned).
- Terminals (std.md 3.6, 15, decision 0162), on every backend:
  `Stdio.stdin_is_tty()`, `stdout_is_tty()`, `stderr_is_tty()`,
  `width()`, `read_secret(prompt)` (a line with the echo off, for
  passwords), `read_bytes(max)` and `read_all_bytes()` (binary input);
  `FakeStdio.set_tty(width)` and `set_input_bytes(data)` for tests; the
  new `std/term` module: `color_enabled(stdio, env)` (NO_COLOR,
  FORCE_COLOR, CLICOLOR_FORCE, TERM=dumb, a terminal), `painter(...)`
  with `bold`, `red`, ... (plain text when color is off), and `strip`.
- Golden programs of internal/llvm and internal/arm64 read `stdin.txt`
  as standard input, as internal/gen's do.
- Richer processes (std.md 3.8, decision 0161), Go backend and test
  fakes: `os.proc.cmd(name, args)` builds a `Cmd` (`dir`, `env(e)` with
  exactly the variables an `Env` shows, so `Env.only` narrows the child,
  `set_env`, `stdin`, `inherit`, `merge_stderr`) and runs it with
  `output()`, `run()` (on the terminal, returns the exit status), or
  `spawn()` (a `Child` with `write`, `close_stdin`, `read`, `read_err`,
  `read_line`, `wait`, `kill`, `pid`); `os.proc.pipeline(cmds)` pipes
  commands together (pipefail status). A child ended by a signal has
  status 128 + n. `Proc.only` applies to every way of running, and a
  `Cmd` keeps the lists of the `Proc` that made it. `FakeProc` answers
  them from `on_run`. Native builds report them as XO1201 (planned).
- `std/cli` (std.md 14, decision 0160), on every backend: command line
  parsing for tools. `cli.command(...)` with `bool_flag`, `str_flag`,
  `int_flag`, `duration_flag`, `choice_flag`, `strs_flag` (repeated),
  `arg`, `rest`, and subcommands; `cli.parse(cmd, args, env)` returns a
  `Parsed` (typed accessors `bool`, `str`, `int`, `duration`, `strs`,
  `_opt` forms, `arg`, `rest`, `command()`) or a `CliErr` (`Help`,
  `Version`, `Usage`); `cli.parse_or_exit(cmd, os.env, os.stdio,
  os.proc)` prints help or version (exit 0) or a usage error (exit 2).
  Flags fall back to variables (`env:`), read through the `Env`.
- Golden programs may set extra environment variables in `env.txt`
  (internal/gen, internal/llvm, internal/arm64); the arm64 golden runner
  passes `args.txt` too.
- Logging is the effect `log` (decision 0156): `std/log`'s `debug`,
  `info`, `warn`, `error` perform `log`, so a `pub` function that logs
  declares `uses log` (inferred for non-pub functions; `log.configure`
  still `stdio`). std's own records stay ambient within std. XO0701 for
  a missing `log` carries a fix, and `xo fix` added it across the repo's
  programs. The effect label list in diagnostics and ceilings includes
  `log`.
- `xo audit [--json] [--no-gc] [<dir>]` (decision 0156): a supply chain
  report per dependency (effects, xo.lock effects state, ceiling, `use c`
  and impure `use go` with their vouches) plus this module's capability
  attenuations, the capabilities it passes into dependencies (attenuated
  or not), and unused vouches.
- Capability attenuation (decision 0156): `os.net.only(hosts)` (exact
  hosts, `*.domain`, `host:port`; checked for raw dials and listens, each
  HTTP request and redirect of a client made from it, and refused for
  `sql.open`), `os.env.only(names)`, and `os.proc.only(cmds)`, on the Go
  backend and its test fakes. `NetErr.Denied(addr)` and
  `ProcErr.Denied(cmd)` are new variants (an exhaustive `match` needs
  the arm). Native builds report `only` as XO1201 (planned).
- Supply chain (decision 0156): an effect lock by default. xo.lock
  records each dependency's effects (`<path> effects <labels|none>`, the
  union of its pub functions' `uses`). `xo get` and `xo mod tidy` record
  them the first time and narrow them; an update that widens them fails
  `xo get`, `xo mod tidy`, and every load with `XO1317` until `xo mod
  accept-effects [<path>...]` or `xo get --accept-effects` records them.
- Supply chain (decision 0156): only the consuming module vouches for a
  dependency's escape hatches. A required module's `use c` lines and its
  `use go` of Go packages that are not pure need a `vouch <module>
  <version> c "<header>" [uses ...]` or `vouch <module> <version> go
  "<package>"` line in the consumer's xo.mod, else `XO1316`; a
  dependency's own `use c ... uses none` no longer counts. A
  dependency's `use c` binding is now written to the main module's
  binding cache (it failed on the read only module cache before).
- Prebuilt runtime (decision 0157): `scripts/release.sh` builds release
  archives (darwin/arm64, linux/arm64, linux/amd64) with the arm64
  backend's C runtime prebuilt in `lib/xo/rtpack/<os>-<arch>/` (every
  variant of std/llrt and the TLS library, byte for byte reproducible;
  linux/arm64 cross compiled with zig cc for glibc 2.31). A native build
  links the pack's object when its key (the runtime sources and flags)
  and checksum match, else compiles the runtime locally as before;
  `XO_RTPACK=<dir>|off` chooses, and the hidden `xo rtpack` builds or
  inspects packs. The first arm64 build of a small program drops from
  1.94 s to 0.13 s (an HTTPS server 4.27 s to 0.28 s). `cc` still
  assembles and links (decision 0150's "no clang" corrected). The zone
  data of std/time is now `xo_tzdata.c`, compiled beside the runtime.
  A failed local runtime compile, and a missing `cc`, name what to
  install.
- Editions (decision 0155): until 1.0 the only edition is `preview`;
  `xo init` writes `edition preview`, and every xo.mod and xo.work of
  the repository says it. `edition 2026` stays an alias of `preview`
  during the 0.x releases: `xo check` gives the fixable note `XO1315` on
  it, and `xo fix` (also `xo fix --edition=preview`) rewrites xo.mod.
  The compatibility promise of core 13 starts with the first frozen
  edition, named at 1.0.
- arm64 backend: enum parameters of 256 bytes or more of direct calls
  are passed by address too (decision 0153); tags and payloads of enums
  in memory are read in place, and large fields and payloads of read
  only memory are passed on by address (the arm64 build of the self
  hosted SSA builder 5 to 9% faster).
- Self hosting: SSA stage 8 (decision 0150): selfhost/ssa runs the
  passes after the build (pruneUnwind, addLandings, cleanup, InsertRC)
  and the verifier exactly as internal/ssa does; the SSA lowering is
  self hosted. `xo ssadump --stage=verify,mutants` prints the
  verifier's verdicts.
- Self hosting: SSA stage 7 (decision 0150): selfhost/ssa builds the
  test binaries of `xo test --no-gc` exactly as internal/ssa does;
  `xo ssadump --test` prints them.
- Self hosting: SSA stage 6 (decision 0150): selfhost/ssa lowers the
  generic intrinsics (JSON codecs, SQL rows, any, cast, status codes)
  and calls of `use c` bindings exactly as internal/ssa does; the Xo
  checker binds `use c` from the binding cache (decision 0152).
- `use c`: two `use c` lines of one header, one with `uses none` and
  one without, no longer share one binding in a process (the second
  line had the first one's effects).
- Self hosting: SSA stage 5 (decision 0150): selfhost/ssa lowers
  scopes, tasks, `await`, channels, `select`, Mutex, Atomic, std/task,
  os.tasks, and the operations that wait exactly as internal/ssa does
  (0 differences, three backends, 1,900 mutations).
- Self hosting: the go build of the Xo SSA builder is 7 to 9% faster
  (`comparable` copied its list of types per recursive call).
- arm64 backend: struct parameters of 256 bytes or more of direct calls
  are passed by address (decision 0151); `var` ones stay in the
  caller's memory, read and updated in place (decision 0148's
  dataflow), non `var` ones are read in place. The arm64 build of the
  self hosted SSA builder is 5.8 times faster (13.2x to 2.3x the Go
  builder) and half the size.
- arm64 backend: register allocation merges live ranges in one pass
  instead of sorting at every assignment (code generation of
  selfhost/ssa 100 s to 60 s).
- Self hosting: SSA stage 2 (decision 0150): selfhost/ssa lowers
  interface values and calls, Error/Display/Debug values, method values,
  `derive Ord` comparisons, and the generated equality and order helpers
  exactly as internal/ssa does (0 differences, three backends).
- Self hosting: SSA stage 3 (decision 0150): selfhost/ssa lowers Map
  and Set (helpers, methods, literals, `m[k] = v`, loops, Debug text)
  and constants of collection types exactly as internal/ssa does.
- Self hosting: SSA stage 4 (decision 0150): selfhost/ssa lowers
  contracts (`requires`, `ensures`, invariants), `defer` with its
  landings and fault continuations, loop bounds, and real-time sections
  exactly as internal/ssa does; stages 1 to 4 of the SSA builder are
  done.
- arm64 backend: a function past 1 MiB of code failed to assemble
  (`b.hs` to a fault stub out of range); its conditional branches now
  go through an unconditional `b` as tbz already did.
- arm64 backend: an overflow fault of `c + x` (constant left) named the
  operands swapped (`6 + 9223372036854775807`); it follows the source
  order, as on the Go and LLVM backends. Found by `progen.Arith`, now
  widened to Float, Str, List[Int], structs, enums with payloads,
  closures, and edge operands (shifts by 63, the minimum divided by -1).
- Go backend: a float `+`, `-`, or `*` of two literals whose result is
  out of range (`2.5e300 * 1e10`) failed to build (a Go constant
  overflow); it is `+Inf` at run time, as on the native backends.
- Tests: TestArithProperty and TestCompileProperty compare a program's
  exit status, all of its standard output, and all of its standard
  error across backends (was the first line of standard error),
  dropping only what native builds legitimately leave out (frames, the
  deadlock's blocked task list, crash bundle paths). bench/nightly.sh
  runs TestArithProperty on 500 programs.
- scripts/reduce-diff.py shrinks a program while two backends run it
  differently (ddmin over lines, then smaller edits; about 40 s for a
  74 line program); TestArithProperty prints the command for a failing
  program.
- Generated Go no longer repeats a `//line` directive on consecutive
  lines (positions are unchanged), declares the unused task `tk` in
  constants whose value does not need it, or ends a function whose body
  already terminates (an `if`/`else` or `switch` returning in every
  branch, by the Go spec's rule) with an unreachable panic. `match`
  emits no `break` after an arm that returns or jumps, declares its
  label only when an arm breaks to it, emits a catch-all arm as a plain
  block (no `Unreachable` after it), and matches an immutable binding in
  place instead of copying it; a `scope` body that returns skips its
  dead epilogue. The generated Go of every golden program now passes
  `go vet` (it had 108 unreachable code reports) and `gofmt -l`
  (TestGeneratedGoVets), and 11 programs keep a snapshot of their Go
  (TestGeneratedGoSnapshot).
- Generated Go: an exhaustive `match` ends in
  `panic(rt.Unreachable(loc))` (Go needs no trailing panic after it),
  statements after one that never completes are not emitted, `x = x` is
  dropped, and the `_ = x` lines Go does not need are removed (191 to 15
  in the snapshot programs; the last ones, `_ = osv` in XoMain and the
  unused trace of a scope's failure, followed on 2026-10-11: the Os
  parameter is `_` when main does not take it, and the trace is `_`
  when the failure becomes a fault). go vet is clean on the examples, the LLVM
  golden programs, the perf benchmarks, and the self hosted compiler too
  (TestGeneratedGoVetsLarge).
- arm64 backend: a conditional branch whose two targets are the same
  block kept its compare out of the output, while the next block reused
  those flags; it branched on stale flags (wrong `if` results after an
  `if` with identical arms). Found by `progen.Arith`, random integer
  programs compared across the Go, LLVM, and arm64 backends
  (TestArithProperty).
- Self hosting (decision 0150): the order of the remaining stages is
  SSA lowering, arm64 emitter, LLVM emitter, Go code generator, and the
  self hosted compiler takes over native builds on arm64 hosts once it
  builds itself to a fixed point there. `selfhost/ssa`, the SSA builder
  in Xo, lowers the core subset (functions, generics, numbers, Str,
  structs, enums, patterns, loops, calls, closures, Option and Result
  with `?` and error frames, Display and Debug text) exactly as
  internal/ssa does: 0 differences on every function it completes over
  the repository and 400 mutations, on the go, arm64, and LLVM builds
  (53% of the Go builder's functions; maps, interfaces, defer, and
  concurrency are later stages). New hidden `xo ssadump [--stage=build,
  unwind,landings,cleanup,rc]`; `xo typedump --native`; the self hosted
  checker gains native mode.
- arm64 backend: the address of a frame slot past 4095 bytes is made
  with a shifted add (the assembler rejected `add x6, sp, #4808`).
- Go backend: `==` on an enum whose variants have no fields compares
  tags instead of boxing both values.
- Spec 5.2: a receiver that is not `var` is evaluated before the
  arguments, like any operand (the behavior of every backend).

- LLVM backend: enums of 256 bytes or more are passed by reference, and
  values in memory nothing writes during the call (by reference
  parameters, their fields and payloads, box contents and list elements
  under a borrowed parameter) are read in place and passed by address
  (decision 0147); `var` parameters passed by reference stay in the
  caller's memory, with fields read and updated there (decision 0148).
  The self hosted checker's LLVM build went from 1.88 times the Go
  checker's time to parity (5.4 s to 2.9 s for 20 checks of selfhost).
- Native backends: an enum parameter is borrowed unless every path
  consumes it, so tree walks pass nodes down without reference count
  traffic (decision 0149).
- The self hosted checker loads modules as internal/check does (decision
  0141): xo.mod, xo.work, xo.lock, the module cache (read, never
  fetched), vendor/, deny rules (XO1312), dependency rules (XO1301 to
  XO1307), and effect ceilings of dependencies (XO1309). Every
  repository run without `use go` or `use c` (610 of 626) is compared
  and identical on the go, arm64, and LLVM builds, and module mutations
  (ceilings, deny rules, versions, lock lines) give no difference. New
  Go goldens for XO1309: `effect_ceiling_work` (xo.work ceilings) and
  `effect_ceiling_vendor` (require ceilings, vendored).
- `cmd/xoplay`, the compiler half of the xolang.dev playground (decision
  0144): `check`, `fmt`, and `gen` as JSON, built for wasip1 with
  `-tags xo_playground`, which leaves the runtime, native, and LLVM runtime
  sources out of the binary. `build.Generate` is the code generation step
  of `xo build` without a build directory or the Go toolchain.
  `xoplay gen --test` generates test binaries, with `xo_nohttp` when they do
  not reach std/http (`gen.ReachesStd`). The playground underlines
  diagnostics, sets `--seed` and `--explore` for tests, downloads brotli
  compressed assets (22.5 MB instead of 33), and is tested in CI before
  every deploy of the site.
- LLVM backend: struct parameters of 256 bytes or more are passed by
  reference in direct calls, `var` ones written back through the
  pointer, instead of field by field (decision 0145); the self hosted
  checker's LLVM build is about 15% faster.
- The self hosted checker reports naming (XO0101, with the rename fix
  that covers every use of a local), time layouts (XO0420, and the
  XO0177 habit of a date pattern from another language), and the
  checker's other XO0177 habits as internal/check does (stage 8 of
  decision 0141): 587 of 609 repository runs are compared whole and
  identical on the go, arm64, and LLVM builds; the 22 left use `use go`
  or `use c` (16) or break dependency rules the port does not have (6).
- The self hosted checker gives every diagnostic the notes and machine
  fixes of internal/check and internal/parser (stage 7 of decision
  0141): edit distance suggestions, habit notes and fixes, effect call
  chains, previous declarations, missing arms and fields as holes;
  `xo typedump` has a `full` section (the `xo check --json` form), and
  every compared run and mutation gives identical JSON on the go, arm64,
  and LLVM builds. `xo astdump` prints notes and fixes too.
- Notes and fixes attached after more diagnostics were reported are no
  longer lost (`xo check --json`, `xo fix`).
- The self hosted parser resynchronizes after a hard error as the Go
  parser does and reports the same errors with the same recovered tree
  (every file and token mutation compared); the self hosted checker now
  checks runs with parse errors too. `xo typedump` no longer panics on a
  use of an unresolved import.
- arm64 backend: spilled values share stack slots when their live
  ranges do not overlap (frames of long functions shrink up to 8 times;
  the self hosted checker's largest went from 28 KiB to 6.4 KiB), and a
  frame past 28 KiB no longer fails to build: such a function addresses
  its far slots through x28 (decision 0146).
- The self hosted checker checks matches for exhaustiveness (stage 6 of
  decision 0141): XO0301 with the same missing patterns, refutable `let`
  and `for` patterns (XO0303), and the XO0305 warning, as internal/check
  does on every repository module (go, arm64, and LLVM backends).
- The self hosted checker applies the value semantics rules (stage 5 of
  decision 0141): `var` places (XO0501, XO0502), overlapping `var`
  arguments (XO0503), moves (XO0510), lost and stored updates (XO0520),
  split critical sections (XO0804), and unused bindings (XO0110,
  XO0201), with the same diagnostics as internal/check on every
  repository module (go, arm64, and LLVM backends).
- The self hosted parser reports the habits of other languages
  (decision 0087) and the other soft errors as internal/parser does,
  with the same messages and spans, and recovers the same tree, so the
  self hosted checker now checks programs written with Go, Rust,
  Python, TypeScript, Kotlin, or Swift habits and gives the same
  diagnostics (the 107 habit fixtures of internal/fix among them).
- The self hosted checker checks generics, effects, and real-time
  functions (`selfhost/check`, stages 3 and 4 of decision 0141): it
  prints the sigs section of `xo typedump` (type parameters, constraints,
  implicit parameters, declared and inferred effects) and reports XO0402,
  comparability (XO0404), XO0701, and XO1401 to XO1405 as internal/check
  does, on every repository module (go, arm64, and LLVM backends). Effect
  ceilings of dependencies (XO1309) are not ported yet.
- The self hosted checker types every expression (`selfhost/check`,
  stage 2 of decision 0141): inference, literals, operators, expected
  types and implicit conversions, calls with named arguments, fields,
  methods, statics, variants, closures, and the Option and Result rules,
  with the same types as internal/check on every repository module it
  accepts (go, arm64, and LLVM backends) and the same diagnostics on
  modules rejected only for names and types. The names section of `xo
  typedump` now also names the fields, methods, statics, variants, and
  parameters (of named arguments) that identifiers resolve to.
- A call reads a `var` receiver or `var` argument after computing its
  arguments (spec 5.2, decision 0142). On the native backends
  `t.get(t.add(7))` ran `get` without the item `add` pushed, and
  `xs.push(f())` dropped what `f` pushed onto `xs`.
- `NAMES[i]`, a constant indexed by a lowercase name, is an index
  (decision 0143); it parsed as type arguments with an effect argument
  and was XO0407.
- The type checker's name resolution written in Xo (`selfhost/check`,
  decision 0141), the first stage of the self hosted checker: the same
  resolution of every identifier as internal/check on every repository
  module the Go checker accepts (on the go, arm64, and LLVM backends),
  and the same diagnostics on modules rejected only for names. Hidden
  `xo typedump [--section=names,types,sigs,diags] [--each] <dirs>`
  prints what the Go checker resolved, a section at a time
  (bench/results/selfhost-checker-2026-10-10.md).
- An assignment computes its value before it reads the binding it
  stores into (spec 5.2, decision 0140). On the native backends
  `t.objs[o].kids = t.add_kids()` dropped what `add_kids` pushed onto
  `t.objs`.
- Native backends: two modules may declare types of the same name; they
  were one type (a field of one was "not available without a garbage
  collector", or the wrong layout was used).
- Go backend: `??` on a nested optional (`opt.ok() ?? None` where `opt`
  is a `T? ! E`) generated Go that did not compile.
- `XO0401` on an `if` without `else` where a value is expected now has
  a note naming the missing `else`; when the expected type came from a
  sibling branch (a closure ending with `if a { n.add(1) } else if b
  { ... }`), a fix discards the branch values with `let _ =`.
- New error `XO0178` (decision 0136): a line that starts with a binary
  operator (`|| b`, `&& c`, `+ d` continuing the line above) gets one
  diagnostic and a fix that moves the operator to the end of the
  previous line. Before, a leading `||` was taken for a Rust closure
  (a wrong XO0177 fix) and cascaded into XO0160 and XO0175.
- `for b in data` over `Bytes` (spec 3.5, decision 0138) yields each
  byte as a `UInt8`; it was `XO0406`. Go's `for _, b := range data`
  gets the fix `for b in data`.
- `while let` (spec 3.5, decision 0137): `while let Some(v) = q.pop() { }`
  loops while a refutable pattern matches; `while let v = opt` is the
  Option shorthand, as for `if let`. Labels, `continue`, and `bound N`
  work; every backend and the self hosted parser.
- Import paths with a `.` or `..` segment (`use example.com/app/./lib`,
  `use ./lib`) are `XO0158`. They were accepted, and since `deny` rules
  compared the path as written while the loader cleaned it, they got
  past `deny` (decision 0095). The loader and `deny` now both use the
  cleaned path, and `XO0158` is the only error such an import gets
  besides a `deny` match.
- Spec: the grammar defines `PathSeg` (ASCII letters, digits, `_`, `-`,
  `.`), matching the lexer; section 4.1 states that dashes outside the
  last segment need nothing (`use github.com/xo-lang/xo/lexer`). No
  behavior change.
- Real-time functions (spec 6.5, decision 0133): `uses realtime` on a
  function makes the checker reject allocation (XO1401), loops without a
  bound (XO1402), calls of functions that are not real-time, function
  values, and interface methods (XO1403), recursion (XO1404), and
  blocking (XO1405). Native builds defer the frees of a real-time
  section to its end (a 256 entry queue per thread, `XO_RTSTATS=1`
  counts) and fault with XO-F007 when a section allocated. Demo
  `examples/realtime-control`: the tick that drops a 20,000 point
  schedule writes its output in about 0.2 us instead of 30 to 114 us
  (bench/results/realtime-2026-10-10.md).
- Loop bounds: `for x in xs bound N { }` and `while c bound N { }` on
  every backend; a constant range longer than N is XO1402, otherwise
  iteration N + 1 faults with XO-F006, and `xo test` fuzzes functions
  with bounded loops.

- Native TLS: ECDSA P-256 signing and verification run on fiat-crypto's
  verified field arithmetic (vendored at a pinned commit,
  `third_party/fiat-crypto`, decision 0134) instead of Mbed TLS's bignum
  code: signatures byte identical, 4.8 times faster (23 us), a full
  HTTPS handshake on the native server takes 251 us of CPU (was 354; Go
  backend 227).
- `bench/perf/http/run.sh` takes `CONFIGS=xo-base-llvm` with
  `XO_BASE=<xo binary>` for before and after runs of the LLVM backend.
- `xo build --hybrid`: loops in Go assembler poll for preemption at every
  back edge (decision 0131), so a long hybrid loop no longer holds up the
  GC's stop the world or other goroutines on its P (11.6 s to 0.058 ms
  during an 11.5 s loop); the hybrid benchmarks are 0 to 7 percent slower
  than before (median 2), still 9 to 31 percent faster than the Go
  backend.
- `xo build --hybrid` compiles loops of programs that start tasks (was
  "no assembly": an RC verifier error on the task runner).
- `bench/perf/hybrid/run.sh` takes `XO_BASE=<xo binary>` to compare two
  compilers' hybrid builds in the same interleaved runs.
- The parser written in Xo (`selfhost/parser`, decision 0127): the whole
  grammar, built with the Go, arm64, and LLVM backends; for every file the
  Go parser accepts it prints the same syntax tree dump, and it rejects
  every file the Go parser rejects (repository, std, spec blocks,
  generated programs, token mutations). Its LLVM build parses 1.3x faster
  than internal/parser (bench/results/selfhost-parser-2026-10-10.md).
- Hidden `xo astdump [--sig] [--bench=N]`: the canonical syntax tree dump.
- arm64: functions with frames up to 28 KiB build (probed a page at a
  time; was a "frame too large" error past 4000 bytes), and values of
  80 or more words no longer fail to assemble (decision 0128).
- The program generator of the compile property test is the package
  internal/progen.
- Native test binaries now report test_once, test_logs, and
  test_shutdown byte for byte as the Go backend (`TestNoGCTests` runs
  them): a fault in a `task.once` function is stored and raised again
  in every caller; `log.configure(os.stdio, ...)` in a test writes
  records to the fake standard error (still kept for `testing.logs()`);
  `os.proc.set_grace` before `testing.run_main` sets that run's grace
  period, and a third signal makes `run_main` return 130 (with
  `exit_code()` `Some(130)`) instead of ending the test binary.
- `==` on `http.Request` works on the native backends (was XO1201): it
  compares the six spec fields and ignores typed request values, as on
  the Go backend (decision 0112 note; internal/llvm golden `request_eq`).
- internal/llvm `TestGolden` runs `fault_defer_chain` and `request_eq`
  under `-short` too.
- Tests for items that had none: native TLS session resumption
  (`TestTLSDiff`, TLS 1.3 and 1.2), decision 0118's reader handoff,
  scope pruning, and magazines (std/llrt/testdata/tasks.c), and decision
  0098's binary trimming (internal/build `TestGoBinaryTrimming`).

## v0.9.0 (2026-10-10)

Spec draft 0.9.

- Module proxies (decision 0126): `XO_PROXY` lists proxy URLs, `direct`
  (git), and `off`, with Go's fallback rules (comma after 404/410, pipe
  after any error; default `direct`), and `XO_NOPROXY` patterns go to
  git. The read only protocol serves `@v/list`, `.mod`, `.zip`, and
  `.lock` per module; `xo mod proxy serve [--fetch] [<cache dir>]`
  serves a module cache and can fill it from git, as a team cache.
  Proxied trees are checked against xo.lock exactly as git fetches are.
- Checksum log (decision 0124): with `XO_SUMDB=<name>+<id>+<key>
  [<url>]`, the first fetch of every module version must match the
  log's record, be in its signed Merkle tree, and extend every tree the
  module cache saw before; otherwise `XO1313` and nothing is cached. An
  unreachable log or malformed settings is `XO1314`; `XO_NOSUMDB` keeps
  private modules out. The default stays `off` (no public log yet).
- Definition hashes (decision 0125, the last step of 0086): `xo query
  hash <name>` prints a content hash per top level definition that
  survives renaming locals and changes with what the definition, or one
  it calls in the module, computes. `xo publish --check` lists the
  definitions (private ones too) changed since the last tag, including
  those changed only through a helper, next to the API comparison.

- std/http speaks HTTP/2 over TLS on every backend (decision 0121):
  `with_tls` servers and `https` clients agree on `h2` by ALPN, plain
  `http` stays HTTP/1.1, and nothing in the API changes. Requests of a
  connection run at once, each as a task; a fault in a streamed body
  resets only its stream. The native backends implement it in C (frames,
  HPACK, flow control) with net/http's limits (250 streams, the header
  list limit with 431, the rapid reset limit) and its error texts, and
  their client keeps HTTP/2 connections for later requests as Go's does.
  Over one connection the native server does 87.8k requests per second
  in the HTTP benchmark, the Go backend 58.2k
  (`bench/results/http2-2026-10-10.md`).
- Native test binaries (`xo test --no-gc`, decision 0120): property tests
  and `testing.gen` work, with the Go backend's generated values for a
  seed, the same failing case, and the same shrunk input in the report.
  A `Set` or `Map` keyed by a sized integer (`Set[Int8]`) now builds
  natively. `xo test --real` gives native tests real capabilities for
  the effects they declare (`fs`, `fs.read`, `fs.write`, `net`, `clock`,
  `env`, `stdio`, `proc`), as on the Go backend.
- Native backends: deferred code that faults no longer skips the rest
  of the cleanup. The deferred code registered before it and that of
  the blocks around it runs, the last fault is reported, as on the Go
  backend (core 3.5, decision 0085 amendment), and a task that caught
  such a fault with `task.isolate` sees cancellation again (it could
  deadlock before).
- LLVM backend (decision 0119): generated code is compiled for the same
  CPU and features as the C runtime (clang's defaults): LSE atomics for
  shared reference counts on darwin/arm64, outline atomics (LSE when the
  CPU has it) on Linux arm64. HTTP benchmark +3.5 percent.
- Native `Mutex` (decision 0118): a writer's release hands the lock to
  the readers waiting, so they no longer wake only to park again: a
  contended readers and writer load (`bench/perf/conc` `rwmutex`, new)
  went from 967 to 131 ms and from 6.3 to 0.25 s of CPU. Long lived
  scopes (the native HTTP server's connections) no longer keep every
  finished task until they close, and the native allocator moves freed
  blocks between threads, so a server's memory stays flat (28 MB instead
  of 295 MB after 300k TLS connections).
- Native HTTPS (decision 0117): full TLS handshakes about 1.6 times as
  fast and at half the CPU (9.7k to 15k per second in the HTTP benchmark,
  Go backend 17k): X25519 key shares through HACL*, faster P-256
  inverses, a spinning lock for Mbed TLS's key store, SHA instructions.
  The native server now sends session tickets as Go's does (TLS 1.3
  resumption with ECDHE, TLS 1.2 tickets; keys rotate daily), so
  returning clients skip the certificate signature. `bench/perf/http`'s
  loadgen has `-resume` (`RESUME=1` in run.sh).
- `xo lsp` (decision 0114): a Language Server Protocol server with
  diagnostics and their fixes as quick fixes, hover (signature, doc, or
  type), definition (into std too), references, rename, document
  symbols, semantic tokens, and formatting. The VS Code extension starts
  it; ```xo fences in Markdown are highlighted.
- VS Code syntax highlighting (decision 0113): `editors/vscode/` registers
  `.xo` files with a TextMate grammar covering spec section 1 (keywords,
  contextual words, interpolation, raw and bytes strings, durations, sets,
  effects in `uses`). Install by symlinking it into `~/.vscode/extensions`.
- `use c "<header>" [link "<lib>"]... [as name] [uses ...]` on the
  native backends (decisions 0093 stage 2 and 0116): bindings generated
  from the header with clang (`xo bind --c`), scalars, Str, Bytes with a
  length, flat structs, and opaque handles at the boundary, the `ffi`
  effect unless the line narrows it, XO1104/XO1105 for headers and names
  that do not bind, XO1201 on the Go backend. Calls are not recorded
  yet (native builds do not record).
- Decision 0111: passing one variable, or overlapping places (`x` and
  `x.f`, `xs[i]` and `xs[j]` unless both indexes are different
  literals, a `var` receiver and an argument naming it), to two `var`
  parameters of one call is `XO0503` on every backend; the native
  backends no longer report XO1201 for it.
- Decision 0112: a value that holds a handle (`Mutex`, `Atomic`, `Chan`,
  `Task`, `Scope`, `Listener`, `Conn`, `task.Group`, the std/http and
  std/sql handles, the test fakes) or a function value is not
  comparable: `==`/`!=`, Map keys, Set elements, `derive Eq`/`Hash`, and
  `T: Eq`/`Hash` arguments are `XO0404` with a note naming the field
  that holds it; `derive Ord` already was. The checker now looks into
  struct fields and enum payloads for `Eq` (it looked only at type
  arguments).
- Native backends (decisions 0107 to 0109): `Proc.run`, `os.tasks` (a
  scope around main and around each test), method values
  (`let f = x.method`), `map_concurrent_until`, base64,
  `Range.to_list`, `Rand.shuffle`, `target` (a compile time constant),
  and in native test binaries MemFs (every method, `fail_next`, case
  sensitivity, `files`), FakeProc `run` and `on_run`, `os.proc.exit`
  inside `testing.run_main`, and `run_main` of a main that cannot fail.
  Fixed natively: a receive arm after an `after` arm lost its value; a
  `?` in a closure printed the function's frame name without
  `.closure`, and one in a `Mutex.with` closure none;
  `FakeClock.advance` returned before the tasks it woke ran; the
  success type of `Err(e)?` was XO1201. One binding passed to two `var`
  parameters is now XO1201 natively (the Go backend shares it, the
  native backends copied it).
- TestCompileProperty (decision 0110): random checker valid programs over
  the optional syntax must build on the Go backend and with `--no-gc`,
  and run alike.
- Fixes from Codex r3: a `select` arm that receives without binding the
  value no longer generates Go that fails to compile; the "propagate with
  ?" fix is offered only where `?` can propagate; help notes for XO0205
  in closures and for passing `TestOs` where `Os` is expected.
- Native backends (`--no-gc`, LLVM and arm64): interface types as values
  (decision 0100): dynamic dispatch, adapters for methods that cannot
  fail, error conversion, interface to interface conversion, Display and
  Debug of the dynamic value, and `==`; `x.debug()` on any value.
- Native backends: newtypes (`Email("x")`, `e.0` read and assigned,
  `Email(p)` patterns, Debug, `==`, Map keys) and list patterns
  (`[]`, `[a, .., z]`). Fixed: an or pattern whose alternatives bind the
  same name (`Leaf(v) | Node(left: v, ..)`) read the first alternative's
  binding natively, a stale value when another alternative matched.
- Native backends (decision 0101): Float32, `Secret[T]` redacted in every
  output path, `Buf[T]`, `<` on `derive Ord` types, `Int32.try` and the
  other sized `try`s, `Int.pow`, `clamp`, `min`/`max` on Str, Set union,
  intersect, difference, `Map.filter`, `List.insert`, `remove_at`,
  `indexed()`, `xs + ys`, std defaults (`crypto.token(r)`,
  `Chan.new()`), `get_or_fault` on any key type, builtin collections
  through interfaces, shorthand field patterns. `==` on a value holding
  a function or runtime handle outside an interface is now XO1201
  natively (it failed in code generation). New test
  TestGenGoldensNative runs 66 internal/gen goldens on Go, LLVM, and
  arm64 and compares them.
- Native backends: `std/path` (std/native/path.xo), `task.semaphore`,
  `task.once` (a fault in its function is not stored natively: the next
  caller runs it again), and `task.map_concurrent`
  (std/native/task.xo); Debug text of `Result[T, Never]` values. Fixed:
  natively, a task whose value is a Result failed its scope when that
  value was an `Err`; shorthand struct patterns `Bag{items}`.
- Native backends (decision 0102): every Fs method and attenuation
  (`sub`, `cwd`, `read_only`, `write_only`, escapes `Denied`),
  `Stdio.write`, and standard input reads (`read_line`, `read_all`,
  `lines`) that a cancellation or a stop request ends with
  `IoErr.Canceled`. Fixed natively: `task.with_timeout` returns
  `TimedOut` when its closure finishes after the deadline without a
  cancellation point, as on the Go backend.
- Native backends: error frames (decision 0104): each `?` records the
  function, location, and parameters lazily, and main's error report
  prints `  at ...` lines identical to the Go backend's.
- Native Fs and standard input code is compiled into a program only when
  it calls them (XO_FS, XO_STDIN; hello world unchanged). Fixed: hybrid
  builds (`--hybrid`) declared a Bool result's argument size rounded up
  (go vet -asmdecl).
- `xo test --no-gc`: `FakeStdio.set_input` feeds the reads; fixed a hang
  after `FakeProc.signal_after` fired (the virtual clock stopped moving).
- Native backends: all of `std/time` (decision 0094; std/native/time.xo
  and std/llrt/xo_time.c, a port of Go's TZif reader and zone lookup):
  zones with the same per use embedded data (or `--tzdata=system`), civil
  time, gaps and overlaps, layouts, strict parsing with the same error
  texts and faults; also `Duration.from_secs`, `scale`, `ceil_seconds`,
  `Time.parse_rfc3339`, and `Time.UNIX_EPOCH` as a value. Programs that
  do not use std/time do not grow. TestTimeDiff compares about 70,000
  lines with the Go backend. Fixed: the arm64 backend failed to assemble
  functions over 8192 instructions (`tbz` range).
- HTTPS in std/http on every backend (std.md 4.6, decisions 0105 and
  0106): `http.tls_cert(cert_pem, key_pem)`, `Server.with_tls(certs)`
  (SNI picks among the certificates), `Client.with_roots(pem)`, and
  `TlsErr`. The native backends (`--no-gc`) speak TLS 1.2 and 1.3 through
  Mbed TLS 4.1.1 (vendored in `third_party/mbedtls`, built once per
  compiler into the user cache, linked only into programs that use
  std/http), verify against the system's roots on macOS and Linux, and
  report Go's error texts; an `https` URL is no longer `XO1201` there.
  std/http speaks HTTP/1.1 only, also over TLS: the Go backend's client no
  longer negotiates HTTP/2. Failed TLS handshakes are not logged.
- `bench/perf/http/run.sh`: `SCHEME=https` serves the Xo configs over TLS
  and `NEWCONN=1` opens a connection (and a handshake) per request.

## v0.8.0 (2026-10-09)

Spec draft 0.8.

- Modules (decision 0086 steps 1 and 2): `require <path> <version>` of Xo
  modules, minimal version selection, git fetch into a read only module
  cache, `xo.lock` hashes verified on every build, `xo get`, `xo mod
  tidy`, `xo vendor`, `xo mod clean`, `--offline`, and `xo.work`
  workspaces (`--workspace=off`). Diagnostics XO1301 to XO1308.
- Codex r2 first run failures: `XO0175` for several match arms on one
  line has a split fix and no longer cascades into a false `XO0301`;
  `.display()` works on every `Display` type (derived, std errors,
  builtins); a closure that never completes infers `Never` instead of
  `XO0402`.
- Effect ceilings and checked semver (decision 0086 steps 3 and 4):
  `require <path> <version> uses none` in xo.mod and `use <dir> uses ...`
  in xo.work (XO1309 at a call above the ceiling), API and effect diffs
  from `xo get` (and `xo get -u`), `xo publish [--check]` against the
  previous tag (XO1310, XO1311; prints the tag command, never pushes),
  `xo init`, `/vN` major version module paths, and `xo serve` noticing a
  new or changed xo.work above the directories.
- `std/time` (decision 0094, Go backend): zones with per use embedded
  data (`--tzdata=system`), civil dates and times with explicit daylight
  saving gaps and overlaps, layouts with named fields checked at compile
  time (XO0420; XO0177 for `{yyyy}`, `%Y`, `2006-01-02` habits).
- Import direction rules (decision 0095): `deny <pkg> -> <pkg>, ...`
  lines in xo.mod with `/...` patterns, checked at every `use` line
  (new code XO1312; a rule naming no package is XO1301), kept by `xo mod
  tidy`; `xo query imports` and `xo query deps` list the `use` lines
  (also in the MCP `query` tool).
- std/time error text (std.md 13): `ZoneErr` and `CivilErr` display as
  sentences (`unknown time zone "Mars/Olympus"`, `2026-03-08T02:30:00
  America/New_York does not exist (skipped by a daylight saving
  change)`) instead of the derived `Skipped(at: ...)` form, matching
  `ParseErr` and `LayoutErr`.
- Newtypes (decision 0097, spec audit gap 4): `e.0` reads the underlying
  value and `Email(s)` destructures it in `let` and `match`.
- `xo check --fix` (decision 0096): applies the unambiguous machine fixes
  (as `xo fix`), reformats, then reports what remains plus an `applied`
  list; the MCP `check` tool takes `fix`, and the bench harness agent
  loop uses it while still scoring first runs as written and after fix.
- Go backend output (decision 0098): hello world 7.9 to 3.6 MB, Go build
  cache growth per program 34 to 2.9 MB, compile memory 286 to 123 MB.
  Binaries carry no DWARF or symbol table (`XO_GO_DWARF=1` keeps them for
  a debugger); programs that do not import std/http do not link net/http.
- Spec audit second pass: `x.debug()` works on every value (std.md 1;
  was XO0601 on concrete types); new tests for single field positional
  variants, `Secret` redaction in Debug and log fields, `target` at run
  time, `Stdio.write`, `Fs.cwd`, `Fs.case_sensitive`, a real `Proc.run`,
  `XO-F005`, `xo test --real`, and byte identical rebuilds (spec 12.1 now
  says "in the same directory").
- Modules in repository subdirectories (decision 0086): a module path
  `github.com/acme/tools/cli` is the directory `cli/` of the repository
  `github.com/acme/tools`, versioned by tags `cli/vX.Y.Z` (as Go);
  fetch, MVS, xo.lock, `xo vendor`, `xo get`, and `xo publish [--check]`
  (prefixed tag command) handle it; nested modules are left out of a
  module's tree and hash.
- Go calls in record and replay (decision 0093 stage 1): `--record`
  records `use go` calls whose arguments and results are values (an
  argument hash and the results) and `xo replay` answers them without
  calling Go; calls with a Go handle or a callback run again and the
  replay reports ``re executed foreign call `<name>` ``; pure std
  packages and xo.mod `replay rerun go <path> [names]` lines are not
  recorded. Binding format v5; traces gain `"foreign": 1`, and older
  traces still replay (every Go call runs again).
- Release builds (decision 0099): `xo build --release` compiles out
  `requires(debug)` and `ensures(debug)` on every backend and reports
  XO1308 for workspace overrides; `--ci` does not imply it.
- Lazy error frames (decision 0092, Go backend): frames keep copies of
  the parameter values and format them, bounded to 64 bytes, only when a
  trace is printed. 1,000,000 errors through 5 frames: 0.93 s to 0.10 s
  with short parameters, 2.78 s to 0.10 s with 90 byte ones; error output
  unchanged. `xo build --release --error-args=false` drops the values.

## v0.7.0 (2026-10-09)

First tagged release. Spec draft 0.7.

- Language: request scoped arenas by default, `use go` packages, two
  phase shutdown, `Mutex.with_read`, `BytesBuilder`, rune and integer
  conversions, native contracts and fault unwinding, habit diagnostics
  (XO0177), `clock.sleep_or_cancel`, cancellable stdin, tuple JSON,
  `Duration.parse` (decisions 0041 to 0088).
- Backends: Go (default), LLVM, native arm64, `--no-gc`, `--hybrid`.
- Toolchain: `xo version`.
- Planned before 1.0: modules and workspaces (0086), `std/time`, native
  TLS, and decisions on the Appendix B questions.
