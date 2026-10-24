# 0360: A whole-build key: reuse the linked binary when nothing that feeds the build changed

Status: Accepted (owner, 2026-10-23): whole-build reuse is on by
default (stage 3); `XO_WHOLE_BUILD=off` turns it off. 0351's stage 3,
by Xo Lang 2. The lead approved stages 1 and 2 (opt-in, `XO_WHOLE_BUILD=on`) on 2026-10-23,
with content hashing by default and the stat path opt-in (below), then
reversed their order once 0236's run cache turned up (see "Stage 1, as
built"); stage 3, on by default, goes to the owner with stage 2's
numbers. For `xo build` this takes up what 0316 left as "not pursued"
(no warm case had measured as a problem; the language twins measured
one).

## Stage 1, as built (2026-10-23)

Decision 0236's run cache (compiler/xo testcache.xo) already did this for
`xo test`: a key known before the check (xo's self_id, the command line
less the run-only flags, every XO_ variable, PATH, HOME, CC, the SDK and
the C tools' identities) and an entry listing every std file, each module
directory's files and listing, the embedded files and the module files up
from the working directory, each with its SHA-256, compared on every
lookup (so the content-hash default holds, and a same-size edit with its
time put back misses). Stage 1 reuses it for `xo build` and `xo run`
under `XO_WHOLE_BUILD=on`:

- the key is run_key's, the command line naming the subcommand (and, for
  `xo run`, without the program's arguments after `--`);
- the build's standard output and error go through a tee (`Os{stdio:
  tee, ...}`, decision 0307) and are kept in the entry, without xo's own
  reports (--phase-times and XO_FE_STATS lines); a hit prints them again,
  so its warnings and `os/arch: path` line are the build's;
- the entry keeps the outputs a build wrote, and a hit copies the binary
  to each by a temporary file and a rename; a run's hit runs it;
- a build's own output is left out of its inputs (`-o prog` inside the
  module directory);
- `xo test` keeps its build's output the same way, so a program with a
  warning is now stored (before, a build that printed a diagnostic was
  never stored, and such a program never hit: the CRUD server's warm `xo
  test` was 0.77 s every time, now 0.17 s).

Results (bench/results/whole-build-2026-10-23.md, `xo bench --ab`, warm):
the CRUD server's build 560 to 56 ms wall (0.099), 1,740 to 40 ms CPU,
546 to 17 MB peak memory; hello's 301 to 44 ms (0.144). The lookup is 28
to 35 ms of a hit: it hashes the whole std tree. Not as designed above:
the key holds the working directory and the full command line, so the
same program built from another directory or into another output misses
(safe, and the common edit loop stays in one directory).

## Stage 2, as built (2026-10-23)

compiler/xo recorder.xo: the Os a stored build is given (build, run or
test) has a recording Fs and Env (RecFs, RecEnv), and the entry lists
what they saw instead of run_inputs' hand list:

- each file read (hashed at the store), each directory listed or walked,
  each probe with its answer then (stat, exists, mode, read_link, glob:
  the xo.work search up from the module, vendor/modules.txt), and each
  environment variable read, unset ones too;
- left out: paths under xo's caches and build directories, every path
  the build wrote, and its outputs (a stat of `prog` before it existed
  would otherwise miss the next build). The temporary directory is not
  left out: a program may live there (the first try did, and an edit
  under $TMPDIR hit: the tests caught it).

Proc is not wrapped: run_key already holds the C tools' identities, as
it did. The manifest is not as small as estimated: a hello build reads
445 of std's files (the checker loads most of std), so the lookup still
hashes about that many; the warm CRUD build's hit went from 56 to 39 ms
(bench/results/whole-build-2026-10-23.md).

Verify: `XO_BACKEND_CACHE=verify` keeps a hit aside, builds, and
compares the binary and what the build printed with the entry's;
a difference is error XO1512 (TestWholeBuildCache plants a wrong binary).

## Context

An unchanged warm build is Xo's largest build gap. The language twins
(bench/results/twins-build-2026-10-23.md, lines 20 to 25) measured the
CRUD server's warm build at 0.57 s for xo 0.27.0, against 0.08 s for Go
and 0.03 s for Rust; hello at 0.28 s against 0.04 and 0.01.

Every cache already hits on such a build. A warm CRUD build with xo at
cbfa9b100 (`--phase-times`, XO_FE_STATS=1, 2026-10-23) shows it:

- parse 194/194 reused, fe 2628/2642 bodies, rc, sum, inline and
  borrows 2995/2995, functions 1364/1364, parts 9/9, objects 10/10.

The time is the walk itself, phase by phase:

- check 62 ms;
- fe keys, read and build 87 ms;
- prune_unwind 25, add_landings 26, cleanup 56;
- insert_rc 76, of which plan 48;
- assume 9;
- inline 61, of which apply 59;
- inline.verify 55;
- fn_keys 7, emit 12, assemble 32, link 54.

These sum to the 0.57 s. Each pass caches per function but still runs
over the whole program, about 3,000 functions with std and the runtime.
The fixed cost grows with the runtime, not with what changed. Making
each pass cheaper attacks 0.3 s spread over ten phases; skipping them
all when nothing changed removes it.

What a hit can cost, on the same machine (Apple M4 Pro, load 4 to 5,
gate class, 2026-10-23):

| step | measured |
|---|---|
| start xo and exit (`xo version`, median of 20) | 5.5 ms (`go version`: 12.6 ms) |
| stat every .xo, xo.mod and xo.lock of std and the CRUD program (273 files, 12.3 MB) | 0.5 ms |
| read and sha256 all of them (the path when stats differ) | 22.8 ms |
| copy the 6.4 MB binary | under 10 ms (`time -p` shows 0.00) |

A hit with unchanged stats would therefore take about 10 ms, under
Go's 80 ms. A checkout that touched every file without changing it
would take about 35 ms.

## Decision

### What the key covers

A build is stored under a **prefix** and a **manifest**.

The prefix is the hash of what is known before the build reads
anything:

- `"xo whole-build v1"`;
- xo's identity, `self_id` (bcache.xo): the hash of xo's own binary,
  memoized by its path, size and modification time. The driver, the
  linker and the passes are all in it. The codegen key (`cache_id`) is
  not enough, since it leaves out the driver and the linker, as the
  binary cache already notes.
- the subcommand and the parsed options: every flag, `--release` and
  each of its settings, the stamps, `--target`, the backend, `--race`
  and `--debug-info`;
- the output's base name (macOS's ad hoc signature names the file;
  fetch_bin keys on it already);
- the absolute program directories and the working directory, since
  build info embeds the absolute directory.

The manifest is the list of everything the build read, recorded while
it ran:

- **Files**: each file read through `os.fs`, by path, with its sha256,
  size and modification time.
- **Probes**: each stat or exists check, by path, with its result
  (missing, a file of this size, a directory). The search for xo.work in
  parent directories, vendor/ and a module looked up but not found are
  probes, so creating such a file later changes the result.
- **Listings**: each `read_dir`, by path, with the hash of its sorted
  names. A new .xo file in a package directory changes it.
- **Environment**: each `os.env.get`, by name, with its value or unset.
  Every XO_* variable the build reads is in the key by construction.
  TestCodegenInputsClassified stays, but the whole-build key does not
  depend on it being complete.
- **Diagnostics**: the build's warnings and notes. The CRUD server's
  build prints an XO0540 warning, and a hit prints them again, as the
  module check cache replays its diagnostics.

The record comes from wrappers: `Fs`, `Env` and `Proc` are predeclared
interfaces (core 6.3), so `RecordingFs`, `RecordingEnv` and
`RecordingProc` wrap the `Os` the build is given. This covers every
read, including ones a hand kept list would miss. These paths are left
out:

- the backend cache, the binary cache and the build directory: they are
  derived from the same inputs, and verify mode checks that;
- the output path;
- the build's own temporary files.

A build that started a process (cc, clang, go, ld, `use c` sources,
`--ld=cc`, the LLVM backend, `--backend=go`) is not stored in stage 2:
the process's own inputs are not recorded. A note under
`XO_FE_STATS=1` names the process. A failed build is never stored.

### The fast path

1. Parse the command and compute the prefix. This reads xo's selfid
   memo and nothing else of the program.
2. Read the newest manifests under `<backend cache>/whole/PP/PREFIX/`
   (up to 4 kept, newest first).
3. For each manifest:
   - compare the recorded environment;
   - redo each probe and listing;
   - read each recorded file and compare its sha256 (the default; lead,
     2026-10-23: correctness first). A same-size edit whose
     modification time was put back (`touch -r`) changes the hash and
     misses. Estimated cost: about 30 ms a hit for the CRUD server (22.8
     ms to read and hash all 273 files, of which the build reads about
     194).
   - `XO_WHOLE_BUILD=stat`, the opt-in fast path: an equal size and
     modification time trusts the recorded hash, and only a file that
     differs is read. A file whose modification time was within 2 s of
     the manifest's write time is always hashed, Go's and git's "racy"
     rule. Estimated cost: about 10 ms a hit. Its known gap, which Go's
     file hash cache shares: a same-size edit with the modification time
     put back is not seen (FileInfo has no inode or change time).
4. On the first manifest that matches, clone the stored binary to the
   output (`clonefile` on APFS, else a copy), replay the diagnostics,
   and exit. On the stat path, a match that needed hashes rewrites the
   manifest's stats, so the next lookup is a stat hit.
5. Otherwise, run the full build with the recorders on, then store the
   binary and the manifest by temporary file and rename (two builds
   racing store two equal entries).

Entries live in the backend cache and are pruned by its size cap with
the other kinds. `XO_WHOLE_BUILD=off`, and `XO_BACKEND_CACHE=off`, turn
the path off.

### Verify mode

`XO_BACKEND_CACHE=verify` treats a hit as a miss:

- it runs the full build with the recorders on, to a temporary output;
- it compares the bytes with the stored binary;
- it compares the new manifest's key with the hit's.

A difference is an error with a new diagnostic code (claimed in stage
2; no existing code changes meaning). The error names both files and
the first differing manifest entry. That case is a missed input or a
nondeterministic build, both bugs. Verify relies on builds being byte
reproducible on Xo's own linkers, which scripts/identity already
checks, and which ZERO_AR_DATE gave darwin cc links (2026-10-23).
Determinism also requires that the clock never reaches the output.
`Clock` is not wrapped, because the build reads it only for phase
times and pruning, and the verify run catches a change to that.

### Stages

1. **Recorders, no reuse.** Wrap the build's `Os`, write the manifest
   after a successful build, and report its size under `XO_FE_STATS=1`.
   Measure a miss's extra cost with `xo bench --ab` on a warm CRUD and a
   cold hello build. The recorder hashes bytes the checker has already
   read, about 12 MB for the CRUD program. The budget is under 3% of a
   cold build; if it is over, hash in a background task after the link.
2. **Hits behind `XO_WHOLE_BUILD=on`**, verify mode, and the new code.
   Tests:
   - an edited file misses;
   - a touched but unchanged file hits, and the stat is rewritten;
   - a new .xo file in the package directory misses;
   - an xo.work created in a parent directory misses;
   - a changed XO_* value the build reads misses;
   - a changed flag or target misses;
   - the same program built into another directory with the same
     output name hits; with another name it misses;
   - a std file edited in place misses;
   - a std file edited at the same size with its modification time put
     back by `touch -r` misses on the default, and hits on
     `XO_WHOLE_BUILD=stat`, where that is the documented gap (lead);
   - two builds of the same program into the same output at once leave
     a whole binary that runs, never a torn one (temporary file and
     rename; lead);
   - a build that ran cc is not stored;
   - verify passes on all of these and fails on a planted wrong entry;
   - the replayed warning matches.
   Then `--ab` on the warm CRUD and hello builds.
3. **Default on** (the owner's decision), then `xo run` (a hit runs the
   stored binary) and `xo test`, whose test binary cache (decision 0236)
   keys only the link today.

## Consequences

- An unchanged warm build is expected to fall from 0.57 s to about
  30 ms for the CRUD server (about 10 ms on the stat path), and from
  0.28 s to less for hello:
  under Go's 0.08 s and level with Rust's 0.03 s (the stat path under
  both), if the estimates hold. The gap the
  comparison pages state closes for the no-change case only.
- Builds after an edit are unchanged. The edit row (0.57 s against Go's
  0.24 s) is the next target, the passes' per-function walk.
- What the default costs a build after an edit (stage 3, measured
  2026-10-24, `XO_WHOLE_BUILD=off` against the default, 14 alternating
  first builds of a program with one new function, clean xo builds,
  load 1 to 3, the real cache): strings 0.382 s off, 0.407 s on (+25 ms,
  6%); http_server 1.385 s off, 1.439 s on (+54 ms, 4%). A warm
  unchanged build pays nothing for it (it is the hit). A phase split of a
  first build showed where the work is: 15 to 20 ms for the key (two
  process spawns for the C tool's id, asked again by the backend's keys),
  13 to 40 ms hashing the 447 recorded input files again, 1 to 9 ms
  copying the binary (a clone would gain little), 0.1 ms for the manifest,
  and a prune of the run cache every two minutes (1 to 146 ms). Hashing a
  file where it is read, one C tool id per process (keyed on the tool's
  path and arguments) and a detached prune of the run cache
  (`xo dev prune-bins`) recovered 3 ms (strings) and 74 ms (http_server,
  inside its 1.33 to 1.54 s range) in the same measurement: no speedup is
  claimed for them. (An xo built from a dirty tree runs with cold
  back-end caches: a first measurement of the fixes, 5 times slower, was
  that, not the change.)
- The design depends on the recorders seeing every input. The wrappers
  make this structural, but a read through a path that bypasses `os`
  (a constant table compiled into xo is part of `self_id`, so it is
  covered) would be a hole. Verify mode exists to find one.
- One more cache kind to prune, of about the binary's size per entry
  (6 MB for the CRUD server), under the existing cap.
- The process exclusion leaves cc, LLVM and Go backend builds on today's
  path until their tools' identities are recorded (tool_id already
  hashes a tool's `--version`; the native binary cache keys the SDK).

## Alternatives

- **A hand built key** from build info's sources, the
  TestCodegenInputsClassified list and the flags. It is cheaper to
  write, but it misses probes (xo.work, vendor, a module not found),
  embedded files (decision 0304) and every input added later. Rejected:
  the recorder is complete by construction.
- **The link-level binary cache** (cache.xo, decision 0178) keys the link
  after code generation, so it saves only the 54 ms link.
- **Cheaper passes**: worthwhile for the edit case, but the no-change
  case would still pay a walk over about 3,000 functions.
- **A resident daemon** (`xo serve` keeping the checked program in
  memory): it would also speed up edits. It is a larger change with its
  own lifetime and invalidation questions, and is a separate decision
  if wanted.

## Method

xo built from cbfa9b100 (`go run ./scripts/xobuild`), the CRUD program
copied from bench/perf/http/xo, a private XO_BACKEND_CACHE_DIR warmed by
four builds; `--phase-times` and XO_FE_STATS=1 on the fifth. The start,
stat and hash figures are medians from a Python timing harness (20 runs
of `xo version`, one pass over the 273 files). Everything ran through
heavy.sh's gate class on the development Mac with other agents running.
