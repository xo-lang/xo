# 0273: --race on the native backends with a prebuilt TSan runtime

Status: Proposed (owner decision 2026-10-19, through the lead; Xo Lang 2).
Part 1 (what --race checks, the two modes) accepted by the lead
2026-10-20 and implemented on the LLVM backend; part 2 on the arm64
backend (2026-10-20); part 3's link by Xo's Mach-O linker (2026-10-20);
the shipped runtime, Linux, and amd64 in progress.

## Context

`--race` builds with LLVM today (decision 0270: "--race picks LLVM"),
which needs clang and its ThreadSanitizer runtime on the machine. The
owner wants `--race` on Xo's own native backends, shipped the way Go
ships its race detector: a prebuilt runtime in the release archive and
seeds, linked by Xo's own linkers, so `--race` needs no clang.

The first experiment (2026-10-20, darwin/arm64): the native backend's
assembly linked with clang's ThreadSanitizer runtime runs (conc_channels
equal to its golden), the runtime turns its fiber hooks on, and a
`__tsan_write8` planted by hand in a function two tasks call is reported
as a race between a task fiber and a worker thread. So linking, shadow
memory, and fibers as ThreadSanitizer threads work; what was missing is
what --race checks.

Until 2026-10-20 --race checked the runtime and the program together:
every function instrumented, the runtime's fibers switching with
`__tsan_switch_to_fiber(f, 0)`, which makes a switch a synchronization.
That ordered every task of a thread after the one before: a race between
two tasks on one thread was never reported (four tasks on XO_THREADS=1:
2 reports, all against the worker's own context; 20 once switches
synchronize nothing; gotchas/tsan-fiber-switch-sync.md). Switching
without synchronization while the runtime stays instrumented turns the
runtime's per thread state into false races, one structure after another
(the scheduler's hand offs, the allocator's thread caches, the heap
record's counters, the spare task context, the deferred release queue):
the fibers of one thread hand them on with no ordering ThreadSanitizer
sees. Checking the runtime between threads and checking the program's
tasks need opposite setups.

## Decision

### Part 1: two modes (LLVM backend, implemented)

**Mode 2, `--race`: races between the program's tasks**, as Go's race
detector finds races between goroutines:

- The runtime (std/native/llrt) is not instrumented (compiler/llvm
  func.xo race_unchecked: each attribute set's twin without
  sanitize_thread, #13 to #17), but llrt/atomic.xo (Atomic's add and max:
  their atomics are the program's, on the cell the inline get and set
  use). The program and the rest of std are.
- Every task runs in a ThreadSanitizer context of its own, made at its
  spawn by the spawner (sanhook.xo task_new: a fiber's creation
  synchronizes with its creator, so it is made at the spawn, ordering
  only what came before it) and taken by tasks.xo run_claimed whether
  the task runs on a fiber, on a cached fiber after another task, or
  inline in an await or a scope's close; it goes in the place a park's
  switches read and is freed at the run's end (or with a task that never
  ran).
