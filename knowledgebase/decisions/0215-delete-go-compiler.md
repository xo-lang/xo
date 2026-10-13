# 0215: Deleting the Go implementation of the compiler

Status: Accepted (lead, delegated by the owner; owner go-ahead
2026-10-13). Done on a branch (2026-10-13), one commit per step.

## Context

Decision 0178 stage 7 step 4. Every readiness criterion holds
(bench/results/batches-after-b-2026-10-13.md: clean batches 3 and 8;
decision 0196 met, with its correction: the Go backend survives as
selfhost/gen). Since switch point B the shipped `xo` is xo1 (selfhost/xo)
and the Go implementation was frozen. The last commit with the Go
compiler is tagged `go-compiler-final` (87873aa5).

## Decision

Delete the Go implementation of the compiler and every Go package of the
toolchain that has an Xo twin, in this order, each its own commit:

1. **Record the Go side.** The TestSelfhost* differential tests compared
   each self hosted component with the Go package doing the same job.
   Their Go side was recorded once more (scripts/selfhost-record-goldens.sh
   on the Mac at the go-compiler-final tree, 2,049 s), then indexed: every
   unit gets a slot, its id without std's sum and, for an input of the
   repository, without the content of its files (an input in the test's
   temporary directory, a mutation, keeps its content). Replay became
   the default. A unit whose input or std changed compares with its
   slot's recording ("stale", reported, not a failure); a unit with no
   recording at all fails the run by name (no silent skip). The recording
   then held 20,160 units, 9,001,023 bytes (gogoldens_test.go has the
   format). The Go backend's output of internal/arm64/testdata/run was
   recorded as testdata/runwant/<name>.txt by the go-compiler-final Go
   xo for the golden scripts.
2. **Scripts and the checkout's xo.** internal/xotool finds the xo the
   Go tests and scripts run: `$XO_CLI`, else the checkout's bin/xo,
   rebuilt when the key of its sources changes (the host, selfhost/, the
   std modules selfhost reaches through `use`), once per key in
   `<user cache>/xo/checkout/<key>` (shared by worktrees, under a file
   lock). The seed is `$XO_SEED`, the earlier bin/xo, the newest cached
   build, `xo` on PATH, else the xo1 of the go-compiler-final tag (git
   archive of the tag, or of commit 87873aa5 in a clone without tags;
   `go build ./cmd/xo`; that xo builds the tag's selfhost/xo).
   `go run ./scripts/xobuild` prints bin/xo. selfhost-goldens.py and
   elf-goldens.py read runwant instead of building with the Go xo;
   release.sh, bootstrap-xo.sh, remote-bootstrap.sh, xo1-host-check.sh,
   windows-batch.sh, the bench harness, and bench/size take their seed
   from xobuild or `XO_SEED`; the comparison scripts against the Go xo
   (selfhost-switchover.sh, selfhost-samebin.py, selfhost-cli.py) are
   deleted. xotool also carries the helpers the remaining Go tests
   build with (Build, Asm, LLVMIR, Check, CompileC, IOFlags; IOFlags is
   checked against selfhost/xo's io_flags).
3. **Delete**, importers first: cmd/xo's Go code and hidden dumps (its
   tests stay and drive xo), internal/selfdriver and the embedded
   selfhost sources, cmd/xoplay, the selfhost tests' Go side, the
   backend tests' Go build calls, internal/build, gen, llvm, arm64, ssa,
   lsp, mcp, serve, edit, doc, release, defhash, query, fix, vet, format,
   check (with std's embeds of the runtimes), cbind, mod (modtest
   stays), sumdb, parser, lexer, ast, diag (its repository checks moved
   to internal/repotest), and token.
4. **Final state.** xo1 is the only compiler. The TestSelfhost* tests
   compare each component, built by xo, with the recording; a recording
   run (`XO_SELFHOST_GOLDENS=record`) records the Xo side for units with
   no recording of their input, `rerecord` for every unit a run
   computes. CLAUDE.md, the spec, toolchain.md, README, and CHANGELOG
   say so.

The testdata of the deleted packages stays in place: its .xo files are
inputs of the recorded units (moving them would change their keys), and
the golden scripts read internal/llvm, internal/gen, and internal/arm64
testdata.

### Lines removed

From the stub commit (47186792) to the last deletion (7fdbe7eb),
`git diff --numstat` over `.go` files: 95,011 lines of non-test Go
deleted (1,264 added, net 93,747), and 18,837 lines of Go tests deleted
(2,102 added, net 16,735). The largest: internal/check 18,159 net,
internal/ssa 16,639, internal/gen 13,424, internal/arm64 12,866,
internal/parser 7,468, internal/build 6,612, cmd/xo 6,200,
internal/llvm 5,768, internal/mod 4,396.

### What stays in Go, and why

39,357 lines of non-test Go outside bench/ and runs/ (2026-10-13):

- std/rt (32,211): the Go backend's runtime; generated Go imports it.
- internal/gobind and cmd/xo-gobind (2,012), internal/gotool (330):
  `use go` binds Go packages through go/types export data; xo execs the
  xo-gobind helper.
- Test infrastructure: internal/xotool (1,129), internal/progen
  (1,397), internal/testtmp (85), internal/mod/modtest (299),
  internal/selfhost (1,377: the frozen mutation lexer and the package
  file), scripts/xobuild (34).
- scripts/disttar (118): the deterministic archives of release.sh.
- third_party/mbedtls and fiat-crypto (281): the vendoring tool that
  writes mbedtls.tgz, and its check.
- std/embed.go (14): the declaration files gobind reads.

Test only packages (no Go code of their own), all driving the xo binary:
cmd/xo (the CLI), internal/llvm, internal/gen, internal/arm64 (the
backends and runtimes), internal/selfhost, internal/repotest.

### Tests not carried over

Tests of the Go implementation's internals went with it (the Go API
tests of every deleted package). Carried over to the xo binary: cmd/xo's
CLI tests, the runtime and backend tests of internal/llvm, gen, and
arm64, the selfhost tests, and the repository checks. Not carried over,
with the next step for each:

