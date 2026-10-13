# Platform library calls from Xo on macOS and Windows (decision 0213)

Date: 2026-10-13. Branch of the platcalls agent. Mac: Apple M4 (darwin/arm64,
macOS 27), shared with about ten agents (counts, not times, are the
figures). xo1go is selfhost/xo built by the Go backend of the branch
(`XO_SELF_DRIVER=0 xo build -o xo1go ./selfhost/xo`, commit f5c28b49),
so its emitters are the branch's selfhost/arm64 and selfhost/llvm; the
Go driver (`XO_SELF_DRIVER=0`) runs internal/arm64 and internal/llvm.

## What was built

- Declarations std/native/sys_darwin.xo (open, read, write, close, stat,
  lstat, fstat, clock_gettime, clock_gettime_nsec_np, getentropy,
  `__error`) and std/native/sys_windows.xo (CreateFileW, ReadFile,
  WriteFile, CloseHandle, GetLastError, GetFileInformationByHandle,
  GetFileAttributesW, GetFileType, GetSystemTimePreciseAsFileTime,
  BCryptGenRandom): calls `rt.pc.<os>.<sym>` in both SSA builders, a
  fault for another OS or a wasm module.
- Emitters: a call of the bare symbol with the C ABI in selfhost/arm64,
  internal/arm64 (Apple variadic arguments on the stack: open's mode at
  `[sp]`), selfhost/llvm, internal/llvm (C types, `...`, `dllimport` on
  Windows); wasm faults.
- Layers std/native/llrt/darwin.xo and windows.xo (linux.xo's
  interface), and std/native/llrt/sysio.xo as the portable policy over
  Linux, darwin/arm64, and Windows.

The arm64 code of darwin.open (xo1go, `otool -tV`):

```
bl   _xo_f_darwin_2ecpath
add  x0, x20, #0x10        ; the path's data
mov  x1, x19               ; flags
str  x21, [sp]             ; the variadic mode, on the stack
bl   _open
sxtw x19, w0               ; an Int32 result
...
bl   ___error              ; errno after -1
ldr  w9, [x0]
```

## Correctness on darwin/arm64 (Mac)

- internal/llvm TestGoldenWant with xo1go (`XO_GOLDEN_WANT=1
  XO_GOLDEN_WANT_XO=xo1go XO_STD=<worktree>/std`): golden summary
  (darwin/arm64): 153 pass, 0 fail, 0 unsupported, of 153.
- internal/arm64 TestGoldenRun (Go driver, every golden on the arm64
  backend against want.txt): ok.
- sys_files_clock with xo1go on `--backend=arm64` and `--backend=llvm`
  and with the Go driver on both: identical to want.txt.
- TestSysioParity (new, internal/llvm; testdata/sysio_parity: read,
  stat, and write of a file, a missing path, a path through a file, a
  directory, `.`, a symbolic link loop, a dangling link, a mode 0 file,
  a 300 byte name, a 1,201 byte path, trailing slashes, the empty path):
  "darwin/arm64: 43 lines identical", the Xo path against
  `XO_LLRT_C=sysio`, with the Go driver; the same program built by
  xo1go on `--backend=arm64` and `--backend=llvm`: identical (43 lines
  each). A changed
  strerror text in darwin.xo fails it (checked, then reverted).
- TestErrnoTables (new): darwin.xo's strerror table equals libSystem's
  strerror for errno 0 to 160 (testdata/errno_probe.c).
- A build with `XO_LLRT_C=sysio` has no darwin.xo function (nm: 0
  symbols) and the default build has them: the parity compares two
  different programs.
- cmd/xo TestNoGCTests with `XO1=xo1go`: ok (every native test
  directory on both backends against the Go backend).
- internal/selfhost TestSelfhostSSA, TestSelfhostArm64, TestSelfhostLLVM
  (the Xo builders and emitters against the Go ones, byte for byte):
  pass (a first run failed only because std/native/llrt changed while
  it ran; the rerun on a fixed tree passed).
- internal/selfhost TestSelfhostWasm: wasm goldens: 133 pass, 20
  unsupported, 0 fail of 153 (the darwin and windows layers are lowered
  there; their platform calls are faults).

## Bar: the Xo path against the C path on macOS (decision 0180)

`bench/perf/sysio/measure.sh xo1go <std> <out> arm64 llvm` (macOS mode
added: `/usr/bin/time -l` "instructions retired" of the process, user and
kernel together, minimum of 5 runs). Workloads: read (os.fs.read of a 4
KiB file, 20,000 times), read_big (a 1 MiB file, 300 times), write
(20,000 writes of 100 bytes), stat (50,000), clock (500,000 Clock.now).
write is kernel bound: its three runs (write, write2, write3) show the
noise.