- Fibers switch with no synchronization (`__tsan_switch_to_fiber_no_sync`).
- The heap is the C library's malloc and free (heapos.xo large_room,
  libcmem.xo malloc_tc: every block a big one), whose interceptors reset
  a block's shadow memory, so a block another task freed is not a race
  with its new owner (Go resets a block's shadow at allocation too).
- The runtime tells ThreadSanitizer what orders tasks, at every
  synchronization a program can use: a spawn releases the task, its run
  acquires it; a task's end releases the task and its scope, an await
  (finished before or waited for) acquires the task, a scope's close the
  scope; a send, try_send, or close releases the channel, a receive or
  try_recv acquires it; a select releases its send arms' channels and
  acquires the chosen arm's; a lock or read lock acquires the mutex, an
  unlock or read unlock releases it.

The table that makes mode 2 trustworthy (internal/llvm
TestRaceSyncTable, testdata/race_sync): each synchronization used
correctly reports no race, and the same program with it left out reports
one, with one worker thread (every task a fiber of it) and with the
default:

| synchronization | correct: no race | left out: a race |
|---|---|---|
| spawn | yes | yes |
| await (join) | yes | yes |
| scope close | yes | yes |
| channel, unbuffered | yes | yes |
| channel, buffered | yes | yes |
| select | yes | yes |
| mutex | yes | yes |
| await of a finished task (its fast path) | yes | yes |
| Atomic set and get | yes | yes |
| Atomic add | yes | yes |

TestTSanSameThreadRace: four tasks race on one thread, and a report
between two task fibers is required.

**Mode 1, the runtime's own race tests**: the runtime instrumented
(XO_TSAN_RUNTIME=1 when building: compiler/llvm Gen.tsan_runtime), fibers
synchronizing at a switch, Xo's own heap; the binary defines
`xo_tsan_runtime` (internal/llvm tsanBuildRuntime), which the runtime
reads (sanhook.xo user_mode). TestRuntime*, the task goldens under
ThreadSanitizer, TestSysCTSan, TestH2Sanitizers, and the shared fault
test run in it, as before. Races inside the runtime are found there and
by the ordinary tests; a --race build does not exercise Xo's thread
caches.

### Part 2: the arm64 backend (implemented)

compiler/arm64 race.xo, a pass over a function's machine instructions
after selection and before register allocation (its calls are K_CALL,
so the allocator keeps values across them as across any call), when the
module is built with --race (ssa Module.race):

- each load and store whose address is not in the frame calls
  `__tsan_readN` or `__tsan_writeN` (N its width: 1, 2, 4, 8, 16; a pair
  load or store one call of both words) with the address, computed into
  a new register (`add` or `sub` of the immediate, `add` of a register
  offset with its extension);
- an atomic with release order (`stlr`, an LSE read modify write or
  compare and swap with `l` or `al`) calls `__tsan_release` before it, one
  with acquire order (`ldar`, `a` or `al`) `__tsan_acquire` after it: the
  same edges the LLVM backend's instrumentation of an atomic gives;
- the runtime is not instrumented, as in mode 1's LLVM build: a function
  of std/native/llrt but atomic.xo (race_unchecked, the same rule as
  compiler/llvm func.xo's), and an instruction the arm64 inliner copied
  from such a function into the program's (its innermost inline site's
  file);
- not instrumented: frame accesses (sp, x29, outgoing arguments), the
  hand written templates of several lines (the crypto kernels) but the
  one atomic line of a template such as compare and swap's `mov`, `cas`,
  `cmp`, `cset` (race_line), templates with fixed registers, an
  exclusive load and store pair (a call between them clears the
  monitor), and an access between an instruction that sets the flags and
  one that reads them (the call clobbers them).

The per function back-end cache is off with --race (its keys do not say
so). The table above holds on it too (internal/llvm TestRaceSyncTableArm64, all 40 runs; the
first run reported a false race for Atomic add, whose compare and swap,
a template of four lines, was not instrumented, so its release was
missing), and TestTSanSameThreadRaceArm64 reports a race between two task
fibers of one thread. amd64 (the x86 templates) is not instrumented yet.

### Part 3: the link (darwin implemented) and the shipped runtime

- `xo build --race --backend=arm64` on darwin links with Xo's Mach-O
  linker (compiler/xo ld.xo): ThreadSanitizer's runtime is a dylib the
  executable loads by its path (a second LC_LOAD_DYLIB after
  libSystem; its interceptors take effect through its `__interpose`
  section), found by race_runtime: `$XO_RACE_RUNTIME`, else
  lib/xo/race/darwin-arm64/libclang_rt.tsan_osx_dynamic.dylib next to
  xo's std (the release archive's and the seed's layout); none is an
  error naming --backend=llvm. With --ld=cc, and for Linux, --race on the
  arm64 backend is refused for now (Linux: its assembly links with
  clang, as the tests do there). Tested by the same table
  (TestRaceSyncTableArm64 builds with xo build on darwin, the runtime
  clang's until one is shipped: 40 of 40, 2026-10-20).
- The runtime: LLVM compiler-rt's ThreadSanitizer (C++), at a pinned
  release (23.1.3, the source checked against its pinned SHA-256 sum;
  21.1.8 crashed on macOS 27, gotchas/tsan-runtime-macos-version.md),
  built per platform by scripts/race-runtime.sh (never on users'
  machines; darwin/arm64 so far: the dylib), shipped in
  lib/xo/race/<goos>-<goarch> with compiler-rt's LICENSE.TXT in
  lib/xo/race: a release's lib/xo/race beside lib/xo/std (release.sh),
  a seed's xo/lib/xo/race (seed-cut.sh builds it, seed-pack.sh packs
  it); race_runtime looks in both. Not in the repository (/lib/ is
  ignored).
- License: Apache 2.0 with LLVM Exceptions, in THIRD_PARTY_NOTICES
  (entry 2); the one non-Xo component of a build, loaded only with
  --race.

## Consequences

- A --race report is about the program's tasks, two of one thread too;
  the runtime's frames are not in it.
- A race inside the runtime is not a --race finding: mode 1 and the
  ordinary tests are where those are found.
- Every new synchronization in the runtime needs its release and
  acquire, and a row in the table, or --race reports false races (left
  out) or hides real ones (too broad).
- A --race program allocates through the C library and makes a
  ThreadSanitizer context per task: slower, as any race build is.
- The leak check counts nothing under --race (every block the C
  library's); the ordinary builds' leak checks stand.

## Alternatives

- Keep one mode, the runtime instrumented, and patch each piece of per
  thread state as it shows up: tried (2026-10-20); each patch exposed the
  next (the scheduler, three allocator layers, the spare context, the
  deferred release queue), with no clear end, and runtime internals in the
  program's reports.
- Release and acquire on each block at free and alloc instead of the C
  library's heap: every reused block would order the freeing task's past
  before the next owner, hiding part of what --race is for.
- ThreadSanitizer's Java heap API (`__tsan_java_alloc`, which resets a
  range's shadow): needs one contiguous heap range declared at start,
  which Xo's heap is not.

## Addendum (2026-10-21): linux/arm64 (owner-queued item 1)

The native arm64 backend's --race links on linux/arm64 too, with Xo's
ELF linker (compiler/macho elflink.xo), the default linker of a Linux
race build (with the C library, dynamic: ThreadSanitizer's runtime is
built on it):

- **The runtime**: compiler-rt's tsan at the pinned LLVM 23.1.3 as the
  static archive libclang_rt.tsan-aarch64.a (scripts/race-runtime.sh,
  in Docker linux/arm64), built with -mno-outline-atomics (the inline
  ARMv8.0 loops: no libgcc __aarch64_* helper, of which the linker
  writes only the 4 and 8 byte ones) and -fno-threadsafe-statics (no
  __cxa_guard_* from a C++ runtime Xo does not link; the runtime
  initializes once, in __tsan_init, before any thread). Found as
  lib/xo/race/linux-arm64/ next to std, or $XO_RACE_RUNTIME.
- **The link**: the archive is linked whole (LinkInput.whole, ld's
  --whole-archive, as clang -fsanitize=thread does): __tsan_init is
  reached only through a member's .preinit_array. The ELF linker gains
  the constructor arrays (.preinit_array, .init_array by priority,
  .fini_array; DT_PREINIT_ARRAY, DT_INIT_ARRAY, DT_FINI_ARRAY and their
  sizes; .ctors and .dtors stay refused), the linker defined _DYNAMIC
  (the runtime reads it), libdl.so.2's imports (dlsym, dladdr: glibc.xo
  regenerated with libdl, a DT_NEEDED only when used), and
  libc_nonshared.a's pthread_atfork (a stub calling __register_atfork,
  as the stat family's).
- **Atomics**: on Linux the read modify writes are calls of outline
  atomics (decision 0200), which race.xo, reading instructions, does not
  see as atomics; the lowering itself puts __tsan_release before and
  __tsan_acquire after such a call by its order (compiler/arm64 calls.xo
  race_release, race_acquire), outside the runtime's code. Without it a
  synchronized Atomic.add reported a race.
- **Measured**: in Docker linux/arm64, testdata/race_sync's table (ten
  cases, one and several threads, with and without the
  synchronization) 40 of 40 as TestRaceSyncTable expects, and
  testdata/tsan_race reports its races with Xo's source lines.
