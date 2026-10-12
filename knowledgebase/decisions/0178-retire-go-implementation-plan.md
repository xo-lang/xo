# 0178: Retiring the Go implementation: inventory and plan

Status: Proposed (2026-10-12; the owner decides every switch-over point
below). The inventory is from main dbb8b05 with this decision's
commits; the measurements are on the same tree (runs 1 to 4 of the
results file). Gaps closed on the way, each its own commit: `xo1
--backend=llvm`; the decision 0172 file symbols in xo1's runtime flags
(with TestXo1IOFlagsSymbols); `xo1 test --backend=go`; `use go` in the
Go backend path (selfhost/gen's `Go_<path>_` names, the binding
adapters, go.mod requirements and go.sum, the Go timers tag). Evidence:
`bench/results/retire-go-2026-10-12.md`. The released `xo` is
unchanged (decision 0150).

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
again (a bootstrap chain, recorded in toolchain.md).

## Owner approvals (2026-10-12)

- Switch point A: approved by the owner on 2026-10-12 (implementation in
  progress: the Go `xo` execs xo1 for native `build|run|test`, with a
  rollback environment variable).
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