| Workload | Backend | Xo path | C path | Xo / C |
|---|---|---|---|---|
| none | arm64 | 19,109,070 | 19,390,831 | 0.985 |
| none | llvm | 16,750,380 | 16,527,712 | 1.013 |
| read | arm64 | 3,761,525,625 | 3,911,529,842 | 0.962 |
| read | llvm | 3,752,870,809 | 3,875,886,290 | 0.968 |
| read_big | arm64 | 226,897,910 | 403,542,653 | 0.562 |
| read_big | llvm | 211,165,276 | 359,051,217 | 0.588 |
| write | arm64 | 9,726,150,119 | 9,605,025,669 | 1.013 |
| write | llvm | 9,698,726,700 | 9,878,739,235 | 0.982 |
| write2 | arm64 | 9,246,315,612 | 9,187,415,520 | 1.006 |
| write2 | llvm | 9,449,113,166 | 9,465,067,013 | 0.998 |
| write3 | arm64 | 9,314,076,560 | 9,353,458,322 | 0.996 |
| write3 | llvm | 9,330,836,012 | 9,543,698,375 | 0.978 |
| stat | arm64 | 1,758,014,476 | 1,765,411,881 | 0.996 |
| stat | llvm | 1,533,674,957 | 1,546,240,997 | 0.992 |

The files meet the bar. read_big reads into one buffer of the file's
size plus one (Go's os.ReadFile), where the C path's fread loop
(xo_fs_read) copies through a 64 KiB stack buffer into a growing one
and then into the result.

### The clock

| Variant | arm64 Xo / C | llvm Xo / C |
|---|---|---|
| clock1: layer() choice, clock_gettime into rt.sys_buf, two words | 1.402 (131,990,854 / 94,126,796) | 1.256 (112,944,127 / 89,946,758) |
| clock2: layer() choice, clock_gettime_nsec_np | 1.042 (97,890,561 / 93,951,824) | 0.934 (83,892,609 / 89,847,502) |
| clock3: `match target.os` inline, `==` on target.arch | 1.076 (101,014,433 / 93,916,088) | 0.936 (83,960,373 / 89,699,515) |

The arm64 emitter does not fold the `match` on `target` (a `mov x9, #1;
cmp x9, #2; b.eq` chain, and `==` on an enum calls the equality
helper; gotchas/arm64-match-target-not-folded.md), so Clock.now stays
on the C runtime on every OS; darwin.realtime_ns and windows.clock_ns
wait for the fold.

## Linux (sysio.xo's dispatch over linux.xo)

EC2 through the lead's queue (instance
i-EXAMPLE12, r8g.8xlarge, linux/arm64, kernel 6.18.54, clang
15.0.7), commit f8b487e5: TestSysioParity "linux/arm64: 43 lines
identical" with xo1go and with the Go driver; sys_files_clock passes
with xo1go (TestGoldenWant), the Go driver's LLVM (TestGolden) and arm64
(TestGoldenRun) backends.

## Windows (windows/amd64, the lead's queue)

Run 1: commit 0d923a28 (before the errno ordering fix and the clock
revert; the files code is otherwise today's), Windows Server 2025 EC2
instance i-EXAMPLE39 (c7i.4xlarge), clang 23.1.3, xo1 built by
the Go seed (scripts/windows-batch.sh):

- batch: 2 windows/amd64: golden summary (windows/amd64): 152 pass, 1
  fail, 0 unsupported, of 153; the one failure is sys_files_clock,
  which also fails on the C path (`XO_LLRT_C=sysio`) and at main
  (1e066156, the sharedlock agent's batch): the golden prints `long: Io`
  without the message on Windows on purpose, and had no want_windows.txt
  (added, commit 9060e907; gotchas/golden-target-output-needs-want-os.md).
- batch: 2 windows/arm64: 153 built, 0 fail, 0 unsupported, of 153.
- batch: 3 LLVM runtime tests: ok. batch: 4 xo test, LLVM against Go:
  16 same, 0 differ.
- TestSysioParity with xo1: "windows/amd64: 43 lines identical": the
  Xo path (CreateFileW and the rest from Xo, _dosmaperr's table, the
  Microsoft C runtime's strerror texts) gives the C path's errors and
  messages byte for byte.
- TestErrnoTables: the probe did not link (`_get_errno_from_oserr` is not
  exported to lld-link with the Windows SDK); it now prints strerror
  only (commit 9060e907), and the _dosmaperr table is checked through
  TestSysioParity's errors.
