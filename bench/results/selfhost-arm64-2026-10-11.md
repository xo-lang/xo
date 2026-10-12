# Self hosted arm64 emitter, 2026-10-11

Decision 0158. The arm64 emitter in Xo (`selfhost/arm64`, about 9,400
lines of Xo) against internal/arm64 (Go), compared through the new
hidden `xo asmdump` piece by piece: every function with its call site
table, every helper, every literal, and xo_entry, with the labels made
from program wide counters renamed (TestSelfhostArm64). The input of
both is the module after the SSA passes and reference counting:
internal/ssa's for the Go emitter, selfhost/ssa's for the Xo emitter,
on the runs both SSA builders build completely (decision 0150 stage 8:
identical after every pass on those runs).

Go mode (goasm.go, gosyntax.go: Go assembler for the Go backend's
hybrid builds) and the layout instrument (align.go, `XO_ARM64_LAYOUT`)
are not ported: the self hosted compiler emits native programs with the
default layout.

# Stage 1: the core, 2026-10-11

Everything but closures, boxes, aggregate equality, `share`, `reuse`
(stage 2), parameters by reference (stage 3), and inlining (stage 4):
value layout, instruction selection of arithmetic, compares,
conversions, aggregates, Option, lists, strings, string equality,
direct and runtime calls, inline counting and the counting helpers of
every type, cancelled retain and release pairs, the memos and value
ranges, strength reduction and rotation, every MIR pass, register
allocation with spills and far frames, frames, fault stubs, landing pads
and their call site tables, the text peepholes, and the literals. Both
emitters run with `--no-inline`.

TestSelfhostArm64, go build of the Xo emitter, every repository run but
the five heaviest (arm64Heavy: selfhost/check, parser, ssa, arm64, and
huge_frame):

| runs (642) | count |
|---|---|
| compared | 302 |
| rejected by both checkers | 278 |
| not built completely by both SSA builders | 62 |
| byte identical as a whole file | 76 |

