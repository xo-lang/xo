# 0275: Backend cache keys by code generation inputs, not xo's identity

Status: Accepted (owner, 2026-10-20), with the lead's two conditions:
a test that every data file and environment variable the code
generation reads is keyed or classified, and the closure failing
closed to self_id (both under "Safety net" below). Drafted by Xo Lang 3

## Context

Every backend cache key (front end, check, rc, inline, fn, part, object,
binary; compiler/xo bcache.xo, asmparts.xo, cache.xo, main.xo) holds
self_id: the SHA-256 of the xo binary. Any change to xo's binary starts
every cache cold, and every commit to compiler/ or std/ changes it: the
94 cached checkout builds in the user cache (2026-10-20) are 94 distinct
binaries, partly because the provenance stamps (decision 0264) put
XO_COMMIT in the binary.

What that costs, internal/arm64 on the Mac (2026-10-20, load about 8):
767 s with a cold cache, 349 s with the same xo's warm cache. Every gate
builds a new xo, so every gate's suites pay the cold wall; so does every
developer build after a pull.

How often the code generation actually changes: of the 569 commits to
main from 2026-10-13 to 2026-10-20 that touched compiler/ or std/, 118
touched a non-test file of the code generation set below; 451 did not
(391 of them std/native, the native standard library and runtime, and
60 the driver, the tools, or std outside native).

## Decision (proposed)