- Tests comparing the Go and the Xo implementations (cmd/xo bind_xo,
  modules_xo, rtpack_xo, lsp_xo, servers_xo, selfdriver,
  selfhost_version, ssadump, llvmdump): nothing left to compare.
- cmd/xo TestChecksumLogCommand: its log server was the Go
  internal/sumdb; next: a log server in Xo (`xo mod sumdb serve`) or a
  small Go fixture.
- The text form of `xo audit` (printed only on a terminal): next, an
  option or a pseudo terminal in the test.
- Share probes (TestGoldenShared, TestGoldenThreads, TestGoldenRunShared):
  a pass of the Go SSA builder that selfhost/ssa does not have; next:
  port it (decision 0058).
- Sanitizer builds of the TLS library (TestTLSLeaks, TestH2Sanitizers
  skip by name; internal/build's TestTLSAccelSanitizers, TestP256):
  next, move internal/build/tls.go's library build into xotool.
- TestZoneDataPerUseNative and the Go backend's timezone test read the
  Go build directory's zone data file; the time goldens still run.
- internal/gen's tests of the generator (arena escape analysis, mono,
  range elision, JSON codegen, go vet of generated trees, snapshots,
  hybrid plans) and internal/arm64's (layout, inlining, op lowering):
  selfhost/gen and selfhost/arm64 have their own Xo tests and the
  replays.
- cmd/xoplay (the playground's compiler half): the site builds it from
  the go-compiler-final tag (playground.md); next, an Xo xoplay over
  selfhost/check, format, and gen, built for wasip1.

### Gaps the deletion exposed

- Programs a native emitter does not lower yet (XO1201) were handed by
  xo1 back to the Go driver under switch point A (XO1_FALLBACK). Now
  they fail: internal/llvm TestTimeDiff's const and dynamic programs (a
  native implementation's Debug text) skip by name with the message.
- The mutation tests pick their sources by position: the order must be
  moduleDirs' (gotchas/selfhost-mutation-sources-order.md).
- A ported test ran `xo test` in a golden's directory and it wrote
  regressions_test.xo there; one commit picked it up (fixed: xotool's
  test builds remove a file they created).

## Consequences

- Every compiler change lands in selfhost/ alone. A change that alters
  a component's output on purpose, or a new input, needs a recording
  run and a look at its diff in the same change. The recorded Go side
  stays the reference for every unit whose input did not change; units
  recorded since are the Xo side's.
- A checkout's first Go test that needs xo builds it (several minutes,
  6.09 GB peak on the Mac with a cached xo1 as the seed, 391 s,
  2026-10-13) unless `XO_CLI` names one; the build is shared by
  worktrees with the same sources.
- A host with no xo needs Go and git once, for the go-compiler-final
  seed; after that, any xo builds the next (scripts/bootstrap-xo.sh).

## Alternatives

- Keep the Go compiler as the bootstrap seed in the tree: two
  implementations to keep building; the tag gives the same seed.
- Keep the live differential tests against the tag's Go compiler (build
  it as a test fixture): every repository run would rebuild an old
  toolchain, and its language drifts from the spec; the recording plus
  record mode keeps the reference without it.
- Port every Go API test of the deleted packages to xo: most test the Go
  implementation's internals; the Xo twins have their own tests and the
  replays cover their outputs on every repository input.