| functions (of the Go emitter's 7,910 in the runs compared) | count |
|---|---|
| identical, with their call site tables | 5,237 |
| skipped by the Xo emitter (a later stage) | 2,673 |
| different | 0 |

All 2,902 helpers the Xo emitter emitted are identical to the Go
emitter's; every literal it emitted is one of the Go emitter's.
TestSelfhostArm64Mutations, 100 code mutations (seed 20261011): 77
compared, 23 rejected by both, 1,164 of 1,866 functions identical, 702
skipped, 0 differences.

A development comparison over every run with the heaviest included
(.tools/acmp.py, the prototype of the test): 10,974 functions, 6,139
identical, 4,835 skipped, 0 different. Why functions were skipped:

| reason | functions |
|---|---|
| stage 3, parameters by reference | 2,121 |
| stage 2, function values | 1,704 |
| stage 2, share and reuse | 514 |
| stage 2, function values and boxes counted inline | 216 |
| stage 2, equality helpers | 101 |
| stage 2, boxes | 95 |
| stage 3, calls by reference | 84 |

Speed, the emitter alone (`--bench=5`, ms for 5 generations, Apple M4
Pro, another agent's test suite running), go build of the Xo emitter;
the Xo emitter skips the functions of later stages, so lines emitted are
shown:

| input | Go emitter | Xo emitter, go backend |
|---|---|---|
| selfhost/lexer | 141 ms, 140,345 lines | 183 ms, 89,125 lines |
| selfhost/parser | 2,322 ms, 2,415,560 lines | 284 ms, 229,835 lines |
| bench/perf/xo | 39 ms, 39,245 lines | 47 ms, 23,935 lines |

Per line emitted, the Xo emitter takes about twice the Go emitter's time
on selfhost/lexer (2.04 against 1.00 microseconds per line).

Found in stage 1: a local `var` may shadow a parameter of the same name
without a diagnostic (gotchas/xo-local-shadows-parameter.md); the first
run faulted on it. No difference in the emitted code needed a second
attempt: the first full comparison found only differences of the
comparison itself (call site numbers, function names holding `: `,
the alignment line after a literal).

# Stage 2: closures, boxes, equality, sharing, 2026-10-11

Closures and their environments (with their drop and share functions),
captures, calls through function values, function addresses, rebinds,
boxes, function values and boxes retained and released inline, the
equality helpers of structs, tuples, enums, lists, and boxes, `share`,
and `reuse`. Both emitters still run with `--no-inline`.

Development comparison over every repository run (.tools/acmp.py; 306
runs compared, 278 rejected by both, 62 not built completely by both SSA
builders):

| functions (of the Go emitter's 10,974) | stage 1 | stage 2 |
|---|---|---|
| identical | 6,139 | 8,699 |
| skipped (stage 3: parameters by reference, calls by reference) | 4,835 | 2,275 (2,121 and 154) |
| different | 0 | 0 |
| runs byte identical as a whole file | 76 | 219 |
| helpers emitted by the Xo emitter, all identical | 3,305 | 9,950 |

TestSelfhostArm64 in the merge tier (every second run, the five heaviest
left out): 321 runs, 149 compared, 111 byte identical; 3,836 of 4,013
functions identical, 177 skipped, 0 differences; 4,340 helpers
identical. TestSelfhostArm64Mutations (30 mutations): 481 of 492
functions identical, 11 skipped, 0 differences.

Every stage 2 construct was identical on its first full comparison.

# Stage 3: parameters by reference, 2026-10-11

Decisions 0151 and 0153 (byref.xo): parameters of 256 bytes or more of
direct calls passed by address, the homes of `var` ones and the forward
dataflow of what they hold (with the landing pads' states), fields read
and written in place, read only parameters read in place and passed on,
results left in memory, frame memory for other arguments, and returns
that store back only what the home does not hold. Every construct the
Go emitter lowers is now ported (Go mode and the layout instrument
aside); both emitters still run with `--no-inline`.

Development comparison over every repository run (.tools/acmp.py; 306
runs compared): all 10,974 of the Go emitter's functions identical,
none skipped, 0 differences; 11,436 helpers identical; 234 of the 306
runs byte identical as a whole file (in the others the two SSA builders
make a few generated functions, such as `xo.debug[Str]`, in another
order, which moves the functions, the helper numbers, and the literal
numbers; every piece is identical). TestSelfhostArm64 in the merge tier:
4,013 of 4,013 functions identical, 118 of 149 runs byte identical;
TestSelfhostArm64Mutations: 492 of 492 functions identical.

The dataflow of decision 0148 was identical on its first comparison,
selfhost/check and selfhost/ssa (the Builder and Checker homes)
included.

# Stage 4: inlining, and the whole file, 2026-10-11

inline.xo: small leaf functions (at most 40 instructions and 12 blocks,
no calls of Xo functions, no landing pads of their own) inlined on the
Xo SSA after reference counting, the module verified again with
selfhost/ssa's verifier (`Module.verify_all`, new), and generated
without inlining when it does not verify (the Go emitter's
InlineError). Both emitters now run with their defaults.

The two SSA builders made a few generated functions in different
orders (72 of the 306 runs differed only in that order at stage 3):
selfhost/ssa built the bodies of Debug text, slice, concatenation,
frame argument, and zero functions after the body that asked for them,
on the assumption that they declare no functions, but the Debug text
of a struct declares the Debug text of its fields. selfhost/ssa now
builds every generated function when asked for, as the Go builder's
genFunc does (build.xo build_gen; decision 0150 amended), and the
function order is the Go builder's on every run. TestSelfhostSSA and
TestSelfhostSSAMutations (merge tier) still pass: 15,906 of 15,935
functions identical in the build stage, 0 differences.

Development comparison over every repository run (.tools/acmp.py):

| | count |
|---|---|
| runs compared | 306 |
| functions identical (of the Go emitter's 10,975) | 10,975 |
| functions skipped or different | 0 |
| helpers emitted, all identical | 11,434 |
| runs byte identical as a whole file | 306 of 306 |

TestSelfhostArm64 now also requires a run with no function skipped to
be byte identical; in the merge tier 149 of 149 runs compared are, with
4,013 of 4,013 functions; TestSelfhostArm64Mutations: 24 of 24 mutated
programs compared byte identical (492 functions).

Speed, the emitter alone with inlining and the verifier (`--bench=5`,
ms for 5 generations; Apple M4 Pro, another agent's tests running),
the same output:

| input | Go emitter | Xo emitter, go backend |
|---|---|---|
| selfhost/lexer (150,315 lines) | 196 | 361 (1.84x) |
| selfhost/parser (2,467,305 lines) | 2,974 | 5,351 (1.80x) |
| bench/perf/xo (40,540 lines) | 46 | 90 (1.96x) |

# Performance fix: updates inside list elements, 2026-10-11

The first arm64 build of the switch-over driver (selfhost/xo) ran over
20 minutes on its own source before it was stopped: on the native
backends `g.blocks[g.cur].insts.push(x)` copies the whole instruction
list at every push (gotchas/xo-native-nested-element-update.md; micro
benchmark, 20,000 pushes onto a list inside a list element: 1,018 ms on
arm64, 757 ms on LLVM, 0 ms moved out and back). selfhost/arm64 now
moves instruction lists out of their block for an update (take_insts,
put_insts), keeps the current block's memo out of the memo list, and
pushes onto lists of lists in place (regalloc.xo push_rng, push_int).
The output is unchanged (TestSelfhostArm64 merge tier, the switch-over
below). With it, the arm64 build of the Xo emitter emits selfhost/lexer
in 195 ms for 5 generations, the Go emitter in 196 ms (`--bench=5`).

# The switch-over experiment, 2026-10-11

`scripts/selfhost-switchover.sh` (decision 0158): `selfhost/xo` (xo1,
the lexer, parser, checker, SSA builder with its passes, and arm64
emitter, all in Xo, then `cc` to assemble and link with std/llrt) built
by the Go compiler with `--backend=arm64` (A); A compiles selfhost/xo
(B); B compiles it again (C). Apple M4 Pro, macOS, another agent's work
on the machine:

| step | wall clock |
|---|---|
| A: the Go compiler, `xo build --backend=arm64 selfhost/xo` (with the link) | 161.6 s |
| the Go emitter's assembly of selfhost/xo (`xo asmdump`, no link) | 114.1 s |
| B: A compiles selfhost/xo (with the link) | 326.1 s |
| C: B compiles selfhost/xo (with the link) | 324.1 s |

Result: **the fixed point is reached on the assembly**. The assembly B
wrote (8,938,072 lines), the assembly C wrote, and the Go emitter's
assembly of the same program are byte identical (sha256
b64ee66c...203e for all three). The binaries B and C differ in 49
bytes: the 16 bytes of the Mach-O LC_UUID and the ad hoc code signature,
whose identifier is the output file's name (`B`, `C`); their code and
data come from identical assembly and the same runtime sources.

B (the self compiled compiler) on the arm64 goldens
(`scripts/selfhost-goldens.py`: the 114 programs of
internal/llvm/testdata/golden against want.txt with no allocation live
at exit, and the 24 of internal/arm64/testdata/run against the Go
backend's output): **132 of 138 pass**; the 6 others are the std/http
programs (http_client, http_fault, http_server, http_stream, http_tls,
http_wire), which xo1 refuses to build because it does not build and
link the TLS library yet.

The released `xo` and what it runs are unchanged (decision 0150: the
owner's decision after the fixed point).

Not done in xo1 (what internal/build adds beyond the runtime): the TLS
library of std/http, time zone data for std/time (a program loading
native/time is refused), `use c` sources, the runtime object cache and
prebuilt runtime packs, cross targets (darwin only), test binaries.

# Known failure at merge, 2026-10-11

The full tier of internal/selfhost (XO_SELFHOST_TIER=full) found one
failure: TestSelfhostArm64/llvm. In 33 runs the Xo emitter built with the
LLVM backend writes frames 256 bytes smaller than the Go emitter (`sub sp,
sp, #256` against `#512`); the go and arm64 builds of the same Xo code
agree with the Go emitter (308 of 308 runs byte identical, 15,043
functions), so it points at an LLVM backend miscompile of the Xo
emitter, not at the port. TestSelfhostArm64Mutations (300) and the
checker mutation tests passed in the same run. Root cause under
investigation (the frame area computation, selfhost/arm64/byref.xo).

# xo1 takes over internal/build's native steps, 2026-10-11

Decision 0174: the steps internal/build adds beyond the C runtime,
ported to xo1 (`selfhost/xo`) in order, each measured with
`scripts/selfhost-goldens.py` on A (selfhost/xo built by the Go compiler
with `--backend=arm64`; its assembly is B's and C's, decision 0158). The
golden set has grown since the switch-over above: 123 programs of
internal/llvm/testdata/golden and 24 of internal/arm64/testdata/run, 147
in all. `selfhost-goldens.py` now passes each golden's args.txt, env.txt,
and stdin.txt as TestGoldenRun does (before the fix the 4 programs that
have them, cli_parse, cli_usage_exit, env_dirs, term_stdio, failed with
both drivers' binaries for want of their inputs).

## Step 1: the TLS library of std/http

xo1 computes internal/build's key of the TLS library (tlsSourceHash over
mbedtls.tgz, xo/, and fiat-crypto's sources read from the repository;
the C compiler's path and `--version`; the SDK; the target; the
sanitizer flavor; mbedtls.FileDefines) and links
`<user cache>/xo/mbedtls/4.1.1-<key>/libxotls.a`, building it when it is
missing (the tree extracted with tar, the compiles run by `xargs -P`,
`ar rcs` with ZERO_AR_DATE=1). Checked: with an empty XO_TLS_CACHE, xo1
and the Go driver choose the same directory (4.1.1-05cf46eeba0fd423aba7
with /usr/bin/cc and the Xcode SDK) and the archives they build are byte
identical.

| step | goldens with A |
|---|---|
| before (the switch-over, 138 programs) | 132 of 138 |
| 1: TLS | **147 of 147** |

## Step 2: zone data of std/time

xo1 writes xo_tzdata.c beside the runtime and compiles with -DXO_TIME,
as timeFlags does (decision 0157's split): the constant zone names
`time.zone` is called with (from the calls in the module xo1 built,
each confirmed as a string literal in the source, since the SSA builder
folds `let n = "X"` into a constant the Go builder counts as dynamic),
or the whole zoneinfo.zip when a name is known only at run time, or
mode 2 with --tzdata=system; the zone data comes from Go's zoneinfo.zip
($XO_ZONEINFO, else `go env GOROOT`), read as a stored zip. Checked: the
xo_tzdata.c of time_zones (two zones) and of time_dynamic_zone (the
whole database) are byte identical to the Go driver's, and the five
std/time programs of internal/gen/testdata/golden (time_zones,
time_dynamic_zone, time_civil, time_format, duration_time) built by
both drivers are byte identical binaries with the same output. The
arm64 goldens use no std/time, so their count does not move: 147 of 147
(measured on the build with steps 1 to 6).

## Step 3: prebuilt runtime packs and the runtime object cache

xo1 takes the runtime from a pack ($XO_RTPACK, else lib/xo/rtpack beside
the binary as $_ names it, since Env.program is not native yet) when its
key (RTPackKey) and checksums match and the flags are a pack variant's,
else from <user cache>/xo/rtobj (the rtobj key: toolID of cc with its
path, size, and modification time in nanoseconds from stat(1), the
flags, the SDK, the TLS flags and source hash, every .c and .h of the
runtime directory but xo_tzdata.*), compiling it on a miss, and trims
the cache as trimCacheDir does. `xo1 keys` prints its keys. Checked:
`xo1 keys` gives the same pack keys as `xo rtpack --key` for
darwin/arm64 (2885da3c...dfc6) and linux/arm64 (410e9133...086b) and the
same TLS directory; a plain program's runtime object lands under the
same name (0df88719...6a89.o) from both drivers; with a pack built by
`xo rtpack` both drivers link arith and http_tls (the TLS library from
the pack, the TLS cache untouched) into identical binaries.

Both drivers link the same binary: `scripts/selfhost-samebin.py` builds
every arm64 golden with xo1 and with `xo build --backend=arm64` under the
same file name and compares: **147 of 147 byte identical** (not just
the code: LC_UUID and the ad hoc signature match too, since the inputs
and names are the same). Goldens with A: 147 of 147.

## Step 4: `use c` sources

xo1 writes each binding's adapter (read from cbind's cache beside the
declaration file, `<prefix>.c`) into OUT.rt/cffi/xo_<prefix>.c, adds the
`link` files (read back from the binding's key) and the libraries as
sorted -l flags after the objects, as cInputs does. Checked:
internal/selfhost/testdata/ssa/stage6c and examples/realtime-control
built by both drivers are byte identical binaries (stage6c's output is
the same; realtime-control prints timings). As decision 0152 says, the
binding must be in the cache: a first build by xo1 alone reports the
program unsupported until a Go build or `xo bind --c` writes it.

## Step 5: the linux/arm64 target

internal/build links only the host target with the arm64 backend; for
linux/arm64 on a Mac it writes the assembly (prog-linux.s) and stops.
xo1 does the same with `--target=linux/arm64`. Checked: the linux
assembly of arith, closures, and http_tls is byte identical to the Go
driver's prog-linux.s. Linking for Linux from a Mac (the linux pack
with `zig cc`) is not something internal/build does yet; no zig is
installed here.

## Step 6: test binaries

`xo1 test [--run=SUB] [--seed=N] [--cases=N] [--json] [-v] <dir>` builds
the test binary (selfhost/ssa's test mode) and runs each test in its own
process (XO_TEST, XO_TEST_SEED), property tests case by case with
shrinking (XO_PROP_SEED, XO_PROP_REPLAY, XO_PROP_BEST), and prints what
`xo test --no-gc` prints. Checked on cmd/xo/testdata/nogc: same, prop,
and real print the same as `xo test --no-gc --backend=arm64 -v` with the
same exit status; native differs in one line, the contract fuzz test the
Go driver lists as skipped, which xo1 does not list (so 0 skipped
against 1). Not done: `--real` (the Xo SSA builder leaves tests with
`uses` out, so they are skipped), contract fuzz listing, and a time
limit (the native Proc has none).

## The fixed point with steps 1 to 6, 2026-10-12

`scripts/selfhost-switchover.sh` on the tree with every step (commit
58d294f), through the shared heavy lock, other agents' work on the
machine:

| step | wall clock |
|---|---|
| A: the Go compiler, `xo build --backend=arm64 selfhost/xo` | 142.4 s |
| the Go emitter's assembly of selfhost/xo (`xo asmdump`) | 119.6 s |
| B: A compiles selfhost/xo | 313.0 s |
| C: B compiles selfhost/xo | 303.7 s |

**The fixed point holds**: the assembly B wrote, the assembly C wrote,
and the Go emitter's are byte identical (9,011,385 lines, sha256
f5f8d55b...bb03). B and C differ in 49 bytes, as before (LC_UUID and the
ad hoc signature, which names the output file). B passes **147 of 147**
arm64 goldens.

| step | goldens | other checks |
|---|---|---|
| before (2026-10-11) | 132 of 138 | |
| 1: TLS | 147 of 147 (A) | TLS key and archive identical to the Go driver's |
| 2: zone data | 147 of 147 (A, all steps) | 5 std/time programs: identical binaries |
| 3: packs, object cache | 147 of 147 (A, all steps) | 147 of 147 binaries identical (selfhost-samebin.py) |
| 4: `use c` | 147 of 147 (A, all steps) | stage6c, realtime-control: identical binaries |
| 5: linux/arm64 | 147 of 147 (A, all steps) | 3 programs: identical linux assembly |
| 6: tests | 147 of 147 (A, all steps) | 4 nogc test dirs: 3 same output, 1 differs by the unlisted contract fuzz test |
| fixed point (B) | 147 of 147 | G.s = B.s = C.s |