- **Shipping** (owner, 2026-10-21): the archive (5.2 MB) ships in
  lib/xo/race/linux-arm64 of the release and the linux-arm64 seed, as
  darwin's dylib does, from the next seed on. seed-cut.sh builds it at cut
  time in Docker (`scripts/race-runtime.sh WORK OUT linux-arm64` on the
  Mac: the same pinned compiler-rt sources, checked against their SHA-256
  sum) into the linux-arm64 cross layout, which seed-pack.sh packs;
  seed-cut refuses to start while Docker is not running. For a release,
  the same command with OUT the checkout puts it where release.sh takes
  lib/xo/race/PLATFORM from. xo finds it next to std by default;
  $XO_RACE_RUNTIME stays an override.

## Addendum (2026-10-21): amd64 (owner-queued item 2)

- Instrumentation: the arm64 pass (compiler/arm64 race.xo) runs for
  --backend=amd64 too. It works on the MIR's arm64 templates before
  x86_retarget, so its address arithmetic and __tsan_* calls are
  retargeted like any other instruction. The x86 atomics (xchg, lock
  xadd, lock cmpxchg) are x86 templates race.xo does not read: calls.xo
  calls __tsan_release before and __tsan_acquire after them by their
  order, as the Linux arm64 outline atomics do. Plain loads and stores,
  ldar and stlr are parsed as on arm64. The hand written multi-line
  templates (crypto) stay uninstrumented, as on arm64. Without --race the
  output is unchanged (both additions are under the race flag).