1. A code generation key, XO_CODEGEN_KEY, joins the provenance stamps:
   internal/xotool (scripts/xobuild, the tests' builds), bootstrap-xo.sh
   and seed-cut.sh compute it and pass it with `xo build --stamp`. It is
   the SHA-256 over, in a fixed order:
   - every file of the compiler modules that decide an entry's bytes
     (the backends and their import closure within compiler/: today
     arm64, llvm, wasm, macho, ssa, check, parser, lexer, cli/bind,
     cli/core/format, tzsel), by path and contents, the set computed from
     the `use` lines, not listed by hand; `_test.xo` files left out;
   - core.VERSION and the cache format versions (each kind's
     *_CACHE_VERSION, already in its key; listed again so a forgotten
     bump is still covered by the sources);
   - the stamp's own version ("codegen key v1").
2. Every cache that keys on self_id keys on XO_CODEGEN_KEY instead, when
   it is set. An unstamped xo (built by hand, without internal/xotool)
   keeps self_id: no stamp, no sharing.
3. The binary cache keeps self_id. It stores linked binaries, and the
   link reads the driver (compiler/xo ld.xo, the cc driver), which the
   codegen key leaves out.
4. What stays per program is unchanged: each entry's own key still holds
   its function's or module's contents, the flags, the target, and the
   std it was built against (the program's std is an input of every
   program, not of xo).

## Safety net: what can change an entry's bytes, and how a miss is caught

An entry is wrong only when something that decides its bytes is in
neither the entry's key nor XO_CODEGEN_KEY. The inputs:

| Input | Where it is keyed |
|---|---|
| The code generation sources (the backends and their compiler/ import closure) | XO_CODEGEN_KEY |
| The program's own sources and the std it compiles | each entry's key (function bodies, module hashes, std_sum) |
| Flags (release, debug info, heap guard, poison frames, nolibc, regs, inline settings, XO_INLINE_DENY) | each entry's key header (fn_key, part_keys, rc and inline keys) |
| Target (goos, goarch, nolibc) | each entry's key header |
| The runtime ABI the generated code calls (rt intrinsics, object layouts) | the compiler's knowledge of it is in compiler/ssa and the backends (XO_CODEGEN_KEY); the runtime's code is in the program's std |
| Cache formats | *_CACHE_VERSION in each key, and their sources in XO_CODEGEN_KEY |
| The driver (compiler/xo): what it passes the compiler (module order, part bounds' inputs, env settings) | NOT keyed: the risk this decision takes |
| The std compiled into xo itself (its maps, strings, sorting) | NOT keyed: the spec fixes their semantics (Map and Set insertion ordered, decision 0091), so a change alters output only through a bug |
| The compiler that built xo | NOT keyed: the fixed point (s2 and s3 identical) shows the same sources give the same compiler |

The three unkeyed rows are the cost. How a mistake there is caught:

- **A test per input.** A new test (internal/xotool or cmd/xo) changes
  each keyed input in turn, a file of each code generation module, each
  flag, the target, and requires that the key changes; and changes a
  file outside the set (compiler/xo, docs, a std file) and requires
  that it does not. A module added to the backends' import closure and
  missing from the list fails the first half.
- **Driver inputs into the keys.** Anything the driver hands the
  compiler that decides output (env settings like XO_INLINE_DENY,
  passes_off, part constants) must be in the entry's key header; the
  rule is to put it there, as fn_key does. A review item for every
  driver change that adds a setting.
- **Gates stay cold, plus a verify job.** gate.sh runs with
  XO_BACKEND_CACHE=off, so a gate's verdict never depends on a cache;
  this decision keeps that. A gate adds one warm run: the goldens with
  XO_BACKEND_CACHE=verify against the user cache warmed by earlier xos,
  which regenerates every entry and fails on any difference, so a stale
  entry carried across xo builds shows in the next gate.
- **The fixed point and the goldens** build the compiler and the
  goldens through warm caches carried over from earlier xo builds, so a
  wrong entry reaches a test that compares output.
- **TestFrontCacheAcrossPrograms** (fc1ff4df) builds every program
  through one shared cache against cache-off builds; under this
  decision it also runs with a cache warmed by the parent commit's xo.
- **An escape hatch.** XO_BACKEND_CACHE=off, and bumping a
  *_CACHE_VERSION, still clear any entry.

### Audit of the inputs outside the closure (2026-10-20, main 2c4b0ff3)

The tables the code generation reads are compiler sources inside the
closure: the static word table (compiler/arm64 state.xo, gen.xo;
TestStaticWords), the intrinsic lists (compiler/ssa call.xo, the
backends' calls.xo and call.xo), the TSan tables (compiler/arm64
race.xo, compiler/llvm), the Windows import table (compiler/macho
pelink.xo; compiler/xo link.xo also names imports, but only for the
link, whose binary cache keeps self_id), the part cut constants
(compiler/arm64 parallel.xo). What the runtime defines (std/native/llrt
rt.xo) is the program's std, an input of every entry's own key.

What the driver (compiler/xo, outside the closure) hands the backends,
by the environment and the module fields it sets
(gm/lm/mi: comp_dir, std_root, debug_info, dw_data_addrs, nolibc,
nolibc_start, poison_frames, race, inline_deny, passes_off):

| Setting | Status |
|---|---|
| XO_ARM64_REGS, XO_POISON_FRAMES, XO_HEAP_GUARD, XO_NOLIBC, debug info, comp_dir, std_root | in the fn and part key headers |
| XO_INLINE_DENY, XO_PASSES | in the inline key header (deny, passes) |
| XO_PASSES, --race (XO_NATIVE_RACE) | the fn cache is bypassed (compiler/xo main.xo:840): race is NOT in fn_key though isel instruments for it (compiler/arm64 isel.xo:62), so the bypass is the only guard; add race to fn_key's header as part of this decision |
| XO_DCE, XO_LLRT_C, XO_CHECK_EAGER | change which functions exist or how they are checked; every entry is keyed by its function's content |
| XO_LLVM_OPT, XO_LLVM_LTO, XO_LLVM_SPLIT, XO_SYSROOT, XO_CC, XO_CLANG, XO_WIN_NOCRT, XO_TSAN_RUNTIME | the cc and link steps: the object and binary keys hold the flags and the IR or assembly they read |
| `--stamp` (decision 0264): rewrites root modules' constants after the check | was in no key (gotchas/fe-cache-stamped-constants.md, 2026-10-20): the front end cache keys program bodies by source text; now keyed by the stamps' hash (bcache.xo stamp_root) |
| XO_EMIT_JOBS, XO_LLVM_JOBS, XO_TEST_JOBS | no effect on output by design (the parts never depend on the task count, L2 of fast-builds) |
| cache, path, reporting and test settings (XO_BACKEND_CACHE*, XO_BIN_CACHE, XO_BUILD_DIR, XO_FE_STATS, XO_INLINE_REPORT, XO_INLINE_STRICT, XO_INLINE_BREAK, XO_MODCACHE, XO_WORK, XO_OFFLINE, XO_TEST_*, XO_TRACEBACK, XO_WARNINGS) | no effect on an entry's bytes (XO_INLINE_BREAK breaks the module on purpose and is a test switch; it joins the bypass list) |

### The guard test

A Go test (internal/xotool, beside the stamp's computation) that fails
on anything unclassified:

- it collects every `env.get("...")` name and every module field
  assignment in compiler/xo and in the closure, and every file read
  (os.fs) in the closure, and requires each in a checked-in table with
  one of the classes above ("key header", "bypass", "content keyed",
  "cc/link keyed", "no output effect"); a new one fails until someone
  classifies it;
- for "key header" it requires the setting's name to appear in the key
  header strings (fcache.xo fn_key, partcache.xo, inlcache.xo,
  rccache.xo, fecache.xo), and for "bypass" in the bypass condition;
- it changes one file of each closure module in a copy of the tree and
  requires a new XO_CODEGEN_KEY, and changes compiler/xo, a std file,
  and a doc and requires the same one.

### Fail closed

The closure is computed from the `use` lines. If any `use` of a closure
module cannot be resolved to a directory, or a listed file cannot be
read, the stamp is not set (empty), and xo falls back to self_id: no
partial hash. The guard test checks this with a tree whose closure has
an unresolvable `use`.

When the key must change (the rule): any change to a file in the code
generation set; any change to a cache format; a new input that decides
an entry's bytes either goes into the entry's key or into the set. When
in doubt, add it to the set: a key that changes too often costs speed,
one that changes too rarely costs correctness.

## Consequences

Measured after the change (2026-10-20, CI, an r8g with 32 cores, load
6 to 10; xo B is xo A plus a driver-only comment, so the same code
generation key):

| | xo A cold | xo B on A's cache | xo B cold |
|---|---|---|---|
| self-build of xo (`xo build ./compiler/xo`, 3 rounds; run 39480350461) | 29.0, 30.7, 30.3 s | 11.8, 12.6, 12.4 s | 30.2, 30.5, 29.7 s |
| internal/arm64 suite (2 rounds; run 39465426358) | 889, 892 s | 926, 923 s | 933 s |

- Per build the gain is real: after a rebuild that leaves code
  generation alone, the next self-build reuses every rc, inline and fn
  entry and 8532 of 8786 front end bodies, and takes 41% of the cold
  time (insert_rc 5.9 to 1.2 s, inline 7.3 to 1.7 s, emit 6.1 to 0.7 s,
  ssa 4.0 to 1.9 s; check, assemble and link unchanged). That is the
  edit loop after a pull, and every gate's xobuild.
- A test suite's wall does not change: its many small programs spend
  their time elsewhere (building, linking, and running test binaries),
  and the binary and test-run caches still key on xo's binary. The
  767 s to 349 s "ceiling" measured on the Mac earlier the same day
  came from running a suite twice with the same xo, which those caches
  serve; it was wrongly attributed to this decision.
- Every stamped xo of the same code generation sources shares entries,
  including xos built by different seeds; the fixed point is the
  evidence that makes that sound.
- A bug in the driver or in xo's own std that changes output while the
  code generation sources don't change would serve stale entries until
  verify mode or a golden catches it. One such hole was found and fixed
  the same day: `--stamp` values (gotchas/fe-cache-stamped-constants.md).

## Alternatives

- **Keep self_id** (today): always sound, cold after every commit.
- **Per-cache keys** (the front end on lexer/parser/check/ssa only, the
  arm64 caches adding arm64): more reuse when one backend changes, more
  sets to keep right. Possible later on top of this one.
- **Strip the stamps from self_id**: the stamps differ per commit, but
  the binary also changes with every std/native or driver edit, so it
  would recover only docs-only rebuilds.
- **Hash xo's code generation functions in the binary** (the text of
  the functions reachable from the backends): exact, but needs the
  linker's call graph at runtime; the source set is simpler to check.
