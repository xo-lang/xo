# 0178: Retiring the Go implementation: inventory and plan

Status: Accepted (owner, 2026-10-12): switch points A and B approved;
B takes effect when its gate passes; deleting the Go compiler packages is
delegated to the lead under the criteria in "Owner approvals". Stage 1
done (2026-10-12, runs 5 and 6 of the results file); switch point A
implemented (internal/selfdriver; section "Switch point A, as
implemented"). The inventory is from main dbb8b05 with this decision's
commits; the measurements are on the same tree (runs 1 to 4 of the
results file). Gaps closed on the way, each its own commit: `xo1
--backend=llvm`; the decision 0172 file symbols in xo1's runtime flags
(with TestXo1IOFlagsSymbols); `xo1 test --backend=go`; `use go` in the
Go backend path (selfhost/gen's `Go_<path>_` names, the binding
adapters, go.mod requirements and go.sum, the Go timers tag). Evidence:
`bench/results/retire-go-2026-10-12.md`.

## Context

Decision 0150 (amended 2026-10-11) retires the Go implementation once
the full suite passes on the self hosted compiler. Every compiler stage
now has an Xo twin (selfhost/lexer, parser, check, ssa, arm64, llvm,
gen) and a driver, xo1 (selfhost/xo: `build`, `test`, `asm`, `keys`;
decisions 0158, 0166, 0167, 0174). But `xo` is more than the compiler:
24 commands and 33 Go packages, and most of the full suite tests them
through Go APIs, not through a binary. This decision lists what exists,
measures how much of `xo build` xo1 already replaces, and proposes the
order in which the rest moves, with the points the owner approves.

## Inventory

Lines of Go excluding `_test.go` and testdata (`wc -l`, 2026-10-12).
"Xo twin" is the self hosted code that does the same job today.

### The compiler (an Xo twin exists, compared on every repository run)

| Go package | lines | what it does | Xo twin | what the twin lacks |
|---|---:|---|---|---|
| internal/token, lexer | 301 + 1,176 | tokens and positions | selfhost/lexer (1,873) | nothing known |
| internal/ast, parser | 1,164 + 6,041 | syntax tree, habits, recovery | selfhost/parser (7,946) | nothing known |
| internal/check | 14,968 | names, types, effects, diagnostics with fixes | selfhost/check (18,408) | XO0900 holes (not reported); binding `use go` and `use c` itself (reads the caches the Go binders write, decisions 0152, 0177) |
| internal/ssa | 15,149 | SSA builder, passes, verifier | selfhost/ssa (17,530) | tests with `uses` (`--real`); `ShareProbes` |
| internal/arm64 | 9,935 | arm64 assembly | selfhost/arm64 (9,512) | none (fixed point, 147 of 147 goldens) |
| internal/llvm | 3,198 | LLVM IR | selfhost/llvm (3,792) | `llvm.Split` (XO_LLVM_SPLIT, decision 0168) |
| internal/gen | 11,551 | Go source | selfhost/gen (13,878) | gofmt is run as a program; `--race`, `--holes` |
| internal/build | 4,217 | the build driver: caches, runtime, TLS, zone data, link | selfhost/xo (2,893) | see "xo1 against `xo build`" |

Sum of the compiler packages (build excluded): 63,483 lines of Go;
their twins are 75,832 lines of Xo with the driver.

### The rest of the toolchain (no Xo twin)

| Go package | lines | what it does | must move? |
|---|---:|---|---|
| cmd/xo | 4,777 | the CLI: flags, JSON output, 24 commands, `xo test`'s native runner (nativetest.go) | yes: it becomes selfhost/xo |
| internal/diag | 2,019 | diagnostic type, JSON and human text, the code table; explain.go (1,560) is the text of `xo explain` | yes; the JSON is already printed by selfhost/check (dump.xo); explain is data |
| internal/mod | 3,436 | xo.mod, xo.lock, MVS, fetching (proxy, tar, zip), vendor, effect lock | yes; reading xo.mod, xo.work, xo.lock and the module cache is ported (selfhost/check modfile.xo, 1,347); fetching, MVS writes, vendor, proxy are not |
| internal/sumdb | 833 | checksum log client and server (ed25519) | yes; needs ed25519 in std/crypto (not there) |
| internal/format | 1,530 | `xo fmt` | yes; first of the tools (everything that rewrites code formats) |
| internal/fix | 711 | `xo fix`: fixes and migrations | yes; the fixes come from selfhost/check already |
| internal/vet | 331 | `xo vet` | yes |
| internal/query | 825 | `xo query` | yes |
| internal/edit | 757 | `xo edit` | yes |
| internal/doc | 176 | `xo doc` | yes |
| internal/defhash | 793 | definition hashes (`xo query hash`, release, lock) | yes |
| internal/release | 593 | `xo publish --check` API diff | yes |
| internal/lsp | 1,788 | `xo lsp` | yes |
| internal/mcp | 604 | `xo mcp` | yes |
| internal/serve | 462 | `xo serve` (unix socket) | yes; needs unix sockets natively (std Net is TCP) |
| internal/cbind | 1,436 | `use c` binder: runs clang on the header, writes the declaration file and adapter | yes (it only runs clang and parses its output); today xo1 reads its cache |
| internal/gotool | 186 | finds `go`, its version (decision 0176) | yes, small; only the Go backend needs it |

### Parts that can stay in Go permanently

These do not contradict "the compiler is written in Xo": they are Go
because their job is Go, or they are test infrastructure.

| Go code | lines | why it stays |
|---|---:|---|
| std/rt | 31,933 | the runtime of the Go backend: generated Go imports it, so it is Go by nature (as std/llrt is C) |
| internal/gobind | 1,890 | binds `use go` packages with `go list -export` and go/types export data; `use go` builds only with the Go backend, which needs the Go toolchain anyway. It stays a Go program (a helper, `xo-gobind`, built by `go build` on first use or shipped), writing the cache selfhost/check reads (decision 0177) |
| internal/progen, testtmp, benchharness | 1,387 + 85 + 4 | test infrastructure: they generate programs and temp dirs for Go tests, not part of `xo` |
| cmd/xoplay | 219 | the playground server, until the WebAssembly backend runs the compiler in the browser (decision 0150 item 5) |
| std/embed*.go, third_party/mbedtls, fiat-crypto Go files | 54 + 233 + 18 | they embed std, the TLS and fiat-crypto sources into the Go binary; a self hosted `xo` reads them from a directory beside it instead (lib/xo/std, lib/xo/third_party, as xo1 already reads `--std` and `DIR/../third_party`) |
| the go command itself | | the Go backend's compiler and linker (`go build`), as `cc` is the native backends' |

Everything else moves: 14,180 lines of Go without any Xo twin (cmd/xo
and the twelve tool packages above, mod and diag excluded), plus the
unported parts of internal/mod (most of its 3,436 lines: fetching,
MVS writes, vendor, proxy), internal/sumdb (833), internal/cbind (1,436),
internal/gotool (186), and the explain texts (1,560 lines, mostly
prose to copy).

## xo1 against `xo build` (2026-10-12)

`bench/results/retire-go-2026-10-12.md`, xo1 built by the Go compiler
with `--backend=arm64` (A). Run 1 is main fdda92f with `xo1
--backend=llvm` only; run 4 is main dbb8b05 with every commit of this
decision:

| suite | how | run 1 | run 4 |
|---|---|---|---|
| arm64 goldens (internal/llvm/testdata/golden want.txt, internal/arm64/testdata/run against the Go backend) | `selfhost-goldens.py A` | 157 of 157 | **158 of 158** |
| the same with the LLVM backend | `XO1_BACKEND=llvm selfhost-goldens.py A` | 157 of 157 | **158 of 158** |
| Go backend goldens (internal/gen/testdata/golden) | `selfhost-gogoldens.py A` (`--backend=go`; `test_` programs with `test --backend=go`) | 96 of 133 | **134 of 134** |

Run 1's 37 Go backend failures were the 27 `test_` programs (no `xo1
test --backend=go`) and 10 `use go` programs (adapters not in the build
tree, selfhost/gen's names for binding declarations, a third party
package's go.mod requirements). Before this decision xo1 had no LLVM
path at all (a gap of decision 0150's list).

What `xo build|run|test` has that xo1 does not (the CLI surface below
lists all of it): more than one directory, `--targets=all` and cross
targets on the Go backend, `run`, `--race`,
`--holes`, `--no-mono`, `--no-arenas`, `--release`, `--ci`,
`--seed`/`--explore` (deterministic scheduler), `--record`/`replay`,
the binary cache (XO_BIN_CACHE), split LLVM builds, `--real` tests,
contract fuzz with regressions_test.xo, the JSON report of a failed
check (xo1 says "run xo check"), binding `use go`/`use c` on a cache
miss, `-explore` and standard input for Go backend tests, the Go line
of the generated go.mod (selfhost/gen writes `go 1.26`; internal/gen
min(installed, 1.26), decision 0176), and Xo's capped Go cache
(GOCACHE, decision 0176).

## Decision (proposed)

Stages, smallest risk first. Each ends at a point the owner approves;
until then the Go implementation stays the reference and the TestSelfhost
differential tests stay.

**Stage 1: xo1 builds what `xo build` builds** (selfhost/xo, about
1,500 lines of Xo, an estimate). The flags and features listed above,
`run`, several directories, the binary cache, the split LLVM build,
and printing the check's diagnostics itself (selfhost/check has them as
JSON). Gate: every golden suite through xo1 equals the Go driver's
result (the three rows above, at 100 percent since run 4 of this
decision, kept there), `selfhost-samebin.py` byte identical binaries
on arm64, and the build, run, and test cases of cmd/xo's tests through
xo1 (below). No switch. First experiment: `xo1 run` (build, then exec
the binary with the arguments after `--`), checked by running
`selfhost-goldens.py` with `run` instead of `build` plus a run.

**Switch point A (owner): the released `xo build|run|test` for native
backends is the self hosted binary** (what decision 0150 already
allows after the fixed point). Mechanism: the Go `xo` execs a shipped
`xo1` for `build|run|test --backend=arm64|llvm` (no behavior change
for other commands), so the Go CLI still owns the rest. Gate:
stage 1's suites, the fixed point (A, B, C byte identical assembly),
and the full suite with `XO_SELF_DRIVER=1` routing the internal/build
tests that build natively through xo1. Rollback: one environment
variable.

**Stage 2: the CLI core in Xo** (about 4,000 lines): `check` (`--json`
and human output: diag's renderer, 174 lines), `parse`, `explain`
(the texts copied from explain.go), `version`, `init`, `fmt`
(format, 1,530), `fix` (711), `vet` (331), `doc` (176). Each is a
subcommand of selfhost/xo. The gate is cmd/xo's own tests (29 files,
3,874 lines; they call `run(args, stdout, stderr)` in process: check
14 times, build 11, run 6, parse 6, replay 5, ...): a helper makes
`run` exec `$XO_CLI <args>` when it is set, so the same tests run
against both binaries, one command at a time as each moves. Native std
surface needed first: Env.program, Fs.mode, Fs.glob, Proc.cmd are
XO1201 on native builds today (decision 0174 worked around them).

**Stage 3: semantic tools** (about 3,000 lines): `query`, `edit`,
`defhash`, `release`, `doc --json`. They need the checker's resolved
symbols, which selfhost/check records (info.xo) for the SSA builder.

**Stage 4: modules** (about 3,000 lines): `get`, `mod`, `vendor`,
`publish`, `audit`, sumdb. Needs ed25519 in std/crypto (a std decision)
and the native HTTPS client (present, decision 0105).

**Stage 5: servers** (about 2,900 lines): `lsp`, `mcp`, `serve`,
`dump`, `replay`. Needs unix sockets (serve) and sending a signal to
another process (dump) natively.

**Stage 6: binders**: `bind --c` in Xo (cbind, 1,436); `bind` (Go)
calls the Go helper `xo-gobind`, which stays Go.

**Switch point B (owner): `xo` is the self hosted binary.** Required
CLI surface, all in Xo: every command of `xo help` (get, mod, vendor,
publish, audit, init, parse, check, build, run, test, bind, fmt, vet,
fix, query, edit, doc, explain, dump, replay, mcp, lsp, serve, version)
with the same flags, exit codes, and JSON; the hidden dumps used by
tests are not needed (they compare the two implementations). Gate: the
full suite, with every test that drives `xo` as a process run against
the Xo binary (`XO_CLI`), and every Go API test of a moved package
ported to an Xo test or to a cmd/xo test of the binary. Release archives then ship
xo (Xo, native), lib/xo/std, lib/xo/llrt, lib/xo/rtpack,
lib/xo/third_party, lib/xo/rt (the Go backend's runtime), and
xo-gobind.

**Stage 7: dropping the differential tests.** After switch point B
the Go packages are no longer the reference. In order: (1) freeze the
Go implementation (no language change lands in it; the spec and the
Xo code are the source of truth); (2) turn each TestSelfhost* test's
Go side into recorded goldens: the last Go dump of every repository
run (lexdump, astdump, typedump, ssadump, asmdump, llvmdump, gendump)
committed as testdata, compared by an Xo test, and regenerated from the
Xo implementation from then on, so the mutation tests keep comparing
against a fixed output; (3) delete the hidden dump commands and the
Go compiler packages in one commit per package (removal is the owner's
call, decision 0150), keeping std/rt, gobind, and the test helpers;
(4) the build tag of the Go `xo` for bootstrapping: the first Xo `xo`
on a new host is built by the previous release's binary, never by Go
again (a bootstrap chain, recorded in toolchain.md). The machinery for
(2) and (4) exists since 2026-10-12, nothing switched (section "Stage 7
preparation"): the recorder and replay mode (`XO_SELFHOST_GOLDENS=
record|replay`, scripts/selfhost-record-goldens.sh) and the bootstrap
chain (scripts/bootstrap-xo.sh, tested on a clean EC2 machine with Go
deleted: fixed point reached). The recording is 1.3 GB, so it is not
committed yet; the layout below is proposed.

## Stage 4 status (2026-10-12)

`selfhost/cli/modules` ports `get`, `mod tidy`, `mod accept-effects`,
`mod clean`, `mod proxy serve`, `vendor`, and the fetching underneath
(git, the module proxy client, the checksum log client) from cmd/xo
mod.go, internal/mod, and internal/sumdb. xo.mod, xo.work, and xo.lock
are read by selfhost/check's modfile.xo (its functions made `pub`); the
Ed25519 it needed is decision 0189. xo1 dispatches the module commands
to the package. Evidence (`bench/results/retire-go-2026-10-12.md`,
"Stage 4"): 8 of 11 module tests of cmd/xo pass with `XO_CLI=xomod`, and
TestModulesXoSame finds xo.mod, xo.lock, vendor/, the cache trees, and
the output (audit reports included) byte identical to internal/mod's and
cmd/xo's over 41 steps. `audit` is ported too (selfhost/check records
the escape hatches it meets). Found on the
way: the native SSA builder named per type generated functions by the
unqualified type (gotchas/ambiguous-type-generated-function-names.md),
and `archive.untar` refused `git archive`'s PAX global header (now
skipped, std.md 18); a native program that only asks whether a stream
is a terminal did not link (IOFlags lacked xo_isatty, fixed in
internal/build and xo1). Pending: `publish` and the API changes `get`
prints need internal/release in Xo (stage 3); TestAudit needs `use go`
bound on a cache miss (stage 6); counting the tests
against xo1 itself once stage 2's commands are merged.
## Stage 1, as done (2026-10-12)

xo1's command line is `xo build|run|test`'s (selfhost/xo opts.xo: Go's
flag rules, the same flags, several directories as one program); `run`
execs the program with its arguments and passes its status through
(run.xo); a failed check prints the diagnostics as `xo` does, human or
`--json` (diag.xo; byte identical on the programs compared); builds go
to the Go driver's build directory and binary cache under the same keys
(cache.xo: an `xo` link and an `xo1` link of the same program share one
entry); `XO_LLVM_SPLIT`, `XO_LLVM_JOBS`, `XO_LLVM_LTO` split LLVM builds
(split.xo, llvm.Split ported); `--release` (selfhost/ssa compiles out
debug contracts), `--stamp` (stamp.xo), `--targets` (cross targets on
the Go backend), `--race`, `--holes`, `--no-mono`, `--no-arenas`,
`--seed`, `--explore`, `--record`; the Go backend's tests write
regressions_test.xo (regress.xo); native test binaries list contract
fuzz tests as skipped; selfhost/ssa attributes a dependency's log
records (decision 0173's `log_from`). Not in xo1 yet: `--ci` (needs `xo
fmt` and `xo vet`: stage 2), `--hybrid`, `--real` on native tests (the
real capability wrappers of the SSA builder), XO1201 diagnostics (the
self hosted SSA builder records no spans for what it skips), workspace
tests (mod.WorkspaceDirs), the native test time limit.

Gate figures (results file, runs 5 and 6): arm64 goldens 159 of 159,
LLVM 159 of 159, Go backend 134 of 134, the same through `xo1 run`;
cmd/xo's 18 tests that call `build`, `run`, or `test` with every such
command through xo1: 11 of 18 before the fixes above (the failures were
`--ci`, XO1201, the fuzz listing, `--real`, and log attribution).

## Switch point A, as implemented (2026-10-12)

- **Which commands**: `xo build|run|test` whose backend is native
  (`--backend=arm64|llvm`, or `--no-gc` without a backend) on a
  darwin/arm64 or linux/arm64 host. The Go backend, every other
  command, and the flags xo1 lacks (`--ci`, `--hybrid`, `--real`) and
  a test of a workspace stay with the Go driver
  (internal/selfdriver.Wants, TestWants).
- **Where xo1 comes from**: package selfhost embeds the selfhost/
  sources (selfhost/embed.go); `xo` writes them with its embedded std
  and third_party trees to `<user cache>/xo/xo1/<key>` (key: the
  SHA-256 of every embedded file; `XO1_CACHE` moves it) and builds
  selfhost/xo there once with its own arm64 backend under a file lock,
  so xo1 is a function of the sources compiled into `xo` and needs no
  repository or network. `XO1=<path>` uses a prebuilt binary instead
  (`XO1_ROOT` names its std and third_party). The first native build
  after a change of those sources builds xo1 (290 s on the busy
  machine of run 6, 694 s on a busier one) and says so on standard
  error; a release would ship the cache entry prebuilt (not done: the
  release script is unchanged).
- **Fallback**: a program xo1 does not support yet (a construct its SSA
  builder or an emitter skips, the checker's unsupported list) is handed
  back: xo1 writes why to the file `XO1_FALLBACK` names and exits 125,
  and `xo` runs the Go driver, which prints the XO1201 diagnostics or
  builds it. The prebuilt runtime is the one beside `xo` (`XO_RTPACK`
  passed to xo1).
- **Rollback**: `XO_SELF_DRIVER=0` (or `off`) runs the Go driver for
  everything, as before. A failure to build xo1 is an error naming that
  variable.
- **Tests**: cmd/xo's tests run with the switch on by default;
  `XO_SELF_DRIVER_ALL=1` (cmd/xo/selfdriver_test.go) routes every
  `build|run|test` of them, Go backend included, through `$XO1` to
  measure stage 1. Scripts that compare xo1 with the Go driver run the
  reference with `XO_SELF_DRIVER=0` (selfhost-samebin.py,
  selfhost-switchover.sh's A).
- **Gate (run 6 of the results file)**: cmd/xo's full suite and
  internal/build's full suite pass with the switch on; selfhost-samebin
  159 of 159 identical binaries; the fixed point holds (the Go emitter's
  assembly of selfhost/xo equals B's and C's); B runs the arm64 goldens
  159 of 159. With every command of cmd/xo's 18 build, run, and test
  cases forced through xo1 alone (no fallback): 12 of 18, the rest being
  `--ci`, XO1201, and native `--real`, which the switch keeps on the Go
  driver.

## Owner approvals (2026-10-12)

- Switch point A: approved by the owner on 2026-10-12; implemented the
  same day (section "Switch point A, as implemented"; gate: run 6 of the
  results file).
- Switch point B: approved by the owner on 2026-10-12, to take effect
  when its gate passes (stages 2 to 6 done, the full suite with every
  process-driving test run against the Xo binary). The approval does not
  skip the gate. Stage 7's deletion of the Go compiler packages remains a
  separate owner call (decision 0150).
- Order after B (owner, 2026-10-12): freeze the Go implementation right
  after switch point B (stage 7 step 1: no language change lands in Go),
  then the runtime port (decision 0180's bar first), then deleting the Go
  compiler packages, which the owner delegated to the lead once these
  hold: B live with the full suite passing against the Xo binary, stage 7
  steps 1 to 3 done (frozen, Go outputs recorded as goldens, bootstrap
  chain tested), two combined full-suite batches after B with no
  regression the live cross-check would have caught, and every output
  users or tests depend on produced by Xo.

## Stage 2 results (2026-10-12)

The CLI core is in Xo in selfhost/cli/core (5,765 lines with the
generated explain texts): `check`, `parse`, `explain`, `version`,
`init`, `fmt`, `fix`, `vet`, `doc` (with `--json`), each with cmd/xo's
flags (a port of Go's flag package: `-x` and `--x`, the same errors and
usage text), exit codes, and output (JSON indented as Go's encoder,
diagnostics escaped as json.Marshal escapes them). xo1 dispatches them
(`core.is_command`). The formatter is selfhost/cli/core/format, a
package of its own with a command line for the comparison test; the
explain texts are generated from internal/diag
(TestSelfhostExplainData keeps them in sync) and the version is checked
against cmd/xo's (TestSelfhostVersion). cmd/xo's tests run against
another binary with `XO_CLI` (cmd/xo/cli_test.go).

- The formatter: **1121 of 1121 repository files** and **1121 of 1121
  copies with changed blanks byte identical** to internal/format
  (TestSelfhostFormat).
- cmd/xo's tests of these commands against xo1 (`XO_CLI=xo1
  XO_STD=<repo>/std`, `-short`): **19 of 19 pass**.
- `scripts/selfhost-cli.py`: fmt and parse **715 of 715** directories
  byte identical; check, check (human), vet, doc **706 of 715**; the 9
  others need a module version the Xo checker would have to fetch.

Not done in stage 2: `check --no-gc` reports no XO1201 (the SSA
builder's no GC diagnostics are not ported; the native check runs);
std is found through `--std`, XO_STD, or lib/xo/std beside `$_` until
Env.program is native (cli-servers' branch); fetching a missing module
during a check (the Xo checker reads the module cache only).

## Stage 3 results (2026-10-12)

The semantic tools are in Xo in selfhost/cli/semantic (4,852 lines):
`xo query` (type, def, callers, effects, holes, api, imports, deps,
hash), `xo edit` (rename, add-field, change-sig, atomic through temporary
files and renames, rejected unless the edited program checks clean), the
definition hashes of decision 0125 (defhash.xo), and internal/release
(release.xo: the API diff and `xo publish`'s comparison, for stage 4's
`publish` command to call). xo1 hands `query` and `edit` to it; `doc`
(with `--json`) is stage 2's (selfhost/cli/core/doc.xo), and signatures
come from stage 2's formatter. selfhost/check gained the effect call
chains of Info.Performed, the xo.mod and version helpers, and an
overlay; holes (XO0900) and the workspace directories came from stage 2.

- Definition hashes: TestSelfhostDefhash compares the JSON of every
  definition (IDs, own and deep hashes, references, files, lines) with
  internal/defhash on every module of bench/, the check, gen, and arm64
  testdata, selfhost/, std/, and examples/: **270 of 270 modules (13,662
  definitions) byte identical**.
- cmd/xo's tests against xo1 (`XO_CLI=xo1`, `XO_STD=<repo>/std`,
  `-short`): TestQueryCLI, TestEditCLI (6 of 6 cases), and TestDocCLI
  pass; **3 of the 5 tests that drive these commands pass**.
  TestQueryHashAndPublish passes its `query hash` steps and stops at
  `xo publish` (stage 4's command), TestModules at `check` of a module
  with requirements before it reaches `query`, `doc`, and `edit` (stage
  4).
- The same commands run by both binaries on the cmd/xo test programs
  (stdout, stderr, exit status, and the files after an edit):
  **37 of 37 byte identical** (bench/results/retire-go-2026-10-12.md).

## Stage 5, the servers (2026-10-12)

`mcp`, `lsp`, `serve`, `dump`, and `replay` are selfhost/cli/servers,
dispatched by xo1 (after the module and semantic commands, before stage
2's). Native std first (decision 0190): `unix:PATH` addresses for
`Net.listen`/`dial` (serve) and `Proc.kill(pid, signal)` (dump), both
gated by attenuation; `Env.program` natively (the servers run their own
binary again). How each command maps to Go's:

- **mcp**: internal/mcp's server, with a JSON reader and writer in Xo
  that reproduces encoding/json (scanner error texts, struct versus map
  key matching, RawMessage kept as written and compacted, sorted map
  keys, HTML escaping, numbers through float64). The tools run the
  binary itself with `XO_NO_SERVE=1` where Go calls `run` in process.
- **serve**: the same socket protocol, so Go and Xo clients and servers
  interoperate; xo1's `check` and `query` forward to a running server
  as cmd/xo does. The cache keeps whole command responses per working
  directory and checked directories (the Go server keeps checked
  sessions), with the same hit and miss counts.
- **lsp**: framing, initialize, document sync with the 150 ms debounce,
  publishDiagnostics from the self hosted checker with the open texts as
  its overlay, codeAction, documentSymbol. Hover, definition,
  references, rename, formatting, and semantic tokens followed with the
  switch point B gaps (resolve.xo, below).
- **dump**: SIGQUIT through `Proc.kill`, then the same polling of the
  dump file.
- **replay**: rebuilds with `xo build --backend=go` as a child and reads
  the new build information from the binary (the string passed to
  rt.SetBuildInfo) to report changed sources.

Gate figures (results file, "Stage 5"): byte identical with the Go
servers on the MCP protocol transcript (36 of 36) and tool calls (10 of
10), dump (9 of 9), the serve protocol (10 of 10) and cache, the LSP
transcript (14 of 14 steps), and replay; cmd/xo's own tests of these
commands against xo1: 4 of 7 (the three others build or `run --record`
through xo1, whose default backend is arm64, not Go).

## Stage 6, the binders (2026-10-12)

- **`xo bind --c` is Xo**: selfhost/cli/bind ports internal/cbind
  (2,299 lines); xo1 dispatches `bind`. TestBindCXoSame runs both
  binders over the repository's `use c` headers, zlib.h, a header of
  edge cases, and the error cases: **17 of 17 byte identical** (output,
  declaration files, adapters), with xobind and with xo1.
- **`xo bind` for Go packages stays Go**: the helper `xo-gobind`
  (cmd/xo-gobind, `gobind.RunCLI`, shared with cmd/xo's runBind). The Xo
  CLI execs it with the same flags; it is found as $XO_GOBIND, beside
  the program, in a release layout's bin/, on $PATH, or built from the
  repository holding std with `go build` into a temporary file. Release
  archives ship it beside `xo` (switch point B's list above).
- **Binding and fetching on a miss** (decided here): the Xo checker
  stays free of process and network effects. It records what a run
  lacks (a `use go` package without a current binding, every `use c`
  header, a module version missing from the module cache), and
  `Checker.check_dirs_bound` and `check_dirs_fetching` bind or fetch it
  and check again from the state before the run, so a run sees what the
  Go checker sees after its in line gobind.LoadOrBind, cbind.Bind, and
  mod.Load. A failed binding or fetch is the Go checker's diagnostic
  (XO1101, XO1104, the module problem). The fetcher is passed in as a
  function because selfhost/check cannot import selfhost/cli/modules;
  `query` and `edit` (selfhost/cli/semantic, which the module commands
  import) bind but do not fetch.
- **`xo publish` and `xo get`'s API changes** are in
  selfhost/cli/modules, with release.Publish's last steps (clean tree,
  mod.Load without a workspace, a check with workspaces off).
- Gate figures (bench/results/retire-go-2026-10-12.md, "Stage 6"):
  against xo1, TestBind and TestBindC pass and the module, publish, and
  audit tests **12 of 13**, the last being TestModules' `build --ci`
  (no `--ci` in xo1); against xomod 13 of 13. Not yet: `--ci` in xo1,
  fetching in `query` and `edit`, binding and fetching in the LSP.
## Switch point B gaps (2026-10-12)

The gaps of xo1 outside stage 6, closed on one branch (results file,
"Switch point B gaps and gate"):

- xo1's `build|run|test` default to the Go backend as `xo` does (arm64
  with `--no-gc`); scripts that want arm64 name it.
- std is found through Env.program, as a release finds lib/xo/std beside
  bin/xo (selfhost/check locate.xo: executable, std_dir), for xo1 and
  every selfhost/cli command; no `$_` anywhere. The gate runs xo1 from a
  release layout with no XO_STD.
- `xo check --no-gc` reports selfhost/ssa's XO1201 diagnostics (each
  unsupported construct at the expression being lowered, cascades
  dropped as ssa.NoGCDiagnostics drops them); native builds print them
  when no Go driver takes the program back.
- `build --ci`, the XO1308 check of `--ci` and `--release`, and native
  `test --real` are in xo1. `--hybrid` builds the plain Go program with
  the Go driver's fallback report: the Go assembler mode of the arm64
  backend is not ported.
- `xo lsp` answers every request of the Go server: 38,944 of 38,944
  responses byte identical over four corpora (TestXoLSPFeatures).
- Fetching a module version the cache lacks during a check is the
  cli-bind branch's (Checker.check_dirs_fetching); this branch's version
  was reverted to avoid two implementations.

Gate (cmd/xo, full, `XO_CLI` = xo1 at commit 1548ddc): 53 of 69 tests
pass. The 16 failures are stage 6 and publish (13: bindings on a cache
miss, `xo bind`, `xo publish`, get's API changes, fetching), and three
commands not in xo1: `ssadump` and `llvmdump` (comparison dumps, not
needed) and `rtpack` (builds the release's runtime packs: port it, or
keep it in a Go helper that release.sh runs).

## Stage 7 preparation (2026-10-12)

Built for stage 7 steps 2 and 4 (the bootstrap chain) without switching
anything; evidence in the results file, section "Stage 7 preparation".

- **Recorder and replay** (internal/selfhost/gogoldens_test.go): every
  Go side of the TestSelfhost* differentials (lexdump, astdump, the
  formatter, typedump, ssadump, asmdump, llvmdump, gendump, defhash,
  and the mutation tests' reference outputs, which are the same kinds on
  mutated inputs) goes through one function. `XO_SELFHOST_GOLDENS=record`
  (scripts/selfhost-record-goldens.sh) runs only the Go side, at the
  full tier's sizes, and writes each unit; `=replay` reads them instead
  of running Go; unset, nothing changes. A unit is keyed by its kind,
  options, and input content (a file's path and bytes; a run's
  directory, working directory, and module tree; the host for the
  native kinds), with the repository root and the test's temporary
  directory as placeholders, so a recording replays in any checkout and
  a changed input shows as a missing unit (TestMain fails the run and
  names it), never as a stale verdict. What a run imports from outside
  its module tree (std, other repository packages) is not in the key:
  regenerate after such a change.
- **Verdicts**: on one tree, replay gives the live verdicts and the live
  logged counts for every differential under `-short` and for the
  mutation, format, and defhash tests at the merge tier's sizes, except
  TestSelfhostCheckerHatches, which skips in replay (the Xo checker reads
  the binding the Go checker writes as a side effect; after the
  deletion it must come from xo-gobind).
- **Size**: 1.4 GB in one gzip file per unit (19,643 units; ssadump 598
  MB, llvmdump 538 MB, asmdump 152 MB; 10 GB uncompressed), so the
  recording is git ignored for now. Measured alternatives: shared
  content defined chunks 574 MB; zstd's long window takes ssadump from
  549 MB to 79 MB but is not in Go's standard library.
- **Proposed layout** (for the owner or the lead to accept before
  step 2 is done): (a) in git, under internal/selfhost/testdata/
  gogoldens/: a hash per unit for the kinds compared byte for byte
  (lexdump, astdump, format, defhash: 10,965 units, about 1.1 MB), since
  a hash gives the same verdict and, once Go is deleted, there is no
  reference text to diff against anyway; plus typedump and gendump as
  chunk pooled gzip (about 16 MB). (b) Out of git: ssadump, llvmdump,
  and asmdump, stored as a versioned artifact (a release asset or the
  xo-ci bucket) named by the recording's commit, with its SHA-256 and
  that commit in a manifest file in git, fetched into
  `XO_SELFHOST_GOLDENS_DIR` by the record script; or, the smaller next
  step, replaced by per function hashes (the comparisons already cut
  these dumps into functions), which would also fit in git. Either way
  the test code is unchanged except for the store behind `recorded`.
  Still open: the mutation generators tokenize with internal/lexer, so
  deleting it needs the mutated inputs recorded or the generators moved
  onto the Xo lexer's dump; llvm.SplitIR, ssa.SplitModule, and
  gen.FormatFile used by the comparisons must be copied into the test
  package first; recording takes 25 minutes and 15 GB of RAM (the Go
  side of the selfhost components), an EC2 job rather than a Mac one.
- **Bootstrap chain**: scripts/bootstrap-xo.sh builds selfhost/xo with
  a given xo (s1), then s1 builds s2 and s2 builds s3, each with empty
  caches and no Go on PATH, and requires s2 and s3 identical (assembly
  and binary) and s3 to build and run a program; scripts/remote-bootstrap.sh
  runs it on a clean EC2 machine after deleting Go. Passed on 2026-10-12
  (Amazon Linux 2023 arm64, commit 30dfd24): s2 and s3 identical, s1
  too; and on the development Mac with Go off PATH (commit 7b3da52),
  the same. It first failed and found a bug of xo1's own TLS build with a
  relative `--std` (gotchas/xo1-relative-std-tls-tree.md). The seed was
  built by Go on the same machine before the deletion, standing in for
  the previous release's binary until a release ships the Xo `xo`.

## Consequences

- Until switch point B every language change is still made twice
  (Go and Xo) or the differential tests fail; stages 2 to 6 add a third
  place for tool changes only while the two CLIs coexist, which is why
  each tool moves in one change with its cmd/xo tests run against
  both binaries.
- Duplicated tables drift silently between the two drivers: xo1's
  runtime flags missed the decision 0172 file symbols until this
  decision (gotchas/xo1-duplicated-tables-drift.md).
- The xo1 LLVM path (`--backend=llvm`) uses clang's target attributes,
  `-O2 -flto -Wno-override-module`, and one module; a large program
  (selfhost/ssa: 34 minutes of clang on one module) needs the split
  build before xo1 replaces `xo build --backend=llvm` for it.

## Alternatives

- **Switch everything at once** after porting all tools: one large
  change with no rollback point; switch points A and B split the risk.
- **Keep the CLI in Go and only the compiler in Xo** (the Go `xo` execs
  xo1 for every build): still two languages for the toolchain; it
  contradicts decision 0150's goal of retiring the Go implementation,
  but it is a fine permanent fallback if a stage stalls.
- **Port gobind to Xo**: it needs Go's export data reader (go/types);
  reimplementing it in Xo is a large job for a feature that needs Go
  installed anyway.