- Runtime: scripts/race-runtime.sh builds linux-amd64
  (libclang_rt.tsan-x86_64.a, in Docker linux/amd64, with
  -ftls-model=initial-exec: sanitizer_coverage's thread local was general
  dynamic, an R_X86_64_TLSGD Xo's ELF linker does not relax) and
  darwin-amd64 (the dylib, cross built on the arm64 Mac); seed-cut.sh
  packs both into their seeds. Xo's linkers link them as on arm64.
- windows/amd64 --race is refused (compiler-rt's ThreadSanitizer has no
  Windows port; item 3 explores Go's route).
- Verified on the owner's Intel Mac (darwin/amd64, binaries cross built
  here): the race_sync table (TestRaceSyncTable's 40 cases) all as
  expected, and tsan_race reports the race between two task fibers. Linux
  amd64 links but was not run: ThreadSanitizer, clang's own C programs
  included, does not start under Docker's x86 emulation on this Mac
  (gotchas/tsan-amd64-not-on-arm64-mac.md).

## Addendum (2026-10-22): windows/amd64 through ThreadSanitizer's Go runtime (owner-queued item 3)

Status: Accepted in direction (lead, owner's overnight delegation,
2026-10-22). Windows --race is not called supported until one Windows CI
run (threads) passes; that run needs the owner's approval (morning).

compiler-rt's ThreadSanitizer has no Windows port for C programs, but its
Go flavor (lib/tsan/go, built into Go's race_windows_amd64.syso) runs on
Windows. Xo would use that flavor on windows/amd64.

- API: every call names the goroutine's ThreadState (Xo: one per task, the
  task's T_TSAN word, and one per root/worker context), a Processor per
  worker (CallbackGetProc returns the current worker's), and a pc. Entry
  points: __tsan_init/fini, map_shadow, read/write(_pc, _range),
  func_enter/exit, malloc/free, go_start/go_end (spawn, task end),
  acquire/release/release_merge, mutex hooks, go_ignore_sync_begin/end.
  No interceptors: llrt's explicit synchronization (mode 2) is told
  through the same table, with thr.
- Lowering: a third variant of compiler/arm64 race.xo for windows/amd64:
  thr loaded from the task context (TLS slot, or kept in a callee saved
  register for the function), pc the call's return address
  (__tsan_read_pc with the caller pc). func_enter/exit on every
  instrumented function (Go reports stacks from them; no unwinder).
- Memory: app memory must lie in 0x1000 to 0xe0_0000_0000; the runtime
  calls __tsan_map_shadow for each region heapos maps (VirtualAlloc with
  addresses in that range) and for fiber stacks.
- Stack: TSan's functions need a large stack (Go switches to g0); Xo's
  fiber stacks: measure the deepest TSan path or switch to the worker's
  system stack around report paths only.
- Symbolization: CallbackSymbolizeCode answers from Xo's own line tables
  (the binary's DWARF or a table the runtime keeps under --race).
- Build: our own, from the pinned LLVM 23.1.3 (not Go's prebuilt
  object): tsan/go's buildgo.sh GOOS=windows with mingw in Docker, one
  COFF object linked by Xo's Windows link (decision 0254 6b's path; it
  needs synchronization, ntdll, dbghelp and psapi imports), shipped as
  lib/xo/race/windows-amd64/ with compiler-rt's license.
- Size estimate: lowering variant ~200 lines, runtime hook table and thr
  plumbing ~300, symbolize callback ~150, build script ~80, tests (race_sync
  table on a Windows CI run, owner approval via the lead).
- Prototype (2026-10-22): a C program driving Go 1.26.1's
  race_windows.syso through this API (init with the three callbacks,
  map_shadow of a heap page, two goroutine states, a planted race, then a
  release and acquire pair), linked by mingw gcc and run under Wine
  (scripts/docker-wine.sh box64, single threaded), reported the planted
  race with the callback's frames and nothing for the synchronized pair.
  The Go flavor's shadow layout works under Box64's Wine, where the C
  flavor does not start under Docker's x86 emulation.

### Implementation (2026-10-22, Xo Lang 2)

- Runtime object: `scripts/race-runtime.sh WORK OUT windows-amd64`
  (pinned compiler-rt 23.1.3, MinGW-w64 in Docker linux/arm64) writes
  lib/xo/race/windows-amd64/race_windows_amd64.syso; scripts/race-windows.cpp
  adds what MinGW's C runtime would (DumpProcessMap, __popcountdi2, _errno,
  exit) and three wrappers: xo_tsan_go_acquire, xo_tsan_go_release (a merge,
  as the C flavor's release) and xo_tsan_go_init (__tsan_init with
  cabi_race.xo's callback). seed-cut.sh puts it in the windows-amd64 seed.
- Link: Xo's PE linker with the object, its imports in compiler/macho
  peimports.xo (win-import-table.py's RACE list), the image at its fixed
  base with no ASLR (PEOpts.fixed_base), .bss in no file bytes (15 MiB in
  the object).
- Lowering: race.xo race_sym: on windows the calls go to
  std/native/llrt/cabi_race.xo's exports (xo_x_race_read8 ...), called as
  System V functions with the address, which add the task's ThreadState
  and the pc (the export's return address); declared only in a windows
  --race build (Checker.race). No func_enter/exit yet: a report has one
  frame per access, and no names (race_cb answers SymbolizeCode with
  none).
- Runtime: sanhook.xo gorace() (rt.sym_present("__tsan_go_start"),
  folded by the builder to the --race flag on Windows). The running
  context's ThreadState in thread table word 6, the Processor in word 10;
  workers, fibers and tasks get theirs from __tsan_go_start, switched at
  the scheduler's hooks; a thread with none gets one at its first use.
  Memory: one 64 GiB arena reserved at 0x00c0_0000_0000, taken upward in
  128 KiB granules, each shadowed as it is taken (heapos.xo map, fiber
  stacks); every block from the race heap (heapos large_room, libcmem
  malloc_tc), its shadow reset. __tsan_fini at an exit with status 0
  (66 when a race was reported).
- Verified under Wine (Box64, XO_THREADS=1, single threaded, as the
  owner allows for Wine): tsan_race reports its planted race (exit 66);
  race_sync's 11 cases each report nothing synchronized and the race
  unsynchronized (lazy_const, which has no race, nothing either way).
  Threads are untested until the Windows CI run (owner approval).
  gotchas/tsan-go-flavor-windows.md has the traps.
