# Toolchain and layout

Last verified: 2026-10-12

## Host

- Development host: macOS (darwin/arm64).
- Go: 1.26.1 (`go version`). The P1 prototype is written in Go and emits Go.
- Git repository, remote `git@github.com:josephspurrier/xo.git`, branch `main`
  (initialized 2026-10-08). `.gitignore` excludes `.tools/`, build outputs
  (`benchtask`, `*.exe`, `*.test`, `bin/`, `dist/`), logs, and editor files.
- `go test ./...` needs a C toolchain for the race detector tests (skipped
  when `go env CGO_ENABLED` is 0).
- clang (2026-10-08): Apple clang 21.0.0 at `/usr/bin/clang` (Xcode 26.5
  toolchain), used by the LLVM backend prototype (`xo build
  --backend=llvm`, see "LLVM backend (P3 prototype)"). `llc` and `opt` are
  not installed; clang compiles `.ll` files directly. The Command Line
  Tools SDK that clang uses by default (MacOSX27.0.sdk) does not link
  with this linker; the build falls back to the Xcode SDK
  (`gotchas/macos-clt-sdk-link-failure.md`). The LLVM tests skip when no
  clang is found (`$XO_CLANG`, then PATH).
- cc (2026-10-08): `/usr/bin/cc` is the same Apple clang 21.0.0. The
  native arm64 backend (`xo build --backend=arm64`, see "arm64 backend
  (native)") uses it to assemble the generated `.s` and compile and link
  `xo_rt.c` (`$XO_CC`, then `cc`, then `clang` on PATH), with the same SDK
  fallback as the LLVM backend. Last verified: 2026-10-08. Since
  2026-10-11 (decision 0157) the runtime comes prebuilt from a release's
  pack when one matches ("Prebuilt runtime"); `cc` still assembles and
  links. On Linux gcc works as `cc` (gcc 10 and 14 compile the runtime,
  checked 2026-10-11).

## Go module

- `go.mod` at the repo root: `module github.com/xo-lang/xo`, `go 1.26`.
  The module path intentionally stays `github.com/xo-lang/xo` even though the
  git remote is `github.com/josephspurrier/xo` (owner's choice, 2026-10-08).
  `cmd/xo` holds only tests since decision 0215; build xo from a checkout
  with `go run ./scripts/xobuild` (writes `bin/xo`, last verified 2026-10-13).
  Before each seed's build, xobuild runs that seed's `xo check` of
  compiler/xo (about 4 s on the Mac) and moves on to the next seed when it
  fails, so an error in the sources is reported in seconds with its
  location, not after a build of minutes; `XO_BUILD_PRECHECK=0` turns the
  check off (internal/xotool CheckXo; with the seed chain on, a seed whose
  check fails is still built, through the chain). Last verified:
  2026-10-20.
- Standard library only. No third party module dependencies, for the
  compiler or for generated programs. (Go packages bound with `use go` are
  dependencies of the generated program, not of the compiler; see "Go
  interop".) Last verified: 2026-10-08.
- Vendored third party code (decision 0059): `std/rt/xcrypto_argon2.go`
  and `std/rt/xcrypto_blake2b.go` hold the portable Go of
  `golang.org/x/crypto` v0.55.0 packages `argon2` and `blake2b` (no
  assembly, no `x/sys/cpu`), renamed into package `rt`, with the Go
  authors' BSD 3 clause license in each file header. They are not a module
  requirement: the generated `go.mod` stays `module xoprog` plus `use go`
  requirements, so builds need no module download for any `--targets`
  entry. Why not a dependency: `rt` is copied whole into every build, so a
  requirement of any `rt` file is one of every program, and with an empty
  module cache and no network the build fails (`module lookup disabled by
  GOPROXY=off`; gotcha `runtime-import-is-every-programs-dependency.md`).
  To update: diff a newer x/crypto release against these files and rerun
  `go test ./std/rt -run 'Argon2|Blake2b'` (RFC 9106 and RFC 7693 vectors,
  plus keys recorded from x/crypto in `std/rt/testdata/argon2id_xcrypto.txt`).

## Go versions (decision 0176)

- Oldest Go for the Go backend: **1.24** (`gotool.MinGo`, internal/gotool):
  std/rt uses `crypto/pbkdf2`, `maphash.Comparable`, and
  `http.Protocols`. `xo build`, `xo run`, and `xo test` on the Go
  backend read `go env GOVERSION` of `$XO_GO` (else `go` on PATH) once
  per process and fail with XO1502 when it is older. Last verified:
  2026-10-12 (`scripts/go-version-check.sh rt-min` with go1.26.1).
- Newest Go the suite has passed on: **1.26** (`gotool.TestedGo`). The
  generated go.mod and a new binding workspace go.mod (`.xo/gobind/_mod`)
  get `go <min(installed language version, TestedGo)>`, so the go command
  is never asked for a newer Go than itself (no automatic toolchain
  download) and a newer Go keeps 1.26 semantics until adopted.
  `XO_GO_LINE` overrides the line. Last verified: 2026-10-12.
- Toolchains present on the development host (2026-10-12): go1.26.1 at
  `/usr/local/go`; go1.26.3, go1.26.6, and go1.27.2 in the module cache
  (`~/go/pkg/mod/golang.org/toolchain@v0.0.1-goX.Y.Z.darwin-arm64`),
  usable with `GOTOOLCHAIN=goX.Y.Z` without a download.
- **New Go release procedure:**
  `GOTOOLCHAIN=go1.27rc1 scripts/go-version-check.sh [--download] [--keep] [-n] [step...]`.
  Steps: `version`, `unit` (`go test -short` over the repo), `rt-min`
  (std/rt vets with `-stdversion` at go MinGo for darwin, linux,
  windows, wasip1, and each rt build tag), `gen-min`
  (TestGeneratedGoVets with `XO_GO_LINE=MinGo`), `goldens` (TestGolden,
  TestGoldenNoMono, TestGeneratedGoVets, the Go timers link tests, at
  the new release's language version), `usego`
  (TestGoBindingSignatures, TestReplayGoCalls, TestReplayGoCallback,
  internal/gobind, cmd/xo TestBind, internal/check TestGoInterop, with
  bindings regenerated in a fresh `XO_GOBIND_DIR`). Without `--download`
  it sets `GOPROXY=off`, so only a toolchain already in the module cache
  runs. Its Go cache, Xo Go cache, and binding cache live in a temporary
  directory removed at the end (`--keep` keeps it). The goldens,
  gen-min, and usego steps are heavy (run one at a time on a shared
  machine). When all pass: raise `TestedGo` in internal/gotool/gotool.go
  (and the repo go.mod's `go` line if the compiler should move too),
  update this section, add a CHANGELOG line. Raising `MinGo` needs a
  decision.

## Versions and releases

`xo version` prints the toolchain version (`compiler/cli/core` `VERSION`),
the git revision, and the host. The toolchain minor equals the spec draft;
releases are git tags `v0.N.0` with a `CHANGELOG.md` entry, and only the
owner declares 1.0 (decision 0089). First tag: v0.7.0. Before tagging,
run the full suite with the full internal/selfhost tier:
`XO_SELFHOST_TIER=full go test -timeout 120m ./...` (see the TestSelfhost
tier lines under Commands). Last verified: 2026-10-11.

**std/VERSION** (2026-10-23): one line, the toolchain version
(`compiler/cli/core` `VERSION`, kept equal by internal/repotest
TestStdVersionMatches; scripts/seed-cut.sh's bump writes both). An xo
whose version is below the stamp of the std it runs with prints `xo:
warning: this xo (X) is older than std at DIR (Y); rebuild it ...` on
standard error for every command but `version` and `help`
(compiler/xo provenance.xo stale_std_note): agents kept running a stale
bin/xo from PATH against a newer checkout. Last verified: 2026-10-23.

**What `xo` is (switch point B, decision 0178, 2026-10-12)**: the
released `xo` is xo1, the self hosted toolchain built from
`compiler/xo` (Xo). Since decision 0215 (2026-10-13) the Go
implementation of the compiler (cmd/xo's Go code and the compiler
packages) is deleted: the last commit with it is tagged
`go-compiler-final` (87873aa5), and xo1 is the only compiler. The
version is `compiler/cli/core` `VERSION`.

The bootstrap chain, as release.sh runs it:

    $XO_SEED, else `go run ./scripts/xobuild`
                                           -> seed (an earlier xo, or the
                                              go-compiler-final xo1; internal/xotool)
    seed: xo build --backend=arm64 --std=std compiler/xo
                                           -> darwin/arm64 xo1
    darwin xo1: --backend=arm64 --target=linux/arm64 --ld=macho
                                           -> linux/arm64 xo1 (no cc: compiler/macho's ELF link)
    darwin xo1: --backend=amd64 --target=darwin/amd64
                                           -> darwin/amd64 xo1
    darwin xo1: --backend=amd64 --targets=linux/amd64
                                           -> linux/amd64 xo1 (no cc: compiler/macho's ELF link;
                                              decision 0362, was --backend=go)
    darwin xo1: --backend=amd64 --targets=windows/amd64
                                           -> windows/amd64 xo1 (Xo's PE linker, no C runtime;
                                              decision 0254 6b; XO_RELEASE_WINDOWS=llvm for the
                                              LLVM build below)
    darwin xo1: --backend=llvm --targets=windows/amd64, split 16, ThinLTO
                                           -> windows/amd64 xo1 (only with XO_RELEASE_WINDOWS=1:
                                              XO_WIN_CLANG, an upstream clang with lld-link beside it,
                                              and XO_WINSYSROOT from xwin; the toolchain is set up on the
                                              Mac since 2026-10-15 (below, "Windows cross builds on the Mac"),
                                              but xo1 itself not built there: its 24 GB is below the 29.5 GB
                                              the Linux run used; XO_RELEASE_WIN_JOBS defaults to 4.
                                              Last verified: 2026-10-15)

After a release exists, the previous release's xo is the seed
(`XO_SEED`, or `scripts/bootstrap-xo.sh SEED`, below). xo1 needs its
release layout: `bin/xo` finds std, the pack, and `bin/xo-gobind`
beside itself (`bin/../lib/xo`, symbolic links followed), so a bare
copy of bin/xo does not run (`XO_STD` and `XO_RTPACK` override;
gotchas/xo1-outside-release-layout.md). A checkout's `bin/xo` (`go run
./scripts/xobuild`) finds `std` at `bin/../std`.

Release archives (decisions 0157 and 0178): `scripts/release.sh
[version]` on darwin/arm64 writes `dist/xo-<version>-<os>-<arch>.tar.gz`
for darwin/arm64, linux/arm64, and linux/amd64: `bin/xo` (xo1, as
above), `bin/xo-gobind` (Go, `use go`), `lib/xo/std` and
`lib/xo/third_party` (the tracked files), `lib/xo/zoneinfo.zip` (Go's
zone database, copied from `go env GOROOT`, so std/time needs no Go;
decision 0157 next step 1, 2026-10-12), README.md, CHANGELOG.md,
LICENSE and THIRD_PARTY_NOTICES (the last two since 2026-10-22: 0.25.2's
archives shipped without them); and
`dist/SHA256SUMS` (no runtime packs since decision 0234, 2026-10-16). It
needs Go, the host's clang, and a clean tree
(`XO_RELEASE_ALLOW_DIRTY=1` to try it); 992 s and 9.3 GB peak RSS (release run 7) on
the development Mac with other agents running (2026-10-12). Nothing
publishes them; releases are on hold (owner).
Since 2026-10-22 it first builds ThreadSanitizer's runtime for native
`--race` (decision 0273) into the checkout's git ignored `lib/xo/race`,
which each archive copies: darwin-arm64 on the host and linux-arm64 in
Docker (`scripts/race-runtime.sh WORK <checkout> linux-arm64`), so it
refuses to start when Docker is not running. `XO_RELEASE_RACE_WORK`
(default `$TMPDIR/xo-release-race`) holds the LLVM sources and build;
`XO_RELEASE_RACE=reuse` keeps a runtime already in `lib/xo/race` (and
then needs no Docker for linux-arm64). `XO_RELEASE_DRY_RUN=1` runs the
checks (host, clean tree, Docker, the Windows tools) and prints each
step's command without building, writing, or deleting anything. Last
verified: 2026-10-22 (the dry run, the refusal with no Docker, reuse).

For the first public binary release (0.25.1, 2026-10-22) release.sh also:

- **Stamps** every xo1 with the provenance of decision 0264 (`xo version
  -v` and `--json`: commit, source key, code generation key; the flags
  come from `go run ./scripts/xobuild --stamp-args=SEED`), and refuses the
  private checkout (it has scripts/lead) unless `XO_RELEASE_PRIVATE=1`: a
  release is built in the published tree, `../xo-publish` after
  `publish.sh prepare` (decision 0249), so the stamped commit is a public
  one and the archives carry the scrubbed std and docs.
- **Builds darwin/amd64** with the amd64 backend (`--backend=amd64
  --target=darwin/amd64`), with its race dylib (`race-runtime.sh ...
  darwin-amd64`).
- **Builds windows/amd64 natively** by default: `--backend=amd64
  --targets=windows/amd64`, linked by Xo's PE linker with no C runtime,
  so no Microsoft SDK is needed (owner, 2026-10-22). The race runtime
  comes from `race-runtime.sh ... windows-amd64` (MinGW in Docker).
  `XO_RELEASE_WINDOWS=llvm` keeps the LLVM and lld-link recipe, and `0`
  skips Windows. `scripts/ci/release-smoke.sh ARCHIVE SHA256 [COMMIT]`
  runs the archive on its host (CI suite=script on xo-windows, the
  archive from ci/release/ of the seed bucket).
- **Signs** the darwin bin/xo, bin/xo-gobind, and race dylibs with the
  Developer ID named by `XO_SIGN_ID` (the identity's SHA-1 from `security
  find-identity -v -p codesigning`; never written into a tracked file)
  and notarizes them in one submission (scripts/sign-macos.sh) with the
  keychain profile `XO_NOTARY_PROFILE` (made once by the owner, `xcrun
  notarytool store-credentials NAME`) or the App Store Connect key
  variables (code-signing.md). Neither a bare Mach-O nor a tar.gz can carry
  a stapled ticket; Gatekeeper looks it up online. `XO_RELEASE_UNSIGNED=1`
  and `XO_RELEASE_NO_NOTARIZE=1` are for trials.
- **Signs** the windows bin/xo.exe and bin/xo-gobind.exe with Azure
  Artifact Signing (scripts/sign-windows.sh; since 2026-10-22, after
  0.25.2): the `AZURE_*` variables or the file `XO_AZURE_ENV`, with
  `JSIGN_JAR` and `JAVA`, checked with a token request before the build.
  `XO_RELEASE_WIN_UNSIGNED=1` skips it (code-signing.md "Releases" under
  Windows). Last verified: 2026-10-22 (dry run; no signed build yet).

release-verify.sh then also checks that the stamped commit is the checkout's
HEAD and, on darwin, the Developer ID signature and notarization
(`XO_VERIFY_UNSIGNED=1` for a trial archive). Given a windows archive on
any host, it checks only the layout and the Authenticode signatures
(scripts/verify-windows-sig.sh); release-smoke.sh on Windows runs it and
asks Get-AuthenticodeSignature. It uses the host's native
backend, amd64 on an Intel Mac or linux/amd64. Go is missing is checked
with `--backend=go`, since native is the default. The owner publishes with
`scripts/release-publish.sh [--draft] TAG DIST NOTES.md` from the
published tree, after `publish.sh push` has pushed the tag. It runs only
from a terminal and checks that the tag on xo-lang/xo is the tree's HEAD,
that SHA256SUMS matches the archives, and that the host archive's stamped
commit is right. Then it asks for the word `release` and runs `gh release
create`. The order:

1. a private commit bumps the version;
2. tag `vX.Y.Z`;
3. `publish.sh prepare`;
4. in `../xo-publish`, `scripts/release.sh`, then release-verify.sh per
   host;
5. the owner runs `publish.sh push`;
6. the owner runs `release-publish.sh`;
7. the Xo against Go comparison on the released darwin/arm64 binary
   (owner, 2026-10-22, every release): unpack the archive, then
   `HEAVY_CLASS=bulk heavy.sh vsgo scripts/release-vs-go.sh
   UNPACKED/bin/xo OUT` in the private checkout, and commit
   OUT/report.md as `bench/results/release-vs-go-vX.Y.Z.md`. It builds
   bench/perf's Go-twin programs (llvm, collections, conc) with xo and
   Go: compile cold, a second build, warm (the third unchanged build:
   xo writes a directory's parts pack on the second build and reads it
   from the third, 2026-10-23) and after a one function edit, peak build
   memory, binary size, and each workload's run under `xo bench --ab`.
   After the cold row it builds std's variants (`xo dev stdcache`, conc
   0 and 1; 2026-10-29: a fresh xo has none and its first builds are
   whole-program while they build in the background, so the 0.33.0 and
   pre-cut reports' edit rows were whole-program builds), and the table's
   last column says which path the edit build took (separate, with the
   std records, or whole), from `XO_CACHE_STATS=1`'s lines in the log
   (wall, CPU, peak RSS; Go is A, Xo is B). Last verified: 2026-10-22
   (smoke run, 3 workloads).
8. the website's comparison pages (owner, 2026-10-23): refresh the
   tables of `content/docs/xo-and-go.md` from step 7's
   `release-vs-go-vX.Y.Z.md` (and any newer `*-vs-go-*` results),
   with the new file and line links, the Go version measured against
   and the newest Go release, and its "Last verified" date; the
   website push needs the owner's OK.
9. clean up (lead, 2026-10-23: the disk sat at the 50 GB trim line,
   about 6 GB of it finished release directories): delete the seed
   cut's WORKDIR once seed-upload.sh has run (step 4 of "Cutting a
   seed"), and the previous release's published tree and race work
   directory (`../xo-rel-X.Y.Z`, `../xo-rel-X.Y.Z.race-work`) once this
   release is published; keep this release's until the next one. Delete
   by literal absolute path; a git worktree (WORKDIR/tree) goes with
   `git worktree remove --force` first.
`scripts/release-verify.sh ARCHIVE [OUTDIR]` checks an archive on its
host from a checkout of the same commit: unpacked clean, with no Go on
PATH `xo version`, `rtpack --status`, `check`, and a hello program on
arm64 (the default linker: macho on a darwin host, cc with `use c`
sources or --as=cc; cc from 6faa44cb to the decision 0182 addendum of
2026-10-16), arm64
`--ld=cc`, arm64 `--ld=macho`, and llvm ("UNSUPPORTED" on
linux/amd64), the time_zones golden on arm64 against its want.txt
(zone data from the archive's lib/xo/zoneinfo.zip), the missing Go reported by the default backend; with Go a
hello on the Go backend, a `use go` program, and cmd/xo's whole suite
with `XO_CLI` = the archive's bin/xo. On EC2: `scripts/remote-full.sh --arch arm64|amd64 --put
dist/<archive> --cmd 'bash scripts/release-verify.sh ~/<archive>'`.
Results: bench/results/retire-go-2026-10-12.md "Switch point B: the
flip". Last verified: 2026-10-12.

## Commands

Since decision 0215 (2026-10-13) there is no Go `xo`: read every `go run
./cmd/xo X` and `.tools/xo X` below as `xo X` with the checkout's
`bin/xo` (`go run ./scripts/xobuild`, internal/xotool) or a release's;
the hidden comparison dumps of the Go implementation (lexdump, astdump,
typedump, ssadump, asmdump, llvmdump, gendump), switch point A's
internal/selfdriver, and the tests of the deleted Go packages are gone
(the lines naming them are history). The TestSelfhost* tests compare
each self hosted component with the recording (internal/selfhost
gogoldens_test.go). The packages that keep tests, all through the xo
binary: cmd/xo (the CLI), internal/llvm, internal/gen, internal/arm64
(the backends and runtimes, test only), internal/selfhost,
internal/repotest, internal/xotool, internal/gobind, internal/gotool,
internal/progen, internal/testtmp, std/rt, third_party, editors/vscode,
scripts/disttar, internal/benchharness. Last verified: 2026-10-13.

Run from the repo root.

```
go build ./...                           # build the compiler packages and std
go test -timeout 30m ./...                # all tests (lexer, parser, checker, spec examples, corpus, CLI, golden programs, race, cross compile) with internal/selfhost in its default merge tier (about 2.5 min, cold or warm; see the internal/selfhost tier lines below). With XO_SELFHOST_TIER=full (nightly, release tagging) use -timeout 90m: the full tier alone took 3,474 s (58 min) with the native binaries uncached and another full suite on the machine (2026-10-11)
go vet ./...
gofmt -l internal cmd std                # must print nothing
.tools/bin/staticcheck ./...             # must print nothing
go run ./cmd/xo parse [--json] [--ast] [--sig] <files or dirs...>
go run ./cmd/xo check [--json] [--backend=go] [--support] [--fix] [--wip] [--dir=DIR] <dirs...>   # by the native backends' rules by default, as xo build builds (owner, 2026-10-23: `use c` passes); --backend=go: the Go backend's rules; --support (old name --no-gc): also the native support check XO1201, which lowers the program (about 2 s more on compiler/xo; through the front end cache later); --fix: apply machine fixes first (decision 0096); --wip: XO0110 and XO0209 as warnings while editing (decision 0293)
go run ./cmd/xo rtpack [--target=os/arch] [--cc="cmd args"] [--ar="cmd args"] [-o lib/xo/rtpack] | --status | --key [--target=os/arch]   # hidden: build the prebuilt runtime of the arm64 backend into <dir>/<os>-<arch> (default the host's cc; linux/arm64 from a Mac: --cc="zig cc -target aarch64-linux-gnu.2.31" --ar="zig ar"); --status: the pack used for the host or why none; --key: the key a pack must have (decision 0157; last verified: 2026-10-11)
xo1 rtpack [same flags]   # (deleted 2026-10-16, decision 0234) the same command in Xo (compiler/xo rtpack.xo, decision 0178 switch point B): the same pack files byte for byte for the same compiler and sources (cmd/xo TestRTPackXoSame with XO_CLI), so scripts/release.sh can run the self hosted xo (last verified: 2026-10-12)
scripts/release.sh [version]             # release archives into dist/, bin/xo = xo1 (decisions 0157, 0178), stamped and signed, from the published tree (see "Versions and releases"; never publishes)
scripts/release-publish.sh [--draft] TAG DIST NOTES.md   # the owner's GitHub Release of TAG's archives (terminal only, typed confirmation)
scripts/release-verify.sh ARCHIVE [OUTDIR]   # check a release archive on its host: no-Go native checks, Go backend and use go, cmd/xo's suite with XO_CLI (see "Versions and releases"; last verified 2026-10-12)
bench/perf/rtpack/first-build.sh <xo> [runs]   # first arm64 build, cold caches, local runtime compile vs pack (bench/results/prebuilt-runtime-2026-10-11.md)
go run ./cmd/xo build [--targets=all|os/arch,...] [-o out] [--json] [--no-mono] [--no-arenas] [--backend=go|llvm|arm64] [--no-gc] [--hybrid] [--ci] [--release] [--debug-contracts|--dbg|--error-args|--profile-support|--trace-support=on|off] [--debug-info=on|off|split] [--opt=O0..O3] [dir...]   # --ci: fail before building on an unformatted file or a dbg left in code (core 10.5, 11.4; 2026-10-09); --release: requires(debug)/ensures(debug) compiled out, XO1308 on workspace overrides (core 8, decision 0099; --ci does not imply it); --error-args=false: error frames without parameter values (decision 0092)
go run ./cmd/xo run [--seed=N] [--record=trace.xotrace] [--holes] [--no-mono] [--no-arenas] [--backend=go|llvm|arm64] [--no-gc] [dir] [-- args...]
go run ./cmd/xo build --stamp VERSION=1.4.0 --stamp COMMIT=$(git rev-parse --short HEAD) .   # set `const VERSION: Str` (and COMMIT) of the main module for this build (decision 0165, XO1501 when no such plain Str constant; also xo run; recorded in the build information for xo replay; last verified 2026-10-11)
go run ./cmd/xo build --no-gc .          # no garbage collector: the native backend (arm64 or amd64; --backend picks); XO1201 for what the native backends cannot build, before code generation (decision 0083; also check, run, test)
go test ./internal/build -run TestNoGCCheckAgrees    # the no GC check agrees with LLVM and arm64 code generation on every golden, bench, selfhost, and example program (seconds; also -short)
xo test --no-gc [-v] [--backend=llvm|arm64] examples/notes-service   # a native test binary: tests and property cases in batches, many per process (XO_TEST_BATCH, decision 0235), a run the process does not survive runs again alone; same report as the Go backend
xo test [--no-gc] --timeout=30s DIR   # each test (each property case, each shrinking replay) may take 30 s of real time; past it the test fails with "the test did not finish within 30s of real time" and, natively, its process is killed (default 60s, 0 for no limit; decision 0235). Last verified: 2026-10-15
XO_TEST_JOBS=1 xo test --no-gc DIR   # the batched test processes one at a time; by default `xo test --no-gc` runs chunks of whole tests (a property test's cases together, one module directory each) in parallel processes, one per physical core, the report the same (decision 0235). The Go tests (internal/xotool TestJobsEnv, cmd/xo's environ) give every xo they run XO_TEST_JOBS=2 unless set, so `xo test` does not take every core alongside the other tests and heavy runs. Last verified: 2026-10-15
XO_TEST_BATCH=0 xo test --no-gc examples/notes-service   # one process per test and per property case (XO_TEST=i, XO_TEST_SEED=s), as before decision 0235. Last verified: 2026-10-15
printf '5/1/r=3,1/b=4,2\n' | XO_TEST=5 XO_TEST_SERVE=1 ./prog.test   # a serving test binary (shrinking's replays, decision 0235): one run per stdin line, index/seed/r=XO_PROP_REPLAY/b=XO_PROP_BEST; a case not smaller than the best prints XO-PROP-NOT-SMALLER and skips its body
XO_TEST=0 XO_TEST_BATCH=0/1,3/1,5/1/0-99 ./prog.test   # a batch by hand: test 0 and 3 with seed 1, cases 0 to 99 of property test 5; each run between XO-TEST-BEGIN and XO-TEST-END lines on stdout, the test world reset between runs
XO_TEST=i XO_TEST_SEED=1 XO_PROP_SEED=1000003 ./prog.test   # one case of native property test i (decision 0120): XO_PROP_SEED is std/rt's seed*1000003+case, XO_PROP_REPLAY=c1,c2,... replays choices; prints XO-PROP-CHOICES and XO-PROP-INPUT lines before the body runs
XO_TEST=i XO_TEST_REAL=fs.read,env ./prog.test         # a `uses` test of a native test binary built with --real: those effects real (decision 0120)
XO_TEST=3 XO_TEST_SEED=1 <build dir>/arm64/bin/<module>.test   # run one test of a native test binary by hand: exit 0 passes, XO-TEST-FAIL on stdout and exit 1 fails, XO-TEST-LOG lines are its log records
go test ./cmd/xo -run 'TestNoGC(Tests|TestLeaks|NotesServer)'   # native test reports vs the Go backend (cmd/xo/testdata/nogc, examples/notes-service), no live allocation at exit, and notes-service served natively on a socket with SIGTERM shutdown (about 40 s)
go run ./cmd/xo replay [--dir=<module dir>] <trace.xotrace | crash.xocrash>   # rebuild and rerun offline from the trace (core 11.3)
go run ./cmd/xo test [--seed=N] [--explore=N] [--run=text] [--race] [--real] [--cases=N] [--json] [-v] [--holes] [--no-mono] [--no-arenas] [--backend=go|llvm|arm64] [--no-gc] [dir...]
go run ./cmd/xo dump [--timeout=5s] <pid>                        # task tree of a running Xo program (core 11.4)
go run ./cmd/xo bind [--dir=<project dir>] [--json] <go import paths...>   # bind Go packages for `use go` (Go interop)
go run ./cmd/xo fmt [--check] [--json] [files or dirs...]        # canonical layout (core 10.5)
go run ./cmd/xo vet [--json] [dirs...]                           # advisory checks, never errors
go run ./cmd/xo fix [--dry-run] [--json] [--from=draft-0.N] [--edition=preview] [dirs...]  # apply check fixes, migrate
go run ./cmd/xo query <kind> [--json|--text] [--dir=<dir>] <arg> # type, def, callers, effects, holes, api, imports, deps (core 10.3)
go run ./cmd/xo edit [--dry-run] [--dir=<dir>] <op> <args...>   # rename, add-field, change-sig (core 10.4); JSON summary, exit 1 when rejected
go test ./internal/edit -run TestGolden -update          # rewrite internal/edit/testdata/*.golden
go run ./cmd/xo doc [--json] [--all] [--dir=<dir>] [module]      # Markdown docs from doc comments
go run ./cmd/xo explain [--json] <code>                          # long form explanation of a diagnostic code
go run ./cmd/xo mcp                                              # MCP server on stdio for agents (see "Agent interfaces")
go run ./cmd/xo lsp                                              # LSP server on stdio for editors (see editors.md)
go run ./cmd/xo serve [--socket=path] [stop|status]              # resident checker; check and query use it when it answers
go test ./internal/format -run TestGolden -update        # rewrite internal/format/testdata/*.golden
go test ./internal/gen -run TestGoldenNoMono -golden-update   # rewrite want.txt of the golden programs
go test ./internal/gen -run TestGoldenRelease [-golden-update]   # testdata/release/*: want.debug.txt, want.release.txt, want.release_noargs.txt (xo build --release [--error-args=false], decisions 0099, 0092); -golden-update rewrites only the files that exist
scripts/go-version-check.sh [-n] [step...]          # the Go backend's version sensitive tests with GOTOOLCHAIN=goX.Y (see "Go versions"); no download without --download. Last verified: 2026-10-12
go test ./internal/gen -run TestGeneratedGoVets       # gofmt -l and go vet on the generated Go of every golden program (not rt); skipped with -short (about 1 minute). Last verified: 2026-10-10, clean
go test ./internal/gen -run TestGeneratedGoSnapshot [-golden-update]   # the generated Go of 11 golden programs vs testdata/gosnap/*.go.golden; review the diff of a lowering change, then update
go test ./internal/gen -run TestGeneratedGoVetsLarge  # the same on the examples, the LLVM golden programs, the perf benchmarks, and the self hosted compiler (about 3 minutes; not bench/paper, which no longer checks). Last verified: 2026-10-10, clean
go test ./internal/gen -run 'TestArith(Checks|Property)'   # progen.Arith: random programs of Int arithmetic (edge operands, shifts to 63 and beyond, division by -1) reaching Float, Str, List[Int], a struct, an enum with payloads, and closures; TestArithChecks (also -short) checks 300; TestArithProperty builds 25 with the Go, LLVM, and arm64 backends, compares exit status, all of stdout and all of stderr (normStderr drops only frames, crash bundle paths, and the deadlock's location and blocked list, which decision 0083 lets differ), and vets the Go; XO_ARITH_PROPS=N (N seeds), XO_ARITH_PROP_FIRST=F (from F, default 1), XO_ARITH_PROP_SEED=S (only S)
python3 -I scripts/reduce-diff.py [--jobs=4] [--timeout=30] [--no-ddmin] [--crash] XO go arm64 IN.xo [OUT.xo]   # shrink a program while two backends (go, arm64, llvm; natives with --no-gc) run it differently: both must build, keep IN's exit statuses, and differ in stdout or normalized stderr (the Python twin of normStderr). ddmin over lines, then first improvement over lines, { } blocks, parentheses, integers; temporary XO_GOCACHE removed after (the first build compiles the runtime). --crash keeps instead the programs the first backend builds and the second fails to build with the same compiler fault (its kind and location without operands, crash_signature): seed 1502's 151 lines to 17 in 4 minutes (2026-10-18). Each backend is named with --backend, `go` too (the default is native since decision 0248; before 2026-10-18 the reducer's `go` was arm64). About 40 s for the 74 line p197 (was 10 minutes with the old line by line reducer). Tests: TestReduceDiffFake (also -short, fake builds), TestReduceDiffReal (real builds, a `bad` backend that adds a line when the program has an xor). Last verified: 2026-10-11
XO_PASSES=-cse,-ranges xo build --backend=arm64 .   # turn off optional optimization passes of the native backends by name (compiler/arm64 passes.xo: inline, devirtualize, specialize_args (compiler/ssa specialize.xo, decision 0368: a callee cloned for a call site's closure of nothing or compared Str literal, before the reference counting; every backend but wasm), chain_pads (compiler/ssa chainpads.xo, in the reference counting; with passes off its cache is skipped too), view_lens (compiler/arm64 viewlen.xo, after inlining: a Str view or block parameter used only for its length becomes the length, its count and release gone), slp_f64 (compiler/arm64 slp.xo, decision 0323: pairs of Float operations as two lane NEON instructions; with XO_DUMP_FN its decisions go to xo-ir-slp-NAME.txt), uniq_var_params (compiler/ssa uniqvar.xo, decision 0322: `var` List parameters every call passes unique skip their copy-on-write check), fold_consts, cancel_rc_pairs, hoist_fields, fold_tuples, cse, lsr, ranges, inplace_elems, the MIR passes of isel.xo function, arg_slots in regalloc.xo, hollow (compiler/arm64 hollow.xo, decision 0371: after emit, darwin and linux on arm64 and amd64 without `use c`, the functions of the assembly nothing reaches become one `brk #0x484f` on arm64 or one `ud2` on amd64)); map_core (compiler/ssa map.xo, mapcore.xo, decision 0372 stage A: a Map's lists key side generated once per key type; off, each Map type has its own, as before; in the fe, fn and part cache keys; stage B, the log's key side per key type, has no switch); the build stays correct; with passes off the per-function cache is skipped. `XO_PASSES=+NAME` turns on a pass a target leaves off; none is off for a target today (hollow on darwin/amd64 was, from 2026-10-25 until 2026-10-26, when its binaries had run on an Intel Mac), so `+hollow` changes nothing but still turns the caches off as any entry does; naming a pass both ways is an error. `tail_dup` (compiler/arm64 taildup.xo, 2026-10-27, the 0.34 pmap item): a loop latch of at most 4 plain instructions or moves and a jump to the header is copied into each of its 2 to 4 same-loop predecessors that end in a plain jump, before reuse_flags and place_latches on arm64 and x86, so the arms of a branching body take one branch an iteration where they took two (the serial collatz kernel 0.956 of its wall, pmap 0.917, 8% fewer instructions in both; sieve, nbody, json and bytessearch the same instructions; xo's __text +0.14%). Last verified: 2026-10-27
XO_RC_CHECK=1 xo build|run|test ...   # decision 0322's checking mode: each copy-on-write check the analysis proved unneeded stays as a check that prints "xo: internal: a var List parameter proved unique (decision 0322) holds a shared list (count N)" and exits 70 when its list is shared (the immortal empty list passes); arm64, amd64, and LLVM (wasm copies as before). Run goldens under it after changing compiler/ssa uniqvar.xo. Last verified: 2026-10-21
xo dev bisect-pass [--backend=arm64|amd64] [--ref=go|off] DIR   # miscompile triage: builds DIR's program with a prefix of the optional passes on (binary search) and names the first whose run diverges from the Go backend's (or the all-off build's); a failed build counts as diverging; says whether turning that pass off alone is enough (seed 1502: ranges, 16 s). Last verified: 2026-10-18
XO_PROP_SAVE=DIR XO_PROP_TRIAGE=off XO_PROP_TRIAGE_MIN=10   # a failing TestCompileProperty, TestArithProperty or TestDeferFaultProperty program is kept in $XO_PROP_SAVE (default $TMPDIR/xo-prop-failures)/TEST-SEED/, and its failure message carries bisect-pass's culprit (arm64, amd64) and reduce-diff.py's reduced program (within XO_PROP_TRIAGE_MIN minutes); XO_PROP_TRIAGE=off keeps only the program; a failed native build prints xo's output (internal/gen triage_test.go). Last verified: 2026-10-18
scripts/progen-amd64.sh WORK compile|arith|defer FIRST COUNT [BATCH]   # progen against Xo's own amd64 backend from the Mac: internal/gen TestProgenAmd64Dump writes the programs and their Go backend linux/amd64 builds, a linux/amd64 container (docker-linux.sh, nice 10) builds each with --backend=amd64 and runs both (scripts/progen-amd64-run.sh), TestProgenAmd64Compare compares them (normStderr) and keeps diverging programs; batches of 250 with the $XO_DISK_FLOOR GB (40) floor, up to 30 minutes' wait; WORK/summary.txt. Last verified: 2026-10-18
scripts/progen-batches.sh OUT compile|arith|defer FIRST COUNT [BATCH]   # a long progen run on this host in batches (default 250), each its own heavy run when XO_HEAVY='scripts/lead/heavy.sh NAME' prefixes it, with the disk floor; compile batches compare LLVM too (XO_COMPILE_PROP_LLVM=1); OUT/summary.txt. Last verified: 2026-10-18
scripts/gate.sh WORKTREE short arm64 llvm cmd gen alloff fp   # an agent's gate as named steps (run under heavy.sh): rebuilds the worktree's xo, one line per step, GATE-DONE; alloff runs arm64's goldens with every optional pass off; logs in $XO_GATE_LOGS. Last verified: 2026-10-18
go test ./internal/gen -run TestArithOracle   # Int arithmetic (checked_add/sub/mul, wrapping_mul, / and %) against an exact 30-bit-limb reference in Xo (internal/gen/testdata/oracle), as `xo test` property tests on go, llvm and arm64; XO_ORACLE_CASES (default 1000); runnable directly with `xo test --backend=B testdata/oracle`. Last verified: 2026-10-18
go test ./internal/llvm -run TestGolden -golden-update   # rewrite want.txt of the LLVM backend golden programs (from the Go backend's output); a golden directory may hold args.txt (arguments), env.txt (extra NAME=VALUE variables), and stdin.txt (standard input), passed by internal/gen, internal/llvm, and internal/arm64 alike (env.txt, and stdin.txt for the native runners, since 2026-10-11)
go test ./internal/ssa -update                       # rewrite the SSA goldens (testdata/golden/*/want.ssa and want.rc.ssa, testdata/bench/*.ssa and *.rc.ssa)
XO_SSA_DUMP=/abs/dir [XO_SSA_RC=1] go test ./internal/ssa -run TestDump -v   # print a program's SSA (after InsertRC), or every builder and verifier error
go run ./cmd/xo ssadump [--stage=build,unwind,landings,cleanup,rc,verify,mutants|all] [--each] [--test] [--bench=N] <dirs...>   # hidden: the SSA module after each stage named (default build: the builder's output before any pass), checked as a native build checks (std/native included), fault positions relative to the working directory; --each builds every dir alone (=== ssa <dir>), a rejected run prints == rejected, builder errors follow == errors; --bench=N times ssa.BuildRaw (the builder alone, one build) N times; --test builds the test binary of `xo test --no-gc` (ssa.Options.Tests); verify prints the verifier's verdict after rc and mutants its verdicts with one retain or release removed (ssa.VerifyMutants), neither part of all (decision 0150; last verified: 2026-10-11)
go run ./cmd/xo run compiler/ssa -- [--std=DIR] [--wd=DIR] [--each] [--all] [--test] [--stages] [--bench=N] <dirs...>   # the self hosted (Xo) SSA builder: the build stage of xo ssadump for the functions it completes, then (--stages, when it skipped nothing) the stages unwind, landings, cleanup, rc, verify, and mutants, then == skipped and <function>: <why>; --all prints the skipped functions too (debugging); checks with compiler/check in native mode; reads XO_MODCACHE, XO_OFFLINE, XO_WORK, XO_CBIND_DIR like the checker; a `use c` header whose binding is not in the cache (a Go build or `xo bind --c` writes it) makes the run unsupported (decision 0152; last verified: 2026-10-11)
go run ./cmd/xo asmdump [--os=darwin|linux] [--no-inline] [--each] [--test] [--bench=N] <dirs...>   # hidden: the arm64 assembly of internal/arm64 (build.Arm64AsmDump: the SSA as xo ssadump builds it, verified, InsertRC, generated, again without inlining when the inlined module does not verify); --each prints === asm <dir> before each run; --bench=N times arm64.Generate alone (decision 0158; last verified: 2026-10-11)
XO_PHASE_TIMES=1 xo asmdump <dir>  /  xo1 asm|build|test --phase-times <dir>   # per phase wall clock on stderr as `phase NAME MS ms`, MS with two decimals since 2026-10-29 (`phase link.resolve 3.42 ms`; whole milliseconds made a phase's laps fall short of it) (Go: check, ssa, verify, insert_rc, verify, inline, emit, print; xo1: check, ssa, tz, prune_unwind, add_landings, cleanup, insert_rc, inline, emit, print or write, assemble (compiler/macho, or the parallel `cc -c` of a split assembly), then cc or link); bench/results/xo1-perf-2026-10-12.md, bench/results/fast-builds-2026-10-13.md (last verified: 2026-10-14)
xo build --phase-times .   # a darwin link through std's store (separate compilation, decision 0388) also prints its steps: link.read_store (the store file read), link.inputs (the program's objects read, the link's options), link.store_prepare (the walk, the live atoms cut), link.resolve, link.layout, link.synthesize, link.write (the image), link.write_exe (the file written and signed); bench/results numbers come from these (~/xo-lead link plan). Last verified: 2026-10-29
XO_LLVM_SPLIT=N XO_LLVM_JOBS=J xo build --backend=llvm ...   # by default (2026-10-22) IR over 100 MB is split into ~50 MB modules with thin LTO, and the clang jobs are min(cores, (75% of physical memory - xo's heap) / 3.5 GB), at least 1; smaller IR stays one module with full LTO. --phase-times prints the split (bench/results/llvm-split-default-2026-10-22.md). Last verified: 2026-10-22
XO_LLVM_OPT=O0|O1|O2|O3 xo build --backend=llvm ...  # decision 0220: clang's level for the program's IR (any other value is an error since 2026-10-19; default O2, `xo test` O0 (decision 0236); --release O2; `--opt=LEVEL` (decision 0354) wins over all of these, in any build); `go run ./scripts/xobuild` and scripts/bootstrap-xo.sh build xo at O0 unless it is set (x86 self build 1:23 on c7i.8xlarge at O0 against 18:51 at O2); xobuild records the level in bin/xo.opt and keys its cache by it; scripts/seed-pack.sh refuses an xo whose bin/xo.opt is not O2 (seeds and releases are O2; release.sh sets XO_LLVM_OPT=O2 for the Windows build); XO_LLVM_JOBS defaults to the physical cores, at most memory/4 GB (last verified: 2026-10-14)
# decision 0338 (2026-10-22, replacing 0333's module wide `-unroll-threshold=300`): the IR marks the loops with Float arithmetic `!llvm.loop` `llvm.loop.unroll.full` (compiler/llvm func.xo loop_md), and every build above -O0 passes `-mllvm -pragma-unroll-threshold=300 -mllvm -pragma-unroll-full-max-iterations=32` (the marked loops' size and trip count limits; LLVM's defaults are 16,384 and 1,000,000) to compiling and to an LTO link: `-Wl,-mllvm,...` to darwin's ld64, `-Wl,-plugin-opt=...` on Linux (neither clang forwards -mllvm to the linker's LTO; GNU ld refuses the plugin option without LTO, so XO_LLVM_LTO=none gets the compile flag only); other loops keep the level's threshold; no knob (compiler/xo split.xo unroll_flags) (last verified: 2026-10-22)
xo build --backend=amd64 [--targets=linux/amd64] -o prog .   # Xo's own x86_64 backend (decision 0254, stage 2a): the arm64 MIR retargeted to x86 (compiler/arm64 x86.xo, x86emit.xo, x86rt.xo); linux/amd64 only; on a linux/amd64 host a no-libc program is assembled by compiler/macho (assemble_elf_amd64) and linked by elflink --static with no C toolchain, a libc one by cc; from another host `xo asm --backend=amd64 --targets=linux/amd64 .` prints the assembly. 182/182 goldens in Docker amd64 with XO_CC=/bin/false (last verified: 2026-10-17)
# Debugging a linux/amd64 binary from the Mac (gdb and ptrace fail under Docker's amd64 emulation): in an arm64 container, `XO_DOCKER_ROOT=1 scripts/docker-linux.sh arm64 W 'apt-get install -y qemu-user gdb-multiarch; qemu-x86_64 -g 12345 /work/prog & gdb-multiarch -batch -ex "set architecture i386:x86-64" -ex "target remote :12345" -ex continue -ex bt /work/prog'`; addresses change from run to run (pin a watchpoint with `eval "watch *(long*)0x%lx", expr`; software watchpoints step only the selected thread); XO_LEAKCHECK=1 on amd64 against a linux/arm64 cross build of the same program narrows a reference count bug (gotchas/amd64-retarget-semantic-gaps.md; last verified: 2026-10-17)
XO_LSE=0 ./prog    # a linux/arm64 program linked by elflink (no libgcc) runs its outline atomics as load and store exclusive loops; by default they use ARMv8.1's cas, swp and ldadd when AT_HWCAP has ATOMICS (xo_lse, set in cabi_nolibc.xo nl_start; bench/results/arm64-lse-outline-2026-10-17.md; last verified: 2026-10-17)
XO_EMIT_JOBS=N xo build --backend=arm64 ...          # decision 0220 stage 1 (the physical cores probed once a process into XO_HOST_CORES, which sets them too: sysfs on Linux, sysctl hw.physicalcpu on macOS, NUMBER_OF_PROCESSORS on Windows; last verified: 2026-10-30): the tasks of insert_rc, the inlined module's verification, the arm64 emitter's parts, and the parts' assembly (default the physical cores, at most 16 and memory/3 GB); a module of 256 or more functions is generated in parts (cut by contents since 2026-10-14: after a function whose name hashes to 0 modulo 128 once a part has 64 functions, at most 512; 42 parts for xo), each part one file `prog-<os>.<i>.s` assembled apart (parallel `cc -c`, or compiler/macho tasks with --as=macho) and linked once; the output does not depend on N. The self build of xo: 59.00 s on the Mac (was 337.28 s), 87.84 s on r8g.8xlarge at 8 jobs and 49.83 s at 16 (was 496.82 s), peak RSS 19.86 GB at 16 (last verified: 2026-10-14)
XO_BACKEND_CACHE=on|verify|off xo build --backend=arm64 ...   # decision 0220 addendum (default on), for a program of 256 or more functions (split into parts), in <user cache>/xo/backend (XO_BACKEND_CACHE_DIR overrides). Stage A (windows/amd64's COFF parts too since 2026-10-27, keyed on their text with the real paths, on parallel tasks; HTTP server body edit 2783 to 1179 ms): each part's object in obj/ by the SHA-256 of the assembler's identity (cc's --version and file, the flags, the source path; compiler/macho: the running xo's executable) and the part's text, so an unchanged part is not assembled again. Since 2026-10-22 the text names no directory: the compile directory and std/'s root are DW_FORM_strp into fixed 1024 byte slots of the part's .debug_str holding placeholders, filled as each object leaves compiler/macho's assembler (and in any text written for cc or a file), so another worktree at the same commit reuses every object (gotchas/cached-object-names-worktree.md). Stage B: each function's canonical assembly in fn/ by the SHA-256 of the xo executable, the target, its SSA after inlining and reference counting, the types it reaches, and the signatures of the functions it names, so an unchanged function is not generated again (not for --release builds; arm64 and, since 2026-10-26, amd64 on darwin and linux, windows/amd64 since 2026-10-27, its entries carrying the C functions' thunks: gotchas/fn-cache-hit-skips-emit-side-tables.md; bench/results/amd64-edit-cache-2026-10-26.md). Stage C (every native backend): each function's reference counting in rc/ by the SHA-256 of the xo executable, the target, the function before insert_rc, the types it reaches, and the borrowed flags of itself and the functions it names (not for --release builds; `cache rc HITS/TOTAL reused`). verify computes every function's reference counting and assembly and fails the build (`XO_BACKEND_CACHE=verify: N functions differ from their cache entries`) when a cached text differs; off runs the plain generation; all three give the same binary. --phase-times adds `phase fn_keys`, `cache functions HITS/TOTAL reused`, and `cache objects HITS/TOTAL reused`. Stage D (2026-10-18): the inlined module's verification skips a function whose verify key verified clean before (one file of keys per compiler, ver/<identity>; every function with off, verify, or --release), the entries are read on parallel tasks, and --phase-times splits `insert_rc.plan|read|apply` and `inline.keys|read|apply|verify` (since 2026-10-21 `inline.verify` comes after `dce`: the verification uses the function keys fn_cache_load then reuses, one computation for both); part 2a caches each function's inlining in inl/ under a key chained from the reference counting keys (`cache inline HITS/TOTAL reused`); both kinds of entry are binary (rc cache v8, float constants included), and the emitter's key hashes the reference counting or inlining key the function came from instead of its text (fn cache v6); a one-constant edit of xo 14.1 s to 9.5 s (10.7 s of CPU) (internal/arm64 TestBackendCacheEditIdentity: a cached build, darwin and linux/arm64, is the file a cache-off build writes; last verified: 2026-10-18). Part 2d (2026-10-19): each build target (the working directory, the command line without its flags and their values: -o, --stamp, every value flag; the target; 2026-10-22: before, xobuild's `--stamp XO_SOURCE_KEY=...` made each tree key a new target that wrote xo's whole parts pack and never read it) also keeps its rc, inl, and fn entries in segment files, pack/<slot>/<kind>/s-<hex>, read with one open each, the loose files staying the shared store; a target's first build writes none, its second one segment per kind (about 390 MB for xo), later builds a small segment of what was new, compacted past four segments; the cache of xo with its pack about 950 MB in all. Part 2b (2026-10-19): each function's inline summary in sum/ (and the pack) under its reference counting key; with arm64 and amd64 a hit with a summary is decoded only if the inliner computes or copies it (`cache rc N/M undecoded` with --phase-times). Part 2e (2026-10-19): each part of a split build in the target's pack only (kind part; xo: 34 parts, 328 MB), under the keys of its functions, its place in the split, the module settings, and with debug information every type's key; read instead of generated, its functions' fn entries then not read (`cache parts N/M reused`); parts from the target's third build; 2026-10-22: a module of 16 parts or more (xo) packs them on the target's second build, a test build of fewer on its first (gotchas/first-build-parts-pack-unread.md); 2026-10-27: also one whose parts are under 128 MiB in all (the HTTP server's 35), so its first edit after a cold build reuses all but the edited part (it regenerated all 35 since the run cache serves the unchanged builds), and a build that used none of its pack's entries (after a rebase) rewrites the pack instead of appending. Decision 0283 stage B (2026-10-21): inline cache entries (v7) carry body facts and a hit no computed function inlines stays undecoded until a reader needs its body (XO_LAZY_INLINE=off turns it off). Part 2f (2026-10-19): borrow inference in parallel rounds; --phase-times splits insert_rc.plan into insert_rc.borrows, insert_rc.types, insert_rc.keys (last verified: 2026-10-19). Decision 0283 stage A (2026-10-21): the borrow inference infers only the functions whose body hash is new and their callers, the rest keeping the build target's record of the last build's flags (pack <slot>/borrow/list; `cache borrows KEPT/TOTAL kept` with --phase-times; full with verify; XO_BORROW_CHECK=1 runs both and fails the build when they differ). Decision 0220 addendum stage 2, step 1 (2026-10-19): the SSA builder's function bodies in fe/ (and the target's pack) under the SHA-256 of the builder's flags, the interface closure of the body's module (its files and its imports', transitively, with function bodies cut out), and the body's file, position and declaration text; a hit replays the body's SSA and the declares it made when the builder's state fingerprint matches (`cache fe HITS/TOTAL bodies` with --phase-times; generic instances, bodies making closures or instances, and contract-wrapped bodies are built as before); XO_FE_CACHE=off turns it off, XO_FE_STATS=1 prints the misses by reason, verify builds every body and fails (`N function bodies' SSA differs from their front end cache entries`) on a difference. Step 2 (2026-10-19, opt in with XO_CHK_CACHE=1: a skipped module's bodies miss the whole-program passes, gotchas/module-check-cache-skips-whole-program-passes.md; XO_CHK_CACHE=auto (2026-10-20, opt in until verified) skips only a module whose entry says every body of it had a front end cache entry, and when one is built from source anyway, or the checked bodies read another module's inferred effects, warns `the module check cache skipped N module(s) ...`, drops the build and checks them again): `xo build` and `xo run` (native, no overlay) keep each program module's check in chk/ under the SHA-256 of xo's identity, the checker's flags, the module's path, its interface closure and its files' sources; a hit checks the module's bodies only when the SSA builder needs them and replays its diagnostics (`cache chk HITS/TOTAL modules`); verify checks every module and fails (`N modules' check differs from their cache entries`); the build target's parse list (pack/<slot>/parse/list) has the files of the last check parsed on tasks before the next one; XO_FE_CACHE=off or XO_BACKEND_CACHE=off turns both off; since 2026-10-22 chk entries are stored only when a build can read them (XO_CHK_CACHE=1 or auto, or verify; else `cache chk off (XO_CHK_CACHE)`: every build wrote about 21 entries none read). Hollowing part by part (2026-10-26, darwin and linux split builds, arm64 and amd64; replaces the day's tail entries): each part's hollow facts (arm64 hollow.xo HFacts: its functions and data objects, what each names, its roots) in hfacts/ under the SHA-256 of its text, the reach over every part's facts, and each part's object and undefined-symbol facts (undef.xo UFacts) in hobj/ and hufacts/ under its text's hash and the names of its functions the reach left dead (with the text they fix the hollowed text); so an edit scans, hollows and assembles only the parts whose text or dead set changed, `phase hollow` covers it and `phase undef` is the combine (0 ms). Edit shapes it covers: every part unchanged (an unused function added, the release comparison's edit) assembles nothing; a body changed in a reachable function rescans and reassembles the part holding it (and any part whose dead set moved); each edit's binary is byte for byte a cold build's in a fresh cache (same output name: an ad hoc signature names the file). ELF parts (linux) also key their facts on every part's extra labels (the labels an ELF part's scan reads from all the parts, each part's list in hextra/ under its text's hash), and carry no undefined-symbol facts; on linux/amd64 hollow went from 196 to 86 ms (HTTP server) and 51 to 30 ms (llvm-fib) per edit (2026-10-26). Builds without the cache hollow the whole text as before; a tool that reads the texts (--ld=cc, XO_KEEP_ASM=1) gets them hollowed from the same facts; verify does not use it. Each store appends `UNIX store KIND BYTES id ID` to DIR/store-log (KIND fe, fn, inl, rc, sum, obj, hobj, hfacts, hextra, chk, or pack/KIND; ID the first 12 hex digits of the SHA-256 of the cache identity, the code generation key), so the prune's churn splits into new keys against repeats (2026-10-22). Entries are marked used and pruned by compiler/prune (decision 0223): hourly or once an eighth of the cap was stored since the last prune (DIR/prune-stored), unused for a week (XO_CACHE_MAX_AGE), then least recently used past 16 GiB (8 until 2026-10-21; XO_BACKEND_CACHE_MAX bytes or XO_CACHE_MAX_GB); xo itself takes 491 MB per version. Self build on the Mac after a one-module edit: 30.60 s and 131.33 s of CPU with the cache off, 11.28 to 12.10 s and 20.42 to 25.30 s of CPU with it (bench/results/fast-builds-2026-10-13.md "Back-end cache stage A", "stage B", "stage C"; last verified: 2026-10-14)
xo build ...   # the C toolchain's identity (compiler/xo tools.xo tool_id: `cc --version` and the tool's size and modified time from stat(1)), which the run cache's key and the object and link keys hash, is memoized across builds in <backend cache>/toolid/ under the hash of the tool's path, the arguments, and tool_stamp: the size and modified time (ms) of the tool on PATH, each symbolic link it follows, and on macOS for a /usr/bin shim the developer directory xcrun uses (DEVELOPER_DIR, else /var/db/xcode_select_link, else the Command Line Tools) with its cc and clang, TOOLCHAINS and SDKROOT; the two spawns took 9.2 and 2.3 ms of every build, hit or miss (2026-10-26). A tool replaced by one of the same size and modified time keeps the old id (self_id's limit); XO_BACKEND_CACHE=off avoids the memo (last verified: 2026-10-26). The same way since 2026-10-27, the build information's hash of std/rt's runtime files (binfo.xo rt_sums, 2.1 MB, about 6 ms of every native build) is memoized in <backend cache>/rtsum/ under each file's path, size and modified time; and a link's binary cache key (macho_bin_key, with XO_BIN_CACHE on: `xo test`, or set) names each part's object by its back-end cache key and the debug slots' paths instead of hashing its bytes (about 5 ms of an llvm-fib edit; internal/arm64 TestLinkKeyFollowsObjects)
XO_CHAIN_KEYS=0 xo build ...       # decision 0382 (default on since 2026-10-27; =0 turns it off): the landings and the cleanup per function through the back-end cache (cln/ and the target's pack) for a program of 256 or more functions, not --release, without XO_PASSES; a key is the SHA-256 of xo's identity, the build's flags, the body (the bytes of the front end cache entry it replayed or was recorded as with what the replay set: lines, name, closure, constant and spawn site numbers; else the SSA in full) and, for each function it names, its may_fault bit, whether it catches, its bits before prune_unwind and the constant fold_known_calls folds it to; a hit skips add_landings and cleanup for that function; the reference counting's and the incremental borrow inference's keys then hash the cleanup key in place of the body, except where specialize_args changed it (Module.body_ids); --phase-times adds `phase cln.keys|cln.read|cln.store` and `cache cln HITS/TOTAL reused`; verify computes every function and fails (`N functions' landings and cleanup differ`) on a difference, and on an entry whose stored full key (the body by key_func) is not this body's. Last verified: 2026-10-27
XO_PDB_OBJ=0 xo build --targets=windows/amd64 .   # (default on since 2026-10-28, 0.34 criterion 6) the PDB is made from each object's part (macho obj_pdb: its functions' line rows, inline sites and finished S_LOCAL/S_DEFRANGE records against the object's own sections, types as DWARF offsets), cached under the back-end cache's pdb/ by the object's cache key with the debug slots' paths, merged at link by pelink's layout (pre_procs) and finished by pe_pdb_facts; the same bytes as the image path, which 0 selects. HTTP server edit: pe.pdb 64 ms to pe.pdb_parts 11 + pe.pdb 20, wall 0.95. XO_PDB_OBJ_CHECK=1 compares each object's functions with the image's, 2 and 3 build the PDB from merged facts or parts beside the real one and compare (internal/arm64 TestWindowsPDBObjCheck). Last verified: 2026-10-28
xo run .   # windows/amd64 (2026-10-30): xo run and xo test write the .exe stamped with its PDB's GUID (macho pe_pdb_stamped: the GUID and time stamp come from the seed or the image, never from the PDB) and make the PDB while the program runs (ld.xo PdbLater, finish_pdbs in a background task), waiting for it before they exit and printing "xo run: the PDB: ..." on a failure; the PDB is written whole or not at all (write_atomic). xo build still makes it in the link. Phases: pe.pdb_stamp in place of pe.pdb_parts, pe.pdb and pe.pdb_write. Native windows/amd64 host, small program: link.pe_link 29.1 ms (build) against 5.2 (run), exe and PDB byte identical to xo build's; Xo's own tracebacks read the image's DWARF (llrt dwline.xo pe_sects), not the PDB. XO_PDB_OBJ=0 and XO_PDB_OBJ_CHECK keep the PDB in the link. Last verified: 2026-10-30
XO_CHK_CACHE=off xo check .         # decision 0351 addendum (2026-10-23): `xo check` skips a program module unchanged since its last check and replays its diagnostics, warnings included (entries under the backend cache's chk/, flags `xo check`), and checks the native runtime's bodies lazily unless a named directory is in std; off (or XO_FE_CACHE=off, XO_BACKEND_CACHE=off) checks everything; compiler/xo 1.8 s to 0.67 s (bench/results/check-cache-2026-10-23.md). Last verified: 2026-10-23
XO_LAZY_PARSE=off xo build|run .   # decision 0351 stage 2 (b), on by default since 2026-10-23 (owner; off parses every body): std and runtime files' function bodies are parsed only when the checker needs one (an unforced body is a sentinel that faults if read; check/stmt.xo names XO_LAZY_PARSE=off); output identical, a small program's warm build 0.589x peak memory (bench/results/lazy-parse-2026-10-23.md). Last verified: 2026-10-23
XO_COLD_PACK=off xo build .        # decision 0351 addendum (2026-10-23, default on since 2026-10-24, owner): a build's 64 or more new cache entries of a kind (fn too) go to the shared pack as one segment (bcache.xo store_entries) instead of a file each, and a target's pack does not copy what the shared pack served; off writes them loose (it was opt in from the evening of 2026-10-23 for main's 10 minute builds on the Windows CI runner's 31 GB cache, cause not found); a cold build 638 files instead of 6,227, 0.687 of the wall time on the Mac (bench/results/cold-pack-2026-10-23.md). Last verified: 2026-10-23
XO_CACHE_STATS=1 xo build .        # (2026-10-23) a summary of the build's file operations in xo's caches on standard error when it ends: `cache stats: AREA OP: N calls, B bytes` per area (entry, pack, shared, prune) and kind (read hit or miss, write, rename, remove, stat, list, mkdir), most calls first, and a total; counts and bytes, not times (a type that is an Fs cannot read the clock: --phase-times gives the phases' times); a prune detached on macOS and Linux runs in its own process and is not counted; scripts/ci/windows-race.sh sets it with --phase-times. Last verified: 2026-10-23
XO_CACHE_STATS=keys xo build .     # (2026-10-24) the same summary plus one `cache key: AREA OP BYTES PATH` line per cache entry read (hit), written or renamed into place, in order, to diff the files a build stores against the files later builds read (churn of the back-end cache: 17 to 50 GiB stored an hour). Last verified: 2026-10-24
XO_CACHE_COMPRESS=off xo build .   # decision 0365 (compression Accepted, on by default since 2026-10-26): back-end cache entries and pack segments are LZ4 compressed (`XOZ` header; loose `z<KEY>`; segments `p-<ID>`, decision 0418 since 2026-10-29: one block per entry, written and read one entry at a time, and the pack directory's name carries the format version, so an older xo never reads them); off reads and writes plain files. Warm llvm-fib build +9 ms, the cache 65% smaller (decision 0365 "Built"). Last verified: 2026-10-29
XO_SHARED_PACK=off xo build .      # decision 0351 (Accepted, on by default since 2026-10-23; off turns it off): a shared pack per kind and target (packs/shared/<id>/<kind>) read after the build target's own pack and before loose files; a build that read 64 or more entries loose appends them, compaction keeps what the compacting build used, so it settles on std and runtime entries. --ab 2026-10-23: a new program's first build wall 0.831, a warm build in a new directory 0.857, the same directory unchanged; XO_FE_STATS=1 prints `cache loose KIND N of M (pack P, shared S, missing X)` per kind. Since 2026-10-23 (a745a66fe) a formed fe pack also keeps the keys its build found nowhere (pack <slot>/fe/neg) and skips their loose reads. Last verified: 2026-10-23
XO_CHK_CACHE=off xo build .        # the module check cache (decision 0220 addendum) is on by default as auto (owner, 2026-10-23): xo build and run with one target skip the check of a module whose every body the front end cache replays, with a fallback when one is built from source; =1 skips every unchanged module; off turns it off. It was the default for a night on 2026-10-22 and reverted when TestModuleCheckAuto's xml_derive failed; the closure owner fix (26bc1f141) and the thunk replay fix (972c91f3d) came before this default. --ab 2026-10-23 (~/xo-lead notes chk-cache-ab.md): compiler/xo warm builds 0.885 wall, peak RSS 0.914; hello and the HTTP server within noise. scripts/gate.sh runs TestModuleCheckAuto* for compiler/check and compiler/ssa changes (XO_GATE_CHKCACHE=0 skips that). Last verified: 2026-10-23
XO_STD_EXPORT=0|1|verify xo build .   # decision 0388 stage 1a-ii (2026-10-27; on by default since 2026-10-28 for a build or run that compiles separately, XO_STD_EXPORT=0 turns it off): xo build and run (native, the backend cache on) keep std's checked declarations as records; stage 2 (2026-10-28): the entry's header stamps std's files and directories (size and time, the content within 2 s of recording), the record's closure makes std's modules without a read, and a separately compiled build parses only the std files holding a generic object, the other trees on demand (XO_CACHE_STATS says "cache stdx record loaded (N modules)"; XO_STD_TREES_TEST=1 forces the rebuild fallback); XO_STD_EXPORT=verify always takes the full path and compares (compiler/check/export.xo: the std tree's objects, types, effect sets and effect variables, one block of the checker's tables, with no syntax tree) in the backend cache's stdx/, one entry per xo, std and checker flags, its fingerprint the std files' contents; a build whose fingerprint matches installs them, renumbered for its tables and with its declarations' and defaults' trees joined from its parses, instead of declaring and resolving std (`cache stdx used|declared` with --phase-times); verify declares std as usual and fails the build (`XO_STD_EXPORT=verify: std's records differ from its declarations: ...`) when the entry differs. No gain alone (--ab, one xo off and on, n=10: llvm-fib's edit 231.3 and 230.7 ms mean, its check 29.9 and 30.7 ms; the HTTP server's 528.1 and 530.5): the check still waits for std's parses; it is stage 2 and 3's base (std not parsed or lowered in an edit). Last verified: 2026-10-27
xo prof FILE.xoraw --layout=xo.layout   # decision 0403 stage 1 (2026-10-29): the CPU profile reduced to a layout file (fn NAME WEIGHT, line NAME LINE WEIGHT, edge CALLER CALLEE WEIGHT, the target it was taken on); take the profile with XO_PROF=cpu XO_PROF_HZ=10000 on a run long enough for thousands of samples
xo build --layout=FILE|off .   # decision 0403 stage 1: the functions ordered by the layout file (the module root's xo.layout by default when there): the sampled ones first by weight, a callee after its caller; XO_CACHE_STATS=1 prints "layout: N of M sampled functions matched"; a layout of another target is ignored with one line; the file's hash is among the build info's sources
XO_SEPCOMP=1 XO_SEPCOMP_FACTS=F XO_SEPCOMP_STORE=S xo build .   # decision 0388 stage 3, opt in (2026-10-28; the default is step d): std's functions are externs answered by std's facts (F: a stdf file, or a directory with conc0.stdf and conc1.stdf, the build taking its concurrency's), so the program's SSA, passes and emit cover its own functions and the generic instances it uses; std is linked from its store S (`xo dev stdcache --conc=0|1` prints the store and facts paths, built once per xo, target and concurrency; XO_SEPCOMP_OBJS=DIR links std's .o files instead). darwin/arm64 native. The program's parts are store mode, no hollowing, its entry file leaves std's runtime to std's object, the zone table always linked. A fib edit 229.0 to 111.5 ms median against the 0.33.0 release (n=20, ssa 35 to 15, insert_rc 23 to 2, inline 25 to 3; check 30 to 31). Tests: TestSepcompLinks (a program built so equals the whole program's run, across an edit), TestStdFactsVerify. Last verified: 2026-10-28
scripts/docker-sepcomp-sample.sh LINUXXO OUT [arm64|amd64]   # decision 0388: the goldens of scripts/docker-sepcomp-sample.txt (26 std-heavy) built by default (separately compiled) and with XO_SEPCOMP=0 in Docker, exit status and output compared; LINUXXO/bin/xo a linux xo cross built through heavy.sh, the script itself outside heavy.sh (docker-up.sh). A change to the store links, std's stores or the linux default runs it. Last verified: 2026-10-28
xo build .   # separate compilation by default (decision 0388 stage 3 step (d), on since 2026-10-28; XO_SEPCOMP_DEFAULT=0 or XO_SEPCOMP=0 turns it off): a native build or run for the host on darwin or linux, arm64 or amd64, or windows/amd64 (not --release, --race, a cc link or the passes changed, nor a flag or variable that changes std's code: --seed, --explore, the memory, preemption, profiling and tracing settings, STD_CODE_ENV; the backend cache on; a program that uses C is built whole) builds separately, checking the program against std's records (XO_STD_EXPORT, on with it since 2026-10-28) and with std's variants from the std cache (std_variant_peek: a release's prebuilt ones, else the cache's); a miss builds the whole program and starts the variant's build in the background (nice, one at a time, lock std/.bg.lock; XO_SEPCOMP_BG=0 turns it off, heavy.sh and the Go tests set it), so no build is slower than a whole program's. XO_SEPCOMP=1 or 0 wins. The driver sets XO_SEPCOMP, XO_SEPCOMP_FACTS(_CONC) and XO_SEPCOMP_STORE(_CONC) for the build (sepenv.xo); the link takes the store of the program's concurrency (sepcomp_store, Ctx.sep_store). Test: internal/arm64 TestSepcompDefault. Last verified: 2026-10-28
XO_PREP_CACHE=off xo build .   # a separately compiled darwin link's prepare cache (compiler/macho prepcache.xo, 2026-10-29): what store_prepare derives from std's store and the std names the program takes (the live atoms, the DWARF cut's kept runs, each object's symbols renumbered) is kept in <backend cache>/prep/<sha256 of xo's id, the store's content-named path and length, macho.prep_key> and read by the next link with the same key (an edit of a function body), marked used and pruned with the cache (store-log kind `prep`); a hit links the same bytes as a miss (off, miss and hit equal on fib and HTTP); only for std's own content-named stores; off decides afresh. A/B 2026-10-29: HTTP link 0.873 (prepare 11.50 to 8.37 ms), fib 0.814. The windows/amd64 link has it too (compiler/macho coffstore.xo coff_store_objs_cached, key coff_prep_key: the entry, whether DWARF is cut and the sorted names the program takes; a hit keeps the live sections, the units whose DWARF goes and the symbol renumbering): off, miss and hit equal, exe and PDB, on a cross built program (2026-10-29), its laps pe.read_store, pe.inputs, pe.prep_key and pe.store; the laps swing with the machine's load (the 38 MB store's read 3.4 to 24 ms over runs that day), so a speed claim needs `xo bench --ab`. A link measurement or identity check sets XO_WHOLE_BUILD=off: the run cache lives outside XO_BACKEND_CACHE_DIR. Last verified: 2026-10-29
xo build .   # windows/amd64 (decision 0409 on PE, 2026-10-30): a separately compiled link keeps std's half in <the prepare cache's directory>/stdhalf (compiler/macho pehalf.xo; std's objects as linked, std's .text, .rdata and .data bytes with only the relocations that can change, the store's maps for the PDB; about 9.7 MB for the HTTP server), and the next link with the same prepare key links the program against it without reading std's store, byte for byte the full link's (exe and PDB); it falls back to the full link when std's sections would move. XO_STD_HALF=off turns it off (and the program's own sections); XO_STD_STORE_REPEAT=N relinks N times and reports "half equal" with the relocations kept and dropped. Laps pe.store std's half, pe.half_save. HTTP edit cross link on the Mac 57.0 to 36.0 ms, small 26.9 to 15.8 (one run each); `xo bench --ab` of the warm HTTP cross build on the linux/arm64 offload host, XO_STD_HALF=off against on, n=30: wall 0.929 (0.914 to 0.941, 125.7 to 116.8 ms), peak RSS 0.869. Last verified: 2026-10-30
xo build .   # windows/amd64, std's half's PDB part (decision 0409 on PE, 2026-10-30): a link from std's half also keeps the PDB's std part beside it, <stdhalf file>.pdb (compiler/xo ld.xo pdb_pre; xopdbpre1, the half's id, then std's functions, files and types as pre_procs_store leaves them after std's store parts, enc_pre_procs): each half gets a random id when it is captured (PeHalf.id, xopehalf2), and the part is used only beside the half of its id, else made again from std.store.pdb. An edit's PDB then reads neither std.store.pdb nor std's parts: HTTP edit cross link on the Mac pe.pdb_parts 13 to 4.6 ms, pe_link 42.5 to 34.9 (one run each), exe and PDB byte for byte the full link's. With it, <stdhalf file>.pdbw keeps the PDB writer's std part (macho.PdbStd: std's placed functions, the files and inlined functions they name first, their module records and line blocks; std's CodeView types come first in every link through the store, cv_types_split), and std's CodeView type run, so an edit's PDB makes only the program's and reads std's functions not at all: HTTP edit pe.pdb_parts 13 to 1.4 ms, pe.pdb 14.5 to 2.3, pe_link 34.9 to 18.4 (Mac, one run each); `xo bench --ab` of the warm HTTP cross build on the linux/arm64 host, std's half off against on, 0.814 (n=30). Last verified: 2026-10-30
XO_STD_HALF=off xo build .   # a separately compiled darwin/arm64 edit link reuses std's half (decision 0409, 2026-10-29). The program's sections are output sections of their own after std's (_ptext, _pconst, ...; __stubs, __got, __unwind_info, __eh_frame and zerofill after them), so std's __TEXT sections keep their addresses. A full link writes std's half: its objects, with the fixed sections' relocated bytes and only the relocations that can change; its globals; and the fixed sections' addresses. It goes to <backend cache>/stdhalf/<sha256 of the prepare cache's key and the header's options> (9.9 MB for the HTTP server, 3.5 MB for fib), marked used and pruned with the cache (store-log kind `stdhalf`). The next link with the same key links the program against it, byte for byte the full link's. It falls back to the full link when the program defines a name std defines, has a common symbol or thread locals of its own, or std's fixed sections would move (a new section kind grows the header). A link with --layout (decision 0403) never splits; off: one output section per kind and no half. A/B 2026-10-29: HTTP edit link 23.68 to 9.89 ms (0.418), fib 9.40 to 3.31 ms (0.352). Last verified: 2026-10-29
XO_STD_STORE_REPEAT=10 xo build .   # linux (decision 0420, 2026-10-29): a separately compiled ELF edit link runs in steps (macho elfstore.xo elf_store_prepare, then ElfStoreLink.resolve, layout and write; laps link.store_prepare/resolve/layout/write with --phase-times). XO_STD_STORE_REPEAT=N relinks N times, timed, each checked against the first. Its prepare decisions (the live atoms, the dropped DWARF) go to the prepare cache as darwin's (<backend cache>/prep, keyed by macho.elf_prep_key; XO_PREP_CACHE=off). std's store objects are whole for the link's second walk (the prepare walk's live set, darwin's semantics), so an edit build's store link is not promised byte-equal to the parts link; XO_STD_STORE_CHECK=1's XO_STD_STORE flow still finds them equal (fib, HTTP, xo). Native linux arm64, n=30: HTTP edit link 48.61 to 38.31 ms, fib 23.40 to 20.20; xo 0.004% larger. Last verified: 2026-10-29
xo build .   # linux (decision 0420, std's half, 2026-10-30): a separately compiled ELF link puts the program's read only data and code in .rodata.xoprog and .text.xoprog after std's (five PT_LOADs), and keeps std's half in <backend cache>/stdhalf (kind stdhalf, pruned with the cache; about 32 MB for the HTTP server, std's DWARF most of it): std's .rodata, .text and DWARF output bytes with only the relocations that can change, std's globals, COMDAT groups and undefined names. The next link with the same prepare key links the program against it (compiler/macho elfhalf.xo), byte for byte the full link's; it falls back to the full link when the program defines (not weak) a name std defines, has a common symbol, or std's .rodata, .text or phnum would move. XO_STD_HALF=off turns it off (the three segment layout); XO_STD_STORE_REPEAT=N reports "half equal" and the relocations kept and dropped. HTTP edit link 39.07 to 13.84 ms on linux arm64 (A/B against the parent's xo, n=30), 60.1 to 39.3 on linux amd64. Last verified: 2026-10-30
lldb ./prog   # a separately compiled darwin build (decision 0401): no __DWARF; Xo's tracebacks read segment __XODW (__xo_line, __xo_info, ...), and lldb and dsymutil read the full DWARF through the binary's debug map (N_OSO, time 0) from std's part objects (<store>.parts) and the program's kept objects (<backend cache>/dbgobj/<sha256>.o), pruned with the cache (unused a week); a binary copied elsewhere after a prune keeps its tracebacks, lldb then has no std names or variables; `dsymutil ./prog` keeps them for good (last verified: 2026-10-29)
XO_LEAKCHECK=1 <native program>                       # when main returns, `xo leakcheck: N live of M allocations` on stderr; a program that ends with os.proc.exit (or a main returning an error) prints `xo leakcheck: skipped (os.proc.exit)` instead (gotchas/leakcheck-skipped-at-exit.md; last verified: 2026-10-14)
XO_HEAPSTATS=1 <native program>                       # at exit, the heap by size class on stderr (`xo heapstats: class C size S free blocks N bytes B`, then the 4 MiB chunks carved); with XO_LEAKCHECK=1 the allocation count; XO_HEAP_MID_MAX=<bytes> (1024 up to 256 KiB) sends larger blocks to malloc (an experiment knob; decision 0199, bench/results/xo1-speed-2026-10-13.md; last verified: 2026-10-13)
xo build|run|test --backend=llvm --race <dir>          # ThreadSanitizer (decision 0241 addendum, decision 0273 mode 2): the program's functions and std's marked sanitize_thread (not the runtime's, std/native/llrt but atomic.xo), built with clang -fsanitize=thread; each task a ThreadSanitizer context of its own, the C library's heap, the runtime's synchronizations as release and acquire; a race between two tasks prints a ThreadSanitizer report (TSAN_OPTIONS=halt_on_error=1 to stop at the first); XO_TSAN_RUNTIME=1 when building instruments the runtime too (mode 1, the runtime's own race tests; the binary must define xo_tsan_runtime); wasm refuses it, the Go backend uses Go's race detector. Last verified: 2026-10-20
xo build|run --backend=arm64 --race <dir>  # decision 0273 parts 2 and 3: the arm64 backend's ThreadSanitizer calls (compiler/arm64 race.xo; the runtime as mode 2's), linked on darwin by Xo's linker with ThreadSanitizer's runtime dylib, from $XO_RACE_RUNTIME, else lib/xo/race/darwin-arm64/libclang_rt.tsan_osx_dynamic.dylib next to std (clang's: `clang -print-file-name=lib/darwin/libclang_rt.tsan_osx_dynamic.dylib`); on linux/arm64 (2026-10-21) by Xo's ELF linker with the static archive lib/xo/race/linux-arm64/libclang_rt.tsan-aarch64.a linked whole, the C library dynamic (`scripts/race-runtime.sh` builds it in Docker: `scripts/docker-linux.sh arm64 WORK -- bash /work/race-runtime.sh /work/w /work/out` with the script copied into WORK, about 10 minutes; -mno-outline-atomics and -fno-threadsafe-statics, so the archive needs no libgcc atomics and no C++ runtime); refused with --ld=cc. --backend=amd64 --race (2026-10-21, decision 0273's amd64 addendum): the same pass before x86_retarget, with lib/xo/race/darwin-amd64/libclang_rt.tsan_osx_dynamic.dylib (`scripts/race-runtime.sh WORK OUT darwin-amd64`, cross built on the arm64 Mac) or lib/xo/race/linux-amd64/libclang_rt.tsan-x86_64.a (`... linux-amd64`, in Docker linux/amd64); windows/amd64 (2026-10-22, decision 0273's Windows addendum; untested with threads until a Windows CI run): ThreadSanitizer's Go flavor, lib/xo/race/windows-amd64/race_windows_amd64.syso (`... windows-amd64`, MinGW-w64 in Docker linux/arm64 plus scripts/race-windows.cpp, about 5 minutes), linked by Xo's PE linker at a fixed base, the calls going to std/native/llrt cabi_race.xo, which adds the task's ThreadState, reports naming each frame's function, file and line from the program's own symbol table and DWARF (racesym.xo; 2026-10-22) (single threaded under Wine: `XO_THREADS=1 wine prog.exe`); amd64 race programs do not run on the arm64 Mac (gotchas/tsan-amd64-not-on-arm64-mac.md). Reports carry the call stack on every native target (2026-10-22, decision 0273's stacks addendum: each instrumented function's entry and exit through std/native/llrt cabi_racefn.xo). Last verified: 2026-10-21 (race_sync's table 40 of 40 and tsan_race in Docker linux/arm64 and on the Intel Mac, darwin/amd64)
XO_PROF=cpu <native program>                            # the sampling CPU profile (decision 0238): SIGPROF at XO_PROF_HZ (100) a second of CPU time, frames by the fault unwinder's tables; at exit a raw file at XO_PROF_OUT (xo-prof.xoraw; %p is the pid; decision 0245) with the samples and their locations named; `xo prof FILE.xoraw` writes pprof (gzip) as FILE.cpu.pprof and a text top 30 by flat and by cumulative as FILE.cpu.txt beside it (checked 2026-10-29); `xo run --profile=cpu` and `xo test --profile=cpu` (xo-cpu-<pid>.pprof per test process) do both steps; darwin and linux, arm64 and llvm, and windows/amd64 (llvm, a sampler thread); `go tool pprof -top xo-cpu.pprof` reads it (last verified: 2026-10-17)
XO_PROF=heap <native program>                           # the heap profile (decision 0238): allocations sampled every XO_PROF_HEAP_RATE bytes on average (512 KiB; 1 = all) into the same raw file; `xo prof` writes alloc and inuse objects and bytes as Go (xo-heap.pprof) and a .txt top 30 by inuse_space and alloc_space; XO_PROF=cpu,heap takes both
XO_PROF=heap XO_PROF_HEAP_AT=inline XO_PROF_HEAP_OUT=at.xoraw xo build ...   # the heap in use when the first debug.mem_label text ending with XO_PROF_HEAP_AT is set (xo labels its phases "building, past phase NAME"): what a phase leaves alive, which the exit profile cannot show; `xo prof at.xoraw` converts it (decision 0352 step D; last verified: 2026-10-23)
xo1 asm --phase-times compiler/xo                      # xo1 compiling itself to assembly: 163.83 s, 2,035.4 billion instructions, 9.66 GB footprint on the Mac (was 588.86 s, 26.1 GB); byte identical to `cd selfhost && xo asmdump xo` (bench/results/xo1-speed-2026-10-13.md; last verified: 2026-10-13)
go run ./cmd/xo run compiler/arm64 -- [--std=DIR] [--wd=DIR] [--os=darwin|linux] [--no-inline] [--each] [--test] [--bench=N] <dirs...>   # the self hosted (Xo) arm64 emitter: checks with compiler/check, builds with compiler/ssa and its passes, prints the assembly of xo asmdump, then == skipped and <function>: <why> for the functions of a later stage of decision 0158; == unsupported for a run the Xo SSA builder does not build completely; --bench=N times the emitter alone (last verified: 2026-10-11)
go run ./cmd/xo run compiler/xo -- build|asm [--std=DIR] [--wd=DIR] [--cc=CC] [-o OUT] [--asm=PATH] <dir>   # xo1, the self hosted compiler driver of decision 0158's switch-over experiment: compiler/check, compiler/ssa and its passes, compiler/arm64, then cc (or $XO_CC, --cc) assembles and links with std/llrt (copied next to OUT as OUT.rt), trying the macOS SDKs internal/build tries ($XO_SYSROOT, the remembered one, none, xcrun's, Xcode's); fault positions relative to the program's line root (the nearest xo.mod); refuses programs that need time zone data (std/time), TLS (std/http), or `use c` sources; not the released xo (last verified: 2026-10-11)
go run ./cmd/xo gendump [--test] [--no-mono] [--no-ranges] [--no-arenas] [--release] [--no-error-args] [--each] <dirs...>   # hidden: the files gen.Generate makes for the Go backend (build.GoGenDump), == file <path> before each; --fmt reads the Xo generator's output and puts its Go files through gen.FormatFile (only gofmt after == prepped) (decision 0167; last verified: 2026-10-11)
go run ./cmd/xo run compiler/gen -- [--std=DIR] [--wd=DIR] [--each] [--test] [--no-mono] [--no-ranges] [--no-arenas] [--release] [--no-error-args] [--prep] <dirs...>   # the self hosted (Xo) Go code generator: checks with compiler/check, prints the text of each package before internal/gen's last step (with --prep after it but gofmt), then == skipped (last verified: 2026-10-11)
go run ./cmd/xo build -o xo1 compiler/xo && ./xo1 build --backend=go [-o OUT] [--go-tree=DIR] <dir>   # the Xo driver with the Go backend: compiler/gen, format_prep, gofmt of go env GOROOT, runtime copied from std/rt, zone data from go env GOROOT's zoneinfo.zip (or XO_ZONEINFO), go build -trimpath (decision 0167; last verified: 2026-10-11)
go test ./internal/selfhost -run 'TestSelfhostGen'    # Xo Go code generator vs internal/gen, declaration by declaration (temporaries renumbered) and whole files after gofmt, on every repository run as a program and, with test blocks, as a test binary; every fourth run also with --no-mono --no-ranges --release --no-error-args; the merge tier without the five largest runs, the full tier every run and variant (go, arm64, llvm builds), every 25th with -short (merge tier with mutations about 30 s); TestSelfhostGenMutations: 60 code mutations (merge), 300 (full); XO_SELFHOST_BACKEND, XO_SELFHOST_GEN_FUZZ=N, XO_SELFHOST_SEED, XO_SELFHOST_KEEP=<dir> (writes gen-<backend>.txt); decision 0167 (last verified: 2026-10-11)
go run ./cmd/xo run compiler/xo -- build|test|asm|keys [--std=DIR] [--wd=DIR] [--cc=CC] [--tzdata=embedded|system] [--target=darwin/arm64|linux/arm64] [-o OUT] [--asm=PATH] [--run=SUB --seed=N --cases=N --json -v] <dir>   # xo1, the self hosted compiler driver (decisions 0158, 0174): compiler/check, compiler/ssa and its passes, compiler/arm64, then cc (or $XO_CC, --cc) links as internal/build does: the runtime from a prebuilt pack ($XO_RTPACK, else lib/xo/rtpack beside the binary named by $_) or the runtime object cache (<user cache>/xo/rtobj, $XO_RTOBJ_CACHE), the zone data of std/time (xo_tzdata.c; $XO_ZONEINFO or go env GOROOT), `use c` adapters from cbind's cache, the macOS SDKs internal/build tries; every key equal to the Go driver's, so both share the caches and link byte identical binaries; --target=linux/arm64 writes the assembly only (as internal/build); `test` builds and runs a native test binary as `xo test --no-gc` (no --real, no contract fuzz listing, no time limit); `keys` prints the pack keys; `--backend=llvm` (build, test, asm) lowers with compiler/llvm and compiles the IR (OUT.ll, or --asm) with clang ($XO_CLANG, else clang, or --cc) as buildLLVM does with one module: clang's target attributes, -O2 -flto -Wno-override-module, no XO_LLVM_SPLIT (decision 0178); not the released xo (last verified: 2026-10-12)
XO_CLI=/abs/xo1 go test ./cmd/xo -run TestX     # cmd/xo's tests against another xo binary (decision 0178 stage 2): the tests' run() execs `$XO_CLI <args>` in the current directory and environment instead of the in process runXo (cmd/xo/cli_test.go; the value is split on spaces, so it may carry flags) (last verified: 2026-10-12)
XO_CLI=<a bin/xo> go test ...   # stale check (internal/xotool StaleCLI, 2026-10-21): when the pinned xo records the source key it was built for (bin/xo.key and bin/xo.commit beside it, written by xobuild; or the cache's checkout/<key>/xo), and the checkout's compiler/ and the std it builds in now have another key (a rebase of the worktree since the build), every test fails at once with `xotool: XO_CLI=... is stale` naming both keys and commits; rebuild it (go run ./scripts/xobuild) or set XO_CLI_STALE_OK=1 for a deliberate cross version run. Without a key file beside it, the key the build stamped into the binary is used (`xo version --json` source_key: xobuild's and the checkout cache's builds, and since 2026-10-21 scripts/bootstrap-xo.sh's s1 to s3; a copy or an `xo build -o` one of those keeps it); an xo with neither (the cross built seeds of seed-cut.sh, a release's) is not checked, and the banner says `no key`. Every Go test process that runs xo prints one line first, `xotool: xo PATH (key K, commit C | no key); tree key K2 [(differs)]`, so the xo a run used is in its log (last verified: 2026-10-21)
xo build --backend=arm64 -o xomod compiler/cli/modules/xomod && XO_CLI=$PWD/xomod XOMOD_FALLBACK=/abs/xo go test -short ./cmd/xo -run TestModules   # the module commands in Xo (compiler/cli/modules, decision 0178 stage 4) on their own: xomod runs get, mod, vendor, publish, audit and hands every other command to the Go xo named by $XOMOD_FALLBACK (not XO_GO, which names the go command); xo1 dispatches the same commands to the same package (last verified: 2026-10-12)
xo1 query|edit <args...>  /  xo build -o semantic compiler/cli/semantic && ./semantic query|edit|defhash <args...>   # the semantic tools in Xo (decision 0178 stage 3, compiler/cli/semantic): `xo query` (every kind, hash included) and `xo edit`, with the Go tool's flags, exit codes, and JSON; std from $XO_STD, else lib/xo/std beside the binary named by $_, else the std/ of the repository above the working directory; `semantic defhash DIR=PATH...` prints every definition hash of each module for TestSelfhostDefhash (last verified: 2026-10-12)
go test -run TestSelfhostDefhash ./internal/selfhost/   # compiler/cli/semantic's definition hashes against internal/defhash on every module of bench/, internal/check/testdata, internal/gen/testdata, internal/arm64/testdata, compiler/, std/, examples/ (byte identical JSON; skipped with -short) (last verified: 2026-10-12)
xo1 check|parse|explain|version|init|fmt|fix|vet|doc [args]   # the core commands of `xo` in Xo (compiler/cli/core, decision 0178 stage 2): the flags, exit codes, human and JSON output of cmd/xo; std from --std=DIR before the command, else $XO_STD, else lib/xo/std or std beside the binary named by $_ (Env.program is not native yet); not yet: check --no-gc's XO1201 (the native check only); a `use go` package missing from the binding cache is bound with xo-gobind and every `use c` header with compiler/cli/bind before the check runs again (check_dirs_bound, decision 0178 stage 6) (last verified: 2026-10-12)
xo1 bind [--dir D] [--json] <Go import path>...  /  xo1 bind --c [--link L]... [--uses E] <header>...   # `xo bind` in Xo (compiler/cli/bind, decision 0178 stage 6): --c binds C headers with the port of internal/cbind (declaration file and adapter byte identical); Go packages are bound by the Go helper xo-gobind, found as $XO_GOBIND, beside the program, in <prefix>/bin of a release layout, beside std's parent, on $PATH, else built with `go build ./cmd/xo-gobind` ($XO_GO) from the repository holding std into a temporary file; a prebuilt helper (beside xo, a release's bin) built by an older Go than the installed one is replaced by a build of the source a release ships under lib/xo with the installed Go, once, in `<user cache>/xo/gobind/<source key>-<go version>/` (decision 0393; `XO_GOBIND_LOCAL=1` makes that build whatever the versions; `xo clean -cache` removes it) (last verified: 2026-10-27)
go build -o xo-gobind ./cmd/xo-gobind && ./xo-gobind [--dir D] [--json] <Go import path>...   # the Go binder of `use go` as its own program (gobind.RunCLI: the output, files, and exit status of `xo bind`); the self hosted xo execs it for `xo bind` and for a `use go` package missing from the binding cache; it stays Go (decision 0178) (last verified: 2026-10-12)
xo build --backend=arm64 -o xobind compiler/cli/bind/xobind && XO_BIND_BIN=$PWD/xobind go test -short ./cmd/xo -run TestBindCXoSame   # the C binder in Xo against internal/cbind: 17 cases (the repository's `use c` headers, zlib.h, a header of edge cases, the errors), output and cache files byte identical; xobind hands other commands to $XOBIND_FALLBACK (last verified: 2026-10-12)
XO_CLI=/abs/xo1 XO_STD=$PWD/std go test -short ./cmd/xo -run 'TestCheck|TestParse|TestFmt|...'   # the gate of stage 2: cmd/xo's own tests of those commands against xo1 (last verified: 2026-10-12)
xo build --backend=arm64 -o rel/bin/xo compiler/xo && ln -s $PWD/std rel/lib/xo/std && ln -s $PWD/third_party rel/lib/xo/third_party && XO_CLI=$PWD/rel/bin/xo go test ./cmd/xo   # the switch point B gate (decision 0178): every cmd/xo test against xo1 laid out as a release (bin/xo beside lib/xo/std), no XO_STD: xo1 and every compiler/cli command find std through Env.program (compiler/check locate.xo); the golden scripts pass --backend=arm64 (xo build|run|test default to the native backend since decision 0248, 2026-10-16; last verified: 2026-10-12)
XO_CLI=/abs/xo1 XO_STD=$PWD/std go test ./cmd/xo -run 'TestXo'   # stage 5 (decision 0178): cmd/xo servers_xo_test.go drives the Go mcp, lsp, serve, dump, replay in process and xo1 with the same requests and compares the responses byte for byte; skipped without XO_CLI; TestXo1ServersVersion (compiler/cli/servers XO_VERSION equals cmd/xo's version) always runs; xo1's lsp, MCP tools, and serve find std through XO_STD (last verified: 2026-10-12)
XO_CLI=/abs/xo go test ./cmd/xo -run 'TestCLIHelp|TestCLIPattern|TestCLIRunNoWalk' [-update-help]   # the shipped xo's help (compiler/cli/cmdhelp: xo help, every command's -h/--help on stdout exit 0, unknown flags exit 2 with the usage on stderr, goldens in cmd/xo/testdata/clihelp), its DIR/... pattern (decision 0210; every walk, fmt and parse too, skips testdata and names starting with . or _, 2026-10-23), and the no-walk rule (decision 0345, 2026-10-22: a plain DIR is that directory alone, only DIR/... walks, bounded at 6,000 .xo files or 60,000 entries; xo run runs exactly one directory); without XO_CLI and without -short the test builds xo1 with the Go backend (about 20 s); the differential tests drop the usage after a usage error (dropUsage), the help being xo1's (last verified: 2026-10-13)
go test ./internal/selfhost -run TestSelfhostFormat     # the formatter of compiler/cli/core/format against internal/format on every .xo file of the repository and on copies with changed blanks, byte for byte (go backend, about 10 s) (last verified: 2026-10-12)
go test ./internal/diag -run TestSelfhostExplainData [-update-explain]   # compiler/cli/core/explain_data.xo is generated from explain.go and codes.go; rewrite it after changing a text (last verified: 2026-10-12)
xo dev goldens [--backend=arm64|llvm|go] [--flags='--target=linux/arm64 --ld=macho'] [-j N] [--force|--no-cache] [--no-each] [--json] [--sample=N] [--only=a,b] [--timeout=S] [--build-only] [--cmd=run] [--build-path=P] DIR...   # hidden (decision 0209, compiler/cli/dev): the golden programs under DIR (a program dir has .xo files and want.txt, or ../<parent>want/<name>.txt as internal/arm64/testdata/runwant) built by this xo (or --xo), run with args.txt, env.txt, stdin.txt, XO_LEAKCHECK=1 on native backends, compared byte for byte; native builds go to `xo dev build-each` workers that parse std once each, largest first; passes cached under <user cache>/xo/dev/goldens/pass by the hash of the xo binary, std, flags, host, and program files (--force ignores, --no-cache neither reads nor writes); --cmd=run builds and runs each with one `xo run` in its directory; the ELF goldens (--flags='--target=linux/arm64 --ld=macho') run directly on a linux/arm64 host and need a linux/arm64 runtime pack in XO_RTPACK (TestDevGoldensELF builds one with `xo dev rtpack --target=linux/arm64` into <user cache>/xo/devgoldens/rtpack/<pack key>, 2026-10-14); it replaced scripts/selfhost-goldens.py, selfhost-gogoldens.py, and elf-goldens.py (deleted 2026-10-14 after both agreed on the Mac and on linux/arm64); go test ./internal/devgoldens wraps it (-short: every 25th, XO_DEV_FORCE=1, XO_DEV_JOBS) (last verified: 2026-10-13)
xo dev goldens ...   # since 2026-10-21 every xo it starts gets XO_INLINE_STRICT=1 (an arm64 module whose inlining does not verify fails the build, as in the Go tests) unless the caller sets XO_INLINE_STRICT; the pass cache keys it; xo dev codegen's xo asm the same (gotchas/dev-goldens-inline-strict.md; last verified: 2026-10-21)
xo dev goldens --backend=wasm ...   # since 2026-10-24 builds the module and runs it under Node's WASI (compiler/wasm/run.mjs: $XO_WASM_RUNNER, else beside the xo binary's checkout), want_wasm.txt before want.txt, no XO_LEAKCHECK; --cmd=run is ignored for wasm (`xo run` has no wasm backend). A sample of 59: 44 pass, the rest are the wasm backend's own unsupported calls (rt.stack_switch, rt.heap_big, sockets) and C library calls (exit 70). Before: the .wasm file was exec'd (permission denied) and --cmd=run printed only `xo leakcheck: skipped (os.proc.exit)`, the compiler's own exit line hiding the real error. Last verified: 2026-10-24
xo dev goldens ...   # since 2026-10-24 a build failure that is a documented limit counts as `unsupported`, not a failure (and the exit status stays 0): the wasm backend's unsupported calls, the `does not cross link` error, and, with a --target in --flags, a program with a `use c` source (use_c_add in hourly.sh's cross-target check once the every-4th sample reached it). The summary line shows `N unsupported`, --json an "unsupported" count. Last verified: 2026-10-24
xo dev errorcheck DIR... | xo dev codegen [--backend=arm64,amd64,llvm] DIR...   # hidden (decision 0217): diagnostics against `// ERROR XOnnnn "re"` comments (test/errorcheck), and per function patterns `// arm64: "re" -"re"` on `xo asm` output (test/codegen); `// amd64:` patterns (since 2026-10-27, in the default backends) on `xo asm --backend=amd64` (AT&T text, functions marked `# func NAME`, the host's object format), 80 of them seeded from the x86 isel and pass wins, seven of which fail with the seven optional x86 passes off (XO_PASSES=-x86_carry_fuse,-x86_const_imm,-x86_fold_loads,-x86_mul_pair,-x86_red_zone,-x86_sink_loads,-x86_merge_blocks) (last verified: 2026-10-27); a `// func MOD.NAME` line names a function of another module (a std runtime module's, as `xo asm` names it) for the patterns below it (test/codegen/consttime, decision 0222) (last verified: 2026-10-14)
xo dev new-code XOnnnn --title=TEXT [--severity=error|warning|note] [--component=NAME] [--ref='core 3.1'] | xo dev new-golden NAME [--dir=internal/llvm/testdata/golden] [--from=FILE] [--no-run]   # hidden (2026-10-20 poll, compiler/cli/dev scaffold.xo), run in a checkout: new-code adds the code's row to compiler/cli/core/explain_data.xo and to spec/core.md 10.6 in code order (internal/repotest TestSpecListsEveryCode keeps them equal) and writes test/errorcheck/xonnnn/main.xo to fill in; new-golden writes DIR/NAME/main.xo (FILE or a stub) and want.txt from `xo run` with this xo (check it before committing) (last verified: 2026-10-20)
xo dev dwarf [--func=NAME] [--types] [--break=FILE:LINE|FUNCTION] [--pc=0xADDR] FILE   # hidden (decision 0253 stage 1, compiler/debug; --break prints the addresses a breakpoint resolves to, one per function or inlined copy as lldb, --pc the file and line at an address): the functions (address ranges), their variables and parameters (type, declaration line, each location range with its pieces: regN, [regN+off], [fb+off], [addr], =const, gone), the calls inlined into them with their variables, the globals, and with --types the types, read from an ELF or Mach-O executable (its embedded `__DWARF`, decision 0346) or a dSYM's DWARF file (PROG.dSYM/Contents/Resources/DWARF/PROG) by the reader `xo debug` will use; --func keeps a function and the functions that inline it; internal/arm64 TestDwarfReader compares it with dwarfdump (last verified: 2026-10-17)
xo dev symbolize [--dwarf FILE] PROG < REPORT   # hidden (decision 0343 stage 3, compiler/cli/dev symbolize.xo): the `traceback:` of a native program's crash report from its `frames:` and `pc ... (image base B)` lines and PROG alone, in the report's own format (the same lines on darwin/arm64 with PROG.dSYM, linux/arm64 and amd64, windows/amd64 PE; checked 2026-10-22): for a report whose traceback timed out or faulted, or a stripped binary's report against its unstripped copy; DWARF from PROG, PROG.dSYM, or --dwarf, names of frames without DWARF from the symbol table (ELF, Mach-O, PE COFF /debug:symtab); a pc nothing names as PROG+0xOFFSET from B. internal/arm64 TestCrashReport checks it against each report (last verified: 2026-10-22)
xo dev unwind --snap FILE [--dwarf FILE] [--locals] EXE   # hidden (decision 0253, compiler/debug unwind.xo): the backtrace of a stopped thread recorded in FILE (`reg N 0xV` and `mem 0xADDR HEX` lines; internal/arm64/testdata/unwind/snap.py records one in lldb: `command script import snap.py`, `snap OUT`), by EXE's unwind tables, with functions, lines, inlined calls as frames, and with --locals each frame's variables; --dwarf a dSYM's DWARF file (last verified: 2026-10-17)
xo dev debug --snap FILE [--dwarf FILE] [--json] -c CMD [-c CMD]... EXE   # hidden (decision 0253, compiler/debug session.xo): `xo debug --batch` on a recorded thread until the process control: break, delete, bt, frame N, locals, print NAME[.FIELD...], threads, quit (run, continue, step, next, finish: not yet); each command echoed as `(xo) CMD` then its output (--json: one JSON line per command, spec 11.5 version 1); exit 1 when a command failed; with no -c the commands come from standard input until its end or quit (prompt `(xo) ` on a terminal, an empty line repeats the last command; with --json the line protocol) (last verified: 2026-10-17)
xo dev dump lex|ast|types|ssa|asm|llvm|go [flags] PATH...   # (`xo dev rtpack` deleted 2026-10-16, decision 0234) hidden: each compiler component's own command line (its dump_main) inside xo, --std of this xo by default; rtpack as `xo rtpack` (decision 0209; last verified: 2026-10-13)
xo1 build|run|test [flags] [dir ...]                 # decision 0178 stage 1: xo build|run|test's flags with Go's flag rules (--targets, -o, --json, --release, --error-args, --stamp, --no-mono, --no-arenas, --no-gc, --holes, --race, --tzdata, --seed, --explore, --record, --run, --cases, -v, --offline, --workspace) plus --std --wd --cc --asm --go-tree --phase-times; several directories are one program; the check's diagnostics printed as xo prints them (human, or --json's indented object); the build goes to the Go driver's build directory (<user cache>/xo/build/<hash>/arm64|llvm|..., XO_BIN_CACHE shared with xo under the same keys), prints `os/arch: path`; XO_LLVM_SPLIT/JOBS/LTO split LLVM builds; `run DIR -- ARGS` execs the program, its exit status passed through; not in xo1 yet: --ci, --hybrid, --real on native tests, XO1201 diagnostics (last verified: 2026-10-12)
(cd selfhost && go run ../cmd/xo build --backend=arm64 -o macho ./macho) && ./macho asm [-o OUT.o] [--minos=X.Y] [--bench=N] IN.s   # the self hosted Mach-O assembler (decision 0182): the arm64 emitter's assembly into an MH_OBJECT with the sections, relocations, compact unwind entries, and eh_frame of `cc -c`; LC_BUILD_VERSION minos default 11.0 (last verified: 2026-10-12)
python3 scripts/macho-compare.py <macho> <file.s...>   # macho asm against cc -c on each file: identical objects, functions, sections, relocation tables (symbols by name), sizes, time; MACHO_KEEP=1 keeps the objects; inputs from `xo asmdump <dir>` (decision 0182; last verified: 2026-10-12)
./macho asm --amd64 [-o OUT.o] IN.s   # the x86-64 assembler (decision 0254, stage 1): the amd64 emitter's AT&T text into the ELF object `clang -c -target x86_64-unknown-linux-gnu` writes, byte for byte (last verified: 2026-10-17)
`xo build --backend=amd64` pads jumps (decision 0398): every jmp, jcc, call and ret, and the cmp/test/add/sub/and/inc/dec a jcc fuses with, gets NOPs before it when it would cross or end on a 32 byte boundary, and code sections are 32 byte aligned; `assemble_elf_amd64_raw` / `assemble_macho_amd64_raw` / `assemble_coff_amd64_raw` (compiler/macho) skip it for byte comparisons with clang. Last verified: 2026-10-28.
./macho elflink --static -o OUT IN.o...   # a static linux executable of ELF objects, arm64 or x86-64 (decision 0254 stage 4 adds x86-64: R_X86_64_* relocations, TLS variant 2, GOTTPOFF relaxed to local exec); x86-64 links statically or against glibc 2.31 (glibc_amd64.xo) (last verified: 2026-10-17)
python3 scripts/amd64-enc-goldens.py forms|survey DIR|objects   # regenerate the x86-64 encoder's clang goldens (compiler/macho enc_amd64*_test.xo, asm_amd64_test.xo from testdata/amd64/*.s); survey takes LLVM's x86_64 .s files (a linux/amd64 xo in Docker: xo asm --backend=llvm, clang -S -O2) (last verified: 2026-10-17)
./macho link -o OUT [--minos=X.Y] [--sdk=X.Y] [--dylib=PATH...] IN.o|IN.a...   # the self hosted Mach-O linker (decision 0182): objects and archives into a PIE MH_EXECUTE with __stubs, __got, chained fixups (DYLD_CHAINED_PTR_64_OFFSET), __unwind_info from the compact unwind entries, thread local descriptors, common symbols, an exports trie of __mh_execute_header, an ad hoc code signature; imports bound to libSystem (ordinal 1), or by flat lookup when --dylib adds a dylib; LC_BUILD_VERSION 11.0/11.0 by default (last verified: 2026-10-12)
xo1 build --as=macho ...                                 # xo1 assembles with compiler/macho (writes OUT.o) and cc links the object; xo dev goldens --flags='--as=macho' runs the goldens that way (decision 0182; last verified: 2026-10-12)
xo1 build --backend=arm64 ...                            # the linker when --ld is not given (decision 0182 next step 1, 2026-10-12): on a darwin host the host's link is --ld=macho's when --as is not cc and no `use c` source is linked, else cc (decision 0182 addendum, 2026-10-16: cc had been the default again from 6faa44cb that morning; last verified: 2026-10-16). A linux host links with Xo's ELF linker too (2026-10-22: `--phase-times` prints `phase link` for a hello built by the 0.25.1 linux/arm64 and linux/amd64 xo in Docker; the line here had said cc). `--ld=cc` forces cc; `--phase-times` prints `phase link` for macho and `phase cc` for cc (last verified: 2026-10-12)
xo build|run|test (no --backend)   # the native backend (decision 0248, 2026-10-16; decision 0270, 2026-10-19, on x86-64 hosts too): arm64 or amd64 for the target's architecture (--targets, else the host's); --race keeps the native backend where race_runtime finds a ThreadSanitizer runtime for the target (lib/xo/race/: darwin/arm64, darwin/amd64, linux/arm64, linux/amd64, windows/amd64; decision 0273 addendum, owner 2026-10-22) and otherwise picks llvm, whose missing clang is an error saying no native runtime ships for that target (llvm with no clang otherwise: an error naming xcode-select --install, the distribution's clang, XO_CLANG, or --backend=go); --backend=go builds through the Go toolchain and a `use go` program needs it (with no --backend: a usage error naming the Go package); --hybrid, --holes, xo test --explore, xo run --seed and --record pick go. Contract fuzzing runs natively too (decision 0250: each target as a generated property test in the checker's overlay, `<file>.xo_contract_fuzz_test.xo`, never on disk; the program checked a second time, about 200 ms on a 20 contract module; tests named `contract fuzz: MODULE.F` as on the Go backend, `--run` matches them; regression tests written the same); a target whose generated test does not check is counted in `MODULE: contract fuzzing needs --backend=go: N contracts not fuzzed` (reason under -v). xo dump works on native programs too (decision 0251: SIGQUIT wakes the poller, whose thread writes the tree; with XO_THREADS=1 and on idle programs too). internal/xotool's Build names --backend=go when a test gives no backend (the tests were written for the Go default). Last verified: 2026-10-16
xo build|run|test --backend=arm64|llvm for linux   # no C library by default (decision 0243, 2026-10-16) for a program with no `use c` module: a static executable, the C library functions the runtime calls defined in Xo (std/native/llrt/cabi_libc.xo and libc*.xo, decision 0230), the backends' own _start; linked on a linux host by default (arm64 backend, no --ld) with compiler/macho elflink's static mode (no loader, no imports; weak undefined symbols 0; step 3), or with --ld=cc by cc -static -nostdlib -Wl,--gc-sections -Wl,--eh-frame-hdr; the LLVM backend links with clang. A cross build for linux/arm64 from any host (`xo build --targets=linux/arm64`, no --ld) links the same way, so the Mac makes a runnable static Linux binary (all 180 goldens built on the Mac pass in Docker, 2026-10-16); with the C library a cross build still emits assembly. -s drops elflink's local symbols, as macho's. XO_NOLIBC=0 links the C library (dynamic, glibc); XO_NOLIBC=1 asks for none and fails for `use c` programs and XO_LLRT_C=net. Sockets, epoll, DNS (hosts, nsswitch, resolv.conf) and TLS roots work without libc (an https GET verified in Docker, 2026-10-16). The toolchain's own native Linux builds of compiler/xo (internal/xotool hostBackend, scripts/bootstrap-xo.sh, xo1-host-check.sh) pass XO_NOLIBC=0 until a Linux self rebuild fixed point passes with none; the releases' and seeds' Linux xo are cross built by the native backends with no C library (decision 0362, 2026-10-24; a Docker linux/amd64 self rebuild of that xo, s2 = s3, with none). XO_LLRT_C names only the C paths left (the std/native/llrt/*_c.xo files: net, sched, text). Last verified: 2026-10-16
xo1 build --backend=arm64 with XO_INLINE_STRICT=1   # a build whose inlined arm64 module does not verify fails (compiler/xo main.xo inline_or_say); without it the build says so on standard error, naming the function and the first error, and goes on without inlining. internal/xotool sets it for every Go test; XO_INLINE_BREAK=1 breaks the inlined module on purpose (internal/arm64 TestInlineFallbackLoud). Last verified: 2026-10-15
xo build (arm64, amd64)   # decision 0342 (2026-10-22): before inlining, every call, closure, and field or payload access of the module is checked against this build's definitions (ssa check_assumptions, phase "assume" in --phase-times); a mismatch, a stale cached piece, is XO1510 and exit 1; XO_ASSUME_CHECK=off skips it (for its --ab)
XO_INLINE_REPORT=name xo build .   # how the native backends' inliner takes each function whose name contains name (1: every function), on standard error: its expanded instructions and blocks, call sites, and small, loop, few, padded, or no (hard, pads, size). Last verified: 2026-10-20
XO_MAP_LOG=0 xo build .            # native Maps and Sets as the copy-on-write lists alone; the default of every backend (since the switch, owner 2026-10-21; wasm the same day) is decision 0291's hybrid: the lists until a value is written while shared a second time, then the log and fork representation (compiler/ssa maphyb.xo, maplog.xo); keyed in the front end and fn caches. Last verified: 2026-10-21
XO_MAP_LOG=1 xo build .            # the hybrid without decision 0291's branching, which is the default on the native backends since 2026-10-22 (owner's rule: on after an HTTP CRUD xo bench --ab showed no loss): a lists Map of 1024 or more live entries written while shared forks its own lists in O(1) instead of copying them, and a fork its only holder writes 64 times goes back to the lists (maphyb.xo maph_fork_lists, maph_goback); wasm does not branch; keyed as mapbr= in the front end, fn and part caches; TestMapBranch runs the Map goldens with XO_MAP_LOG=1, TestTSanSharedMaps all three forms. Last verified: 2026-10-22
XO_INLINE_DENY=name,prefix* XO_BACKEND_CACHE=off xo1 build --backend=arm64 .   # a measurement knob: the arm64 inliner leaves those functions out (a name, or a prefix ending in `*`), to weigh each callee's text against its cost (bench/results/inline-ledger-2026-10-17.md); turn the backend cache off, since its key does not include the knob. Last verified: 2026-10-18
xo dev dump ssa --fn=NAME --stage=S DIR, xo dev dump asm --ssa=rc|inline --fn=NAME DIR   # one function's SSA after a pass or after the inliner (agent-tools.md, 2026-10-21; a std function works)
XO_LOWER=-name,... xo build ...   # turns optional SSA builder lowerings off (compiler/ssa lowerings.xo: eq_lower, user_cap_call), for bisecting inside a commit; keyed in the front end entries (agent-tools.md, 2026-10-21)
XO_DUMP_FN=NAME xo build --backend=arm64|amd64 ...   # that function's SSA and its MIR (after allocation: each virtual register with its register or slot) to $XO_DUMP_DIR (else $TMPDIR, else /tmp) as xo-ir-NAME.txt, the path on standard error; a backend internal error writes its failing function's the same way without the variable (2026-10-21, owner approved). A function inlined into every caller has none (the driver says so). Keyed in the backend caches (a cached function would not be generated). The backend's two register aborts (scratch_reg's "no free register for a spilled operand", amd64's x86_borrow) are failures with the dump too since 2026-10-21 (they were faults that stopped the process first). Last verified: 2026-10-21
Backend failures past the checker name the Xo source (2026-10-21, from the language poll): `file:line:col: in \`fn\`: ` before the native and LLVM emitters' errors and skips (ssa `Func.at_source`, the value being lowered or the function's first line; an inlined callee in its own file), the Go generator's errors with no node (gen `fn_at`), and after `clang failed` each IR line clang names mapped to its Xo function (llvm srcmap.xo). Last verified: 2026-10-21
XO_STD_ROOTS=1 XO_DCE=off XO_PASSES=-hollow xo build -o prog .   # decision 0388's std store, groundwork (darwin/arm64): every std module loaded and every body checked, every function with a body in std or its implementations (not the runtime's native/llrt modules, not the test world outside a test build) a root; the parts get `.subsections_via_symbols`, so compiler/macho's linker splits them into atoms and keeps only what the entry reaches (strip.xo), and each function's async preemption unsafe ranges are a table of its own indexed in __DATA,xounsafe (bit 40 of the xo_unsafe_parts count). Last verified: 2026-10-27
XO_STD_OBJ=DIR XO_DCE=off XO_PASSES=-hollow xo build -o x .   # decision 0388 stage 4's std object (darwin/arm64), from an empty program: an XO_STD_ROOTS build that emits only std's functions (and generic instances and fileless generated ones), its helpers and instances weak definitions (Module.weak_shared), and writes the part objects and their store (std.store) to DIR instead of linking (`std object: N parts and std.store in DIR`); the program's `_xo_f_main` and `_xo_entry` (g.entry, which depends on main's signature) are left to the program's object. Last verified: 2026-10-28
XO_SEPCOMP_STORE=PATH xo build ...   # decision 0388 stage 3 (darwin): a separately compiled program's link reads std from the store at PATH (xo dev stdobj's OUT/std.store) instead of std's part objects: every input of the build is the program's, kept whole, and only the std atoms it reaches are read (link_store; the program's weak definitions root the store's of the same name, which the link keeps). Last verified: 2026-10-28
xo dev stdcache [--conc=0|1] [--target=OS/ARCH]   # decision 0388: std's store and facts for the variant from the back-end cache (std/<key>.store, std/<key>.stdf; compiler/xo stdcache.xo std_variant), built on a miss (xo dev stdobj in a directory of its own, renamed into place), their paths on two lines; exit 1 when none (the cache off, or the build failed). `cache std KEY conc=C reused|built|failed` on standard error. --peek answers as std_variant_peek: ready (released or cached) prints the paths; a miss starts a detached, niced `xo dev stdcache` (env XO_STD_BG_CHILD=1, which builds only while it holds the file lock std/.bg.lock, so one background build runs at a time and a dead builder leaves no lock) and exits 1 (`std variant: miss, building in the background` on standard error); XO_SEPCOMP_BG=0 starts none (internal/xotool sets it for the Go tests; heavy.sh too); no background build on windows. A plain `xo dev stdcache` says when a background build is running (`a background build is running (linux/arm64 conc=false since ...)`, from std/.bg-info). Last verified: 2026-10-28
xo dev stdobj [--conc=0|1] [--facts=FILE] [--target=OS/ARCH] OUT   # decision 0388: std's object for one conc variant (default 0) in OUT (its parts and std.store: XO_STD_OBJ and XO_STD_CONC over an empty program, no dead function removal or hollowing), and with --facts std's facts (an XO_STD_ROOTS build's XO_STD_FACTS_DUMP in a cold cache of its own). Last verified: 2026-10-28
XO_STD_CONC=0|1 XO_STD_ROOTS=1 ...   # decision 0388: fixes the builder's unwinding checks (conc) for std's object or facts, one per variant (an XO_STD_ROOTS build is otherwise always conc=true); with 0 a body needing concurrency faults where it would (ssa Builder.conc_off). Last verified: 2026-10-28
XO_STD_ROOTS=1 XO_DCE=off XO_PASSES=-hollow XO_STD_STORE=PATH xo build -o prog .   # decision 0388 stage 4's indexed store (darwin/arm64, compiler/macho store.xo): when PATH is absent the build's parts (prog-darwin.N.o) are packed into it; when present the link goes through it (only the atoms the program reaches are read, a dead function's subprogram, line sequence and location lists cut, dwtrim.xo) without reading the parts; XO_STD_STORE_CHECK=1 also links the parts and fails unless the two binaries are equal byte for byte; XO_STD_STORE_REPEAT=N prints N more timed links (`store link N us (prepare P, resolve R, finish F)`). internal/arm64 TestStdStoreUnwind (not -short) checks a store build's caught fault, traceback and lldb backtrace against a normal build's, and a store link against the parts'. Last verified: 2026-10-28
XO_DCE=off|debug|list xo1 build --backend=arm64 .   # dead function removal before arm64/amd64 code generation (compiler/arm64 reach.xo, bench/results/native-dce-2026-10-18.md): off keeps every function, debug prints the counts, list also the dropped names (on standard error). Last verified: 2026-10-18
xo1 build --backend=arm64 with XO_ARM64_REGS=N   # test only: the register allocator uses N caller saved and N callee saved registers per class, so most values spill and the emitter's spilled operand paths run (internal/arm64 TestGoldenRunSpill, N=2 and 4); part of the function cache key. Last verified: 2026-10-16
XO_THREAD_SWITCH=TEXT|!TEXT|@FILE xo build .   # triage only (decision 0397): threaded dispatch (compiler/arm64 threadsw.xo) on the functions whose name contains TEXT, on all but those, or on those named in FILE (one a line); bypasses the backend caches like XO_PASSES; XO_PASSES=-thread_switch turns the pass off everywhere. Last verified: 2026-10-28
XO_ARM64_PAD=K xo build --backend=arm64 .   # measurement only (decision 0075, ported 2026-10-28): K nops before every function label, never run, so the whole program shifts by 4K bytes; a code shape change on the native backend is judged by its median over K = 0 to 15 (interp moved 15 percent with its placement, 2026-10-28), not at the one placement it lands on; keyed in the fn and part caches. Last verified: 2026-10-28
xo1 build --backend=arm64 with XO_POISON_FRAMES=1   # debug: every frame's spill slots, outgoing argument words, and memory area (frame homes, aggregates) start as 0xdeadbeefdeadbeef (the prologue writes it), so a read of a word no path wrote shows at once instead of reading a stale one; not far frames; the standalone compiler/arm64 driver takes it too (internal/arm64 TestGoldenRunPoison); part of the function cache key. Last verified: 2026-10-18
xo1 build with XO_HEAP_GUARD=1             # debug, native backends: every runtime record (rt.heap_large) ends against an inaccessible 64 KiB page, so an access past it faults at once, and an rt.slot_* on a List checks its index against the length (std/native/llrt libcmem.xo guard_large, compiler/ssa json.xo slot_bounds); in the front end, function and part cache keys (internal/arm64 TestHeapGuard, TestGoldenRunHeapGuard; gotchas/svc-roots-slot-index.md). Last verified: 2026-10-19
xo build [-s] [--debug-info] [--dsym] ...   # DWARF line tables (decision 0239): on by default for build, run, and test on arm64 and LLVM; -s: none, and no local symbols either (compiler/macho on darwin leaves them out, a cc link gets `-Wl,-x` on darwin and `-s` on Linux; the exports and imports stay, and a traceback of such a build prints `traceback: (no function symbols: built with -s, or stripped)` rather than naming runtime exports, 2026-10-25; --ld=macho's ELF link for linux/arm64 still writes them; a `--release` link keeps the local function symbols, so tracebacks still name them, 2026-10-25); --release: none unless --debug-info (=on; decision 0354: `--debug-info=off` in any build); `--debug-info=split` (decision 0355): darwin PROG.dSYM beside a program with no __DWARF (UUID-matched; Xo's Mach-O link, or dsymutil's bundle kept for a cc/LLVM link), linux PROG.debug from Xo's ELF linker (`.note.gnu.build-id` in both, `.gnu_debuglink` in the program; `llvm-symbolizer --obj=PROG ADDR` and addr2line find it beside the program; the runtime's traceback and `xo dev symbolize` too), an error for a linux cc/clang link, wasip1 and --backend=go; `xo asm`: none. darwin: compiler/macho embeds the DWARF uncompressed in PROG's `__DWARF` segment (decision 0346; no PROG.dSYM unless `--dsym`, which writes the bundle too and which a binary cache hit restores; an earlier --dsym build's bundle is removed); a cc link, the LLVM backend's or --ld=cc, runs dsymutil after the link and then moves the bundle's DWARF into PROG as the same segment (decision 0346 stage 3, compiler/macho embed.xo; the link passes -Wl,-headerpad,0x1000 for its load command, and the ad hoc signature is made again), keeping PROG.dSYM only for --dsym or when embedding fails (a warning says why); check with `dwarfdump --verify PROG`, `dwarfdump --lookup=ADDR PROG`, `atos -o PROG ADDR`, `lldb -b -o "b main.xo:9" -o run PROG`); Linux: .debug_* in the ELF (`llvm-dwarfdump --verify`, `llvm-symbolizer --obj=PROG ADDR`). llvm-dwarfdump --verify flags tail-merged .debug_str entries of LLVM objects as invalid (gotchas/dwarf-verify-tail-merged-strings.md). Last verified: 2026-10-25
XO_MAX_MEM=8G xo build .   # a memory budget (decision 0349): past 8 GiB in use by its heap (chunks and records as mapped, large blocks as handed out) the program (xo or any native program) stops with status 70, `fatal error: memory budget exceeded: N MB mapped, more than XO_MAX_MEM (M MB)` and `  while: LABEL` (std/debug mem_label; xo labels its phases); K, M, G suffixes, `off` for none. xo's default (owner, 2026-10-22): three quarters of physical memory, at least 16 GiB, 18 GiB on this Mac (its first, half of physical memory counted as mapped bytes, stopped every self build: gotchas/mapped-bytes-overstate-memory.md); `xo build --phase-times` prints the heap and its peak per phase (a cold self build of xo: 9.9 GB). heavy.sh's HEAVY_MAX_GB stays the outer guard. Last verified: 2026-10-22
XO_TRACEBACK=0 ./prog   # an uncaught fault prints only its one line report: by default (owner, 2026-10-22) it is followed by `traceback:` and `  FUNCTION (FILE:LINE)` per frame down to main (decision 0239, spec core 8); each fault, a caught one included, pays about 0.55 us for its trace (bench/results/fault-frames-default-2026-10-22.md). The profiler (XO_PROF=cpu) names lines from the same tables: `go tool pprof -list FUNC PROG.pprof`. Last verified: 2026-10-16
XO_TRACEBACK=locals ./prog   # the same traceback with each frame's variables under it (`      label = "widget-2"`, `p = Point { x: 2, y: 42 }`, `m = Mode.Slow(level: 7)`, `names = ["a-2", "b"]`: a List's first elements of any whole-word element type, from its object's items type), read from the program's DWARF at the fault (decision 0268; dwline.xo's variables; internal/llvm TestFaultLocals on both backends); `<unavailable>` where the frame does not keep the value (a caller saved register, or past its last use). An inlined call's variables are printed under its `[inlined]` line, each line once; a literal's binding (`let label = "order-7"`) shows its value on arm64 (decision 0268's 2026-10-22 addendum; TestFaultLocalsFrames). On windows/amd64 too since 2026-10-30: the frames' registers come from the RtlVirtualUnwind walk's CONTEXT (unwind.xo win_walk_regs; before, the program printed one note that the variables were not available, Windows field report W9, 2026-10-23). Off by default. Last verified: 2026-10-23
XO_CRASH_REPORT=0 ./prog   # turns off the hardware fault report (decision 0343): on Linux and macOS a native program's SIGSEGV, SIGBUS, SIGILL, SIGFPE or SIGTRAP prints `fatal error: SIGSEGV: segmentation violation (code 2) at address 0x10`, `pc ... (image base ...)`, the registers, `frames:` (the pcs, a recursion's run as `(xN)`) and `traceback:` (named from the DWARF, XO_CRASH_TIMEOUT seconds at most, 5 by default), then the default action ends it (status 128+signal, core files as before). windows/amd64: `fatal error: EXCEPTION_ACCESS_VIOLATION: access violation (code 0xc0000005, write) at address 0x10` and the same lines, the exit code still the NTSTATUS; an exception a C `__try` takes is not reported (crash.xo on_crash_veh and on_crash_win). Resolve the frames offline with `xo dev symbolize PROG < REPORT`, or a pc with `atos -o PROG -l IMAGE_BASE PC` (macOS) or `addr2line -e PROG PC-IMAGE_BASE` (a Linux PIE). Test: internal/arm64 TestCrashReport; Windows: scripts/ci/windows-crash.sh (suite=script on xo-windows). Last verified: 2026-10-22
XO_CRASH_SLEEP=30 ./prog   # opt in, for debugging a fault in a live process (2026-10-27, the lz4 SIGSEGV hunt): after phase 1 of a SIGSEGV or SIGBUS report the process prints `crash: holding for 30 s, pid N` and waits that many seconds (1 to 3600) before the default action, so `vmmap -v N` and `lldb -p N` can read it (the other threads keep running; send SIGSTOP to freeze them first); off by default, with a case in internal/arm64 TestCrashReport. Last verified: 2026-10-27
xo1 build --ld=macho ...                                 # no cc: compiler/macho assembles and links (no runtime pack since decision 0234; before, with the runtime of a prebuilt pack ($XO_RTPACK or lib/xo/rtpack beside xo1; a program calling the Security framework adds it and CoreFoundation); std/time's zone data is written as an object (ld.xo tz_object; the zip from XO_ZONEINFO, else lib/xo/zoneinfo.zip beside xo1's lib/xo or beside $XO_RTPACK, else go env GOROOT); `use c` sources still need cc; XO_RTPACK=DIR xo dev goldens --flags=--ld=macho --build-path=/nonexistent builds every golden with an empty PATH (decision 0182; last verified: 2026-10-12)
./macho asm --elf [-o OUT.o] IN.s                      # the self hosted ELF assembler (decision 0182 stage 3): the emitter's linux/arm64 assembly (`xo asmdump --os=linux`) into an ELF64 relocatable object byte identical to `clang --target=aarch64-linux-gnu -c` (RELA relocations, mapping symbols, the DW.ref.xo_personality COMDAT, .eh_frame); numeric labels (`1:`, `1b`) (last verified: 2026-10-12)
python3 scripts/elf-compare.py <macho> <file.s...>    # macho asm --elf against $ELF_CC -c (default `clang --target=aarch64-linux-gnu`, Apple clang works) on each file: identical objects, functions, sections, relocation tables (symbols by name); ELF_KEEP=1 keeps the objects, ELF_VERBOSE=1 names differences (decision 0182; last verified: 2026-10-12)
./macho elflink -o OUT [--interp=PATH] IN.o|IN.a...   # the self hosted ELF linker (decision 0182 stage 3): objects and archives (GNU or BSD ar) into a dynamically linked ET_EXEC for glibc at 0x400000: _start and the outline atomics (__aarch64_*) synthesized, imports versioned from compiler/macho/glibc.xo (libm, libpthread, libgcc_s, libc NEEDED), calls through 16 byte stubs and GOT slots bound at load (GLOB_DAT, BIND_NOW), TLS descriptors relaxed to local exec, .eh_frame_hdr, a .symtab of functions and objects (last verified: 2026-10-12)
xo1 build --target=linux/arm64 --ld=macho ...          # from any host, no cc: the linux/arm64 executable, compiler/macho assembling and linking with the runtime of the linux/arm64 pack in $XO_RTPACK (std/time's zone data as an ELF object, ld.xo tz_object_elf); `--as=macho` alone writes the ELF object for cc on a linux host (decision 0182 stage 3; last verified: 2026-10-12)
python3 scripts/elf-glibc-versions.py DIR > compiler/macho/glibc.xo   # regenerate the ELF linker's import versions from a linux/arm64 libc.so.6, libm.so.6, libpthread.so.0, libgcc_s.so.1 (copied out of debian:bullseye, glibc 2.31; command in the script) (decision 0182; last verified: 2026-10-12)
xo rtpack --target=linux/arm64 --cc="clang --target=aarch64-linux-gnu --sysroot=DIR"   # a linux pack on macOS without zig: DIR from `apt-get download libc6-dev linux-libc-dev` in debian:bullseye extracted with dpkg-deb (bullseye's security mirror 404s for apt-get install); no archiver since the TLS library's removal (2026-10-15)
go test ./internal/selfhost -run 'TestSelfhostArm64'    # Xo arm64 emitter vs internal/arm64, piece by piece (functions with their call site tables, helpers, literals, xo_entry; program wide label counters renamed), on every repository run both SSA builders build completely: every second run without arm64Heavy (the mergeHeavy runs and compiler/parser) in the merge tier (421 s, 295 s of it the go subtest, in the 1,329 s merge tier of 2026-10-12, which still emitted compiler/llvm; not remeasured since), every run in the full tier (go, arm64, llvm builds), every 25th with -short; TestSelfhostArm64Mutations: 30 code mutations (merge), 300 (full), none with -short; XO_SELFHOST_BACKEND, XO_SELFHOST_ARM64_FUZZ=N, XO_SELFHOST_SEED, XO_SELFHOST_KEEP=<dir> (writes arm64-<backend>.txt); XO_SELFHOST_DIRS=text,text takes the runs whose directory contains one of the texts instead of the tier's (a targeted run, as TestSelfhostSSA; added 2026-10-12, decision 0184); decision 0158 (last verified: 2026-10-12)
go run ./cmd/xo build --backend=llvm -o .tools/xossa compiler/ssa   # native build of the Xo SSA builder (also --backend=arm64; the LLVM build links for about 10 minutes)
go test ./internal/selfhost -run 'TestSelfhostSSA'      # Xo SSA builder (go, arm64, llvm in the full tier; go otherwise, and every 25th run with -short) vs internal/ssa's build stage, function by function, on every repository run (closures compared by rank in their function, poison types skipped), then every pass stage (the merge tier compares only the build stage of the mergeHeavy runs, tier_test.go: huge_frame and every compiler/ component but the lexer and the parser; 115 s plus 492 s for its go subtest in the 1,329 s merge tier of 2026-10-12, before compiler/arm64, compiler/llvm, and compiler/xo were build only), and 400 code mutations (TestSelfhostSSAMutations: the first 300 in the merge tier, none under -short; XO_SELFHOST_SSA_FUZZ=N, XO_SELFHOST_SEED, XO_SELFHOST_BACKEND); XO_SELFHOST_BACKEND=go|arm64|llvm also limits TestSelfhostSSA to one backend (about 25 s warm for go, a development loop); XO_SELFHOST_KEEP=<dir> writes ssa-<backend>.txt, the counts and every reason a function was skipped (last verified: 2026-10-11)
go run ./cmd/xo llvmdump [--each] [--test] [--func=NAME] [--bench=N] <dirs...>   # hidden: the LLVM IR llvm.Generate makes (no target attributes), checked as a native build checks, fault positions relative to the working directory: == ir and the module, == rejected, or == error <message>; --each lowers every dir alone (=== llvm <dir>); --func=NAME prints one definition (llvm.SplitIR's names: x.main, xo.dup.Str); --bench=N times llvm.GenerateSSA alone on one module (llvm.PrepareSSA) N times (decision 0166; last verified: 2026-10-11)
go run ./cmd/xo run compiler/llvm -- [--std=DIR] [--wd=DIR] [--each] [--test] [--stage=N] [--bench=N] <dirs...>   # the self hosted (Xo) LLVM emitter: checks with compiler/check, builds with compiler/ssa and its passes, prints what xo llvmdump prints, then == skipped and <function>: <why> for the functions left to a later stage (--stage=N: the stages up to N of decision 0166, default 3, all); a run the Xo SSA builder does not build completely prints == unsupported (last verified: 2026-10-11)
go run ./cmd/xo run compiler/wasm -- [--std=DIR] [--wd=DIR] [--each] [--why] [--lax] [--annotate] [--out=DIR] <dirs...>   # the WebAssembly backend, written only in Xo (decision 0179): checks with compiler/check, builds with compiler/ssa and its passes, lowers to WAT, assembles it (compiler/wasm/asm.xo), prints == ok <bytes> and writes DIR/<name>.wasm and .wat, or == unsupported <why> (no module); --why lists every unsupported construct, --lax makes an unsupported runtime call fault when it runs (experiments only); --annotate comments the code of every SSA instruction in the .wat with its opcode (`;; @op`), to attribute a module's bytes to constructs (2026-10-12); --asm=IN.wat -o OUT.wasm assembles a WAT file (last verified: 2026-10-12)
go run ./cmd/xo build -o xo1 compiler/xo && ./xo1 build --backend=wasm [-o OUT.wasm] <dir>   # the Xo driver with the WebAssembly backend: OUT (default <name>.wasm), and OUT's .wat beside it only with XO_KEEP_WAT=1 (2026-10-23, field report b05: it was 770 KB by default); imports only WASI preview1 (decision 0179; last verified: 2026-10-12)
node --no-warnings compiler/wasm/run.mjs PROG.wasm [args...]   # runs a module of the wasm backend under Node's WASI preview1 in a worker, the working directory preopened as fd 3 and / as fd 4 (1 GiB stack, single threaded GC: gotchas/node-wasm-concurrent-gc-crash.md; no incremental marking and no lazy wasm compilation: gotchas/wasm-busy-loop-crashes-node.md, wasm-lazy-compile-worker-crash.md); a trap prints `wasm trap:` and exits 134; XO_LEAKCHECK=1 prints the leak check line; Node 22.22.1 (/opt/homebrew/bin/node; no wat2wasm, wasmtime, or wasmer installed; last verified: 2026-10-12)
go test ./internal/selfhost -run TestSelfhostWasm      # the wasm backend on every LLVM golden (merge and full tiers, not -short; skips without node): each supported golden must match want.txt and free every allocation, unsupported ones are counted, at least wasmMinPass (116 of 134 since stage 2) must pass; task programs run under the single threaded scheduler of compiler/wasm/rt3.xo (decision 0179; last verified: 2026-10-12)
XO_WASM_GOLDENS='tls*,map*,set_*' go test ./internal/selfhost -run '^TestSelfhostWasm$' -count=1  # a subset of the wasm goldens (comma separated globs of golden names; no wasmMinPass floor): the hourly main check's TLS and Map goldens, 22 goldens in 113 s wall on the Mac with a warm xo against 790 s for all 264 (last verified: 2026-10-21)
go test ./internal/selfhost -run TestSelfhostWasmFsWrite   # rt.fs_write: a program built by compiler/wasm writes files under Node's WASI and must print what the go backend prints (merge and full tiers; last verified: 2026-10-12)
go test ./internal/selfhost -run 'TestSelfhostLLVM'     # Xo LLVM emitter vs internal/llvm on every repository run and the test binaries (functions by name with string literals and fault locations by contents, helpers, whole modules byte for byte counted), and mutations (TestSelfhostLLVMMutations); tiers: -short every 25th run and no mutations, merge every third run but the mergeHeavy ones and 30 mutations (73 s plus 115 s for the go subtest in the 1,329 s merge tier of 2026-10-12, which still lowered compiler/arm64, compiler/llvm, and compiler/xo; not remeasured since), full every run, every backend, 400 mutations; XO_SELFHOST_BACKEND, XO_SELFHOST_LLVM_STAGE=N, XO_SELFHOST_LLVM_FUZZ=N, XO_SELFHOST_KEEP=<dir> (llvm-<backend>.txt: counts and why functions were skipped), XO_SELFHOST_DIRS=text,text (the runs whose directory contains one of the texts instead of the tier's, as TestSelfhostArm64; added 2026-10-12, decision 0192) (decision 0166; last verified: 2026-10-12)
XO_SELFHOST_E2E=lexer|parser|check|ssa|llvm go test ./internal/selfhost -run TestSelfhostLLVMEndToEnd -v   # decision 0166 stage 4: the self hosted pipeline (the compiler/llvm binary, go build or XO_SELFHOST_BACKEND) lowers compiler/<component>, the IR must be byte identical to internal/llvm's, clang builds a binary from that IR (build.Options.LLVMIR, XO_BIN_CACHE=off) and its output on a fixed input must equal the go build's; runs in the full tier with lexer (about 3 s); ssa: 27 s to lower, 34 minutes of clang (-O2 -flto on 399 MB of IR) (last verified: 2026-10-11)
go test ./internal/selfhost -run 'TestSelfhostSSA'      # Xo SSA builder (go, arm64, llvm in the full tier; go otherwise, and every 25th run with -short) vs internal/ssa's build stage, function by function, on every repository run (closures compared by rank in their function, poison types skipped), then every pass stage (the merge tier compares only the build stage of the mergeHeavy runs, tier_test.go: huge_frame and every compiler/ component but the lexer and the parser; 115 s plus 492 s for its go subtest in the 1,329 s merge tier of 2026-10-12, before compiler/arm64, compiler/llvm, and compiler/xo were build only), and 400 code mutations (TestSelfhostSSAMutations: the first 300 in the merge tier, none under -short; XO_SELFHOST_SSA_FUZZ=N, XO_SELFHOST_SEED, XO_SELFHOST_BACKEND); XO_SELFHOST_BACKEND=go|arm64|llvm also limits TestSelfhostSSA to one backend (about 25 s warm for go, a development loop); XO_SELFHOST_DIRS=text,text limits it to the runs whose directory contains one of the texts (about 7 s for six runs, 2026-10-11); XO_SELFHOST_KEEP=<dir> writes ssa-<backend>.txt, the counts and every reason a function was skipped (last verified: 2026-10-11)
XO_LEAKCHECK=1 ./prog                                  # an LLVM backend binary prints `xo leakcheck: N live of M allocations` at exit
go test ./internal/llvm -run 'TestRuntimeThreads|TestGoldenThreads'   # shared counts and the runtime under ThreadSanitizer (decision 0058; about 10 s)
go test ./internal/llvm -run 'TestRuntimeTasks|TestGoldenTasks|TestDeadlock'   # the task runtime and the task goldens under ThreadSanitizer (decision 0068; about 15 s)
go test ./internal/llvm -run 'TestRuntimeEventLoop'   # fibers, timers, sockets in C under ThreadSanitizer, 1 and 3 workers, then -O2 (decision 0069; about 10 s)
go test ./internal/llvm -run 'TestNativeSignal'       # SIGTERM two phase shutdown of testdata/signal built with the LLVM and arm64 backends (core 7.5; about 5 s)
go test ./internal/llvm -run 'TestHTTPParseDiff'   # native HTTP/1.1 reading and writing vs a net/http server (decision 0074; AddressSanitizer and UBSan; a few seconds). TestJSONParseDiff, the C JSON parser's, went with it (decision 0198); TestJSONTextDiff checks the Xo parser (last verified: 2026-10-13)
go test ./internal/llvm -run TestTLSDiff           # native HTTPS vs the Go backend: client errors, SNI, ALPN, versions, plain HTTP on the TLS port, SIGTERM (decisions 0105, 0106; about 15 s)
go test ./internal/llvm -run TestH2Diff -count=3    # HTTP/2 (decision 0121): Go vs LLVM vs arm64 backends as client and server, net/http's client with 400 streams at once, 70 raw frame cases h2spec style; repeat to catch racing cases (about 6 s each)
go test ./internal/llvm -run TestH2Sanitizers      # the TestH2Diff program (LLVM) under ThreadSanitizer and under AddressSanitizer with UBSan (about 10 s)
go test ./internal/llvm -run "TestDigestDiff|TestCryptoXoDiff"   # the native std/crypto (digest.xo, ct.xo, Ed25519) vs Go's crypto: SHA-256 and HMAC of random data, verify_password on x/crypto Argon2id and Go PBKDF2 hashes, mutated; crypto.equal and Ed25519 (RFC 8032 and random). TestCryptoDiff and its C reference (crypto_ref.c, the hashes decision 0198 deleted) went with decision 0241 (last verified: 2026-10-16)
XO_HTTP_DIFF_SEED=7 XO_HTTP_DIFF_N=30000 go test ./internal/llvm -run TestHTTPParseDiff   # more random and mutated connections
go test ./internal/llvm -run '^$' -fuzz FuzzHTTPDiff -fuzztime 60s -parallel 6   # Go's fuzzer on the same comparison (more workers than about 6 starved it on 2026-10-08: "context deadline exceeded")
XO_FUZZ_SECONDS=60 FUZZ_SEED=3 go test ./internal/llvm -run TestHTTPFuzz -v   # std/llrt/testdata/http_fuzz.c: mutated HTTP/1.1, HTTP/2 frames (server and client), and JSON under AddressSanitizer and UBSan (5 s by default, 1 s with -short; prints the HTTP/2 requests answered)
go test ./internal/llvm -run 'TestGoldenASan'         # net_echo and the http_* goldens with AddressSanitizer and UBSan, 1 and per CPU workers
go test ./internal/check -run TestNativeModules       # std/native implementation modules check without diagnostics
XO_THREADS=1 ./prog                                   # a native binary with one worker thread (default: one per CPU the process may run on: on Linux its sched_getaffinity mask, so taskset and a cpuset limit it, since 2026-10-21); a task main spawns then starts at main's next park or join, or at the monitor's tick (about 1 ms, more under load), as Go's GOMAXPROCS=1 (decision 0077's addendum)
XO_CAP=4 ./prog                                        # opt-in for a request server (decision 0377): only the first 4 workers wake or start, which cuts the wake and park cycling: CRUD CPU a request -16% at 32 connections and -25% at 128 with p99 equal or better and about 1% fewer requests a second (12 CPU Mac, 2026-10-26 and 27); not for build tools or CPU bound batch programs (the compiler's own build was 26% slower at 4); XO_CAP=0 is the default (every CPU). Last verified: 2026-10-27
XO_LOCKCHECK=1 ./prog                                  # the scheduler's lock order checked (a park's lock before the pool's, pool.xo plock and klock): the wrong order is a fatal `xo: lock order` error at once instead of a rare deadlock (gotchas/scheduler-lock-order.md); the scheduler tests set it. Last verified: 2026-10-20
XO_MAIN_FIBER=0 ./prog                                 # main's body on its own thread, as before decision 0286 step 1: a kill switch for bisecting a scheduler regression, kept until 0.24.0 is cut and then removed (default: main on a fiber of 1 GiB reserved, so a parked main moves to an idle worker). Last verified: 2026-10-20
XO_RTSTATS=1 ./prog                                   # native: scheduler counters on stderr at exit (spawns, steals, wake ups, idle sleeps, spin rounds, fiber parks, thread waits, slow locks; decision 0077) and socket counters (net_read, net_write, net_again = EAGAIN results, poll_return, poll_event, poll_wake; 2026-10-09) and real-time sections (rt_section, rt_defer, rt_overflow; decision 0133) and fiber stacks mapped now and at most at once (fiber_live, fiber_peak; 2026-10-13, gotchas/linux-tsan-fiber-stacks.md) and futex calls and slow lock paths by lock (futex_wait, futex_wake; lock_inject, lock_pd, lock_pool, lock_park, lock_http, lock_chan, lock_mutex, lock_task; 2026-10-21, decision 0301's measurements: count a server's futex calls without strace, which slows every system call and makes workers idle far more often) and wakep's calls by site (wake_chain, wake_spread, wake_push, wake_mon), an idle worker that waited in the poller or could not (idle_npblock, idle_nopoll), and park signals by waker (sig_ready, sig_mutex, sig_chan, sig_task, sig_h2; 2026-10-22, the 6 core churn's attribution). Last verified: 2026-10-22
XO_PREEMPT=0 ./prog                                    # no worker is asked to yield (decision 0286 step 3): the loop polls of the program's own code stay and never fire; the default asks a worker that ran one fiber 10 ms while work waits to yield at its next loop iteration (arm64, native and LLVM). Last verified: 2026-10-21
XO_DIRECT=0 ./prog                                     # a park goes through the scheduler (decision 0394; the default hands off straight to the fiber its wake queued)
xo build --no-preempt ./prog                            # loops without preemption polls (decision 0286 step 3's opt out; XO_PREEMPT_POLLS=off does the same, for build, run, and test): the loop rows' speed without the poll's load and branch per iteration (sieve +25% with polls, 2026-10-21); a task that loops without calling the runtime can then starve its worker, so for benchmarks, not for servers. XO_PREEMPT=0 alone does not remove the polls' cost. Keyed in the front end, function, and part caches (nopreempt=). Last verified: 2026-10-21
XO_ASYNC_PREEMPT=0 xo build ./prog                       # the native backends (arm64 and amd64 on darwin and linux, and windows/amd64 by suspending the thread) preempt with no loop polls by default (decision 0318: the monitor signals a worker past its slice, the handler yields unless the PC is in the runtime or runtime code inlined into the program, xo_unsafe_* tables); this keeps the loop polls instead (decision 0286 step 3). The LLVM backend preempts by signal too (decision 0318 stage 4: runtime functions that reach a thread's own structures hold a per-fiber no-preemption count, the program's functions in their own xo_text section). --race always polls; --no-preempt turns both off. Keyed in the front end, function, and part caches (async=). Last verified: 2026-10-21
sample <pid> 5 -mayDie -file prof.txt                 # native server under load: macOS sampling profiler, no root; read "Sort by top of stack" (idle threads show as __ulock_wait and kevent); the llvm build has Xo function names (x.<fn>), arm64 folds some into xo_entry (last verified 2026-10-09)
clang -O1 -g -fsanitize=thread std/llrt/testdata/rc_threads.c -o /tmp/rct && /tmp/rct   # the C stress test by hand (add -isysroot <SDK> when the default SDK does not link)
go test ./internal/arm64 -run TestGoldenAsm -update     # rewrite the golden assembly internal/arm64/testdata/{fib,sieve}.{darwin,linux}.s (the programs' own functions only, since 2026-10-14; a runtime change no longer rewrites them)
go run ./cmd/xo build --backend=arm64 --targets=linux/arm64 .   # emit <build dir>/arm64/prog-linux.s (a build that stops at the assembly keeps it there; linked only on a linux/arm64 host)
XO_RANGE_SEEDS=25 go test ./internal/gen -run TestRangeDifferential   # slow: more random programs, range analysis vs all checked
XO_LIST_SEEDS=40 go test ./internal/gen -run TestListModel   # slow: more random List value semantics programs vs a deep copy model (XO_LIST_DUMP=<dir> keeps them)
XO_COMPILE_PROPS=1000 go test ./internal/gen -run TestCompileProperty   # every checked program builds (decision 0110): random programs over the optional syntax, built on Go and --no-gc and run alike (24 with -short, 150 in the full suite, about 4 min for 1000; XO_COMPILE_PROP_SEED=S one seed, XO_COMPILE_PROP_KEEP=<dir> keeps them; last verified: 2026-10-09)
go run ./scripts/stdsync [-check]                     # every file a std declaration change derives: std/rt's std_gen_M.go and std_sigs_M.go, spec/std.md's prelude index, and the habit goldens when std/prelude.xo's declarations changed (cmd/xo/testdata/habits/prelude.sha256 records what they were made from); -check names each stale artifact and prints this command (2026-10-24)
go run ./scripts/stdgen [-check]                       # regenerate std/rt/std_gen_M.go and std_sigs_M.go, one pair per std module, from std/*.xo, removing any left over (-check: exit 1 when one is stale, missing, or left over; its message names scripts/stdsync; 2026-10-21)
go run ./scripts/gentuples [-check]   # writes std/rt/tuples_gen.go: the Go backend's Tuple7 to Tuple64 (Tuple2 to Tuple6 are hand written in core.go), each with XoMark, XoDebug, XoEq, XoHash, XoCmp, XoJsonEnc, XoJsonDec; its Max equals compiler/gen GO_TUPLE_MAX, and a wider tuple is a Go backend error naming the limit (core sets none; the native backends have none). internal/repotest TestTuplesGenCurrent (last verified: 2026-10-21)
go run ./scripts/stdindex                              # rewrite spec/std.md Appendix A alone, the prelude index (every function and method of std/prelude.xo by type, internal/stdindex), after changing the prelude; internal/repotest TestStdIndexCurrent (-short) fails while it is stale, and its message names scripts/stdsync, which does this and the rest (last verified: 2026-10-24)
go run ./scripts/stdcoverage                           # print every std function and method no corpus or golden calls (internal/stdcoverage: std/*.xo and std/native/prelude.xo against internal/stdtest/testdata/corpus, internal/llvm's and internal/arm64's goldens, internal/gen's goldens and cmd/xo's testdata, following std's own calls and the prelude lowering); -check exits 1 when internal/stdcoverage/allow.txt does not name a declaration with its reason, and the -short TestStdAPICovered runs the same check (2026-10-24)
go run ./scripts/sizebreak [-top=N] [-json] [-reach] BINARY   # where a built binary's code goes: every text symbol sized by the gap to the next, checked against the __text size (fails if they differ), grouped by module from the _xo_f_<module>_2e<name> mangling with the Map families (xo.map, xo.mlog, xo.maph) broken out per element type, plus hidden _xo_h* and non-Xo symbols; Mach-O and ELF; -reach adds a lower bound on unreachable code (arm64, direct branches and materialized addresses). A size claim quotes its __text and group table (2026-10-24)
go run ./scripts/stdfuzz                               # seeded random inputs through pure std functions (strings, bytes, strconv, json, runes, path, url, base64/base32/hex, time, regexp, csv, netip, big, compress, archive, mime, multipart, xml, sql, http, template, the internal.crypto_op paths for cryptostream, x509, ECDSA and RSA, a limits area at the chunk and buffer edges up to 1 MiB, a runes area over every Rune predicate, case mapping, utf8_len and quote at the class, block, bitmap, surrogate and scalar-range edges, and a maps area of put, get, remove, contains, len, update and re-put over six key and value type pairs with growth cases to 100,000 entries, printing the iteration order because a Map is insertion ordered) on arm64, LLVM and the Go backend, comparing stdout line by line; it prints each divergence with the case name, the case's source and a -only=NAME rerun, and exits 1; -seed, -n (cases per area), -area and -keep=/DIR (keep the generated program) tune it, and -release builds the arm64 and LLVM programs with `xo build --release` and runs the binaries (the Go backend stays a `xo run` oracle), so the release driver's contents, the DWARF readers left out and the hollow pass that empties unreached functions, are compared too. A difference is a finding to report, not to fix there (2026-10-24)
go run ./cmd/xo lexdump [--bench=N] <files...>              # hidden: canonical token dump of the Go lexer (or lexing time), same text as compiler/lexer
go run ./cmd/xo run compiler/lexer -- [--bench=N] <files...>   # the self hosted (Xo) lexer: same dump or timing
go run ./cmd/xo build --backend=arm64 -o .tools/xolex compiler/lexer   # native build of the Xo lexer (also --backend=llvm)
go run ./cmd/xo test compiler/lexer                     # the Xo lexer's own tests (20, 6 of them property tests)
go run ./cmd/xo astdump [--sig] [--bench=N] <files...>     # hidden: canonical syntax tree dump of the Go parser (ast.Text + diag lines) or parse time, same text as compiler/parser for accepted files (decision 0127)
go run ./cmd/xo run compiler/parser -- [--sig] [--bench=N] <files...>   # the self hosted (Xo) parser: same dump (with the lexer's diagnostics and soft errors, habits included; after a hard error only the diagnostics up to it) or timing
go run ./cmd/xo build --backend=llvm --no-gc -o .tools/xoparse compiler/parser   # native build of the Xo parser (also --backend=arm64)
go run ./cmd/xo test compiler/parser                    # the Xo parser's own tests (9)
go test ./internal/selfhost -run TestSelfhostParser     # Xo parser (go, arm64, llvm backends; arm64 and llvm at the -short sizes in the merge tier) vs Go parser: identical dumps (tree and diagnostics) for every file the Xo parser parses to the end, the files whose Go diagnostics are all habits among them, same rejections: repo .xo, std/*.xo (--sig), spec blocks, progen programs (24/150), token mutations (300/2000), habit edits of repository files (a quarter as many); XO_SELFHOST_PROGS=N, XO_SELFHOST_PARSE_FUZZ=N, XO_SELFHOST_SEED, XO_SELFHOST_KEEP (last verified: 2026-10-10)
go run ./cmd/xo typedump [--section=names,types,sigs,diags] [--each] [--native] [--bench=N] <dirs...>   # hidden: canonical dump of what the Go checker resolved (decision 0141): names, expression types, signatures and effects, diagnostics as JSON; --each checks every dir alone; --native checks as a native build (std/native loaded); run from the repository root with relative dirs (gotchas/typedump-paths-depend-on-working-directory.md)
go run ./cmd/xo run compiler/check -- [--section=names,types,sigs,diags,full] [--std=DIR] [--wd=DIR] [--each] [--native] [--bench=N] <dirs...>   # the self hosted (Xo) checker, stages 1 to 8 and module loading: the sections of xo typedump (default all), or timing; --native loads and checks std/native as a native build does (decision 0150); std read from DIR (default std); --wd is the absolute working directory (default $PWD); reads XO_MODCACHE, XO_OFFLINE, XO_WORK like internal/mod but never fetches (a version missing from the cache while online makes the run unsupported; last verified: 2026-10-10)
go run ./cmd/xo build --backend=llvm -o .tools/xocheck compiler/check   # native build of the Xo checker (also --backend=arm64)
go run ./cmd/xo test compiler/check                     # the Xo checker's own tests (13: paths, JSON, names and types of small modules in the test file system, versions, ceilings, deny rules, Go binding headers, vouches, effect locks; last verified: 2026-10-12)
go test ./internal/selfhost -run TestSelfhostChecker    # Xo checker (go, arm64, llvm in the full tier; go otherwise) vs Go checker: identical names, types, and sigs sections on every accepted repository run, the same diagnostics on runs rejected only with diagnostics of stages 1 to 6 and the parser's (stageDiag, parseDiag), 1000 name, type, generics, effects, habit, value semantics, and pattern mutations (150 with -short, go backend, or XO_SELFHOST_BACKEND=arm64|llvm), 400 module mutations (TestSelfhostCheckerModuleMutations: ceilings, deny rules, versions, xo.lock and vendor/modules.txt lines, added `use` lines over the fixtures internal/selfhost/testdata/modules/app and the golden effect_ceiling_work, effect_ceiling_vendor, imports_deny; 40 with -short; XO_SELFHOST_MOD_FUZZ=N); both checkers run with XO_MODCACHE=internal/selfhost/testdata/modules/cache, XO_OFFLINE=1, XO_WORK=""; XO_SELFHOST_CHECK_FUZZ=N, XO_SELFHOST_SEED, XO_SELFHOST_KEEP=<dir> (writes check-<backend>.txt, the class of every run; do not point it inside the repository: the run directories it writes are .xo modules the next walk finds; last verified: 2026-10-10); `use go` runs read the binding the Go checker writes first (gobind cache, $XO_GOBIND_DIR; TestSelfhostCheckerHatches binds what is still missing with xo-gobind, as xo1 does, so it also replays) and runs with `use c` are compared again with --native (decision 0177); TestSelfhostCheckerHatches checks a copy of internal/selfhost/testdata/hatches (escape hatches, vouches, the effect lock, XO1102; skipped with -short; last verified: 2026-10-12)
go test ./internal/selfhost                             # merge tier (the default; tiers below). Xo lexer (built with the go, arm64, and llvm backends) vs Go lexer byte for byte: repo .xo files, spec blocks, runes, 3000 mutated inputs (300 with -short, and for arm64 and llvm in the merge tier); XO_SELFHOST_SEED, XO_SELFHOST_FUZZ=N, XO_SELFHOST_RUNE_STEP=1 (all runes), XO_SELFHOST_KEEP=<dir> keeps the inputs; one backend: -run 'TestSelfhostDifferential/arm64'
                                                        # tiers (internal/selfhost/tier_test.go, XO_SELFHOST_TIER; 2026-10-11, GOMAXPROCS=4, -p 1 -parallel 2, another full suite running on the machine):
                                                        # XO_SELFHOST_JOBS=N (default min(GOMAXPROCS, 4); scripts/remote-full.sh: the core count) sizes one pool (internal/selfhost/jobs_test.go) that every native build, every Xo process on a batch of runs, and every Go side comparison item takes a slot of; XO_SELFHOST_HEAVY_JOBS=N (default one per 8 GiB of memory, one per 16 GiB in the full tier, at most XO_SELFHOST_JOBS: 3 on the 24 GB Mac, 15 in the full tier on r8g.8xlarge) also bounds the builds and the mergeHeavy runs, which get a process each, largest first (bytes of .xo source with the repository packages they use); backend subtests run at once and overlap the Go side results; the full tier with no -run starts every component's native builds at TestMain; XO_LLVM_JOBS (the clang compiles of the process's split LLVM builds) defaults to the pool size, at least 4, at most one per 2 GiB. Expected peak per unit (go -v logs each batch's longest time and peak RSS): a light batch 40 to 300 MB, a Go side item under 1 GB, a mergeHeavy Xo process 0.4 to 2 GB in the merge tier (huge_frame build only 1.9 GB, compiler/xo checker 0.9 GB, generator 1.0 GB) (compiler/llvm: SSA builder with stages 4.7 GB, LLVM emitter 5.9 GB) and an Xo arm64 emitter process on a self hosted component 5 to 15 GB in the full tier, a clang on a split part about 1.1 GB; whole merge tier process tree peak 4.7 GB at the default, 2.7 GB at XO_SELFHOST_JOBS=1 (2026-10-12; bench/results/selfhost-parallel-2026-10-12.md). Last verified: 2026-10-12
                                                        #   short (-short): go backend (lexer and parser also arm64 and llvm), small corpora, the SSA builder and LLVM emitter on every 25th run but the mergeHeavy ones; 31 to 52 s on the Mac under load (2026-10-12; 175 to 208 s before, when the sample held compiler/llvm: gotchas/selfhost-heavy-runs-memory.md)
                                                        #   merge (default without -short): go backend at full size (every run, every pass stage but those of the mergeHeavy runs, 1000 checker and 400 module mutations, 300 SSA mutations), the arm64 emitter on every second run and the LLVM emitter on every third (30 mutations each), both without the mergeHeavy runs (tier_test.go: huge_frame and every compiler/ component but the lexer and the parser), arm64 and llvm lexer and parser at -short sizes, no native build of a self hosted component, no LLVM end to end test; 139 s warm before the arm64 and LLVM emitter comparisons (2026-10-11). Before/after of 2026-10-12: 1,329 s (load average 11.8 at the start, 5.4 at the end; binary cache warm; TestSelfhostArm64 421 s with its go subtest 295 s, TestSelfhostLLVM 73 s with its go subtest 115 s, TestSelfhostSSA 115 s with its go subtest 492 s, TestSelfhostSSAMutations 42 s, TestSelfhostLLVMMutations 5 s, TestSelfhostArm64Mutations 4 s, TestSelfhostLLVMEndToEnd skipped, the rest under 20 s each) because the fixed heavy lists missed the new components compiler/arm64, compiler/llvm, and compiler/xo; a per run sweep (4 runs at a time, load average 6 to 20) put each of compiler/arm64, llvm, ssa, and xo at 870 to 1,150 s in the Xo arm64 emitter and 200 to 290 s in the Xo SSA builder's stages, against 82 s (arm64 emitter), 77 s (SSA stages), and 73 s (LLVM emitter) for the other 663 runs together; after mergeHeavy and the job pool (2026-10-12, test binary run directly, other agents loading the Mac, binary cache warm; bench/results/selfhost-parallel-2026-10-12.md): 233 s at the default and 468 s at XO_SELFHOST_JOBS=1 with the closure renamer and parallel comparisons (commit 73feab0; TestSelfhostSSA/go 53 s against 157 s); before them 281 s at the default (4 jobs, 3 heavy; TestSelfhostSSA/go 106 s, TestSelfhostSSAMutations 40 s, TestSelfhostWasm 23 s, TestSelfhostGen/go 19 s, TestSelfhostArm64/go 15 s, TestSelfhostLLVM/go 11 s) and 476 s at XO_SELFHOST_JOBS=1 (TestSelfhostSSA/go 196 s, SSAMutations 67 s, Arm64/go 45 s, Gen/go 45 s, LLVM/go 31 s); (the lead times the merge tier on the combined main; target 2.5 to 4 min idle); -timeout 30m is plenty
                                                        #   full (XO_SELFHOST_TIER=full): every backend, every corpus, every pass of every run, 400 SSA mutations; 3,490 s cold on r8g.8xlarge with XO_SELFHOST_JOBS=32 (scripts/remote-full.sh, peak memory in use 157 GB; critical path the arm64 comparison of compiler/xo after the llvm build of compiler/arm64; 2026-10-12, bench/results/selfhost-parallel-2026-10-12.md; over 3 hours before the job pool); about 11 min warm (owner, 2026-10-11, not remeasured); 3,474 s (58 min) cold, measured 2026-10-11 with the native compiler/check and compiler/ssa binaries uncached and another full suite running (TestSelfhostChecker 589 s and TestSelfhostSSA 2,366 s, mostly their native builds; the LLVM link of compiler/ssa alone is about 10 min); use -timeout 120m (raised from 90m on 2026-10-11: 58 min cold under load left about 30 min of margin). Run on demand through CI (`gh workflow run ci.yml -f suite=full -f runner=xo-arm64-big`, ci.md, decision 0225) and before tagging a release; run it by hand for a change under compiler/ or the compiler's SSA or native backends that the merge tier does not exercise (a construct only compiler/ssa or compiler/check uses, an arm64 or LLVM lowering)
                                                        # since SSA stage 8 (2026-10-11): TestSelfhostSSA's full tier compares every stage after the build, the verifier and its mutants on three backends (about 32 minutes of a cold run); the first cold run timed out at -timeout 40m building the LLVM binaries, the next (partly warm) took 2,264 s
                                                        # time of the full package before SSA stage 8 (2026-10-11, GOMAXPROCS=4): about 160 s with the binary cache warm (was 329 s), most of it TestSelfhostSSA (51 s) and the mutation tests; a cold cache (any change to the compiler) adds the builds of compiler/check and compiler/ssa with three backends, now built at the same time (they were one after another: about 740 s of the 1510 s cold run). The backends' subtests, the Go checker and SSA builder over every run, and the batches of the Xo binaries run in parallel (up to min(GOMAXPROCS, 4), `parallel` in lexer_test.go); coverage is unchanged
scripts/selfhost-record-goldens.sh [--merge]            # records the Go side of every TestSelfhost* differential at the full tier's sizes (no Xo binary built) as hashes into $XO_SELFHOST_GOLDENS_DIR (default internal/selfhost/testdata/gogoldens, committed): per unit one line per whole dump, list, meta value, or family (the functions, helpers, files, declarations of a unit: member count and root hash), the member hashes once per kind in pool-<x>.txt; units keyed by kind, options, and input content (decision 0178 stage 7, format in internal/selfhost/gogoldens_test.go): 20,034 units, 6,405,857 bytes in 208 files (2,573,573 gzip -9), 1,789 s, max RSS 36.6 GB (GNU time, the whole script) on an r8g.8xlarge on demand, Linux arm64, 2026-10-13 (main 2c63a802; `scripts/remote-full.sh --on-demand --put rec.sh --cmd 'bash ~/rec.sh'`, the recording returned as base64 tar in the log); the run kinds key on the std tree too, so a std change shows as units to record again; the native run kinds (ssadump, asmdump, llvmdump) key on the recording host's GOOS/GOARCH, so a Linux recording replays fully on Linux only (on darwin/arm64: 96 units missing; record those on the Mac with RUN and --merge); RUN='<regexp>' limits the tests. Last verified: 2026-10-13
XO_SELFHOST_GOLDENS=replay go test ./internal/selfhost  # the TestSelfhost* tests hash the Xo side's pieces and compare them with the recording instead of running the Go side (any tier); a unit with no recording fails the run and is named (inputs changed: record again), a piece that differs fails its test and is listed at the end with how to see both dumps; TestSelfhostCheckerHatches binds `use go` with xo-gobind as xo1's checker does; XO_SELFHOST_GOLDENS_DEBUG=<text> prints the key of the units whose label holds it. Same verdicts and logged counts as live on 2026-10-12 (-short: 121 s against 214 s; merge tier, the 16 differentials: 255 s against 373 s, recording of d9a9c2c; -short again on the merged tree f786e42: identical, 30 s against 39 s)
go test -short ./...                                    # 15 to 19 s on the shared Mac at the default parallelism (35 s before 2026-10-14; ./cmd/xo alone 4 to 6 s, was 25 s; GOMAXPROCS=4 -p 2 -parallel 2 takes 45 s; bench/results/short-tests-2026-10-14.md; last verified: 2026-10-14): skips race, cross compile, CLI build, replay, the --no-mono and --no-arenas golden lowerings, and the notes-service arena differential; use per commit, run the full suite before merging. The tests keep caches across runs: XO_BIN_CACHE (<user cache>/xo/testbin), XO_CC_CACHE (<user cache>/xo/testcc, internal/llvm's C compiles, xotool.EnableTestCCache, and xo's own clang runs, xotool.EnableTestClangCache), <user cache>/xo/testgobind/<key> (internal/gen's `use go` bindings, xotool.TestGobindDir), <user cache>/xo/testprog (content keyed program directories, xotool.TestProgramDir); the last three remove entries unused for XO_CACHE_MAX_AGE (default 24 h, decision 0223). cmd/xo's tests are all parallel: set a test's environment and directory with setenv(t, ...) and chdir(t, ...) (cmd/xo/env_test.go), never t.Setenv, t.Chdir, or os.Setenv
scripts/linux-test.sh [arm64|amd64] [go test args...]  # the suite in a Linux container (Docker; see "Linux (containers)"): arm64 vet plus the full suite natively, amd64 -short plus internal/llvm under emulation
bench/nightly.sh                                        # full suite (-timeout 120m) with XO_SELFHOST_TIER=full XO_RANGE_SEEDS=25 XO_LIST_SEEDS=40, -race on std/rt, vet, then TestArithProperty with XO_ARITH_PROPS=500 from a seed that moves every day (status line `arith=`); log in bench/results/nightly/DATE.log (gitignored), one line per run in summary.txt; schedule with cron (not installed)
go test -race ./std/rt -run 'TestMutex|TestSched'      # runtime lock and scheduler under the race detector (Mutex.with_read readers, writers, cancellation; last verified 2026-10-08)
go test ./std/rt -run '^$' -bench . -benchmem           # runtime micro benchmarks (Map put/get/update/iter/copy/fork, Set, Eq, Hash)
go test ./std/rt -run '^$' -bench 'Order|Frames' -benchmem   # spec audit benchmarks: Map iteration order (decision 0091) and error frame cost (decision 0092: eager before, impl as emitted now, FramesPrint the cost at print time); about 90 s (last verified 2026-10-09)
go test ./internal/diag -run 'TestSpecLists'           # core 10.6 lists exactly the codes of internal/diag/codes.go and the XO-F fault codes the runtimes raise
python3 bench/harness/score.py solutions --claude        # rescore the stored agent benchmark outputs with the current toolchain (bench/harness/README.md)
python3 bench/harness/run_openai_compatible.py --out runs/<model>   # rerun the agent benchmarks with any OpenAI compatible model
bench/perf/run.sh                                       # Xo vs Go collection, List write, and generic workloads (see Benchmark tooling)
MONO=1 bench/perf/run.sh                                # Xo --no-mono and Xo (default, --mono) columns
bench/perf/http/run.sh                                  # task 01 HTTP load: Go vs Xo --no-arenas (config xo) vs Xo with arenas (config xo-arenas, the default build; see Benchmark tooling)
bench/perf/llvm/run.sh                                  # compute and string workloads: Go vs Xo (Go backend) vs Xo --backend=llvm (RSS=1: peak memory table)
bench/perf/llvm/native.sh                               # same workloads: Go vs Xo --backend=llvm vs Xo --backend=arm64, time and peak RSS (arm64 host)
bench/perf/hybrid/run.sh                                # Xo (Go backend) vs xo build --hybrid vs --backend=arm64: median wall ms and cycles of 7 interleaved runs (arm64 host, decision 0084)
bench/perf/conc/run.sh                                  # concurrency: Go vs Xo (Go backend) vs llvm vs arm64 (and arm64 XO_THREADS=1); medians of wall, CPU, cycles, interleaved (see Benchmark tooling)
python3 bench/perf/conc/profile.py p.trace 40 ./bin     # summarize an xctrace CPU Profiler trace by function (PROFILE_FUNC=name: hot offsets in one function)
go test -run '^$' -fuzz=FuzzParse -fuzztime=60s ./internal/parser/   # optional fuzzing
go test ./internal/check -run TestGolden -update          # rewrite golden diagnostics after an intended change
XO_CLI=/abs/xo go test ./cmd/xo -run TestHabitGolden [-update-habits]   # the habit hints of decision 0087 and compiler/check habits.xo against the shipped xo: each case in cmd/xo/testdata/habits (in.xo, want.json from `xo check --json`, fixed.xo after `xo fix`, which must check); -update-habits rewrites them, review the diff (moved from internal/fix, deleted by decision 0215, 2026-10-23)
python3 -I bench/habits/measure.py .tools/bin/xo cmd/xo/testdata/habits golden   # habit cases: errors, fixes, green after xo fix (also snap and drafts modes; last verified 2026-10-09)
go test ./internal/check -run TestCorpusV03 -corpus-report # rewrite the generated counts in bench/results/check-corpus-v0.4-spec.md
go test ./internal/check -run TestCorpusMutations -mutations -v   # slow: plant mutations in the corpus, count detections
.tools/bin/xo check examples/notes-service && .tools/bin/xo test examples/notes-service && .tools/bin/xo build --targets=all examples/notes-service   # example app; not run by go test (last verified 2026-10-08)
```

`./...` is safe since `bench/go.mod` made `bench/` its own module (see
`gotchas/root-module-absorbs-bench-go.md`). Last verified: 2026-10-07.

`xo parse` walks directories for `.xo` files, prints diagnostics in a
`file:line:col: error XOnnnn: message` form on stderr (plus a summary line),
and exits 1 if any error was reported, 2 on usage errors. `--json` prints
`{"files": [...], "diagnostics": [...]}` on stdout with diagnostics in the core
10.2 shape (notes and fixes are always arrays). `--ast` adds each file's syntax
tree (every node has `kind` and `span` "line:col-line:col"). `--sig` accepts
function declarations without bodies (spec/std.md style signature listings).

`xo check` treats every directory under the given directories that holds
`.xo` files as a module (`testdata` and dot directories are skipped), loads
the user modules they import through the nearest `xo.mod` (`module <path>`:
`use <path>/<dir>` maps to `<xo.mod dir>/<dir>`), and loads std modules from
the embedded declaration files in `std/`. It prints `file:line:col: severity
XOnnnn: message` lines (with notes and fix titles) and a summary on stderr, or
with `--json` `{"modules": [...], "diagnostics": [...]}` on stdout. Exit status
is 1 when any error is reported (warnings such as XO0900 holes do not count),
2 on usage errors.

`xo fmt` (2026-10-08, `internal/format`) prints each file from its syntax
tree in the one canonical layout (rules at the top of
`internal/format/format.go`): 2 space indentation; one statement,
declaration, member, or arm per line; blank line runs collapse to one; a
bracketed list is split one element per line with a trailing comma when
the source breaks the line right after the opening bracket, else joined
on one line without one; a body stays on one line only if it was on one
line with at most one statement (struct, enum, and interface bodies with
any number of members); breaks the grammar allows mid expression are kept
(before `.` in a chain, after a binary operator, after `=>`, before `->`
and each clause of a function header), with continuation lines one level
deeper; `} else {`, `} invariant`, `} derive` on the brace line;
comments kept, trailing comments of consecutive lines aligned; literals
copied byte for byte. It writes changed files in place and prints their
names; `--check` changes nothing, lists unformatted files, and exits 1
when there are any; files with syntax errors are reported and skipped
(exit 1). Dot directories are skipped. Files that only parse as signature
listings (`std/*.xo`) are formatted in that mode. Tests:
`internal/format` `TestLayout`, `TestGolden` (`testdata/tricky.xo`),
`TestSpecExamples` (every core.md example that parses), and `TestCorpus`
(temporary copies of bench/xo, bench/paper, internal/check/testdata,
internal/gen/testdata, and std: each file must parse to the same tree
without spans, comments are kept, a second format is a no op, and each of
110 programs must type check with the same diagnostics, code, severity,
and message, before and after). (`xo build --ci` exists since 2026-10-09.)

`xo vet` (2026-10-08, `internal/vet`) checks like `xo check` and reports,
for the root modules, only warnings and notes (exit 0; check errors are
counted on stderr, `check_errors` in JSON): the check warnings (XO0900,
XO0305, XO0804) plus XO0703 (an effect written in a `uses` clause that
the body never performs; a sub effect counts for its parent and the
parent for a sub effect; implicit effect variables are never reported),
XO0207 (`x ?? [].f()`: a selector chain on the right of `??` whose base is
a literal, with a fix that parenthesizes the `??`), XO0208 (note: every
prelude `.ignore_err(reason)` call with its reason and the Result type),
XO0538 (warning, 2026-10-21: a `let f = fn(..)` closure reading a `var` local that a later statement writes before `f` is used again, the closure having copied it), XO1103 (note, one per module: `use go` packages and the functions whose
performed effects include `go`), XO0901 (`dbg(...)` with a fix that keeps
the argument). Human output goes to stdout, the summary to stderr.

`xo fix` (2026-10-08, `internal/fix`) first runs the migration rewriters
selected by `--from=draft-0.N` (also `0.N`, `v0.N`; every rewriter whose
change came in a later draft), then loops: check the sources through
`check.CheckOverlay` (rewritten contents in memory, nothing written yet),
apply the first fix of every diagnostic whose edits do not overlap one
already taken this round (a fix is all or nothing; two inserts at the same
point conflict), and repeat until a round applies nothing, the sources
return to an earlier state, or 25 rounds. Changed files are reformatted
with `xo fmt` when they parse, then written (not with `--dry-run`). Output:
one `file:line: rule: title` line per change and a summary; `--json` gives
`{dry_run, changes, files, rounds, remaining}`. Exit 0 even when errors
remain (listed on stderr), 2 on usage errors. Rewriter registry
(`fix.Registry`, run in order): `variant-case` (0.2: `ok( err( some(` and
bare `none` to `Ok Err Some None`; skips `.ok()`, `fn ok(`, `let err`, and a
variable `err` not followed by `(`), `scope-timeout` (0.2: `scope s timeout
d { b }` to `task.with_timeout(clock, d, fn() { scope s { b } })` plus `use
std/task`, only in files that name `clock`; elsewhere reported as skipped),
`interp-dollar` (0.3: `{x}` to `${x}` in plain and triple quoted strings
when the content starts with a letter, digit, `_`, or `(` and holds no
quote), `discard-reason` (0.4, the XO0206 fix), `map-get-or-fault` (0.6,
the XO0410 fix added to the checker: `m[k]` to `m.get_or_fault(k, "TODO:
why the key must exist")`). Code based rewriters are applied by the loop
anyway; the report credits them as `name (XO0nnn)`.

Measured 2026-10-08 on copies of `bench/paper` (originals untouched),
modules with no check errors before and after `xo fix --from=<draft>`:
v0.1 0/5 to 0/5 (135 errors to 36; the rest are API drift such as
`std/test`, `http.Server.handle`, `?` on `Fs`), v0.2 0/5 to 4/5
(04-file-sync keeps 9 XO0401 `?` on `Fs`), v0.3 5/8 to 8/8.

Editions (2026-10-08, renamed 2026-10-11 by decision 0155): `edition
<name>` in xo.mod is recorded on `check.Module.Edition` (a string;
`check.CurrentEdition` is `preview`, used without the line or without an
xo.mod). `check.EditionAliases` maps old names to editions (`2026` to
`preview`); `check.ResolveEdition` resolves both. An edition outside
`check.Editions` and the aliases, or a malformed line, makes `xo check`
fail with a usage error (exit 2) naming the xo.mod. An alias builds and
gets the note XO1315 once per xo.mod of a checked (non dependency)
module, with a fix that writes the edition name. `findModRoot` reads
xo.mod through the check overlay, and `fix.Run` loads the xo.mod files
under its dirs plus the one of the module around each dir into the
overlay, so the fix loop applies xo.mod fixes (not reformatted, and the
text rewriters skip them). `xo init` writes `edition preview`.
The edition names live in internal/mod (`mod.CurrentEdition`,
`mod.Editions`, `mod.EditionAliases`, `mod.ResolveEdition`; the check
names wrap them) so that `mod.ReadWork` resolves the `xo <edition>` line
of xo.work with the same resolver: an unknown name is XO1301 on that
line, the alias sets `Work.Edition` to `preview`, and the checker
(`workEditionAlias`, after `mod.Load`) reads the xo.work through the
overlay and gives XO1315 on `xo 2026`; `fix.Run` loads the xo.work that
applies to each dir (`mod.NearestWork`) too. The self hosted checker
(compiler/check modfile.xo) gives the same XO1301. Last verified:
2026-10-11.
Rules that change at an edition boundary are entries of `editionRules`
(`internal/check/edition.go`, `Since` edition plus a per file check), run
after `checkModule` for modules with `Module.AtLeast(Since)` (order is
the position in `check.Editions`); the list is empty for `preview` and
`TestEditionRuleHook` exercises it with a fake `next` edition.
`xo fix --edition=<name>` only validates the name (`preview` or the
alias `2026`) until a second edition exists.
Last verified: 2026-10-11.

`xo query` (2026-10-08, `internal/query`) checks `--dir` (default `.`; for
`type`, also the file's directory when it lies outside) and answers:
`type file:line:col` (innermost typed expression or binding pattern; the
natural type too when an implicit conversion applied), `def <name>`
(`name`, `Type.member`, `module.name`, `module.Type.member`, `module` a
local name such as `http` or an import path; functions, methods, statics,
types, constants, fields, variants; signature printed by
`format.Decl`), `callers <fn>` (calls and uses as a value in user
modules, with the enclosing declaration), `effects <fn>` (declared or
inferred set, and each performed effect with its call chain, the
XO0701 note chain, `check.Info.Performed`), `holes` (the XO0900
diagnostics), `api <module path or dir>` (pub declarations outside test
files; struct signatures show only pub fields and no invariant). JSON is
the default when stdout is not a terminal (`--text` forces text);
unknown names exit 1, usage errors 2.

`xo edit` (2026-10-08, `internal/edit`, decision 0044) checks `--dir`
(default `.`), resolves the target with `query.Session.Objects`, collects
every reference from the checker's records (`Info.CallOf`, `Info.Ref`,
`Info.TypeOf` of struct literals, `Info.VarOf` of parameters), applies the
edits in memory, rechecks with `check.CheckOverlay`, and only then writes
all files (temporary file plus rename per file). Files that were in
canonical layout are reformatted. Always prints the JSON summary `{op,
target, ok, written, reason, files, edits, diagnostics, sites}`; exit 0
when applied (or checked with `--dry-run`), 1 when rejected (nothing
written), 2 on usage errors (unknown name, bad signature). Limits: rename
covers functions, methods, and constants, not types, fields, or interface
methods; change-sig takes no clauses (`uses`, `requires`).

`xo doc` (2026-10-08, `internal/doc`) prints Markdown for the root modules
under a directory (default `.`) or one module by import path (`--dir`):
an index, then constants, types (methods under their type), and
functions, each with its signature in an `xo` block, its doc comment
(example fences without an info string retagged `xo`), the number of
examples, and its position. `--all` includes non pub declarations,
`--json` emits the same data with `examples` (the bodies of the fenced
examples). Running the examples (core 1.2: `xo test`) is not implemented.

`xo explain <code>` (2026-10-08, `internal/diag/explain.go`) prints the
long form explanation of a code (`XO0201`, `xo0201`, or `0201`): title,
severity, detail, an example that triggers it, and the fix;
`TestExplanationsComplete` requires an entry for every code in
`codes.go`, and every example was run through `xo check` when written.
Last verified: 2026-10-08.

Diagnostic positions: lines and columns are 1 based, columns count bytes, and
spans are half open (`end_col` is one past the last character), which matches
the insertion fix example in core 10.2.

`xo build` checks (errors and holes block the build), generates Go, and runs
`go build -trimpath` with `CGO_ENABLED=0` per target. A program that binds
a Go package with callback adapters (`NewGoCallback` in an
`rt/xo_gobind_*.go`) also gets `-tags=xo_gotimers
-ldflags=-checklinkname=0`, which link `runtime.timeSleepUntil` into std/rt
for deadlock reports through Go calls (decision 0073); `XO_GOTIMERS=off`
drops them, and a failed link of that symbol is retried without them with
one warning line (`xo build: warning: deadlock detection through Go
callbacks is off ...`; gotcha `go-runtime-timer-linkname.md`; Last
verified: 2026-10-08 with Go 1.26.1). Every Go backend build also passes
`-gcflags=all=-dwarf=false` and `-ldflags=-s -w` (merged with the
`-ldflags` above): no DWARF in the cached objects (a third of each) or the
binary, and no symbol table. Xo's stack traces, faults, crash bundles, and
`xo dump` read the pclntab through the Go runtime, which stays; only a
native debugger (Delve) needs DWARF, and `XO_GO_DWARF=1` keeps both. A
program (not a test binary) that does not import std/http, directly or
through std modules, is built with the tag `xo_nohttp` (gen.Program.Tags),
which swaps the real server and client in `std/rt/http_net.go` for the stubs
in `http_nonet.go`, so the binary does not link Go's net/http (hello world
3.6 MB instead of 4.9 MB; bench/results/binary-size-2026-10-09.md; Last
verified: 2026-10-09). It prints one
`os/arch: path` line per binary. `--targets=all` is linux/amd64,
linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64, and
`wasi/wasm32` (Go's wasip1/wasm) with --backend=go; with no --backend
`all` is the six a native backend builds, each with its own (main.xo
backend_groups and build_by_backend: arm64, amd64, wasm), and a note
says windows/arm64 needs --backend=go (decision 0270 addendum, owner
2026-10-22); windows/arm64 named in a list with no --backend is an error
after the rest are built (exit 1). Binaries go
to `-o` (a file for one target, a directory for several, each
`NAME-OS-ARCH[.exe|.wasm]`) or the build directory's `bin/`; `-o NAME`
for windows with no extension writes NAME.exe. Last verified: 2026-10-22. `xo run`
builds for the host and runs with the arguments after the directory or
`--`, passing the exit status through. `xo test` builds the test blocks of
every root module into one test binary and runs it: it prints failures (and
passes with `-v`) and a summary line `ok: N passed, M failed, K skipped`;
exit status 1 when a test fails. Tests that declare `uses` run only with
`--real`, with real capabilities for the declared effects. Each test runs in
its module's directory (as `go test` does per package), in a fresh test
world (virtual clock, MemFs, FakeNet, ...) on the deterministic scheduler
(core 7.4, see "Deterministic scheduling and live inspection"), seeded
with the test's pinned seed (`test "x" seed N`), else `--seed`, else 1;
the FAIL line prints the seed. A test whose tasks are all blocked with no
timer pending fails with fault XO-F001 and the wait cycle (or every
blocked task) with locations; a test that runs over 60s of real time
fails. `--explore=N` runs each concurrent test that passed under its seed
again under the next N-1 seeds and reports the first failing one, with
how to reproduce and pin it. Property tests (`test "x" for v: T`) and contract
fuzzing (every function with `requires`/`ensures` in a root module, core
9.1) run 100 generated cases (`--cases=N`) and shrink a failing input.
Last verified: 2026-10-07.

Build directories live in `$XO_BUILD_DIR` or `<user cache>/xo/build/<hash of
the module paths, mode, lowering, arenas on or off, and layout tag>/` (stable, so Go's build
cache is reused; the layout tag keeps older xo binaries out, see
`gotchas/build-dir-shared-across-xo-versions.md`): `go.mod` (module
`xoprog`), `rt/` (the runtime, copied from the embedded `std/rt`),
`m/<module ident>/xo.go` per Xo module, `main.go`, `xo_adapt.go`
(package main: interface adapters registered for generic code, often
empty), and (build mode, since 2026-10-08) `xo_buildinfo.go` (package
main: `rt.SetBuildInfo` with the build id, source hashes, main module
directory, and options, `build.Info`). Last verified: 2026-10-08. Crash bundles go to
`$XO_CRASH_DIR`, else `<user cache>/xo/crash` (macOS
`~/Library/Caches/xo/crash`, Linux `~/.cache/xo/crash`; owner,
2026-10-22), which keeps the newest 100 and which `xo clean -cache`
leaves alone; the path is printed on standard error (`crash bundle:
PATH`). Last verified: 2026-10-22. `XO_GO` overrides the go
command (default: `go` on PATH). A program that calls `time.zone` also
gets `xo_tzdata.go` (package main: `rt.TZEmbed` per constant zone name,
or `rt.TZFull` over `xo_zoneinfo.zip` embedded with go:embed when a name
is known only at run time, or `rt.TZUseSystem` with `--tzdata=system`).
The zone data is std's IANA database, `std/tzdata/zoneinfo.zip` (its
version and the archives' SHA-256 in `std/tzdata/VERSION`; decision
0094's addendum, 2026-10-16), on the native and Go backends alike, so a
build needs no Go for it; `XO_ZONEINFO=<file>` names another zip, and
an older release's `lib/xo/zoneinfo.zip` is read when std has none.
Refresh it per release: `scripts/tzdata-update.sh [VERSION]` (IANA's
tzcode and tzdata, its zic with backzone, a stored zip with sorted
names and fixed times, as Go's lib/time/update.bash; the same VERSION
gives the same bytes), then commit both files. Last verified:
2026-10-16 (2026e).

## Deterministic scheduling and live inspection

Last verified: 2026-10-08. Code: `std/rt/sched.go` (threads, scheduler,
monitor, reports, dump), hooks in `task.go`, `scope.go`, `chan.go`,
`sync.go`, `once.go`, `world.go`, `stop.go`, `caps.go`, `http.go`,
`testing.go`, `main.go`; `cmd/xo/dump.go`.

- Threads: every goroutine that runs Xo code is a `thread` (main, test,
  spawned task, HTTP request, runtime helper); tasks on one goroutine (a
  scope body and its opener, an in memory request's handler task) share
  it. `Task.th`. A thread records what it blocks on (`blockOn(kind, obj,
  cancelable)`) for reports. Cancellation tree nodes carry a kind, their
  task, a creation sequence number, and (scope bodies) the opening site.
- Scheduler (`sched`): one thread holds the turn; scheduling points
  (`Task.Yield`) are channel operations (send, recv, try_send, try_recv,
  close, select), await, Mutex.with, Mutex.with_read, Semaphore.with,
  Once.get, spawn,
  Atomic operations, sleep, `task.is_canceled()`,
  `task.stop_requested()`, and the effect calls of the fakes (clock.now,
  stdio, MemFs, FakeNet) and of real clock and stdio. At each point a PCG
  seeded with the seed picks the next ready thread. A blocking thread
  gives the turn away; wakers move it to the ready list. In a test world
  the scheduler shares `World.mu` and drives the virtual clock (fires the
  next timer when no thread is ready, ends `FakeClock.advance`, and
  closes `World.stuck` when nothing can fire). Tests with real
  capabilities run without it. `xo run --seed=N` sets `XO_SEED=N` for the
  program (any built binary honors `XO_SEED`): the same scheduler with
  the real clock; real timers and I/O wake threads from outside, and
  blocking I/O (stdin reads, HTTP client and server waits, `proc.run`)
  leaves the scheduler while it waits. Interleavings are reproducible as
  long as timer and I/O timing does not decide them.
- Deadlocks (fault XO-F001): test worlds report when the scheduler or
  the virtual clock finds every thread blocked with no timer pending.
  Real runs have a `monitor` (`RunMain`): live threads versus threads
  blocked on another task (channel, mutex, semaphore, await, scope end,
  Once); waits on timers, `after` arms, `task.stopping()`, signals, and
  I/O do not count. Wakers uncount the woken thread before signaling it,
  cancellation bumps a generation counter, and a suspicion is reported
  only after 200 ms with no change; then the fault, a crash bundle (with
  the task tree), exit status 70. The report is the first wait cycle
  over "waits for" edges (mutex holder, awaited task, a scope's running
  tasks, Once runner), then the other blocked threads; without a cycle,
  every blocked thread. Each line: thread, spawn site, operation, call
  site, and the object (channel or mutex with where it was made, holder
  and since where).
- `xo dump <pid>` sends SIGQUIT; `RunMain` prints the task tree to stderr
  and writes it to `$XO_DUMP_DIR` or the temp dir as
  `xo-dump-<pid>.txt` (temporary file and rename), and the program goes
  on (Go's default SIGQUIT exit is replaced). `xo dump` polls for the
  file, prints it, removes it. Format: `xo dump: pid N, K tasks`, one
  indented line per task or scope with its state (`blocked in <op> at
  <loc> ...`, `in scope`, `waiting for its tasks`, `running`, `ready`,
  `(canceled)`), `end of xo dump`.
- Call sites: test worlds and the scheduler capture them with
  `runtime.Callers` at each block; plain real runs do not (about 200 ns
  per block, `gotchas/runtime-callers-per-block-cost.md`) and read them
  from the goroutine profile through a per thread `runtime/pprof` label
  when a dump or report needs them.
- Tests: goldens `test_explore` (args.txt `-explore 40`: a lost update
  race found at seed 2, pinned failing and passing seeds),
  `test_deadlock` (two task mutex cycle, a channel nobody sends on, a
  task awaiting itself), `deadlock_real` (real run, exit 70), changed
  `test_runner`; all three new ones in `TestGoldenRace` (which now passes
  `args.txt`); `TestRunSeedReproduces` (testdata/sched_run with
  XO_SEED), `TestTestSeedReproduces`; `cmd/xo` `TestDump`; std/rt
  `sched_test.go`.

## Record and replay, crash bundles (P2)

Last verified: 2026-10-08. Code: `std/rt/replay.go` (modes, recorder,
replayer, scheduler events, timers, verdicts), `std/rt/fxcodec.go` (value
codec), hooks in `caps.go`, `http.go`, `sched.go`, `main.go`, `stop.go`,
`chan.go`, `sync.go`, `log.go`, `core.go` (`Secret_new`);
`internal/build` `Info`; `cmd/xo/replay.go`. Spec: core 11.2, 11.3.

- Modes (read by `RunMain`, then removed from the environment so the
  program and its children do not see them): `XO_RECORD=<file>` (set by
  `xo run --record=<file>`, honored by any built binary) records a full
  trace, written when the program ends (normal end, error, fault,
  deadlock fault, `proc.exit`, third signal; a SIGKILL loses it; the
  trace is kept in memory until then). `XO_RING=N` keeps per request ring
  buffers of the last N effects (request 0 is main and its tasks), no
  scheduler, attached to crash bundles. `XO_REPLAY=<file>` (set by `xo
  replay`) replays a `.xotrace` or a `.xocrash`.
- Effects: every capability method runs its I/O through `fx(tk, op, io,
  args, real)`: recorded as op, encoded args, encoded result, Xo location;
  replayed from the per thread queue (per request for ring buffers)
  without doing the I/O. `io` effects (stdin reads, `proc.run`,
  `http.send`) repeat `leave`/`enter` in a replay so the interleaving is
  the same. Covered: `fs.*` (reads and writes; a replay never touches the
  filesystem), `net.listen`/`dial`, `clock.now`/`sleep`, `rand.*`,
  `env.*`, `stdio.read_line`/`read_all`, `proc.run`/`exit`, `http.send`,
  `http.serve`, log record times (`log.time`), the stop grace clock
  (`rt.mono`). Output (`stdio.print*`, `write`) is not recorded; a replay
  prints it again.
- Interleavings: a recording runs every task on the deterministic
  scheduler (seed `XO_SEED` or random, kept in the header) and logs every
  pick by thread id; `yield` always decides. Timers (`realAfter`, used by
  sleep, `after` arms, `with_timeout`, the grace timer), SIGINT/SIGTERM,
  accepted HTTP requests, and server ends are scheduler events (inbox,
  run at the next decision with no thread running). See
  `gotchas/record-replay-scheduler-decisions.md`.
- HTTP server: a recording reads the request, posts it as an event that
  creates the request thread, and logs the response (status, headers,
  body; streamed bodies teed). A replay's listener has no socket;
  `serve` waits for the trace's server end and requests are delivered
  from the trace; each response is compared with the recorded one.
- Trace format (JSON lines): header (`xotrace`, `mode` full or ring,
  `complete`, `seed`, `args`, `target`, `build`), `{"secret":k,"src":
  "env:NAME","at":...}`, effects `{"n","t","q","op","a","r","at"}`,
  requests `{"req","l","r","resp"}`, scheduler stream `{"s": "1 1*4 2 t3
  r1 g e1"}` (pick, run length, timer, request, signal, server end; `x`
  is a server end that cancels its requests), `{"end": {code, fault,
  fcode, loc, error, isolated, req}}`. Values: strings (non UTF-8 as
  `{"b64":...}`), numbers exact, struct fields without `F_` and omitted
  when zero, List as array, Map as [key, value] pairs, enums as
  `{"tag", "p": {"$": payload type, "v"}}`.
- Secrets: `Secret[T].new(v)` registers Str or Bytes values; every trace
  line has them replaced by `<redacted:k>` when written. A replay puts
  them back from `XO_REPLAY_SECRET_k` or the recorded env variable and
  fails before running (exit 3) naming both when neither is set.
- Replay verdicts on stderr, prefixed `xo replay:`: `identical run`
  or `reproduced the crash` with the outcome and counts; a divergence
  (other op or args, no more effects, unknown timer or server, a picked
  thread that never becomes ready for 3 s, different end or response)
  names the effect index, the task, and both locations, exit status 3
  (`rt.ReplayDiverged`). Bundles are prefixes: running past their end
  stops the replay; a reproduced isolated handler fault exits 0.
- Crash bundles (`writeCrashBundle`): written for a fault reaching main,
  an error returned from main (new 2026-10-08, `kind: "error"` with
  `frames`), a deadlock fault, and a fault isolated in a real server's
  request handler (at most 100 per process); stderr gets `crash bundle:
  <path>`. Fields: `kind`, fault, code, location, stack, detail or error,
  frames, `isolated`, `request`, `tasks`, `build` (id, dir, sources,
  options), `target`, `time`, `args`, and with a recording or ring
  buffers `trace` (one JSON line per element; ring bundles hold main's
  and the failing request's buffers, see
  `gotchas/ring-replay-other-requests.md`).
- `xo replay` reads the header, rebuilds from `build.dir` (or `--dir`)
  with the recorded options, warns when source hashes differ, and runs
  the binary with `XO_REPLAY`, the recorded args, and no stdin.
- Not recorded: `use go` functions that perform effects, the 5 s limit of
  shielded defers (real time), wall clock reads of the runtime outside
  capabilities. The LLVM backend has no record (`--record` is refused).
- Tests: `internal/gen/replay_test.go` (`TestReplayEffects`,
  `TestReplayInterleave`, `TestReplayServer`, `TestReplayDivergence`,
  each also on a -race build) on `internal/gen/testdata/replay/*`;
  `cmd/xo/replay_test.go` `TestRecordReplayCLI`.

## A/B measurement: xo bench --ab

`xo bench --ab [flags] A [B] [-- ARGS...]` (decision 0340,
compiler/cli/bench, 2026-10-22) runs two programs, or one program under
two settings (`--env-a/--env-b`, `--args-a/--args-b`), in interleaved
rounds (the order alternates each round) and prints per metric each
side's median and interquartile spread, the ratio B/A with a 95%
percentile bootstrap interval over the rounds, and a verdict (better,
worse, within noise; too few rounds under 5; no ratio when A's median
is 0 and B's is not, both 0 is ratio 1; too coarse when both medians are
under 10x the metric's resolution, cpu_ms being 10 ms). Metrics: wall_ms, cpu_ms,
cpu_us_per_op, instructions, instructions_per_op, cycles, cycles_per_op and
ipc (instructions per cycle, higher is better; these from macOS `time -l`'s
"instructions retired" and "cycles elapsed" lines, 2026-10-26; on Linux
GNU time reports neither, and the text output says so in an `n/a` line
under the table), max_rss_kb, and every number on
the program's last JSON line (`loadgen`'s rps, p99_us, requests, ...).
Before each run it waits for the guards: the quiet window file
(`XO_QUIET_FILE`, default `~/xo-lead/quiet`; a future epoch in it means
another session is measuring; `--no-guard` skips this), `--max-load X`
(1 minute load average) and `--guard CMD` (`sh -c`, exit 0 to go); after
`--load-wait` (10 min) the round runs marked loaded and is left out of
the statistics unless `--keep-loaded`. Without `--max-load` (2026-10-26)
the guard does not wait for the load, but a side that starts at a 1 minute
load of 6 or more marks its round loaded all the same (the idle sessions
hold the Mac near 2; a run at a steady load of 20 used to report "0
loaded"). The load before every run is printed and in `--json`, and under
the table a load line gives each side's median load and "loaded: N of M
rounds", with a warning when a median is at or above the limit (`--json`:
`load_note`). Stage 1 measures through `/usr/bin/time -l`
(macOS, centisecond CPU resolution, instructions retired) or GNU
`time -v` (Linux; the Docker image has it since tag `-4`), so runs of
under about 100 ms are dominated by that resolution: make a run do more
work or use wall. When wall moves and instructions do not (or move the
other way), the cycles and ipc rows say whether the same instructions ran
at a different rate (an IPC drop: Lang 3's cut_suffix, 2026-10-25, +1.6%
instructions and +9.7% wall). Last verified: 2026-10-26.

## Experimental and sizing flags

Last verified: 2026-10-25

- `--inline-str` (decision 0370; arm64 native only, default off; amd64 refuses it with a warning, compiler/xo/main.xo:678): a
  Str of at most 11 bytes keeps its text in its two words, with no heap object.
  A list of one million short keys takes 0.38 of the memory, a `Map[Str, Int]`
  0.69, its sort 0.62; instructions on the Str heavy benches rise 1 to 11%
  and a release server's `__text` 8.4%. Build with it for memory bound programs;
  the flag changes no semantics (the goldens, the corpora and stdfuzz agree).

## Profiling

Last verified: 2026-10-16

Native programs (arm64 and LLVM backends, darwin and linux) carry a
sampling CPU profiler (decision 0238). Use it first to find where the
time goes; Docker callgrind (bench/perf/llvm/rust-docker.sh's method,
or valgrind in scripts/docker-linux.sh) is for exact instruction counts
and for diffing two builds function by function, not for a first look.

- `XO_PROF=cpu ./prog`: SIGPROF at `XO_PROF_HZ` per second of CPU time
  (default 100; 1000 for a short program), frames walked with the fault
  unwinder's tables. At exit (main's end or os.proc.exit) it writes a
  raw file at `XO_PROF_OUT` (default `xo-prof.xoraw` in the working
  directory; `%p` is the process id; decision 0245) and one line on
  stderr naming it (`XO_PROF_QUIET=1` drops the line). A profile never
  overwrites a file: remove the previous one before a rerun, or the run
  writes nothing ("could not write"). Since f79ea3f9d the samples count
  CPU time on macOS and Linux (gotchas/xo-prof-macos-undercounts-samples.md).
  The CPU profile holds at most 32,768 samples (llrt prof.xo CAP): at
  1,000 Hz about 33 s of CPU summed over every thread, at 10,000 Hz about
  3 s; later samples are dropped and counted: the exit line says
  "N more dropped" and `xo prof`'s summary "N lost" (Last verified:
  2026-10-30). For a long or many-threaded run lower `XO_PROF_HZ`. `xo prof FILE|URL
  [--raw-out=PATH] [--cpu-out=PATH] [--heap-out=PATH]` writes FILE's name with
  `.cpu.pprof` (and `.heap.pprof`) for its `.xoraw`, `.pb.gz` or `.gz`
  (`cold.pb.gz` gives `cold.cpu.pprof`; 2026-10-21, it was `xo-cpu.pprof`
  for every input) and a text summary beside it (`.txt` for `.pprof`):
  the totals, then the top 30 functions by flat and by cumulative
  samples, as `go tool pprof -top` prints. The program named the
  locations, so `xo prof` needs neither the binary nor its dSYM. Its
  default names are replaced on a rerun; a path given with `--cpu-out`
  or `--heap-out` is never overwritten (files mode 0600). A URL (a running server's std/debug
  `/debug/profile?seconds=N` or `/debug/heap`, decision 0325) is fetched
  into `xo-prof.xoraw` (or `--raw-out`) first; `/debug/sched` is printed.
  `xo run --profile` and `xo test
  --profile` do both steps. XO_PROF and XO_TRACE apply to the process that reads
  them (owner, 2026-10-21): the runtime drops the switch from its own
  environment once armed, so the processes it starts (a test binary,
  xo's detached prune) are not profiled or traced; `XO_PROF_CHILDREN=1`
  passes them on (darwin, linux; on windows a child still inherits the
  OS block). `xo build --profile=cpu|heap|cpu,heap` profiles the
  compiler itself: xo runs itself again with XO_PROF set
  (XO_PROF_SELF=1) and writes `xo-build.cpu.pprof` (`.heap.pprof`) and
  their `.txt` in the working directory. On windows/amd64 a sampler thread suspends
  each thread that used CPU (no SIGPROF), and names come from the COFF
  symbol table Windows links keep (no lines); to check it on the Mac,
  cross build (above) and run under Wine with Box64:
  `scripts/docker-wine.sh box64 WORK 'cd /work && XO_PROF=cpu,heap wine
  /work/win/P.exe'`, then `xo prof WORK/xo-prof.xoraw` on the Mac (decision 0238
  "Windows"; Box64 cuts some stacks short, gotchas/box64-thread-context-skewed.md).
- `xo build --release` leaves the profiler out (XO_PROF then prints one
  line saying so) unless `--profile-support` is given; `xo run`, `xo
  test` and a plain `xo build` keep it. A profile file is created new
  (O_EXCL, no symbolic link followed, mode 0600): a run whose output
  exists says so and writes nothing (decision 0238's addendum).
- `xo run --profile=cpu [dir]` sets it for the program (with no
  `--backend` it picks the host's native backend; `--backend=go` is
  refused: a Go backend program uses runtime/pprof). `xo test
  --profile=cpu` (or `heap`, `cpu,heap`) writes `xo-cpu-<pid>.pprof`
  (`xo-heap-<pid>.pprof`) and `.txt` per test
  process in the directory xo test ran in (not property test cases).
- Reading: the `.txt` file is plain text; `go tool pprof -top
  xo-cpu.pprof`, `-peek 'regexp'` (callers and callees of a function),
  `-traces`, `-web`; several files merge (`go tool pprof -top
  xo-cpu-*.pprof`); speedscope and flame graph tools read the file too.
- Names: Xo functions as `pkg.name` (main's as `name`); a C library
  function by dladdr (macOS, `__ulock_wait`) or its shared object
  (Linux, `[libc.so.6]`); `?` for a pc nothing holds. Functions only:
  source lines wait for decision 0239's DWARF line tables.
- Limits: the ring holds 32,768 samples (5.5 minutes of one busy thread
  at 100 Hz) and 64 threads; samples past either are reported as lost.
  Windows is not implemented yet.
  A recursive or large function shows by its own name; a small one the
  backend inlined shows as its caller.
- Off it costs one environment lookup (242 instructions a process on
  Linux); on macOS loading the larger runtime costs a short program
  about 2% (bench/results/prof-cpu-2026-10-16.md).
- `XO_PROF=heap ./prog` (decision 0238's heap profile; `cpu,heap` for
  both): allocations sampled on average every `XO_PROF_HEAP_RATE`
  bytes (512 KiB, Go's MemProfileRate; 1 samples every allocation, for
  exact counts of a short run). The profile's four values are Go's
  (`go tool pprof -sample_index=alloc_space|alloc_objects|inuse_space|
  inuse_objects`; inuse is the default), scaled from the samples. The
  `.txt` beside it has the estimated totals and the top 30 by
  inuse_space and by alloc_space; a stack's leaf is the code that
  allocated (the allocator's own frames are left off). In use means
  still allocated when the profile is written: at main's end that is
  only leaks (main's values are dropped); at os.proc.exit, what other
  tasks still hold. Profiling costs an allocation bound program 1.13x to
  1.33x (bench/results/prof-heap-2026-10-16.md); 65,536 samples at most.
- `XO_TRACE=FILE ./prog` (decision 0240): the execution trace. The
  scheduler records spawns, task starts and ends, parks with their
  reason (chan send, chan recv, select, mutex, join, sleep, net, http,
  test world), wakes (hand offs, and wakes of a park already signaled),
  deferred and late wakes, worker idling and waking, the monitor's ticks
  and the poller's wakeups, as raw 32 byte events in per worker 1 MiB
  chunks (`XO_TRACE_MB`, default 64; events past them are counted lost).
  FILE is written at exit, created new (an existing file or a symbolic
  link is refused), mode 0600; `%p` is the process id. Timestamps are
  ns (CLOCK_UPTIME_RAW on macOS). darwin and linux; not Windows yet.
- `xo trace FILE [--json OUT] [--text]` reads it: Chrome trace event
  JSON (default FILE.json; open it in ui.perfetto.dev or
  chrome://tracing: a track per worker with each task's runs named by
  its spawn site (`main.xo:14 #1`), wake arrows to the run they caused,
  idle slices, and a track per task with its runs and parks), and a text
  summary on stderr: time parked by reason, runnable latency (wake to
  run: median, p99, max), the top wakers, the longest parks, worker
  idle time, deferred and late wakes, wakes of an already signaled park.
  `xo run --trace=FILE` runs the program and then this; `xo test
  --trace=FILE` writes FILE.<pid> per test process (or FILE with `%p`).
- Release builds: `xo build --release` leaves the recorder out unless
  `--trace-support` (the owner's rule, decision 0238's addendum); then
  XO_TRACE does nothing. Development builds always have it.

## Benchmark tooling

Last verified: 2026-10-07

- `bench/` is its own Go module (`bench/go.mod`, module `xobench`) so the
  root module never builds benchmark code. Each Go solution under
  `bench/go/<arm>/<task>/` also has its own `go.mod`.
- Go analyzers for the strict arm are installed project locally in
  `.tools/bin` (staticcheck, errcheck, nilaway, exhaustive), installed with
  `GOBIN=$PWD/.tools/bin go install <pkg>@latest`. A `.tools/bin/xo` left
  there from before decision 0215 is a stale Go-built xo (0.8.0 on
  2026-10-13); the current one is `bin/xo` (copy it over to refresh).
- Rust baselines (2026-10-16): bench/perf/llvm/rust (the compute
  workloads, std only) and bench/perf/conc/rust (tokio 1.47.1 and std
  threads; Cargo.lock committed), built with `cargo build --release`
  (thin LTO, one codegen unit). The development Mac has stable Rust
  1.99.1 installed user-local by rustup (owner, 2026-10-16:
  `~/.rustup`, `~/.cargo`, `--no-modify-path`, profile minimal; the
  scripts find `~/.cargo/bin/cargo` when cargo is not on PATH).
  `bench/perf/llvm/rust.sh` measures Go, Rust, and both native Xo
  backends (best wall, fewest instructions retired, peak RSS;
  `CALLGRIND=1` for callgrind counts); `bench/perf/conc/run.sh` has a
  `rust` configuration; `bench/perf/llvm/rust-docker.sh` gives callgrind
  counts on linux/arm64 in Docker (Rust built in the pinned
  `rust:1.99.1-slim-trixie` image, Xo by a Linux xo built here with the
  Go backend). Results in `bench/results/rust-baseline-2026-10-16.md`.
  Last verified: 2026-10-16.
- `bench/size/measure.sh <dirs>` (2026-10-09): binary size, Go build
  cache growth, peak RSS, and wall time of `xo build` per program against
  a fresh cache with the runtime warmed (`MODE=test` for `xo test`,
  `XO=<binary>` to compare builds); results in
  `bench/results/binary-size-2026-10-09.md`. Last verified: 2026-10-09.
- `bench/perf/unicode/measure.sh <xo> <out dir> <backend...>`
  (2026-10-12, decisions 0180 and 0184): builds bench/perf/unicode with
  each backend and prints `<workload> <backend> <min instructions> <best
  us> <checksum>` over 5 runs of each workload (`/usr/bin/time -l`,
  darwin); `XO_UNICODE=c` built the C path until decision 0198 deleted
  it (2026-10-13). Run it through the heavy lock. Results in
  `bench/results/runtime-pilot-2026-10-12.md`. Last verified: 2026-10-12.
- `bench/perf/text/measure.sh <xo> <out dir> <backend...>` (2026-10-12,
  decision 0193): the same for bench/perf/text (Int parse, Duration and
  Time text, Debug quoting, split, lines, replace, trims, rune_count,
  and since step 3 int_fmt, float_fmt, float_parse, base64, hex, since
  step 5 debug, zone_offset, zone_fmt, zone_civil, zone_load, since
  decision 0201 utf8; `none` is the input setup alone);
  `XO_LLRT_C=text,show,tz,scan` builds the C paths (strings and num
  went in decision 0198, 2026-10-13), `WORKLOADS` picks workloads. Run
  it through the heavy lock with `XO_SELF_DRIVER=0` in the environment
  for the Go driver, and rebuild the xo binary after a std edit
  (gotchas/xo-binary-embeds-std-native.md). Results in
  `bench/results/runtime-port-2026-10-12.md`,
  `runtime-port-step3-2026-10-12.md`, `runtime-port-step5-2026-10-13.md`,
  and `scan-intrinsics-2026-10-13.md`. Last verified: 2026-10-13.
- `bench/perf/portable/measure.sh`, `bench/perf/digest/measure.sh`
  `<xo> <out dir> <backend...>` (2026-10-13, decision 0195): the same
  for bench/perf/portable (sized integer arithmetic, conversions, UInt64,
  Str key hash, Time.unix, checked_ methods, Float32) and bench/perf/digest
  (SHA-256, HMAC, PBKDF2 and Argon2id verify); `XO_LLRT_C=ints` and
  `XO_LLRT_C=digest` build the C paths. `bench/perf/glob/measure.sh` and
  `bench/perf/gen/measure.sh` build a test binary (`xo test --no-gc`) and
  run each test (`XO_TEST`) 5 times, printing `<test> <backend> <min
  instructions>`; `XO_LLRT_C=glob`, `gen`. Through the heavy lock with
  `XO_SELF_DRIVER=0`. Results in
  `bench/results/runtime-port-portable-2026-10-13.md`. Last verified:
  2026-10-13.
- `bench/perf/tls/measure.sh <xo> <out dir> <backend...>` (2026-10-15,
  decision 0202): TLS handshakes per group and server key (X25519,
  P-256, P-384; ECDSA P-256, RSA-2048, Ed25519; TLS 1.2) and bulk reads
  (AES-128-GCM, AES-256-GCM, ChaCha20-Poly1305) of std/http's client
  against crypto/tls servers (serve/, its own PID, stopped at the end),
  built with the Xo TLS (Mbed TLS, the other side until 2026-10-15, is
  removed); prints `<workload> <backend> xo <min instructions> <best wall ms>`
  (N handshakes, default 200; SIZE bytes, default 64 MiB; PORT and the
  next 9). Through the heavy lock. Results in
  `bench/results/tls-perf-2026-10-15.md`. Last verified: 2026-10-15.
- `XO_LLRT_C=rc` (deleted 2026-10-13 by decision 0198's second round:
  since 2026-10-14 the name fails the build; kept for the results files): the C runtime's
  own reference counts (rc_inc, rc_dec, rc_claim, xo_rc_mark compiled
  with -DXO_RC_C, which both drivers add when the generated code calls
  rc_c.xo's rt.rc_*_c) instead of std/native/llrt/rc.xo through the
  xo_x_rc_* exports (the default). TestRuntimeThreads runs
  std/llrt/testdata/rc_threads.c (the Xo path alone since 2026-10-14) under ThreadSanitizer. The
  comparison (instructions of bench/perf/nested, xo, text, json, conc)
  is a scratch script: build each program with and without
  `XO_LLRT_C=rc`, minimum of 5 runs (conc: 11 or 21, XO_THREADS=1;
  their counts vary by 10 percent between runs). Results in
  `bench/results/raw-memory-2026-10-13.md` (stage 2). Last verified:
  2026-10-13.
- `XO_LLRT_C=alloc` (deleted 2026-10-13 by decision 0198's second
  round: since 2026-10-14 the name fails the build; kept for the results files): the C
  runtime's own allocator (alloc_in, free_in, alloc_mt, free_mt,
  usable, resize compiled with -DXO_ALLOC_C, which both drivers add
  when the generated code calls alloc_c.xo's rt.alloc_*_c) instead of
  std/native/llrt/alloc.xo through the xo_x_heap_* exports (the
  default). `go test ./internal/llvm -run TestRuntimeAlloc` runs
  std/llrt/testdata/alloc_threads.c (the Xo path alone since 2026-10-14) under ThreadSanitizer
  (about 5 s; -O2 with ITER 30000 without -short).
  `bench/perf/alloc/compare.sh <xo> <out> <program dir> <workload...>`
  builds a program both ways on arm64 and LLVM and prints the minimum
  instructions of 5 runs and their ratio (bench/perf/alloc: none small
  mixed grow strings maps tasks; also nested, xo, text, json, conc);
  through the heavy lock. Results in
  `bench/results/raw-memory-2026-10-13.md` (stage 3). Last verified:
  2026-10-13.
- `XO_LLRT_C=sched` (deleted 2026-10-14, zero-C audit item W1: the C
  below, sched_c.xo, and -DXO_SCHED_C are gone, and the name now fails
  the build like every deleted one; TestRuntimeTasks and
  TestRuntimeEventLoop run the Xo path only; the history follows)
  (2026-10-13, decision 0212): the C runtime's own
  scheduler core (the fiber switch assembly, `__thread` context and
  worker, the run queue ring, fiber run, park, wake, start, task
  states: xo_rt.c compiled with -DXO_SCHED_C, which both drivers add
  when the generated code calls sched_c.xo's rt.sched_c) instead of
  std/native/llrt/sched.xo through the xo_x_rq_*, xo_x_fiber_*, and
  task exports and the emitted xo_stack_switch, xo_fiber_entry, and
  xo_tlocal_* (the default). TestRuntimeTasks and TestRuntimeEventLoop
  run tasks.c and evloop.c both ways under ThreadSanitizer. The bar is
  per operation: two builds of bench/perf/conc spawn (200 and 600
  rounds) and pingpong (100k and 300k round trips), XO_THREADS=1,
  median of 9, the difference over the extra operations (whole runs
  follow the idle thread's spinning,
  gotchas/scheduler-instruction-counts-spin.md); http: server
  instructions of bench/perf/http's cycle route, 5,500 minus 500 units,
  fresh servers. Scratch scripts, through the heavy lock. Results in
  `bench/results/task-switch-2026-10-13.md`. Since stage 2 (2026-10-13)
  the same switch also selects the C runtime's worker pool, locks' slow
  paths and futexes, worker thread starts, fiber stacks, parks,
  cancellation tree, and spawn and join code instead of
  std/native/llrt/pool.xo (xo_x_lock_slow, xo_x_park_*, xo_x_node_*,
  xo_x_scope_add, xo_x_scope_join, xo_x_worker_main, ...; the C services
  pool.xo calls are xo_sched_rt). Measure it with XO_SPIN_ROUNDS=0 too:
  the spinning worker's instructions are part of a ping pong's per
  operation count. Last verified: 2026-10-14 (deleted).
- Checking compiler/macho against clang after an arm64 emitter change
  (2026-10-13, gotchas/selfhost-assembler-lags-emitters.md): `xo build
  -o macho ./compiler/macho`, `xo asmdump [--os=linux] DIR` (without
  its first line), `clang -c -target arm64-apple-macos11` (or
  `aarch64-linux-gnu`) and `macho asm [--elf]`, `cmp` the objects; with
  XO_SELF_DRIVER=0. Last verified: 2026-10-13.
- `go test ./internal/llvm -run 'TestIntsDiff|TestDigestDiff'` and `go
  test ./cmd/xo -run 'TestGlobDiff|TestGenDiff'` (2026-10-13, decision
  0195): sized integers, std/crypto's hashes, path.Match, and generated
  values on arm64 and LLVM, Xo and C paths, against the Go backend (or
  Go's packages); not under -short. `go test ./internal/llvm -run
  TestDigestTables -golden-update` regenerates
  std/native/llrt/digest_tables.xo (SHA-256 unrolled). Last verified:
  2026-10-13.
- `go test ./internal/llvm -run TestCABIDiff` (2026-10-13, decision 0207):
  the C runtime's pieces that call Xo through the C ABI exports of
  std/native/llrt/cabi.xo (fault texts, Float to Int and Float32 text,
  UTF-8, Net.only on loopback) on arm64 and LLVM against the Go backend,
  about 5 s warm; not under -short. A C only runtime test (a file of
  std/llrt/testdata that includes xo_rt.c) links the exports through
  `cOnly` in internal/llvm threads_test.go. `go test ./internal/ssa -run
  'TestGolden|TestBench' -update` and `go test ./internal/arm64 -run
  TestGoldenAsm -update` rewrite the SSA and assembly goldens; the
  assembly goldens hold only the programs' own functions since
  2026-10-14 (TestGoldenAsm's ownFuncs; the whole text is still
  assembled as ELF), so an export or runtime change does not rewrite
  them. Last verified: 2026-10-14.
- `bench/perf/report/measure.sh <xo> <out dir> <backend...>` (2026-10-13,
  decision 0193 step 5): builds the test binary of report_test.xo
  (`xo test --no-gc`, kept in XO_BUILD_DIR) and runs 16 cases of each
  property test (XO_TEST, XO_PROP_SEED; `0b`, `1b` also with
  XO_PROP_BEST=0, as while shrinking), printing `<test> <backend> <sum
  of the min instructions of 5 runs> <bytes of output>`;
  `XO_LLRT_C=report` builds the C path. Through the heavy lock, with
  `XO_SELF_DRIVER=0`. Last verified: 2026-10-13.
- `bench/perf/json/measure.sh <xo> <out dir> <backend...>` (2026-10-13,
  decision 0193 step 4): the same for bench/perf/json (std/json decode
  of small objects, Int, Float, and string arrays, Map objects, the
  parser alone, invalid texts; encode of structs, strings, Floats; Bytes
  both ways; `none` is the input setup alone); `XO_LLRT_C=jsontext`
  built the C path until decision 0198 (2026-10-13). Run it through the heavy lock with
  `XO_SELF_DRIVER=0`. Results in
  `bench/results/runtime-port-step4-2026-10-13.md`. Last verified:
  2026-10-13.
- `go test ./internal/llvm -run TestJSONTextDiff` (2026-10-13, decision
  0193 step 4): std/json on arm64 and LLVM (the Xo path; the C path's
  subtests went with it in decision 0198) against the
  Go backend on about 28,000 generated and edge case inputs (not under
  -short; `XO_JSONDIFF_N` sets the random count, default 6000). Last
  verified: 2026-10-13.
- `go test ./internal/llvm -run TestScanDiff` (2026-10-13, decision
  0201): std/native/llrt/scan.xo (find, split, lines, rune_count,
  to_hex, quoting runs, UTF-8 validity, Bytes rune count) on arm64 and
  LLVM, Xo and C paths, against the Go backend on about 82,000 inputs
  (needles at every offset, invalid UTF-8; not under -short;
  `XO_SCANDIFF_N` sets the random count, default 3000). Last verified:
  2026-10-13.
- `bench/perf/httpparse/measure.sh <xo> <out dir> <backend...>`
  (2026-10-13, decision 0193 step 6): builds the server of
  bench/perf/httpparse with each backend and, for each workload (h1_get,
  h1_chunked, h1_url, h2_get, h2_varied; `none` starts and stops it),
  runs it 5 times under `/usr/bin/time -l` while load/ (a Go client,
  built into the out dir; it also writes the TLS certificate) sends N
  requests (default 20000), then stops it with SIGTERM to the server's
  PID (the child of the time process the script started); prints `<w>
  <backend> <min instructions retired>`. `XO_LLRT_C=http1,hpack` builds
  the C paths; PORT (18090). Through the heavy lock, with
  `XO_SELF_DRIVER=0`. Last verified: 2026-10-13.
- `bench/perf/httpclient/measure.sh <xo> <out dir> <backend...>`
  (2026-10-14, audit item B10): builds the client of
  bench/perf/httpclient with each backend, starts serve/ (a Go server of
  fixed raw responses, built into the out dir, stopped by its own PID at
  the end), and for each workload (len, chunked, big, redirect; `none`
  is the start and the end) runs the client 5 times under
  `/usr/bin/time -l` with N requests (default 3000), each on a
  connection of its own; prints `<w> <backend> <min instructions
  retired>` of the client. The C client is gone: an xo from before B10
  (main 16524ee6) with its own std measures it. PORT (18095). Through
  the heavy lock. Since B11 (2026-10-14) also h2len and h2big: HTTP/2
  over TLS (serve/ on PORT+1 with a certificate it makes and writes to
  the out dir's ca.pem), every request on one pooled connection; the C
  HPACK is measured with main c013b78d's xo, whose copy needs std and
  third_party next to it (gotchas/baseline-xo-needs-third-party.md).
  Last verified: 2026-10-14.
- `bench/perf/fsproc/measure.sh <xo> <out dir> <backend...>`
  (2026-10-13, decision 0193 step 6): the Fs walks, CreateTemp, the
  working directory, and process environments (mkdir, tree, remove,
  temp, cwd, proc_plain, proc) in a fresh scratch directory per run;
  `XO_LLRT_C=fspath,procspec` builds the C paths. The counts include the
  kernel's instructions. Last verified: 2026-10-13.
- `bench/perf/stdio/measure.sh <xo> <out dir> <label=std dir>... -- <backend...>`
  (2026-10-14, zero-C work item B03): standard output and error (lines,
  ints, small, big, err, mixed, none) built by one xo against each std
  dir given (`c=` an archive of an earlier commit's std/ tree, whose
  prelude has no llrt_print, so the compiler calls the C runtime's
  xo_print; `xo=` the checkout's std), output to /dev/null; prints the
  minimum of 5 runs of user instructions (/usr/bin/time -l on macOS,
  perf stat on Linux) and ms. WORKLOADS overrides the list. Last
  verified: 2026-10-14.
- `bench/perf/sysio/measure.sh <xo1> <std dir> <out dir> <backend...>`
  (2026-10-13, decision 0206; macOS mode with `/usr/bin/time -l`,
  decision 0213): os.fs.read, write, stat, Clock.now, channels, and
  sleeps (read, read_big, write, stat, clock, chan, sleep, none) built
  as xo (the Xo path) and nolibc (`XO_NOLIBC=1`, Linux); the c variant
  (`XO_LLRT_C=sysio`) is gone (decision 0198 deleted the C path, and
  an unknown XO_LLRT_C name fails the build since W0), so a C
  baseline is the same program built with an earlier commit's std
  (an archive of that commit's std/ tree passed as the std dir; the
  clocks of work item B01 were measured so,
  bench/results/clocks-2026-10-14.md). Prints the minimum of 5 runs of
  user and kernel instructions and task-clock ms from `perf stat`
  (needs kernel.perf_event_paranoid <= 1 for the kernel count; `dnf
  install perf` on AL2023). VARIANTS and WORKLOADS override the lists.
  Last verified: 2026-10-14.
- `bench/perf/net/measure.sh <xo1> <std dir> <out dir> <backend...>`
  (2026-10-13, decision 0206 stage 3, Linux only): loopback echo round
  trips, dials to 127.0.0.1, and dials to localhost (/etc/hosts and the
  address sort) (echo, dial, resolve, none) built three ways (xo: the Xo
  path; c: `XO_LLRT_C=net`; nolibc: `XO_NOLIBC=1`), printing the
  minimum of 5 runs of user and kernel instructions and task-clock ms
  from `perf stat`; a variant that does not build is reported and left
  out. Ran on EC2 hosts (bench/results/linux-syscalls-2026-10-13.md,
  stage 3). Last verified: 2026-10-13.
- `go test ./internal/llvm -run TestResolverDiff` (2026-10-13, decision
  0206 stage 3, Linux with user namespaces, not under -short): builds
  internal/llvm/testdata/resolver with the Xo resolver and with
  `XO_LLRT_C=net` (glibc's getaddrinfo) and runs both under `unshare -r
  -n -m` with the test's /etc/resolv.conf, hosts, and nsswitch.conf
  bind mounted and its own DNS server (TestResolverDNSServer, the test
  binary re-run, on 127.0.0.1:53, stopped by its PID); outputs and the
  questions the server saw must be the same. Skips without unshare,
  user namespaces, or /etc/nsswitch.conf. Last verified: 2026-10-13.
- `go test ./internal/llvm -run 'TestHTTP1ParseDiff|TestProcSpecDiff'`
  (2026-10-13, decision 0193 step 6, not under -short): a server program
  built with the Go backend, LLVM, and arm64 answers the same raw
  HTTP/1.1 connections over loopback (`XO_HTTP_DIFF_N` random requests,
  default 2000, `XO_HTTP_DIFF_SEED`); env(1) and printf(1) children with
  the environments procspec.xo builds. `XO_LLRT_C=http1` or `procspec`
  checks the C path. TestH2Diff's probeH2HPACK covers hpack.xo.
  `-run 'TestHPACKTables|TestHTTP1Tables' -golden-update` regenerates
  `std/native/llrt/hpack_tables.xo` and `http1_tables.xo`. Last
  verified: 2026-10-13.
- `bench/go/strict-check.sh <dir>`: vet, staticcheck, errcheck, nilaway,
  exhaustive, `go test -race`. Analyzers come from `$XO_TOOLS_BIN` when set,
  else the checkout's `.tools/bin` (a worktree has none; point
  `XO_TOOLS_BIN` at the main checkout's). Last verified: 2026-10-08.
- `bench/harness/` (2026-10-08): reruns the agent benchmarks with any model
  and scores them; Python 3 standard library only, no third party packages.
  `prompts.py render|all|tokens` renders the per arm and planted author
  prompts (the Xo ones include `spec/core.md` and `spec/std.md`) and
  estimates tokens; `run_openai_compatible.py --out DIR` drives any OpenAI
  compatible endpoint (`OPENAI_BASE_URL`, `OPENAI_API_KEY`, `OPENAI_MODEL`,
  optional `OPENAI_EXTRA_BODY`) through the feedback loop, up to
  `--max-runs` runs; `score.py solutions DIR|--claude` and
  `score.py planted DIR|--claude` rerun every cell in a temporary copy and
  print the three arm and planted tables. When a cell's first run sources
  exist (`<cell>/.attempts/run-1`, or `<DIR>/history/<task>/1` for a run
  made another way), `solutions` also reruns them as is and, for Xo, after
  `xo fix` ("First try now", "First try after `xo fix`", and a "First
  runs" table; since 2026-10-09). The scorer builds `xo` from the
  checkout into `$TMPDIR/xo-harness/xo` unless `XO_BIN` is set, and finds
  the analyzers through `XO_TOOLS_BIN`, `.tools/bin`, or the main
  checkout's `.tools/bin`. Self test: `python3 bench/harness/test_harness.py`
  (also `go test ./internal/benchharness`, about 7 s before 2026-10-09 and about 35 s since, with the Xo `xo fix` case; skipped without
  python3; uses a fake endpoint on 127.0.0.1). Scoring all stored Claude
  solutions took about 12 minutes, all planted sets about 60 (measured
  2026-10-08).
- `bench/perf/conc/run.sh` (2026-10-09, decision 0077; `rwmutex` added
  2026-10-10, decision 0118): six concurrency
  programs (`xo/spawn`: 200 rounds of 1000 discarded tasks; `fanout`: 100
  rounds of 1000 awaited tasks; `pingpong`: 100,000 round trips over two
  unbuffered channels; `mutex`: 8 tasks x 25,000 `with`; `rwmutex`: 64
  tasks x 4,000 accesses, a quarter `with` holding the lock for a Collatz
  count, the rest `with_read`, so waiters park; `pmap`: 20 rounds
  of 64 Collatz chunks) and idiomatic Go (`go/`, `concperf <name>`).
  Configurations `go`, `xo-go`, `xo-llvm`, `xo-arm64`, `xo-arm64-1`
  (`XO_THREADS=1`); `REPS` (7) rounds interleaved; medians of wall
  (perf_counter), CPU (child rusage, user + sys), and cycles (`/usr/bin/time
  -l`, Darwin). `WORKLOADS`, `CONFIGS`, `KEEP=dir` (keep binaries), `XO`.
  Prints `uptime` first: other processes on the host skew wall time most.
  Profiling: `xcrun xctrace record --template 'CPU Profiler' --output
  p.trace --launch -- ./bin`, then `xcrun xctrace symbolicate --input
  p.trace --dsym ./bin --output s.trace` and `bench/perf/conc/profile.py
  s.trace 40 ./bin` (names the runtime's static functions from `nm`;
  leaf addresses in the export are the PC plus one). ThreadSanitizer runs
  of these programs need reduced sizes
  (`gotchas/tsan-fiber-trace-crash.md`).
- `bench/perf/run.sh`: builds `bench/perf/xo` (Xo) and `bench/perf/go`
  (hand written Go, its own `go.mod`), runs each workload `REPS` times
  (default 5), and prints a Markdown table of the best times and the
  ratio; checksums must agree. Workloads: collection (`putget get push
  update iter copymut branch`), List writes (`setat stack fields grid
  insrem copyset`, added 2026-10-08, Go slices on the Go side; an xo
  binary from before 2026-10-08 does not finish `setat` or `stack`), and
  generic (`sortint sortpt hof maxby iface`, idiomatic Go generics on the
  Go side). `MONO=1` builds the
  "Xo" column with `--no-mono` (when the binary has the flag; an older
  binary's default is that lowering) and adds an "Xo --mono" column (the
  default lowering), `XOFLAGS` passes flags to the first Xo build,
  `XO=<path>` uses another xo binary (for before and after runs),
  `WORKLOADS="..."` selects workloads, `PERF_OUT=<dir>` keeps the
  binaries. Timing drifts on the laptop host; interleave before and after
  runs (`gotchas/bench-host-timing-drift.md`). Last verified: 2026-10-08.
- `bench/perf/http/run.sh` (2026-10-08): HTTP load benchmark of task 01.
  `go/` and `xo/` are copies of `bench/go/plain/01-crud-api` and
  `bench/xo/v0.4/01-crud-api` that listen on 127.0.0.1 only (since
  2026-10-08 `xo/` reads the store with `Mutex.with_read`, so the
  `xo-base` configs need an xo binary that has it; for older binaries
  build a copy of the source with `with` instead); `loadgen/`
  (own module) runs `CONC` keep alive workers (default 32) through a
  create, get, patch, get, delete cycle (and a list every eighth cycle)
  for `WARMUP` + `DURATION` (2s + 10s). Each server starts fresh with
  `GODEBUG=gctrace=1` and is stopped with `kill` of its own PID. Prints
  raw lines and a Markdown table of medians over `REPS` (3) interleaved
  runs: req/s, p50/p99/p99.9, GC count, total STW pause, peak heap, GC
  CPU percent, peak RSS, GCs per 10k requests, and (since 2026-10-09) the
  server's CPU time per request in the measured window (`ps -o time` of
  its PID before and after, user plus system, microseconds; loadgen's
  `cpu_us_per_req`). `CONFIGS` (default `go xo
  xo-arenas`; since decision 0066 `xo` builds with `--no-arenas` and
  `xo-arenas` with the default; `xo-base` builds with the binary in
  `XO_BASE` in its own build directory and that binary's default,
  `xo-base-arenas` the same with `--arenas`; `xo-arm64` and `xo-llvm` build
  with `--backend arm64` and `--backend llvm`, the native runtime of
  decision 0074; first measured 2026-10-09, results "Native backends"), `XO`, `PERF_OUT`, `PRELOAD`, `PORT` (18080). The load
  is closed loop (`gotchas/closed-loop-latency-confound.md`). Results:
  `bench/results/perf-arenas-2026-10-08.md`,
  `bench/results/perf-throughput-2026-10-08.md`.
- Heap goal of built Xo programs (decision 0065, 2026-10-08): `RunMain`
  keeps the GC goal at or above a floor (default 2 MiB per P, at least
  16 MiB) with GOGC=100 growth above it, by setting the GC percent after
  every cycle (std/rt/gc.go). `gc min_heap|percent|memory_limit` lines in
  the main module's xo.mod change it; `GOGC` (turns the floor off),
  `GOMEMLIMIT`, and `XO_GC_MIN_HEAP` (0: no floor) in the environment win.
  So run.sh at default settings compares Go at GOGC=100 with Xo at its
  floor; pass `GOGC=` to compare equal settings (both get it). allocs.sh
  GC counts are taken at the default floor too. Last verified: 2026-10-08.
- Execution trace (2026-10-08): `XO_TRACE=<file>` writes a `runtime/trace`
  of the whole run when `RunMain` returns (stop a server with `kill <its
  PID>`); read with `go tool trace <file>`. Last verified: 2026-10-08.
- Profiling a built Xo program (2026-10-08): `XO_CPUPROFILE=<file>` writes
  a CPU profile of the whole run and `XO_BLOCKPROFILE=<file>` a block
  profile (where goroutines waited: locks, channels, select) when
  `RunMain` returns, so stop a server with `kill <its PID>` (SIGTERM, a
  clean shutdown), not SIGKILL. Read with `go tool pprof -top -cum
  <binary> <file>`. Last verified: 2026-10-08.
- Allocation profiling (2026-10-08): `XO_MEMPROFILE=<file>` writes the
  `allocs` profile when `RunMain` returns (`XO_MEMPROFILERATE=N` samples
  one allocation per N bytes, 1 for every allocation; read with `go tool
  pprof -sample_index=alloc_objects -lines -top <binary> <file>`, or
  `alloc_space`), and `XO_MEMSTATS=<file>` writes `mallocs`,
  `total_alloc`, and `num_gc` of the whole run. The Go server of
  bench/perf/http writes the same three lines with `MEMSTATS=<file>` on
  SIGTERM. `bench/perf/http/allocs.sh` uses them: per config (`go`, `xo`
  = `--no-arenas`, `xo-arenas` = default, `xo-base` and
  `xo-base-noarenas` with `$XO_BASE`) and route (`get list patch
  createdel cycle`), two fresh servers get N1 and N2 units of the route
  from `loadgen -route <r> -n <n>` on one connection (port 18190), and
  the difference gives allocations, bytes, and GCs per request. The
  counts are deterministic to about 0.1 allocation and do not depend on
  host load. An `xo-base` binary needs the XO_MEMSTATS hook (commit
  8b65b93); for an older checkout apply that commit's std/rt/main.go
  change first. Allocation budgets of the hot runtime paths are pinned
  in `std/rt/alloc_test.go` (skipped where a sync.Pool is involved under
  `-race`, which drops pooled items at random). Last verified:
  2026-10-08.
- Lock hold and wait times (2026-10-08): `XO_LOCKSTATS=<file>` makes every
  `Mutex.with` and `Mutex.with_read` of a built Xo program record how
  long it waited to acquire and how long it held the mutex, per call
  site, and `RunMain` writes a Markdown table (count, p50, p99, mean in
  us) to the file on return (stop a server with `kill <its PID>`;
  `std/rt/lockstats.go`). Off by default (one bool load per
  acquisition). The Go server of bench/perf/http records the same table
  per store operation with `HOLDSTATS=<file>` (written on SIGTERM;
  `bench/perf/http/go/lockstats.go`), so both columns compare directly.
  The samples stay in memory (tens of MB), which changes GC pacing: do
  not read throughput from an instrumented run
  (`gotchas/sample-recording-changes-gc-pacing.md`).
  Last verified: 2026-10-08.
- `bench/perf/llvm/run.sh` (2026-10-08): the LLVM backend prototype
  against the Go backend and hand written Go on compute workloads in the
  LLVM subset (`fib sieve nbody sort interp collatz strings`; `strings`
  added 2026-10-08 for allocation churn). `RSS=1` adds a peak memory
  table (`/usr/bin/time -l`). Each Xo workload
  is its own program (`xo/<name>/main.xo`, the subset has no arguments or
  clock); `go/` (own module) is one binary `llperf <name>`, plus
  `llperf interp_tuned` (a tuned interpreter, not part of the script).
  Wall clock time per process, best of `REPS` (5); a run longer than
  `TIMEOUT` seconds (60) is stopped and shown as `>60000`, needed with xo
  binaries from before 2026-10-08, whose list element writes were O(n)
  (`gotchas/go-backend-list-set-quadratic.md`). Checksums must agree.
  Needs python3 for timing. `XO`, `PERF_OUT`, `WORKLOADS` as in
  `bench/perf/run.sh`. Results: `bench/results/llvm-prototype-2026-10-08.md`,
  `bench/results/llvm-rc-2026-10-08.md` (reference counting).
- `bench/perf/llvm/native.sh` (2026-10-08): the native arm64 backend
  against the LLVM backend and hand written Go on the same workloads.
  Binaries run interleaved (Go, LLVM, arm64) `REPS` times (5) under
  `/usr/bin/time -l`; the table has best wall time (`STAT=median`: the
  median, added 2026-10-08), the two ratios, and peak RSS of all three.
  Checksums must agree. `XO`, `PERF_OUT`, `WORKLOADS` as above. Check the
  load average first (`gotchas/bench-host-timing-drift.md`).
  `/usr/bin/time -l` also prints instructions retired and cycles, which
  separate "more instructions" from "slower instructions". Results:
  `bench/results/arm64-native-2026-10-08.md`,
  `bench/results/arm64-perf-2026-10-08.md`.
- Code placement sweeps for the arm64 backend (decision 0075; Last
  verified: 2026-10-08): `XO_ARM64_LAYOUT=pad=K xo build --backend=arm64`
  puts K nops (never run) before every function label, moving all code;
  build K = 0 to 15 and compare the median cycles over the 16 binaries
  (and 16 copies of one binary as the noise floor: not optional, a run
  under about 100 ms varies about 14 percent run to run, interp
  2026-10-29, agent-tools.md "Measuring") to judge a code shape
  change. Also `func=N` and `loop=N` (log2 alignment of functions and
  innermost loop tops), `fall=1`, `off=K`, `outer=1`. Unset is the
  default placement; a bad value fails the build. The build directory is
  keyed by the program, not the layout: copy `prog-darwin.s` (kept
  with `XO_KEEP_ASM=1`, or written anywhere with `--asm=FILE`)
  before the next build.
- Instruction level profiles on macOS (Xcode 16 `xctrace`, no root;
  Last verified: 2026-10-08). Make the program run for seconds first
  (more rounds in a copy of the bench program), then
  `xctrace record --template 'CPU Profiler' --output p.trace --launch -- ./bin`
  (a sample every 1M cycles, user and kernel) and
  `xctrace export --input p.trace --xpath '/trace-toc/run[@number="1"]/data/table[@schema="cpu-profile"]'`:
  each row has a `cycle-weight` and a backtrace whose first frame is the
  sampled PC; its `binary` element (often a `ref` to an earlier id) has
  the `load-addr`, so `pc - load-addr + 0x100000000` is the address in
  `otool -tvV ./bin`. Samples skid to the instruction after a stall. The
  `CPU Counters` template (default "bottleneck" mode) gives, in table
  `CounterMetricAggregatedForProcess`, one row per 1 ms of four values
  in hundredths of a percent: useful, instruction processing,
  instruction delivery, and discarded (that order). Useful is
  instruction based: compare it per instruction of work, not across
  programs that retire different counts. xctrace sometimes dies with
  `Trace/BPT trap` under load; rerun. Neither is in the repository's
  scripts; the parsing is a few lines of Python.
- `bench/planted/run_detection.py [task...]`: runs planted bug variants
  through the plain and strict loops, writes `bench/planted/results.jsonl`.
- `bench/harness/score.py planted DIR|--claude`: the same detection for the
  Go set and both Xo sets in one run, with the current toolchain, for
  planted sets written by any model (see `bench/harness/README.md`). Since
  2026-10-09 a test failure counts as a catch only for a test that passes
  on the unplanted base (`new_test_failures`, also in
  `bench/planted-xo/run_detection_xo.py`; tested by
  `TestPlantedTestCatches` in `bench/harness/test_harness.py`). Last
  verified: 2026-10-09.

### The backend caches' identity (decision 0275, 2026-10-20)

The backend caches (fe, chk, rc, inl, fn, part, obj, the inline verify
keys) key on XO_CODEGEN_KEY, a stamp internal/xotool sets (StampArgs,
CodegenKey): the SHA-256 of the backends (compiler/arm64, llvm, wasm,
macho) and every compiler module they use, transitively, without test
files, and core.VERSION. An xo rebuilt from changes that leave those
files alone (compiler/xo, std, tools, docs) keeps its warm cache. An
unstamped xo (bootstrap-xo.sh, a hand build) and a tree whose closure
cannot be computed key on xo's own binary (self_id), as before; the
binary and test run caches always do. internal/xotool
TestCodegenInputsClassified fails on an environment variable, module
field, or file system read of the driver or the closure that is not
classified (key header, bypass, program content, link, or no effect);
TestCodegenKeyChanges checks the key moves with every closure module
and not with compiler/xo or a test. gate.sh's `warm` step builds the
base's xo, fills a cache with it, and checks this xo's builds through
it (TestFrontCacheAcrossPrograms, XO_TEST_WARM_XO) and TestGoldenRun
with XO_BACKEND_CACHE=verify against the user cache. Last verified:
2026-10-20

### Build stamps and the bench guard (decision 0264, 2026-10-18)

Every xo built by internal/xotool (scripts/xobuild, the tests) or by
scripts/bootstrap-xo.sh carries build stamps: `xo version --json` prints
the commit, `dirty` and the diff hash, the source key (internal/xotool
Key; `go run ./scripts/xobuild --key` prints the checkout's), and the xo
that built it. A cached build is shared by every commit with the same
sources, so it keeps the commit that first built it; compare keys, not
commits. bench/perf/xocheck.sh, sourced by the bench runners, refuses an
xo whose key is not the checkout's (or that has no stamp) unless
XO_BENCH_ANY=1, and sets XO_INLINE_STRICT=1. Last verified: 2026-10-18.

### A program's build information (decision 0354 stage 2, 2026-10-23)

Every program xo builds carries its build information as one JSON
object (compiler/xo binfo.xo build_info_json): `{"id":...,"dir":...,
"sources":{...}, ..., "backend", "target", "xo", "settings":{...}}`, the
settings being 0354's effective values. The Go backend passes it to
rt.SetBuildInfo; a native link adds a small object (Mach-O, ELF, COFF
from compiler/macho, or xo_buildinfo.c for a cc link, kept through
--gc-sections by `-Wl,-u,xo_buildinfo`) with the NUL-terminated symbol
`xo_buildinfo` and the function `xo_buildinfo_line`, whose one line the
crash report prints (std/native/llrt/crash.xo w_build, through the weak
sys_c.weak_xo_buildinfo_line, so a runtime linked by an older xo prints
none). build_native hashes the sources before lower releases the
checker (decision 0352) and carries them in Opts.binfo_sources. `xo
version -m [--json] BINARY...` and `xo replay` find the object by its
`{"id":"` prefix (compiler/cli/core buildinfo.xo); replay passes the
recorded debug-contracts, dbg and error-args settings explicitly. A
native build whose inlined module did not verify, built without
inlining (gotchas/arm64-inline-verify-fallback.md), records the first
verification error as `"inline_fallback"` (outside the build ID): `xo
version -m` prints `build inline=fallback (ERROR)`, the crash report's
line ends `inline=fallback`, and `xo bench --ab` refuses such a binary
(exit 2, naming it) unless `--allow-fallback` (2026-10-24; a scratch
measurement read a fallback build's numbers as a 3 to 7% slowdown).

No path of the build ships (2026-10-25, decision 0354's addendum, Go's
-trimpath): a native binary's `"dir"` is `""` (the Go backend keeps it:
`xo replay` rebuilds from it), `"sources"` lists the program's files and
one `"<std>"` entry, the sha256 of xo's own files (paths relative to the
std directory and their hashes, so it is the same wherever xo is
installed; it was 600 lines, 16 KB of every program), and `--trimpath`
(on; `--trimpath=off` keeps the absolute directories) records the DWARF
compile and std directories relative to the working directory of the build
(`.` when not below it). A debugger finds the sources when started in that
directory; started elsewhere, point it at them: lldb `settings set
target.source-map ./app /path/to/app` (the directory in `DW_AT_comp_dir`,
`dwarfdump --debug-info PROG | grep comp_dir`, replaced by the real one;
verified 2026-10-25 with a program built as `xo build app`). scripts/
release.sh builds the shipped darwin/arm64 xo with xo1 and fails if a
release executable holds the checkout's or the home directory's path
(cmd/xo TestBuildCarriesNoBuildPaths).
Last verified: 2026-10-25.

### HTTP callgrind: short or full

`bench/perf/http/callgrind-docker.sh XO OUT` (decision 0180's quiet
numbers: instructions per request under valgrind in Docker, the same run to
run) has two modes (2026-10-18):

- `MODE=short`: plain and tls, one repeat, N=30: about 3.3 minutes on
  the Mac with a fresh linux xo, 1.8 with the one OUT already holds
  (its commit stamp matches); repeats agree within about 0.3% (arm64) and
  1% (LLVM tls), but byte identical arm64 servers built from two
  checkouts measured plain +0.51% and tls -0.85% (2026-10-18), so
  `cmp OUT1/srv-arm64 OUT2/srv-arm64` before reading a difference under
  1% as the change's. The default for a gate touching HTTP or the runtime.
  On 2026-10-21 (Xo Lang 3) tls repeats of one tree spread more: 34,717
  to 35,339 (1.8%) and 34,302 to 34,716 (1.2%) a request, h2 56,110 to
  57,276 (2.1%); for a change under 2%, run SCENARIOS="tls h2" REPS=3
  on the change and on its parent and compare the means (and the
  ranges: claim it only when they do not overlap, or say they do).
- `MODE=full` (the default): all five scenarios (plain, plain-newconn,
  tls, tls-newconn, h2), REPS=2, N=100: 25 to 45 minutes. Run it nightly,
  per batch, and for a change in:
  - the TLS handshake (std/native/llrt tls13*, tls12*, tlssrv, x25519,
    ed25519, nistec, mlkem, sha*, hkdf, hmac, x509*, tlscert,
    tlsticket): tls-newconn;
  - HTTP/2 (h2srv, h2w, hpack, hconn): h2;
  - connection setup (accept, netconn, poll, the scheduler, pool,
    tasks): plain-newconn;
  - code generation across the board (compiler/ssa's RC and inliner,
    compiler/arm64, compiler/llvm): all of them.
  A change in http1, the router, or std/native/http.xo is covered by
  short.
- `ARCH=amd64` works with either mode; N, REPS, and SCENARIOS given
  explicitly override the mode's.
- `ANN=1` keeps each run's per function counts (callgrind_annotate) as
  OUT/run/ann.SCENARIO.CYCLES with the requests in req.SCENARIO.CYCLES;
  `bench/perf/http/cgcompare.py BASE/run NEW/run [SCENARIO...]` prints
  each scenario's instructions a request, base against new, and the
  functions that changed most (the 2N run minus the N run, so startup
  cancels). Last verified: 2026-10-18.

### Tests in std/ and compiler/ (decision 0262, 2026-10-18)

Repository convention, not a language rule (user code keeps inline
`test` blocks): under std/ and compiler/, tests live only in `_test.xo`
files. xo's source key (internal/xotool Inputs) takes every .xo file of
compiler/ and the std files the compiler uses, so an inline test block
there would rebuild xo for a test change; Inputs skips `_test.xo` files
(TestInputsSkipTests). internal/repotest TestTestsInTestFiles rejects a
`test "` block in any other tracked .xo file under std/ and compiler/.
The tests of compiler/cli/dev and compiler/cli/cmdhelp (moved out of the
sources 2026-10-18) run in internal/selfhost TestSelfhostCLIXoTests
(merge tier). Last verified: 2026-10-18.

std modules' own tests (decision 0262): a std module's tests, with its
private functions, go in a `_test.xo` file beside its source
(std/native/llrt/scan_test.xo for scan.xo) and run with `xo test
--std-module=MODULE [--backend=llvm|arm64]`, MODULE as `use` names it
below std (`json`, `native/http`, `native/llrt/scan`). Like `--std=DIR`
it is a developer flag of xo1, not in `xo test -h`. internal/stdtest
runs every such module: -short on the host's native backend, the
default on LLVM and arm64, XO_STDTEST_BACKENDS to name them. The
modules run as parallel subtests, XO_STDTEST_JOBS at a time (default
min(GOMAXPROCS, 4)). Keep a std module test quick (a few seconds): a
large differential corpus generated from Go goes in
internal/stdtest/testdata/corpus/NAME as a plain test directory over
std's public API, which TestStdCorpora runs outside -short, and the
module's _test.xo keeps a sample (2026-10-21: gomath's 21,460 rows took
143 to 254 s of -short and pushed it past go test's 10 minutes; now 694
rows, about 12 s, and the whole table in testdata/corpus/gomath).
`go run ./scripts/stdcorpus MODULE...` writes the corpora generated from
Go's own package (testdata/corpus/MODULE/MODULE_test.xo: Go's results
on its test file's strings and seeded random inputs, mapped as std/rt
maps them), and `go run ./scripts/stdcorpus -check` fails when one
differs from its generator (2026-10-21: mail first; the Go oracle kept
as data whatever implements a module on the Go backend). std/uuid's
oracle is github.com/google/uuid, not Go's standard library: its
generator runs scripts/stdcorpus/uuidoracle, its own module (go.mod
and go.sum there; the repository's go.mod stays standard library only),
which needs that module in the Go module cache or the proxy.
Last verified: 2026-10-21.

### Gate and fuzz runners (2026-10-18)

- `scripts/pg-diff.sh XO WORK [IMAGE...]` (decision 0278): the Postgres
  differential test of std/sql's driver: the statements of
  scripts/pgdiff/cases.txt through pgx's pgconn (scripts/pgdiff/gopgx, its
  own Go module pinned to pgx v5.11.0) and through the Xo driver
  (scripts/pgdiff/xo) against PostgreSQL in Docker (postgres:17-alpine and
  postgres:14-alpine by default; the containers it starts are removed by
  ID), outputs compared line by line; then (TLS=0 skips it)
  postgres:17-alpine with ssl=on and a test CA made with openssl: every
  statement over sslmode=verify-full, and the ok or fail outcome of each
  sslmode with right, wrong, and no roots (`PG_ROOTS` for the Xo side,
  `sslrootcert` for pgx). Runs outside the heavy lock. Last verified:
  2026-10-20.

- `scripts/gate-http.sh OUTDIR [STEP...]`: the gate for a change to
  std/native, the runtime, HTTP, TLS, or the compiler, from the checkout
  under test: build (xobuild, unless XO_CLI is set), short, llvm goldens,
  arm64 goldens, tls (the TLS, crypto, and HTTP/1 diff tests with the fuzz
  corpora), h2diff (alone: it is load sensitive), codegen (the constant
  time checks), fp (bootstrap-xo.sh from $XO_SEED or the newest cached
  seed; the stage outputs deleted after). OUTDIR/gate.log has a
  STEP=STATUS line per step; each step's output is OUTDIR/STEP.log. Run
  it in a heavy slot. Add `MODE=short bench/perf/http/callgrind-docker.sh`
  for the cost of an HTTP change.
- `scripts/pgtls/run.sh [XO]`: std/tls against a real Postgres
  (decision 0281): a test CA and server certificate (openssl),
  postgres:16-alpine with ssl=on in Docker on 127.0.0.1:55432, then
  scripts/pgtls/main.xo on the arm64 and Go backends (verify-full,
  verify-ca, require, each followed by a StartupMessage over TLS), which
  must print the same lines. Last verified: 2026-10-20
- `scripts/fuzz-http.sh OUTDIR [SECONDS]`: TestHTTPFuzz (seeded from the
  clock) and the Go fuzzer on FuzzHTTPDiff, SECONDS each (default 1800);
  new FuzzHTTPDiff failures land in internal/llvm/testdata/fuzz for
  review.

### internal/llvm's full run (2026-10-19)

internal/llvm's full tests took about 55 minutes of a heavy slot (3,333
s on the shared Mac, 2026-10-18), most of it clang at -O2 with LTO on
the golden programs. Since 2026-10-19 (owner approved, Xo lang 4):

- **Clang cache**: TestMain points $XO_CLANG at a caching clang
  (internal/xotool clangcache.go, the command internal/xotool/clangcache,
  built per test binary), which keys each IR to object or link call on
  clang and its version, the arguments, and the contents of every input
  (not paths), in XO_CC_CACHE's directory (<user cache>/xo/testcc,
  entries `clang-<key>`, trimmed after XO_CACHE_MAX_AGE). A change that
  leaves the IR alone (an arm64 emitter change, a test change) skips
  clang: TestGolden 279 s cold, 26 s warm. XO_CLANG_CACHE=off turns it
  off; the -object_path_lto object lives in the entry, so dsymutil finds
  a hit's DWARF. A test that picks a clang of its own when $XO_CLANG is
  unset (the Windows cross toolchain's) reads xotool.UserClang, $XO_CLANG
  as the user set it.
- **-O1 goldens**: TestGolden, TestGoldenSplit, TestDebugInfoSameOutputLLVM,
  and TestGenGoldensNative build at XO_LLVM_OPT=O1 (single module builds
  with no LTO), except the -O2 sample o2Goldens (llvm_test.go:
  shortGoldens plus crypto_mlkem, native_crypto, native_json, json_text,
  tls13_rfc8448, tls_nistec, cpu_sha256, http_tls), which build as a
  release build would. `XO_TEST_LLVM_OPT=O2` (O0 to O3) builds every
  golden at that level, as release builds in TestDebugInfoSameOutputLLVM:
  use it for a change to what clang's optimizer sees (attributes,
  metadata, intrinsics).
- **No Go reference in TestGolden**: want.txt pins the output; the Go
  backend builds only under `-golden-update` (which writes want.txt).
  TestGoldenGoParity (XO_TEST_GO_PARITY=1) builds every golden with the
  Go backend against want.txt; scripts/gate.sh's llvm step sets it when
  the branch changes compiler/gen or std/rt, and the batch full tier
  always should. A golden with a `want_go.txt` (a known Go backend gap:
  net_only and native_sql, whose Postgres driver is native only,
  decision 0278 point 9; 2026-10-21) is checked against that file
  instead, and `-golden-update` writes its want.txt from the LLVM run.
  `xo dev goldens --backend=go` reads want_go.txt the same way
  (compiler/cli/dev/goldens.xo, 2026-10-21).
- **TestGoldenSplit**: no LTO, ThinLTO too for shortGoldens only.
- **A gate's goldens** (owner, 2026-10-22: the whole package cost 46 to
  56 minutes a gate): scripts/gate.sh's llvm step runs TestGolden,
  TestGoldenSplit, TestGoldenGoParity and TestDebugInfoSameOutputLLVM
  only for the golden programs the change touches, named in `XO_LLVM_GOLDENS` (comma separated; `none`
  for no golden; unset runs every one; the o2Goldens sample joins any
  selection). A golden is touched when its directory changed or its LLVM
  IR did: TestGoldenIRManifest (`XO_LLVM_IR_MANIFEST=FILE`) emits every
  golden's IR with TestGolden's options and a clang that fails at once
  (`--asm=FILE`, about a second a golden), replaces the repository's path,
  and writes each golden's SHA-256; the step makes it with the branch's xo
  and with the merge base's (built in a worktree of its own, the branch's
  manifest test copied in; kept by commit in `$XO_LLVM_IR_CACHE`, default
  `<user cache>/xo/llvm-ir`) and compares them. Every golden runs for
  `XO_GATE_LLVM=all`, a change to internal/llvm's tests, internal/xotool,
  compiler/xo (what builds and links them past the IR) or compiler/llvm
  debug.xo (the --debug-info IR), or a manifest that could not be made; the log line says which and how many
  (`llvm goldens: N of T goldens, ...`). The package's other tests (the
  diff tests, TestLeakCheck, TestGoldenTasks and TestGoldenASan, which
  build with runtimes of their own, ...) run as before. The IR is hashed
  without the lines and columns of std files and of the debug metadata
  (normalizeIR): a golden's output names only its own main.xo's
  positions, and a function added to the prelude moves every program's
  std lines (Str.clone, 44099405f: 300 of 300 goldens before, 1 of
  300 after: views_clone, the golden it added). Measured 2026-10-22 on
  the Mac under load (gate class, each manifest about 100 s): the whole
  package 4,158 s; 44099405f (std) 1 of 300 goldens, 2,092 s;
  177602f83 (compiler/llvm and ssa, ML-KEM's NEON) 41 of 299, 3,044 s;
  a deliberate break of base64 encode 4 of 300 (base64_basic,
  crypto_keys, websocket, websocket_tls), and TestGolden, TestGoldenSplit
  and TestGoldenTasks failed on base64_basic, crypto_keys and websocket.
  The rest of the step's time is the package's other tests.
  Last verified: 2026-10-22.
- **TestDebugInfoSameOutputLLVM**: o2Goldens as --release with and without
  --debug-info; the rest -s against a plain -O1 build (TestGolden's,
  from the cache).

Measured one at a time on the shared Mac (load averages 30 to 250, so
read these as rough), four heaviest tests, seconds:

| Test | main | items 3, 4 (all -O2) | + -O1 | + cache, cold | warm |
|---|---|---|---|---|---|
| TestGolden | 310 | 360 | 276 | 270 | 25 |
| TestGoldenSplit | 250 | 211 | 126 | 170 | 38 |
| TestDebugInfoSameOutputLLVM | 566 | 589 | 379 | 400 | 65 |
| TestGenGoldensNative | 134 | 192 | 103 | 76 | 39 |

(The last two rows' cache columns are the final form, -s pairs and -O1
without LTO, at load averages of 85 to 190; the -O1 column of
TestDebugInfoSameOutputLLVM compared two identical builds, fixed since.)
The whole package (`go test ./internal/llvm`): 3,333 s on main, 1,692 s
with a cold clang cache, 567 s warm (an unchanged IR), all passing.

Last verified: 2026-10-19

## Package layout (P1, implemented parts)

```
cmd/xo/                 CLI entry point; `xo parse`, `xo check`, `xo build`, `xo run`, `xo test`, `xo bind`,
                        `xo fmt`, `xo vet`, `xo query`, `xo doc`, `xo explain` (tools.go), `xo dump` (dump.go),
                        `xo mcp`, `xo lsp`, `xo serve` (agent.go), hidden `xo lexdump` (lexdump.go),
                        `xo astdump` (astdump.go), `xo typedump` (typedump.go), `xo ssadump` (ssadump.go)
                        `xo fmt`, `xo vet`, `xo query`, `xo doc`, `xo explain` (tools.go), `xo dump` (dump.go), `xo fix` (fix.go)
editors/vscode/          VS Code extension (TextMate grammar, Markdown injection, xo lsp client); see editors.md
internal/token/         token kinds, keywords, Pos and Span (file, line, col, offset)
internal/lexer/         tokens, newline termination (core 1.3), string interpolation parts,
                        doc comments; dump.go: the canonical token dump shared with compiler/lexer
internal/ast/           syntax tree for the whole 0.4 grammar; JSON dump (ast.ToJSON/Dump)
internal/parser/        recursive descent parser with error recovery; SignatureMode
internal/diag/          diagnostics (core 10.2 JSON), codes.go is the registry of all codes
internal/check/         name resolution and type checking (see below); dump.go: the canonical
                        `xo typedump` sections (names, types, sigs, diags) shared with compiler/check
internal/format/        canonical printer for `xo fmt`; format.Decl prints signatures
internal/vet/           `xo vet` advisory checks (XO0207, XO0208, XO0703, XO0901, XO1103)
internal/fix/           `xo fix`: the check fix loop and the migration rewriter registry
internal/query/         `xo query` over a check run; Surface is shared with internal/doc
internal/edit/          `xo edit`: rename, add-field, change-sig through resolved symbols
internal/mcp/           `xo mcp`: MCP (JSON-RPC 2.0 over stdio) tools check, query, explain, fmt, test, build
internal/lsp/           `xo lsp`: LSP 3.17 over stdio; rechecks open documents as a check overlay (editors.md)
internal/serve/         `xo serve`: unix socket server, cache of checked sessions keyed by file mtimes
internal/doc/           `xo doc` Markdown and JSON documentation
internal/gen/           Go code generation with //line directives (see "Code generation")
internal/build/         check, generate, write the build directory, `go build` per target;
                        llvm.go: the LLVM backend build (IR + std/llrt runtime -> clang)
                        arm64.go: the native backend build (assembly + std/llrt runtime -> cc)
                        rtobj.go: the local runtime object cache; rtpack.go: prebuilt runtime packs
                        (decision 0157, see "Prebuilt runtime")
internal/llvm/          P3 prototype: LLVM IR from internal/ssa (see "LLVM backend (P3 prototype)")
internal/arm64/         native AArch64 backend from internal/ssa (decision 0049, see "arm64 backend (native)")
internal/ssa/           shared typed SSA IR for native backends (decision 0045): builder from the
                        checked AST, verifier, printer; goldens in testdata/ (see "SSA IR");
                        stages.go: BuildStages and BuildRaw for `xo ssadump` (decision 0150)
internal/selfhost/      Go side of the self hosted components (tests only): build compiler/lexer,
                        compiler/parser, compiler/check, and compiler/ssa and compare their dumps with
                        lexer.Dump, parser.Dump, check.Result.Dump, and the build stage of
                        ssa.BuildStages (decisions 0076, 0127, 0141, 0150)
internal/progen/        the random program generator of decision 0110 (TestCompileProperty, the parser differential)
compiler/xo.mod         compiler/ is one Xo module, github.com/xo-lang/xo/compiler (the parser imports the lexer)
compiler/lexer/         the lexer in Xo (decision 0076): token.xo, lexer.xo, dump.xo, main.xo (CLI),
                        lexer_test.xo; Unicode and quoting come from the prelude (decision 0079)
compiler/parser/        the parser in Xo (decision 0127): ast.xo, parser.xo (tokens, lists, docs, declarations,
                        types), stmt.xo, expr.xo, pattern.xo, dump.xo, main.xo (CLI), parser_test.xo
compiler/check/         the checker in Xo, stages 1 to 8 and module loading (decision 0141): objects.xo (tables:
                        objects, modules, files, records), load.xo (directories, xo.mod, std, imports), base.xo
                        (diagnostics, records, lookups, scopes), decl.xo (declarations, type declarations,
                        signatures, type expressions), types.xo (the type table, substitution, text), infer.xo
                        (unification, assignability, interfaces), effects.xo (effect sets of function types),
                        expr.xo, call.xo, stmt.xo, pattern.xo (the typed walk), habits.xo (habits that recover a
                        type), dump.xo (passes, names, types, and diags sections), modfile.xo (xo.mod, xo.work,
                        xo.lock, versions, the module cache, vendor/, content hashes: internal/mod for a check),
                        deps.xo (dependency rules, deny rules, effect ceilings), main.xo (CLI), check_test.xo,
                        info.xo (the records the SSA builder reads: natural types, constructors, calls,
                        patterns, parameter bindings; native mode loads std/native, decision 0150),
                        cimport.xo (`use c`: the binding's key and prefix, its declaration file read from
                        the cache cbind wrote, `.xo/cbind` or $XO_CBIND_DIR; decision 0152)
compiler/ssa/           the SSA builder in Xo, stages 1 to 8 of decision 0150: ir.xo (types, values, blocks,
                        functions as tables; the printer), build.xo (types, functions and generics, Braun
                        SSA construction, checks after calls, generated functions), expr.xo, stmt.xo,
                        call.xo (calls, closures, `?`, std calls), show.xo (Display and Debug text, error
                        frames, sized integers), iface.xo (interface values, Error/Display/Debug values,
                        method values, equality and order helpers), map.xo (Map and Set), contract.xo
                        (contracts, invariants, loop bounds, real-time sections), defer.xo (`defer`, the
                        builder's landings and fault continuations), conc.xo (scopes, tasks, channels,
                        waits), json.xo (the generic intrinsics: JSON codecs, SQL rows, any, cast, status;
                        calls of `use c` bindings), testmode.xo (test binaries: test functions, the entry,
                        `expect`, the fakes of std/testing, generated values), passes.xo (pruneUnwind,
                        addLandings, cleanup), rc.xo (InsertRC: borrow inference, liveness, retains and
                        releases, cancellation, reuse, sinkLandings, pruneLandings), verify.xo (the
                        verifier and its verdicts on mutants), main.xo (CLI)
compiler/llvm/          the LLVM emitter in Xo (decision 0166): gen.xo (types and layout, globals, the reference
                        counting, equality, environment, and box helpers), func.xo (functions, instructions,
                        landing pads), call.xo (calls, inline operations, the task runtime), held.xo (the
                        memory of `var` parameters by reference), text.xo (IR strings, Float bits), ranges.xo
                        (value ranges, facts, assumes, the inlinable set: decision 0192), main.xo (CLI)
compiler/arm64/         the arm64 emitter in Xo (decision 0158): state.xo (the emitter's state, SSA access),
                        gen.xo (the program: symbols, sections, literals, xo_entry, value layout), mir.xo,
                        isel.xo and calls.xo (instruction selection: arithmetic, compares, lists, strings,
                        calls, runtime calls, inline counting), opt.xo (memos, bounds, threading, dead code,
                        constant hoisting, compare selection, flag reuse), ranges.xo, lsr.xo (strength
                        reduction, rotation, load pairs, latch placement), jumptab.xo, regalloc.xo,
                        emit.xo (frames, templates, parallel moves, fault stubs, call site tables, far
                        slots and branches), rc.xo (counting helpers), closures.xo (closures, boxes,
                        equality), byref.xo (decisions 0151, 0153), inline.xo, stages.xo (what a stage
                        of the port does not emit yet), main.xo (CLI)
compiler/prune/         age and size pruning of xo's caches (decision 0223): prune.xo (run, mark_used,
                        Policy, from_env), prune_test.xo (MemFs and the fake clock; last verified: 2026-10-14)
compiler/tzsel/         the zone data a program needs (decisions 0094, 0157): tz_use (the constant names
                        std/time's zone is called with), tz_select (their TZif data from std/tzdata/
                        zoneinfo.zip, the whole zip for a name known only at run time, or the host's
                        database), zip_entry; used by compiler/xo (native and wasm builds) (last verified:
                        2026-10-17)
internal/selfhost/testdata/modules/  module fixtures of the checker differential: app/ (a main module with
                        requires, ceilings, deny rules, xo.lock) and cache/ (the module cache XO_MODCACHE points
                        at in TestSelfhostChecker; last verified: 2026-10-10)
internal/selfhost/testdata/ssa/stage4/  a program of the SSA stage 4 constructs (defer at every way out,
                        faults in deferred code, contracts, invariants, loop bounds, a real-time section);
                        every repository directory of .xo files is a run of TestSelfhostSSA and a mutation
                        source (last verified: 2026-10-11)
internal/selfhost/testdata/gogoldens/  the recorded Go side of the TestSelfhost* differentials as hashes
                        (scripts/selfhost-record-goldens.sh; one directory per kind, [0-9a-f].txt per unit,
                        pool-[0-9a-f].txt the member hashes; decision 0178 stage 7; last verified: 2026-10-12)
internal/selfhost/mutlex/  a frozen copy of internal/lexer and internal/token for the mutation generators
                        (TestSameAsLexer while internal/lexer exists); internal/selfhost/frozen_test.go holds
                        copies of llvm.SplitIR, ssa.SplitModule, gen.FormatFile (TestFrozenCopies;
                        last verified: 2026-10-12)
scripts/linux-test.sh   the test suite in a Linux container (arm64 or amd64), image from bench/linux/Dockerfile
                        (see "Linux (containers)")
scripts/xo1-host-check.sh, scripts/remote-xo1-x86.sh  xo1 built on this host as `xo` builds it, its goldens,
                        and the bootstrap fixed point without Go; the second runs it on an EC2 linux/amd64
                        host (see "xo1 on x86-64 hosts"; last verified: 2026-10-12)
internal/benchharness/  test only: runs the bench/harness Python self test under go test
internal/gotool/        the go command (`$XO_GO`, PATH), its version once per process, MinGo and
                        TestedGo, the go.mod `go` line, and Xo's Go cache dir (decision 0176)
internal/gobind/        `use go` bindings: go list + go/importer + go/types -> Xo declaration
                        file and Go adapter (decision 0041 prototype, see "Go interop")
internal/cbind/         `use c` bindings: clang's JSON AST dump of a header -> Xo declaration
                        file and C adapter (decision 0116, see "C interop")
std/                    std declaration files (prelude.xo, archive.xo, cli.xo, csv.xo, go.xo, http.xo,
                        json.xo, log.xo, path.xo, regexp.xo, sql.xo, task.xo, term.xo, testing.xo,
                        time.xo, toml.xo) embedded by std/embed.go
std/rt/                 package rt, the runtime linked into every program (Go, embedded
                        by std/embed.go as std.Runtime and copied into each build)
std/llrt/               xo_rt.c, the C runtime of the LLVM and arm64 backends (embedded as std.LLRuntime),
                        and the files it includes: xo_std.c, xo_bytes.c,
                        xo_http.c (decisions 0074, 0081; weak stand-ins and forwarders only: the server,
                        its connections, sinks, the client and TLS are std/native/llrt's Xo);
                        testdata/*.c, C tests and differential drivers (not embedded)
scripts/release.sh      builds the release archives into dist/ (decision 0157, see "Prebuilt runtime")
scripts/disttar/        writes a reproducible .tar.gz of a directory (release.sh)
std/native/             std implemented in Xo for the native backends (decision 0074): <name>.xo
                        implements std/<name>; rt.xo, rtjson.xo, rthttp.xo declare C intrinsics
                        (embedded as std.NativeFiles, loaded only for native builds)
std/native/llrt/        the native runtime written in Xo (decision 0180): unicode.xo (case mapping,
                        Rune classes, quoting), unicode_tables.xo, and unicode_classes.xo (regexp's
                        \p{...} classes), both generated from Go's unicode and strconv tables:
                        `go test ./internal/llvm -run TestUnicodeTables -golden-update`; imported as
                        `use native/llrt/<name>` by std/native only.
                        text.xo (Int parse, Duration and Time text, Debug quoting) and
                        strings.xo (replace, trims), decision 0193,
                        with C path text_c.xo; num.xo (Int and Float text,
                        Float.parse, hex, base64) and jsontext.xo (std/json's parser,
                        tape readers, string, Float, and Bytes text); show.xo
                        (Rune and UInt64 text of Display and Debug), tz.xo (std/time's zone
                        rules), report.xo (a property test case's report) with show_c.xo,
                        tz_c.xo, report_c.xo (step 5). `XO_LLRT_C=<name>[,<name>]` (or `all`)
                        at build time selects a module's C path (-DXO_TEXT_C, -DXO_SHOW_C,
                        -DXO_TZ_C, -DXO_REPORT_C, compiled locally, never in a pack). Decision
                        0198 deleted the C paths of unicode (XO_UNICODE=c), strings, num, and
                        jsontext; those names now select nothing; ints.xo (sized integer faults, UInt64
                        division, floor division, Str key hash, Int's checked_ methods), glob.xo
                        (path.Match), digest.xo (SHA-256 via the generated digest_tables.xo,
                        HMAC, PBKDF2, base64, verify_password), gen.xo (property test values)
                        with ints_c.xo, glob_c.xo, digest_c.xo, gen_c.xo (decision 0195;
                        -DXO_INTS_C, -DXO_GLOB_C, -DXO_DIGEST_C, -DXO_GEN_C).
                        Decision 0198's second round (2026-10-13) deleted the C paths of
                        tz, report, show, atomic, fspath, gen, glob, digest, ints, rc,
                        alloc, scan, sysio, http1, hpack, and procspec with their
                        <name>_c.xo; only text, sched, and net still have one
                        (`XO_LLRT_C=text,sched,net`); -DXO_INTS_C now guards only the C
                        versions of the intrinsics
                        (Last verified: 2026-10-13)
```

`std/*.xo` are bodiless signature listings of `spec/std.md` (draft 0.4),
parsed in `parser.SignatureMode`. The encoding of std.md pseudo syntax
(`[var]` receivers, `Int.parse` statics, implicit receiver type parameters,
opaque handles, `@moves`) is documented at the top of `std/prelude.xo`. The
prelude imports `std/task` for `task.Group` (`os.tasks`, `s.group()`), so
`std/task` is always loaded. Last verified: 2026-10-07.

`internal/check` (package `check`, entry point `check.Check(dirs)`):

- `load.go`: module discovery, xo.mod roots, import resolution (XO0602,
  XO0604), std loading.
- `decl.go`: top level objects, type declarations, signatures, methods,
  effect clauses, type expressions (XO0101, XO0603, XO0606, XO0702).
- `types.go`, `infer.go`: types, substitution, unification with a trail,
  assignability with the three implicit conversions (core 2.10), interface
  satisfaction (structural, plus builtin rules for Eq, Hash, Ord, Json,
  Display, Debug), error conversion at `?`.
- `expr.go`, `call.go`, `stmt.go`: bidirectional checking of bodies. The
  expected type of a value is a type, a `retWant` (return position of
  `-> T ! E`), or `discard` (a value whose Result must be handled, XO0201).
- `pattern.go`: pattern typing and exhaustiveness (usefulness algorithm with
  witnesses, XO0301 to XO0304).
- `effects.go`: effect sets with labels, effect parameters, effect inference
  variables, and references to the inferred effects of non pub functions;
  fixpoint and XO0701 obligations with call chains (`chainSteps`, shared
  with `query.go`).
- `query.go` (2026-10-08): accessors for the tools: `Inspect`,
  `Info.Performed` (effects a body performs with call chains),
  `Info.Funcs`. `File.Src` keeps each file's source.
- `flow.go`: moves of `Buf`/`StrBuilder` (XO0510) and lost updates (XO0520:
  copies out of a collection, including `.or_err(...)?`, and locals mutated
  after being stored into a collection), run after typing.
- `groups.go`: task groups (core 7.1): failure of group tasks (XO0801) and
  values holding a group escaping their scope (XO0802).
- Effect argument elision (core 6.2) is in `decl.go` (`resolveArgsFor`,
  `elidedEffect`): implicit effect variables in parameter and receiver
  types, implicit effect parameters on struct and enum fields, inference in
  local bindings and in result types.

Checker tests in `internal/check`: `rules_test.go` (table driven positive and
negative cases per rule), `planted_test.go` (the Xo side of
`bench/planted/`), `golden_test.go` with `testdata/golden/<case>/want.json`,
`rules04_test.go` (rules new in spec 0.4: XO0206, `is`, tuple fields, local
`const`, widened XO0520, task groups, effect elision, std 0.4 surface),
`corpus_test.go` (bench/paper/v0.3 as data under the current spec, and spec
Appendix A), `mutation_test.go` (opt in), `coverage_test.go` (every emitted
code has a positive case).

Parser notes for spec 0.4 (last verified 2026-10-07): `x is Pattern` is an
`ast.IsExpr` parsed at the precedence of `==` when the identifier `is`
follows an operand; a tuple field `t.0` is an `ast.SelectorExpr` whose `Sel`
is the decimal index (the lexer does not read a fraction after `.`); a local
`const` is an `ast.ConstDecl` statement; `&= |= ^= <<= >>=` are rejected; a
result type may start on the line after the parameters (`->` continuation,
std.md 7).

Parser tests live in `internal/parser`:

- `parser_test.go`: table driven expression, statement, pattern, declaration,
  doc comment, span, error recovery, and fix tests; a no panic test on
  truncated input; `FuzzParse`.
- `spec_test.go`: extracts every fenced block from `spec/core.md` and parses it
  as a file, then as a function body, then in signature mode. Non Xo blocks are
  in an explicit skip table, the 1.6 literal listing is lexed only, and spec
  bugs are listed in `specKnownFailures` (empty since spec 0.4).
  `spec/std.md` blocks are parsed in signature mode after rewriting `[var]`
  to `var`; pseudo signatures such as `Int.parse(s: Str) -> Int?` are dropped
  and logged.
- `corpus_test.go`: every `.xo` file under `bench/paper/v0.2/` must parse;
  `bench/paper/v0.1/` is parsed for information only.
- `sexpr_test.go`: compact S-expression printer used by the table tests.

## Build pipeline (P1)

`.xo` sources -> lexer -> parser -> checker -> Go source (internal/gen, with
`//line` directives) -> build directory with the runtime -> `go build` with
`GOOS`/`GOARCH` per target and `CGO_ENABLED=0` -> static binaries. All stages
are implemented. Last verified: 2026-10-07.

Build modes (decision 0099, 2026-10-09): `build.Options.Release` (only
in `gen.Build` mode, `Options.release()`) sets `gen.Options.Release`,
`ssa.Options.Release` (LLVM, arm64, `--hybrid`), the build info field
`release`, the build id, and a `release` variant of the build directory
key. `gen.contractOn` and the SSA builder's `contractOn` drop clauses
with `Debug` set. Tests and `xo run` never pass it.
`build.Options.NoErrorArgs` (`--error-args=off|false`, any build since decision 0354) makes
`internal/gen` emit `rt.Frame0` for every `?` (build info
`no_error_args`, build directory variant `noargs`). Last verified:
2026-10-09.

**Release preset (decision 0354, 2026-10-23).** `--release` sets the
release value of each setting (`--debug-contracts`, `--dbg`,
`--error-args`, `--debug-info`, `--profile-support`, `--trace-support`,
`--opt`); an explicit flag wins in any build, and a bare flag means on.
compiler/xo opts.xo holds each as -1 (unset), 0 or 1 and resolves it
with the `*_eff(o, release)` helpers; the checker's `set_*` fields reach
the SSA builder (`debug_contracts_on`, `dbg_on`, `error_args_on`,
`trace_on`) and the front end cache key (`settings=`); the Go backend
gets `gen.Opts.debug_contracts`, `dbg` and `no_error_args` (build
directory variants `nodebugc`, `nodbg`, `noargs`). Last verified:
2026-10-23.

With `--backend=llvm` (2026-10-08): checker -> SSA with reference
counting (internal/ssa) -> LLVM IR (internal/llvm) ->
`<build dir>/llvm/prog.ll` plus `xo_rt.c` -> `clang -O2 -flto` -> one host
binary. The build directory keeps `prog.ll` for inspection.

With `--backend=arm64` (2026-10-08): checker -> SSA (internal/ssa,
`InsertRC`, verified) -> assembly (internal/arm64) ->
`<build dir>/arm64/prog-<os>.s` plus `xo_rt.c` -> `cc -O2` -> one binary
for darwin/arm64 or linux/arm64 when that is the host; another arm64
target is written as assembly only (`build.Result.Asm`).

Both native links (and every C compile through `build.CompileC`, the
sanitizer tests included) add `-fexceptions -funwind-tables` (2026-10-09,
decision 0085): faults unwind through the runtime's frames and generated
code's (call frame information, call site tables, `xo_personality`), so
no C function may be assumed not to unwind. When linking a kept
`prog-darwin.s` (`XO_KEEP_ASM=1`) by hand, add the same flags. Last verified: 2026-10-09.

## Code generation

Last verified: 2026-10-08. Rules and choices; details in
`bench/results/gen-run-v0.4.md`.

- One Go package per Xo module (`xoprog/m/x_<module path>`), one file
  `xo.go`; the main module's package also has `xo_main.go` with `XoMain`
  (2026-10-09), the run of `main` that package main calls, so package
  main names no runtime type (see the bullet on runtime generic types). Std modules map onto package `rt`. Go names: types keep their
  Xo name (std types are `<Module>_<Name>` in rt, `Http_Request`), payload
  types `<Enum>_<Variant>`, functions `F_<name>`, constants `C_<NAME>`,
  methods `M_<name>`, fields `F_<name>`, locals `v_<name>` (numbered when
  reused), temporaries `x_<kind><n>`, type parameters `tp_<Name>`.
- Every function, method, and closure takes `tk *rt.Task` first.
- Types: Int is int64 (sized types map to Go sized types), Float float64,
  Str string, Bytes `rt.Bytes` (a string), Rune `rt.Rune`, Duration and
  Time `rt.Duration`/`rt.Time` (int64 nanoseconds), `()` `rt.Unit`, tuples
  `rt.TupleN`, `T?` `rt.Option[T]` (a value: Some flag and V), `Result`
  `rt.Result[T, E]` (with the error frame trace), structs Go structs,
  enums `struct{ Tag int; P any }` with a payload struct per variant (so
  recursive enums work), interfaces Go interfaces with `M_` methods,
  `Display`/`Error`/`Debug` `rt.IDisplay`/`rt.IError`/`rt.IDebug`,
  `Chan`/`Mutex`/`Atomic` pointers to rt handles, List/Map/Set rt
  persistent collections.
- Runtime generic types carry only protocol methods (2026-10-09):
  `XoEq`, `XoHash`, `XoCmp`, `XoDebug`, `XoJsonEnc`, `XoJsonDec`,
  `XoJsonRead`, `XoGenAny`, `XoMark`, `M_display`, `M_debug`. Prelude
  methods of List, Map, Set, Option, Result, Chan, Secret, and json.Patch
  and the generator's helpers are package level functions:
  `rt.List_len[T](tk, xs)`, `rt.Map_put(tk, &m, k, v)`,
  `rt.List_At(xs, i, loc)`, `rt.List_PushFast(xs, x)` (`rtFuncMethods`,
  internal/gen/decl.go). Go compiles every method of every generic type a
  package mentions, in that package, so methods there cost every
  generated package (`gotchas/go-generic-method-sets-per-package.md`,
  enforced by `TestGenericMethodSets`). These types satisfy Xo interfaces
  through generated adapters (also in the `--no-mono` registry).
- A function value of type `fn(A) -> R ! E` is `func(*rt.Task, A)
  rt.Result[R, E]`; one that cannot fail uses E = `rt.Never`, so the form
  never depends on whether a generic error parameter was instantiated with
  Never. Declared functions return R, `rt.Result[R, E]`, or nothing for
  `()`; a call whose declared error became Never takes `.V`. Declared
  functions used as values are wrapped in closures; subsumption (a function
  that cannot fail where a failing one is expected, a converting error)
  is an adapter closure.
- Generics are Go generics (constraints are `any`; Eq, Hash, Ord, Display
  go through rt functions that switch on the dynamic type or call the
  generated `XoEq`/`XoHash`/`XoCmp`/`XoDebug`/`M_display` methods). A
  method is a Go method unless it has its own type parameters, its
  receiver is a scalar prelude type, its receiver restricts the type's
  arguments (`List[Str].join`), or its signature instantiates its own
  receiver type with other arguments (Go instantiation cycle); then it is a
  top level function `<Type>_M_<name>` (user) or `<Type>_<name>` (rt).
- Expressions that need statements (blocks, if, match, `?`, `??`, scope,
  select, `with`) are hoisted; operands before one that needs statements
  are spilled into temporaries to keep left to right evaluation, except
  places (`var` receivers, `&` arguments). Blocks deliver their value to a
  destination: discard, assign, or return (so arms of a tail if/match
  return directly, core 2.6).
- A block that contains `defer` runs as `func() int { ... }()`; Go's
  defer gives block scoping, `tk.Shielded` the cancellation shield (core
  3.5, 7.2). `return`, `break`, `continue`, and `?` cross it as control
  codes dispatched after the call. `scope` is the same with
  `rt.Scope.Close` deferred.
- Integer arithmetic, conversions, indexing, and slicing call rt functions
  that fault with the values and the `file:line:col` of the operation;
  rt faults without a position use the innermost `.xo` frame of the stack.
  Int (int64) operations (2026-10-08): before each body is generated, a
  range analysis (internal/gen/ranges.go: intervals per binding through
  constants, `let`/`var`, range loops, comparisons, `%`, masks, `len()`,
  sized integer widening, loops iterated to a fixed point with widening,
  plus symbolic `i <= len(xs) - 1` bounds for lists under parameters and
  `let`/`for` bindings) marks operations that cannot fault; they are
  plain Go, and `xs[i]` proven in range is `rt.List_AtU`. Other operations
  with a constant operand use one comparison forms (`rt.AddIUp`,
  `rt.SubIDown`, `rt.MulIK`/`MulKI`, plain `/` and `%` by a constant other
  than 0 and -1); the rest call `rt.AddI..RemI` and `rt.List_AtM` (typed,
  inlinable, fault by `panic(&arithFault{...})`, converted to a
  `*FaultErr` with the operation's stack at the first recover), or under
  `--no-mono` the generic `rt.AddS` family. `TestRangeElision`,
  `TestRangeDifferential` (analysis on and off must print the same values
  and faults), and golden `test_range_checks` cover it.
- `for i in a..b` and `a..=b` over a range literal are counted Go loops
  (bounds evaluated once); other Range values iterate `.All()`
  (2026-10-08, `gotchas/range-over-func-loop-cost.md`).
- A struct field whose type contains the struct by value (through Option,
  Result, tuples, other structs) is stored in `rt.Box[T]`.
- Assignment resolves the place first (index operands left to right), then
  writes. Through List elements and fields it writes in place: each list
  on the path is made the unique owner of its array (`if
  !rt.List_Mine(p) { p = rt.List_Unshared(p) }`), then
  `rt.List_PtrM(p, i)` or `rt.List_PutM(p, i, v)`; a compound
  assignment uses one address for the read and the write. Through a Map
  value or a box it reads, marks, modifies, and writes back
  (`m[k].count += 1`). A std `var` method on an element (`xs[i].push(x)`)
  gets the element's address; a user `var` method gets a marked copy
  that is written back. The value is computed first when it could write
  the place's root (2026-10-08).
- List ownership (2026-10-08, internal/gen/own.go, rationale and numbers
  in `bench/results/perf-list-2026-10-08.md`): a read of a binding,
  field, or element whose type holds lists is wrapped in an own mark
  (`rt.List_Own(x)`, or a function literal calling generated `XoMark` methods
  for structs, tuples, Options, Results; `rt.Own` for type parameters)
  unless the reading expression borrows it: field and element bases,
  receivers of read only List methods, comparison, `+`, and
  interpolation operands, loop subjects the body does not write, by value
  arguments of user calls without `var` parameters (each checked with
  `mutates`; `var` parameters may alias). Std call results holding lists
  are marked except fresh producers (`freshStd`); closures mark what they
  capture; interface conversions mark. Structs and newtypes whose fields
  hold lists get `func (v *T) XoMark()`. `push` on a binding or field
  path is `if rt.List_CanPush(p) { p = rt.List_PushFast(p, x) } else { p
  = rt.List_PushSlow(p, x) }`. A field read of an element is
  `rt.List_RefM(xs, i, at).F_f` (no copy of the
  element) when the statement cannot write the list. Rules for changes:
  `gotchas/list-own-marks.md`. Tests: golden `list_value_semantics`
  (also under `-race`), `TestListModel`.
- Monomorphization (internal/gen/mono.go; added 2026-10-08 as `--mono`,
  the default since 2026-10-08; `--no-mono` on `xo build`, `run`, `test`,
  `gen.Options.NoMono`, `build.Options.NoMono` turn it off, `--mono` is
  still accepted): calls of user generic functions and methods with
  concrete type arguments call specialized copies `<name>__<types>` generated into the
  calling package (methods of generic types as top level copies, receiver
  first); `==`/`<` on a type parameter in a copy lower as for the concrete
  type, with typed `XoEqT`/`XoCmpT` methods generated for non generic
  structs and newtypes; constraint method calls call the concrete method;
  `List.map/filter/fold/any/all/find` with an inlinable closure literal
  become loops with the body inlined; a non capturing literal passed to a
  user generic function gets a `_lit` copy that inlines it. Fallback to
  the Go generic function: non concrete type arguments, copy chains
  deeper than 8, more than 64 copies of one function per package. Go
  forms are decided on the generic types, so copies keep the generic
  signature form. The build directory hash includes the lowering.
  `TestGolden` (default lowering, Go frame lists dropped) and
  `TestGoldenNoMono` (exact) run every golden program both ways. Numbers:
  `bench/results/perf-mono-2026-10-08.md`.
- Request scoped arenas (2026-10-08, internal/gen/arena.go; on by
  default since decision 0066, `--no-arenas` on `xo build`, `run`, `test`
  turns them off, `--arenas` is accepted for older command lines;
  `gen.Options.Arenas`, `build.Options.NoArenas`, build info
  `no_arenas`; off generates exactly the code of before arenas; the
  llvm and arm64 backends ignore them): the generated main calls
  `rt.EnableArenas()`. A
  closure literal passed to `r.handle` on a local router whose request
  parameter is used only through `json`, `param`, `param_int`, `query`,
  `header`, and the Str fields (never in a nested closure) is followed by
  `rt.Http_Router_arenaLast(&r)`. `http.json`/`http.text` calls whose
  value is the handler literal's result (tail of the body through
  if/match/block tails, or a `return`) become `rt.Http_json_arena` /
  `rt.Http_text_arena`. `m.values()`, `m.keys()`, `m.entries()`,
  `s.to_list()` that are only the receiver of a List consumer that keeps
  nothing (`arenaConsumers`) or the iterable of a `for` become
  `rt.Map_values_arena` and friends. The build directory hash includes
  the flag. Tests: `TestArenaEscapeAnalysis`, `TestArenaConfineRandom`
  (160 random handler bodies, confined exactly when no escaping use),
  `TestGolden` (arenas) with `TestGoldenNoArenas` (every golden with
  `--no-arenas`, same want.txt), `TestGoldenRace` with
  `TestGoldenRaceNoArenas`, goldens `arena_escape` and `arena_holes`,
  `TestExampleArenaDiff` and `TestExampleArenaServeDiff`
  (examples/notes-service tests and real server both ways),
  `TestReplayServer` (recorded arena bodies), `TestArenaHTTP2` (std/rt,
  not with -short: the server over a TLS listener with net/http's
  built in h2 client from `httptest`; `rt.arenaPoison` fills released
  arena buffers with `#` so a late reader shows at once) and
  `TestServeNoH2C` (plain listeners speak HTTP/1 only). On HTTP/2 serve
  writes a heap copy of an arena body. Last verified: 2026-10-08.
- Interface conversions (2026-10-08, internal/gen/adapt.go): a value whose
  Go methods differ from the interface's (a method that cannot fail where
  the interface's can) is wrapped in a generated adapter; inside a
  specialized copy the concrete type is known; an interface value is
  wrapped around the source interface; generic Go code converts with
  `rt.Iface`, which consults adapters that package main registers with
  `rt.RegAdapter` for every concrete user type named in generated code and
  every interface generic code converts a type parameter to.
- The std structs, enums, newtypes, and interfaces are generated into
  `std/rt/std_gen_M.go`, one file per std module M (the prelude's in
  std_gen_prelude.go; one shared std_gen.go until 2026-10-21), by
  `go run ./scripts/stdgen` (internal/stdgen: a program using every std
  module through `xo dev dump go --std-types`, compiler/gen stdtypes.xo,
  gofmt, then split by each declaration's module prefix; never edit the
  files by hand). Each file's third line records a hash of its module's
  type declarations: internal/repotest's `TestStdGenCurrent` (-short, no
  xo) fails when one changed since, and internal/gen's
  `TestStdGenOutput` (full tier, runs xo) when a file differs from the
  generator's output, is missing, or is left over.
  Fields only the runtime uses (Http_Request's XParams, XValues,
  XRawQuery, XFields; Http_Response's XStream, XFields) are listed in
  stdtypes.xo rt_extra_fields. Opaque std handles and every std function are hand
  written in rt, except the modules listed in std/go_compiled.txt
  (decision 0312, 2026-10-21: path first). For those, the checker loads
  std/native/M.xo for the Go backend too (check load.xo
  load_go_compiled), gen routes calls to its functions (call.xo go_impl),
  and stdgen writes no signature checks for them. The same command generates `std/rt/std_sigs_M.go`, a
  `var _ func(*Task, ...) R = Module_name` line (a free prelude function
  is `Prelude_name`, compiler/gen std_free_prefix, 2026-10-21: the
  prelude's types and methods have no prefix, and a lower case free
  function name would not be exported) for each std and prelude
  function and method that has no Xo body and is not generic, nor a
  method of a generic type (List, Map, Option, ...), nor `fault` or
  `dbg` (lowered where called): 571 on 2026-10-20, a Go method as
  `(*T).M_name`. `go vet ./std/rt` fails when a hand written signature
  is not the one the Go backend calls. Last verified: 2026-10-20.
- std/sql (2026-10-08, std.md 10): `std/sql.xo` declares Db, Tx, Row, SqlValue,
  SqlErr; `std/rt/sql.go` implements them over a Go `SqlBackend` interface
  (values nil, int64, string, float64, bool, Time; errors `*SqlError` by kind) with
  a cancelable pool (`lock`, `max_open` permits, plus one writer permit for single
  writer backends). `std/rt/sqlmem.go` is the in memory driver (small SQL parser
  and evaluator; tables are immutable and swapped on write, a transaction is a
  copy of the table map). Real drivers: implement `rt.SqlOpener`, call
  `rt.RegisterSqlDriver` (to be bound through `use go`; none yet). Goldens:
  `sql_basic`, `sql_tx`, `test_sql`, `sql_decode`. Not recorded by record and replay.
  `derive SqlRow` (2026-10-08, decision 0061): `internal/gen/sqlrow.go` emits
  `(*T).XoSqlRowDec`, one `rt.SqlRowField` call per field
  (`std/rt/sqlrow.go`, a type switch over the column types);
  `Sql_Row_decode[T]`, `Sql_Db_query_as[T]`, `Sql_Db_query_one_as[T]` and
  the `Sql_Tx_` forms assert `sqlRowDecoder` on `*T`. `RETURNING` in
  `sqlmem.go`: writes return a `memWrite` (count, last id, projected rows);
  `memBackend.Query` and `memTx.Query` run on a copy of the table map and
  publish it on success. Checker golden: `derive_sqlrow`.
- std/http request values and scoped middleware (2026-10-08, decision
  0055): `Http_Key[T]`, `Http_Request_with_value`/`_value` (a persistent
  list in the hidden `XValues` field of `Http_Request`, listed in
  `rtExtraFields` in internal/gen/json.go), `Http_Router_wrap[E]` (generic
  now; was the Go method `M_wrap`), `M_mount` (flattens the sub router's
  routes, each behind `chain(sub.mws, h)`), `Listener.M_addr` (the
  `addr` field, also in the recorded `net.listen` result). Golden:
  `http_scoped`.
- std/crypto (2026-10-08, decision 0056, std.md 11): `std/crypto.xo`,
  `std/rt/crypto.go` on `crypto/sha256`, `crypto/hmac`, `crypto/subtle`,
  `crypto/pbkdf2` (Go 1.24+); random functions call the `Rand`
  capability's `M_bytes`. Goldens `crypto_basic` (known vectors),
  `test_crypto` (seeded rand); unit tests `TestCrypto*` in std/rt.
  Argon2id (2026-10-08, decision 0059): `Crypto_hash_password_argon2id`
  and the `$argon2id$` branch of `Crypto_verify_password` call
  `argon2Key` in the vendored `xcrypto_argon2.go`; tests in
  `std/rt/xcrypto_test.go` (`TestArgon2RFC9106`, `TestBlake2bRFC7693`,
  `TestArgon2idMatchesXCrypto`, `TestCryptoArgon2idHash`) and
  `BenchmarkArgon2idDefault` (about 19 ms per hash on Apple M, 2026-10-08).
- std/time (2026-10-09, decision 0094, std.md 13): `std/time.xo`;
  `std/rt/timezone.go` (Zone handle, zone registry, a stored zip reader
  so rt does not link archive/zip, `local`), `timecivil.go` (dates by
  day number, `instant` with gap and overlap detection over the offsets
  two days either side), `timefmt.go` (layout parser shared in spirit
  with `internal/check/timelayout.go`, kept equal by
  `TestTimeLayoutAgrees`; format, strict parse). Per use zone data:
  `internal/gen/timezone.go` records `time.zone` calls (constant names
  through `check.ConstString`), `internal/build/tzdata.go` writes the
  data and the notes (`build.Result.Notes`, printed by `xo build`).
  Goldens `time_zones`, `time_civil`, `time_format`, `time_layout_fault`,
  `time_dynamic_zone`; `TestZoneDataPerUse` (what each kind of use
  embeds, `--tzdata=system`); checker golden `time_layout`; habit cases
  `cmd/xo/testdata/habits/time-layout-*`. Sizes on darwin/arm64:
  UTC only 7,968,146 bytes, one constant zone 7,968,994, a run time
  name 8,382,498 (std.md 13.5).
- Test log capture (2026-10-08, decision 0057): `runOneTest` gives each
  test world `newTestLogSink()` (std/rt/log.go); `logRecord` appends a
  `Log_Record` and, with no configured output, the text line to the
  capture instead of stderr; `RunTests` prints the lines under `--- FAIL`
  (and `--- PASS` with `-v`), `--json` adds `log`. `testing.logs()` is
  `rt.Testing_logs`. Golden: `test_logs`.
- Fake network listeners (2026-10-08): `memListener.served` is fired when
  `serve` starts; `Testing_FakeNet.dispatch` waits on it (wait kind
  `waitAccept`) so a request sent before `serve` queues like a backlog
  (gotcha `fake-listener-before-serve.md`).

## SSA IR

Last verified: 2026-10-08. Decisions 0045, 0046, 0048. `ssa.Build(res)`
lowers the functions reachable from `main` to SSA with block parameters
(generic functions monomorphized per type arguments, closures as
`Indirect` functions with a counted environment, recursive fields as
`Box[T]`, failing functions returning their Result); `Module.Verify()` checks structure,
dominance, and types; `Module.String()` prints it. `ssa.BuildOpts` takes
a `LineRoot` for fault positions. `Module.InsertRC()` (decision 0046,
`rc.go`) adds reference counting: borrow inference (`Func.Borrowed`,
printed `borrow v0`), retain and release per heap leaf, cancellation,
and `reuse` before list updates not proven unique; afterwards
`Module.RC` is set and `Verify` also checks RC balance on every path
(`verify_rc.go`). Tests: `go test ./internal/ssa`
(testdata/golden/<name>/main.xo with want.ssa and want.rc.ssa;
testdata/bench/<name>.ssa and .rc.ssa for bench/perf/llvm/xo; verifier
rejections; `TestVerifyRCMutations` deletes each inserted retain and
release and requires a rejection). Rewrite goldens with
`go test ./internal/ssa -update`. internal/llvm lowers from it
(below). `xo ssadump` prints the module after each stage of a build
(build, unwind, landings, cleanup, rc; `ssa.BuildStages`), the form
compiler/ssa is compared in (decision 0150; last verified: 2026-10-10).

## LLVM backend (P3 prototype)

Last verified: 2026-10-08. Experiment for decision 0040 ("own backend");
results and design: `bench/results/llvm-prototype-2026-10-08.md` (the
first, AST based prototype, since deleted) and
`bench/results/llvm-rc-2026-10-08.md` (SSA with reference counting).

- `xo build --backend=llvm` and `xo run --backend=llvm`; host only,
  programs only (not `xo test`), no `--holes` (arenas are ignored),
  `--targets` other than the host. `build.Options.Backend`.
- Hosts (2026-10-12): darwin/arm64, linux/arm64, linux/amd64,
  darwin/amd64 (Intel Mac), and windows/amd64 (decision 0197: the
  runtime's Win32 layer is Xo (std/native/llrt windows.xo, sock.xo,
  poll.xo; std/llrt/xo_win.c was deleted 2026-10-15); clang's default
  x86_64-pc-windows-msvc target with lld; XO1504 for the std APIs not
  ported yet, none since stage 2 (processes, 2026-10-12); see "Windows
  runs"). The one
  cross build: a Windows host builds the other Windows architecture
  (`--targets=windows/arm64`, `build.LLVMCrossTriple`).
- One path: `llvm.Generate` builds SSA (`ssa.BuildOpts` with the line
  root), verifies, runs `InsertRC`, verifies again (RC balance per
  path), and lowers with `GenerateSSA` (`internal/llvm/fromssa.go`;
  decisions 0046 and 0048): SSA values are LLVM values, block parameters
  phis, `retain`/`release` call generated `xo.dup.<T>`/`xo.drop.<T>`
  helpers, `reuse` is a count check with a cold copy (`xo_rl_copy`), list
  updates write in place, `==` on aggregates calls generated `xo.eq.<T>`
  helpers. A construct outside the SSA builder's subset is a build error
  naming it and its position ("the SSA builder does not support ...").
  The AST path (bump allocator, never freed) was deleted on 2026-10-08.
- Value ranges and inlining (decision 0192; last verified: 2026-10-12):
  `internal/llvm/ranges.go` computes per function the intervals of Int
  values (decision 0184's analysis) narrowed by dominating branch
  comparisons, `&&` joins, and indexes taken; a checked add, subtract,
  or multiply they prove is emitted `nsw` without its check, and each
  binary search midpoint gets an `llvm.assume` of its interval (only
  midpoints: loop parameter assumes slowed the sieve 1.6 times,
  gotchas/llvm-loop-phi-ranges.md). `internal/llvm/inline.go` is
  arm64's cost model; calls of the functions it finds inlinable carry
  `#6` (`attributes #6 = { alwaysinline }`). Twin: compiler/llvm
  ranges.xo.
- Split builds (decision 0168; last verified: 2026-10-12):
  `XO_LLVM_SPLIT=n` (or `build.Options.LLVMSplit`) cuts the IR into n
  modules by size (`llvm.Split`, internal/llvm/modules.go), writes
  `prog.<i>.ll` in `<build>/llvm-split<n>-<lto>/`, compiles them with at
  most `XO_LLVM_JOBS` clang processes per xo process (default min(cores,
  6); a clang on a twelfth of compiler/ssa peaks near 2 GB, on a sixth
  near 4 GB), and links. `XO_LLVM_LTO=thin` (default) or `none` (`off`
  is the same). Any other value of XO_LLVM_LTO, or a non-number in
  XO_LLVM_SPLIT or XO_LLVM_JOBS, is an error (since 2026-10-21: `off`
  silently fell back to thin LTO). Unset,
  `0`, or `1`: one module, whose LTO is `XO_LLVM_LTO` (`full`, `thin`,
  `none`), else none at `-O0` and full otherwise (last verified:
  2026-10-15, `bench/results/llvm-single-lto-2026-10-15.md`). Test processes of
  internal/selfhost call `build.EnableTestLLVMSplit` (12 modules, 4
  jobs, no LTO unless the environment says otherwise; `XO_LLVM_SPLIT=0`
  for the release configuration). Timings:
  `bench/results/llvm-build-time-2026-10-12.md`.
- Parse prefetch (decision 0220 addendum stage 2; last verified:
  2026-10-21): build, run and test builds read the build target's parse
  list (`<backend cache>/packs/<xx>/<slot>/parse/list`, every file the
  last check parsed) and parse those files on tasks before the check.
  The list is written by the target's first check too (the test run
  cache serves the unchanged builds after it, which check nothing).
  `--phase-times` prints `cache parse H/N reused`; `XO_PARSE_STATS=1`
  also lists each file the check parsed itself (`parse miss PATH`).
  Measured: an edit build of `xo test --std-module=native/path` checked in
  0.33 to 0.47 s with 185 of 185 parses reused, 0.8 to 1.2 s with none.
- Test build split (2026-10-21, xo-fa; last verified: 2026-10-21): an
  LLVM `xo test` build with the back-end cache on (and no
  `XO_LLVM_SPLIT`, no --race, the host target) cuts the IR into
  modules placed by source file (the DIFile of each function's
  subprogram; by name without debug info), not by size:
  `XO_LLVM_TEST_SPLIT=files` (the default) gives each file its own
  module, at most 32 (`bench/results/llvm-test-split-2026-10-21.md`:
  the fastest edit build); a number N hashes the files into N modules;
  `off` keeps one module. Each module carries only the debug
  metadata its own functions reach, numbered by the module (the IR
  numbers metadata for the whole program, so one variable added would
  renumber every module). The modules are written beside the build
  directory (`llvm-split32files-none-modules/` by default), since the binary cache's key
  hashes every file in it. They compile at -O0 with no LTO,
  and each module's object is kept in the back-end cache (`llobj/`,
  keyed by the module's IR, clang and its arguments), so an edit
  recompiles only the modules of the files it changed. The split runs
  only after a miss of the binary cache, which keys the whole IR.
  `--phase-times` prints `cache llvm modules R/N reused`. Test:
  internal/llvm TestTestSplitReuse.
- Target (decision 0119, 2026-10-10): `build.ClangTargetAttrs` reads
  clang's default `-target-cpu` and `-target-feature`s (`clang -### -c
  -x c /dev/null`, once per compiler) and the IR's function attribute
  groups `#1` and `#4` carry them (`llvm.Options.TargetAttrs`):
  darwin/arm64 `apple-m1` (LSE atomics inline), Linux arm64 `generic`
  with `+outline-atomics` (Debian clang 19), Linux amd64 `x86-64`.
  Check with `otool -tv bin | grep -c ldaxr` (0 on darwin/arm64).
- Atomics (decision 0200, 2026-10-13): the cell intrinsics `rt.slot_*`
  lower to `load atomic`, `store atomic`, `atomicrmw xchg|add`, and a
  strong `cmpxchg` (failure ordering the strongest allowed: acquire for
  acquire and acq_rel, monotonic for relaxed and release) at object +
  24 + 8i, `align 8`; relaxed is `monotonic`, a load's release or
  acq_rel and a store's acquire or acq_rel become `seq_cst`; `rt.fence`
  is `fence` (relaxed none). The target attributes above decide the
  machine code (LSE on darwin/arm64, outline atomics on Linux arm64,
  `lock` instructions and `xchg` on x86-64). `rt.spin_hint` is nothing:
  the IR is target independent (compiler/llvm call.xo `atomic`,
  internal/llvm fromssa.go `atomic`, identical IR) (last verified:
  2026-10-13).
- Layout: Int i64, Bool i1, Float double, `()` and capabilities `{}`, Str
  `{ptr obj, i64 len}` with obj `{i64 rc, i64 cap, bytes}` (literals are
  constants with the immortal count INT64_MIN, never written), List a
  pointer to `{i64 rc, i64 len, i64
  cap}` plus elements, structs and tuples LLVM structs, enums `{i64 tag,
  [N x i64]}` (payloads through a stack slot), function values `{ptr
  code, ptr env}` with env null or `{i64 rc, ptr drop, ptr share,
  captures...}`,
  `Box[T]` a pointer to `{i64 rc, T}`. A failing function returns its
  Result enum. Checked Int operations use `llvm.s*.with.overflow` and cold
  fault calls; loop counters are unchecked (the range analysis of
  internal/gen is not used by this backend).
- Subset: what `ssa.Build` lowers (its doc comment lists it): Int, Bool,
  Float, Str, printing, structs, enums, Option, Result, tuples, `match`,
  `if let`, `is`, the usual patterns (not list patterns), loops, `for`
  over ranges, lists, and `xs.indexed()`, functions, methods, `var`
  parameters, recursion, generics, closures, function values, recursive
  types, failing functions with `?` and `from` conversion, `??`,
  `or_fault`, `fault`, `const`, `==` on every type, the List methods
  push, pop, set, get, first, last, len, is_empty, and the Int, Float,
  and Str methods the bench programs use; since 2026-10-08 also core 7
  (decision 0068): `scope`, `spawn`, `spawn_background`, `await`,
  `s.cancel()`, groups from `s.group()`, `task.is_canceled()`, `Chan`
  (send, recv, try_send, try_recv, close, len, `for x in ch`), `select`
  (with `after` since decision 0069), `Mutex` (`with`, `with_read`), and
  `Atomic`. Not yet: `map_concurrent(_until)`, `isolate`, `Semaphore`,
  `Once`, `os.tasks`. `defer` since 2026-10-08 (decision 0070: inlined
  at every exit, shielded; a fault does not run it). Decision 0069 adds
  `os.clock` (`now`, `sleep`, `since`), `task.with_timeout`,
  `task.stop_requested`, `task.stopping`, `os.proc.set_grace`,
  `os.env.get`, Duration (literals, arithmetic, `from_millis`,
  `from_nanos`, `from_secs_int`, `millis`, `nanos`, `seconds`, `abs`) and
  Time (`from_unix`, `from_unix_millis`, `unix`, `unix_millis`) as Int
  nanoseconds, and Bytes as a Str (`Str.bytes`, `to_str`, `len`,
  `is_empty`). Decision 0081 (2026-10-08) adds the sized integers
  (Int8 to UInt64) and Rune as KInt types in the Int word (checked
  arithmetic through `rt.int_op`, UInt64 ordered by flipping the sign
  bit, widening conversions as `iconv`), rune literals as integers,
  `b[i]` on Bytes (`bytes.index`, inline in both backends), slicing of
  Str, Bytes, and List, `b"..."` literals, `StrBuilder` and
  `BytesBuilder` (the text so far, appended in place), the Rune, Str,
  and Bytes text methods of decision 0079 (std/native/prelude.xo),
  `os.fs.read`, `os.env.args`, `os.proc.exit`, and constants of
  collection types built once per run (`rt.const_once`). Enough for
  `compiler/lexer`.
  Map and Set (decision 0072) are structs of lists with generated
  operations: literals, get, contains, get_or_fault, put, remove,
  update, keys, values, entries, add, to_list, `m[k] = v`, and `for`
  loops; `==` on them since decision 0074; not Float keys, filter, or
  Set algebra.
  `os.net.listen`/`dial` (dial gives up after 30 s like the Go
  backend), `Listener.addr`/`accept`/`close`, and
  `Conn.read`/`write`/`close` (decision 0071) run on the event loop;
  SIGINT/SIGTERM give the two phase shutdown of core 7.5.
  Since decision 0074 (2026-10-08) std is implemented for the native
  backends in Xo: `std/native/<name>.xo` implements `std/<name>`
  (`prelude.xo` the prelude: Str methods, Int and Float parse, the List
  closure methods, sort, Option and Result methods), loaded by the
  checker only for native builds (`check.Options.Native`,
  `build.CheckNative`) and lowered in place of the std declarations
  (`internal/ssa/build_native.go`); `std/native/rt.xo`, `rtjson.xo`,
  and `rthttp.xo` declare the C intrinsics they call (only native
  modules can `use native/...`). So std/http (server, client, routing,
  middleware, request values), std/json (codecs per type,
  build_json.go), and std/log run natively; also interpolation of any
  type (Display, Debug of `derive Display`, Duration, Time,
  build_show.go), `main ! Error` (`error: <text>`, exit 1), `requires`
  clauses, and values of Error, Display, Debug as their text. Values of
  other interface types (decision 0100, 2026-10-09,
  `internal/ssa/build_iface.go`) are `xo.iface[I]` structs of closures
  sharing one environment (`OpRebind`), with hidden `xo.eq`, `xo.tid`,
  `xo.show` fields; goldens `iface_*`. Decision 0101 adds Float32 (a
  double rounded through `rt.f32_round`), `Secret[T]`, `Buf[T]`,
  `xo.cmp[T]` order helpers, and more std value methods.
- TestGenGoldensNative (`internal/llvm/gen_native_test.go`, 2026-10-09)
  builds every internal/gen golden named in
  `internal/llvm/testdata/gen_native.txt` with Go, LLVM, and (on arm64
  hosts) arm64, and requires the same output and no live allocation; add
  a golden there when a gap closes.
- std/native/path.xo (std/path) and std/native/task.xo (Semaphore over a
  channel of permits, Once over a Mutex holding the result,
  map_concurrent over a scope) are native implementations in Xo
  (2026-10-09); goldens `std_path`, `task_native`.
- Fs natively (decision 0102, 2026-10-09): `xo.Fs {root, mode}`, methods
  `fs_*` in std/native/prelude.xo over `std/llrt/xo_fs.c`; standard input
  in `std/llrt/xo_stdin.c` (descriptor 0 on the poller); TestStdinNative
  pipes input into Go and native builds and compares.
- Error frames natively (decision 0104, 2026-10-09): Err carries a hidden
  `xo.tr` trace (internal/ssa build_frames.go); TestErrorTracesNative
  compares whole standard error, frame lines included. The
  differences from the Go backend are listed in decision 0074 (a fault
  in a handler exits; no `http.stream`). HTTPS (std.md 4.6) is
  Xo (decision 0202; Mbed TLS from 2026-10-09 to 2026-10-15, decisions
  0105, 0106; see "TLS"), with HTTP/2 over it since 2026-10-10 (decision
  0121; std/native/llrt/h2srv.xo since 2026-10-14, xo_h2.c before):
  serve_conn asks `hc_h2` after the handshake, then runs each stream's
  request as a task of a scope of the connection's task (`h2srv`
  `H2Conn.next`, `end`).
- std/time natively (2026-10-09, decision 0094 "Native implementation"):
  std/native/time.xo (civil time, layouts, format, parse) over
  std/llrt/xo_time.c (Go's TZif reader and zone lookup, intrinsics in
  rttime.xo). xo_std.c includes xo_time.c only with `-DXO_TIME`, which
  `timeFlags` (internal/build/tzdata.go) passes only for programs that
  load std/time, together with the generated `xo_tzdata.c` in the build
  directory, compiled as a source of the link beside the runtime
  (`tzSource`; xo_time.c declares `xo_tz_mode`, `xo_tz_blobs`,
  `xo_tz_zip`, `xo_tz_zip_len` extern, so the runtime object does not
  depend on the program, decision 0157; per use zone data: mode 0 TZif
  per constant name, 1 the whole zoneinfo.zip, 2 `--tzdata=system`).
  Until 2026-10-11 it was `xo_tzdata.h`, included by xo_time.c. Tests:
  TestTimeDiff (`XO_TIMEDIFF_OUT=<prefix>` keeps the program and its
  output), TestZoneDataPerUseNative, and the time goldens in
  gen_native.txt. Last verified: 2026-10-11.
- Counts (decision 0058): positive is thread local (plain updates),
  negative is shared (atomic: `atomicrmw`, counts read with `load atomic
  unordered`), INT64_MIN immortal. `OpShare` calls generated
  `xo.share.<T>` helpers that mark a value's heap graph shared. The
  task lowering emits `OpShare` where values cross threads (decision
  0068); `build.Options.ShareProbes` / `llvm.Options.ShareProbes` (tests)
  marks every heap value shared as it is defined (builds go to
  `<build dir>/llvm-share`).
- Runtime `std/llrt/xo_rt.c`: size class allocator (16 byte classes to
  1 KiB, powers of two to 256 KiB, malloc above; free lists; 4 MiB
  chunks) with allocation counters (`XO_LEAKCHECK=1` prints live and
  total at exit); per thread heaps after `xo_rt_threads_begin()` (one
  pointer load while single threaded), `xo_rc_inc`/`xo_rc_dec`/
  `xo_rc_claim`/`xo_rc_mark` (the count semantics generated code inlines),
  a locked output buffer when threaded; the C tests in
  `std/llrt/testdata/` include the runtime and get main's setup from
  testdata/xcalls.h's constructor (main itself is Xo's,
  std/native/llrt/cabi_c.xo, decision 0234; `-DXO_RT_NO_MAIN` and
  `-DXO_RT_TEST_THREADS` went with C main; Last verified: 2026-10-15); reference counted strings (`xo_rs_*`, append in place when
  unique) and lists (`xo_rl_*`); `xo_rc_defer`, a worklist that frees
  closure environments and boxes whose count reached zero without
  recursing on long chains; buffered stdout; fault messages identical to
  std/rt's and exit status 70; Go style float formatting (shortest round
  trip `'g'`, `.0` appended); `main` runs the program on a thread with a
  1 GiB stack. The C interface passes only scalars and pointers. Tasks
  (decision 0068, section "Tasks" of xo_rt.c): a pool of worker threads
  (one per CPU, `XO_THREADS=n` overrides), Go style run queues and
  spinning workers, root threads that run queued work while they wait
  (decision 0077; `XO_RTSTATS=1` counts scheduler events,
  `XO_SPIN_ROUNDS=n` tunes spinning, default 0 since decision 0077's addendum, 16 before;
  a Mutex release yields to the writer it woke when the releaser took
  at most 8 locks since its last I/O wait (decision 0118's 2026-10-22
  addendum; never with one CPU): `XO_MUTEX_YIELD=0` never yields (a
  lock bound program), `XO_MUTEX_YIELD=1` always does;
  `XO_NETPOLL=0` turns off decision 0324's workers polling the network
  themselves on darwin and Linux, back to the poller thread alone, last
  verified: 2026-10-21), futex based locks and
  parks (`__ulock_wait` on Darwin), awaits and scope joins that run tasks nobody started, a cancellation
  tree, channels, select, Mutex (readers and writer, writers preferred),
  Atomic, and deadlock detection (XO-F001). Since decision 0069 every task
  runs on a fiber (64 MiB reserved stack with a guard page, 8 MiB under
  ThreadSanitizer on Linux since 2026-10-12 (gotcha
  linux-tsan-fiber-stacks), `XO_TASK_STACK_MB=n` overrides, 64 cached; register switch in top level
  assembly for arm64 and x86_64, ThreadSanitizer fiber API at each
  switch): every wait parks the fiber and frees its worker. A poller
  thread started in `main` (kqueue on Darwin, epoll on Linux, a self
  pipe) runs the timer heap, socket readiness, and SIGINT/SIGTERM (two
  phase shutdown, core 7.5); SIGPIPE is ignored; buffered stdout is
  flushed before any context waits. Thread locals only through
  `noinline` accessors, errno only through `noinline` syscall wrappers
  (`gotchas/fiber-thread-local-address.md`). Generated code calls it as
  `rt.<name>` SSA calls (C function `xo_<name>`), passing values as
  lists of one; handles (Task, Scope, Chan, Mutex, Atomic, Listener,
  Conn) are counted objects created shared. Runtime calls also take and
  return Floats, return a Str (the object, its length through a last
  pointer argument), and `rt.rs_*` calls take their Str operands as
  string objects and consume the first (appending in place). xo_rt.c
  ends by including `xo_std.c` (the native std intrinsics: strings,
  numbers, durations, times, base64), `xo_bytes.c` (decision 0081: checked arithmetic and
  conversions of sized integers, Rune text and quoting, Bytes slicing and
  decoding, the builders' appends, `fs.read`, `env.args`, and the slots
  of constants of collection types), and `xo_http.c` (`xo_json.c`
  until decision 0198);
  builds write every `std/llrt/*.c` next to xo_rt.c and compile only
  xo_rt.c.
- Fault locations are `file:line:col` strings in the binary (no DWARF).
  Fault output has no Go frame lines and no crash bundle; the golden test
  compares fault messages after dropping those lines.
- Tests: `internal/llvm/llvm_test.go`. TestGolden builds each program in
  `testdata/golden/` with the Go backend and the LLVM backend and
  requires equal exit status, stdout, and fault messages, also matching
  `want.txt`; programs that exit normally must free every allocation
  (`XO_LEAKCHECK=1`). The `rc_*` programs are copies of SSA goldens
  (`internal/ssa/testdata/golden`) that stress ownership. TestLeakCheck
  does the same for the bench programs. TestGoldenShared runs every
  golden with share probes. `threads_test.go`: TestRuntimeThreads
  (`std/llrt/testdata/rc_threads.c` under ThreadSanitizer, then `-O2`
  with more iterations) and TestGoldenThreads (goldens that exit 0, IR
  with probes, four threads at once, ThreadSanitizer except the deep
  recursion ones, `gotchas/tsan-deep-recursion.md`); TestRuntimeTasks
  (`std/llrt/testdata/tasks.c`, the task runtime driven from C, under
  ThreadSanitizer with one and three workers, then `-O2 -DN=20000`),
  TestRuntimeEventLoop (`std/llrt/testdata/evloop.c`: 2000 sleepers on
  few workers, an 8 MiB echo through a socket, close and cancellation
  waking parked socket operations, a refused dial, select `after`; same
  runs), TestGoldenTasks (the goldens that use tasks, built with
  ThreadSanitizer without probes and run with `XO_THREADS` 1, 2, and
  the default against want.txt), TestDeadlock (internal/gen's
  `deadlock_real`, read only, natively: XO-F001); they skip when clang
  has no ThreadSanitizer. The `conc_*` goldens are concurrency programs
  adapted from internal/gen's goldens without clock, I/O, Map, or
  Display, so the Go, LLVM, and arm64 backends print the same. TestUnsupported checks the error
  for constructs outside the subset (`testdata/unsupported/`).
  `cmd/xo/llvm_test.go` drives the CLI.

## arm64 backend (native)

Last verified: 2026-10-08. Decisions 0049, 0051, and 0054; results
`bench/results/arm64-native-2026-10-08.md` and
`bench/results/arm64-perf-2026-10-08.md`.

- `xo build --backend=arm64` and `xo run --backend=arm64`; programs only,
  no `--race` (refused: use `--backend=llvm`), `--holes` (arenas are ignored); targets darwin/arm64 and
  linux/arm64. `build.Arm64Asm(res, root, goos)` returns the assembly
  (it retries without inlining if the inlined module does not verify,
  `arm64.InlineError`). Every op the SSA builder and InsertRC emit is
  lowered (decision 0050: closures, boxes, structural equality); an op
  outside that is an error, no fallback.
- Pipeline per function (`internal/arm64`): `isel.go` (SSA to MIR over
  virtual registers with assembly templates; values flattened into words,
  `layout.go`), `opt.go` (memos inherited from dominators, list length
  registers, dead code, constant tag and inlined predicate threading,
  branch bounds, constant hoisting out of loops, branchless select of a
  compare picked by another compare, flag reuse), `jumptab.go` (`match`
  compare chains laid out as runs of tests, jump tables from 8 values,
  decision 0051; `XO_ARM64_JT_MIN=n` overrides the threshold for
  measurement, 0 disables tables), `rc.go` also cancels a payload retain
  against the release of its enum (`ssa.CancelledRelease`, which the
  LLVM backend uses too), `regalloc.go` (liveness, live ranges with holes, move
  coalescing, greedy first fit by spill weight, spill slots),
  `emit.go` (frames, parallel moves, fault stubs with operand recovery),
  `rc.go` (dup and drop helpers in assembly), `lsr.go` (decision 0054:
  loops found on the SSA, pointer induction variables for list element
  accesses, induction variable bounds, `xs[i + c]` from xs[i]'s pointer,
  rotation of a back edge at a bounds check (`_c` labels), integer `ldp`
  pairs as a text peephole, latch layout around inner loops),
  `ranges.go` (intervals of Int values: unchecked multiplies, `and`/`asr`
  for `%`/`/` by 2^k of non negative values), `closures.go` (function
  values, environments, `blr x0`), `boxes.go` (Box cells), `eq.go`
  (`==` helpers), `inline.go` (small leaf functions, SSA to SSA, before
  lowering).
- ABI: AAPCS64 (x0 to x7, d0 to d7, 8 byte stack slots for more Xo to Xo
  arguments; result words past 8 of a class are stored by the callee in
  the caller's stack argument area). Scratch x15 to x17 and
  d29 to d31; x18 never used. Frame record kept in non leaf functions,
  callee saved pairs stored high register first
  (`gotchas/arm64-stp-pair-order.md`). Darwin: `_` prefix, Mach-O
  sections, `@PAGE`/`@PAGEOFF`; Linux: ELF, `:lo12:`, `.note.GNU-stack`.
- Layout differs from the LLVM backend's: an enum payload has one 8 byte
  word per field word (a Bool field takes a word); list elements are
  8 bytes per word except `List[Bool]` (1 byte). The runtime sees only
  element sizes and callbacks.
- Counts (decision 0058): retains inline `tbnz #63` to a cold atomic
  block, releases `cmp #1; b.le` to a cold path that frees at one,
  ignores the immortal count (bit 62 clear) and otherwise drops a shared
  reference atomically: LSE `stadd`/`ldaddal` on Darwin, `ldxr`/`stxr`
  and `ldaxr`/`stlxr` loops on Linux (scratch x17 and w15 in inline
  templates). Environments are `{rc, drop, share, captures}` (24 byte
  header). Literals are in `__TEXT,__const` / `.rodata`. `iCondBr` with
  `tbz`/`tbnz` tests `inst.bit`.
- Atomics (decision 0200, 2026-10-13): `rt.slot_load`, `slot_store`,
  `slot_swap`, `slot_cas`, `slot_add`, `slot_addr` (a cell: element i of
  a `List[Int]` or an Atomic's value, at object + 24 + 8i), `rt.fence`,
  `rt.spin_hint` lower inline (compiler/arm64 calls.xo `atomic`,
  internal/arm64 isel.go `atomic`, identical output): relaxed loads and
  stores `ldr`/`str`, the others `ldar`/`stlr`; on Darwin `swp`, `cas`,
  `ldadd` with the order's `a`/`l`/`al` suffix (a cas is `mov; casal;
  cmp; cset`), on Linux calls of libgcc's outline atomics
  `__aarch64_{swp,cas,ldadd}8_{relax,acq,rel,acq_rel}` (what the C
  runtime's `-moutline-atomics` objects call; compiler/macho elflink.xo
  writes them when linking without cc); fences `dmb ishld` (acquire) or
  `dmb ish`; the spin hint `yield`. The instructions are `keep` (never
  deleted with a dead result, never folded into a branch) and early
  (gotchas/arm64-plain-defs-assumed-pure.md). Atomic's get, set, swap,
  and compare_and_swap are one of these each (seq_cst); add and max are
  std/native/llrt/atomic.xo (decision 0198 deleted `XO_LLRT_C=atomic`
  and the C xo_atomic_* operations, 2026-10-13). Litmus tests:
  `bench/litmus/run.sh <xo1>` (last verified: 2026-10-13).
- Unwinding (2026-10-09, decision 0085): every function has `.cfi_*`
  call frame information (leaf functions' fault stubs push a frame record
  first); a function with landing pads (`inst.pad`) gets
  `.cfi_personality 155, _xo_personality`, `.cfi_lsda`, and a call site
  table in `__gcc_except_tab` / `.gcc_except_table` labelled
  `GCC_except_table_xoN` on Mach-O (a symbol: see
  gotchas/arm64-lsda-local-label.md); call sites are `fN_csKa`..`fN_csKb`
  around each `bl`. A check only a fault takes is an `iJump` with the pad
  as a second target (liveness and reachability only); values live into a
  pad entered from a fault stub avoid caller saved registers.
- Tests: `go test ./internal/arm64` (golden assembly; on an arm64 host
  it builds and runs every golden of internal/llvm against its want.txt
  (62 on 2026-10-08, with the native_* and http_* goldens of decision 0074), the bench programs, and `testdata/run/*` against the Go
  backend, all with the leak check; TestGoldenRunShared runs the goldens
  with share probes and assembles their Linux output as ELF).
  crossblock_test.go (also -short) holds the MIR passes that rely on
  instructions of other blocks (threadConst, threadCset,
  selectCompares, reuseFlags, placeLatches) to their assumptions: each
  shape fires, and each broken assumption (a second predecessor, a
  redefined register, other targets, a fault exit in a latch) leaves the
  code alone; testdata/run/cross_block runs those shapes against the Go
  backend (reviewed 2026-10-11, no bug found besides the flags one of
  gotchas/arm64-reused-flags-of-dropped-compare.md). To try a code change by hand, build
  with `XO_KEEP_ASM=1`, edit `<build dir>/arm64/prog-darwin.s` and link it with
  `cc -O2 prog-darwin.s xo_rt.c` (add `-isysroot <Xcode SDK>` when the
  default SDK does not link; on 2026-10-08 the default MacOSX27.0 SDK
  failed with "tapi error: unknown architecture arm64e.x1" and
  `-isysroot /Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk`
  linked). On linux/arm64 the same tests link ELF and run every golden
  (68 on 2026-10-09, `scripts/linux-test.sh arm64`). `XO_BUILD_DIR=<dir>` keeps the build directory (and the `.s`)
  where you want it. `regalloc` fails with "reads virtual register n
  before it is written" when MIR uses a register no path defines
  (`gotchas/arm64-mir-param-without-moves.md`).

## Hybrid builds (Go backend with Go assembler)

Last verified: 2026-10-10. Decisions 0084, 0131; results
`bench/results/go-asm-hybrid-2026-10-09.md`,
`bench/results/hybrid-preemption-2026-10-10.md`.

- Deprecated since 2026-10-22 (owner): `--hybrid` is a plain Go backend
  build with a warning (no Go assembler mode in compiler/arm64), to be
  removed. What it was: `xo build --hybrid` (Go backend, Build mode): `internal/build/hybrid.go`
  runs the native check, SSA with reference counting, and
  `arm64.GenerateGo` (Go mode, `internal/arm64/goasm.go`); a program
  outside the SSA subset builds as plain Go (`HybridInfo.Err`). The CLI
  prints `hybrid: f, g in Go assembler` on stderr. `build.HybridPlan`
  reports the plan without building; `Options.HybridAll` takes every
  eligible function (tests), `Options.HybridPure` builds the same
  directory with tag `xo_pure`, `Options.HybridNoPoll` leaves the
  preemption polls out (tests, measurements; no CLI flag).
- Eligible: free functions of the main module (no methods, generic
  instances, `ensures` clauses, structs with invariants); parameters and
  results Int, Float, Bool, structs of Int and Float fields, Lists of
  those (and of Bool); `var` only on Lists; no allocation, no reference
  count, no Str; calls only to eligible functions and the inline list
  and scalar methods (`goMethods`). Selected (wrapped): eligible with a
  loop. Rejection reasons are in `GoProgram.Rejected`.
- Generated files in the main module's package: `xo.go` (the rest),
  `xo_pure.go` (`//go:build !arm64 || xo_pure`, the Go versions),
  `xo_hybrid.go` (`//go:build arm64 && !xo_pure`: ABI0 declarations
  `func xoasm_f(p0 []Rec, p1 int64) (r0 int64)`, `//go:noescape` when the
  function calls no Xo function and has no loop, wrappers that `Unshared()` written `var`
  Lists and pass `.Slice()`, `xoHybridFault` and the position table,
  `xoHybridYield` (`rt.HybridYield`, `runtime.Gosched`)),
  `xo_hybrid_arm64.s` (`//go:build !xo_pure`). Imports a file does not
  name are left out of it (`usesIdent`).
- Go mode differences from the native target: List word is the slice
  base (`hdr()` 0), lengths are parameter registers (`goLens`); arguments
  in memory (`name+off(FP)`, outgoing at 8(RSP)); registers x0 to x14,
  x19 to x25, d0 to d28 all caller saved (x18, x26 to x30 untouched,
  x15 to x17 and d29 to d31 the emitter's scratch); values live across a
  call keep their register and are stored and reloaded around it
  (`inst.saves`); frame `$outArgs+8*slots`, NOSPLIT unless the function
  calls an Xo function, `NO_LOCAL_POINTERS` when it calls anything; no
  jump tables; faults `CALL ·xoHybridFault(SB)` with (kind, a, b, c, pos),
  kinds in `goFaultKind`, raised by `rt.HybridFault`.
- Preemption polls (decision 0131, `insertPolls` after `placeLatches`):
  before the terminator of every block with a back edge, `ldr x16, [x28,
  #16]; tbnz x16, #63, Lpoll` (x28 is translated to `g`); the stub after
  the body stores the registers live across it (`inst.saves`), calls
  `·xoHybridYield(SB)`, reloads, and jumps back. A poll is an `iCall`
  with `poll` set: it does not make the function lose NOSPLIT, but it
  does lose `//go:noescape`, and its frame is at least 8 bytes.
- `gosyntax.go` translates the GNU text line by line; `TestGoSyntaxEncoding`
  (darwin/arm64 with cc) assembles every emitted form both ways and
  compares the machine code (Go writes `mov x, #0` as `orr`).
- Tests: `go test ./internal/gen -run 'TestGoldenHybrid|TestHybridRandom|TestHybridPreemption'`
  (eligible goldens against want.txt, 12 random kernel programs (3 with
  `-short`) against their `xo_pure` build, all four fault kinds; `go vet
  -asmdecl` on each build directory; TestHybridPreemption runs
  testdata/hybrid_preempt with GODEBUG=gctrace=1: stop the world under 10
  ms and a goroutine's gap under 100 ms during a 0.3 s (-short) or 2 s
  hybrid loop, the race detector and a build without polls as control
  in the full suite; not parallel, three attempts per bound; a missed
  bound runs the xo_pure build as a load control and widens to twice its
  figure plus a margin, capped at a quarter of the loop:
  gotchas/hybrid-preemption-stw-under-load.md; Last verified:
  2026-10-13). Use
  `-asmdecl` only: the generated Go has unreachable code that plain `go
  vet` reports.
- `XO_BASE=<old xo> bench/perf/hybrid/run.sh` adds the old build's hybrid
  binaries to the interleaved runs (columns "base ms", "hybrid/base").
  `bench/perf/hybrid/xo/sieve_chunk` (not in the default list; argument:
  chunk size) is sieve_split called from Go in chunks.
- Profile a hybrid binary with `XO_CPUPROFILE=p.prof ./bin` and
  `go tool pprof -top p.prof` (assembly functions appear as
  `xoasm_<name>`).

## Prebuilt runtime (decision 0157)

Gone since 2026-10-16 (decision 0234): a program has no C runtime, so
there are no packs, no `xo rtpack` or `xo dev rtpack`, no runtime object
cache, and `--ld=macho` links the program's objects alone; release
archives have no lib/xo/rtpack (scripts/release.sh, release-verify.sh).
XO_RTPACK is read only by the drivers of seeds before 0.16.0
(scripts/bootstrap-xo.sh still sets it off for them). What follows is
the history.

Last verified: 2026-10-11. Results:
`bench/results/prebuilt-runtime-2026-10-11.md`.

- A pack is `<root>/<os>-<arch>/`: `rt*.o`, one object of std/llrt per
  variant (one, `rt.o` with no defines, since format 5, 2026-10-15,
  compiler/xo rt.xo variant_file; 2 with and without `-DXO_TIME` in
  format 4; 4 with `-DXO_PROC` in format 3; 16 with `-DXO_FS` and
  `-DXO_STDIN` in format 2; 32 with TLS and a `libxotls.a` before
  format 2), and
  `rtpack.json` (format, target, key, compiler, flags, SHA-256 per
  file). Packs exist for darwin/arm64 and linux/arm64 (the arm64
  backend's targets). About 16 MB per target.
- Root: `$XO_RTPACK` (a directory, or `off`), else `lib/xo/rtpack`
  beside the directory of the xo binary (symlinks resolved), the layout
  of a release archive. `xo rtpack --status` prints the pack used for
  the host or why none is.
- Used by `ccBuild` (internal/build/llvm.go) when the link's flags are
  exactly `-O2` plus a variant's defines (the arm64 backend's), the
  manifest's key equals `build.RTPackKey(Host())` (pack format, target,
  `rtPackFlags`, the variant table, the runtime sources embedded in
  `std.LLRuntime`; not the compiler; format 2 since 2026-10-15: 16
  variants, no TLS library), and the file's SHA-256 matches. Otherwise the runtime is
  compiled locally into `<user cache>/xo/rtobj` (`XO_RTOBJ_CACHE`, or
  `off`), as before. `XO_RTPACK=off` forces the local compile. The
  binary cache key includes the pack (`rtPackID`).
- Pack flags (`rtPackFlags`): `-O2 -g0 -fexceptions -funwind-tables`;
  darwin `-mmacosx-version-min=11.0`; Linux `-fPIC -moutline-atomics`;
  Every compile maps its work directory to `/xo` (`-ffile-prefix-map`).
- Compilers: darwin/arm64 the host's Apple clang (21.0.0 on
  2026-10-11); linux/arm64 zig cc 0.15.1 `-target
  aarch64-linux-gnu.2.31` with `zig ar`, cross compiled on the Mac (zig
  from ziglang.org, SHA-256 checked against its index.json; set
  `ZIG_GLOBAL_CACHE_DIR` to keep its cache out of the home directory).
  Apple clang `--target=aarch64-linux-gnu --sysroot=<Linux headers>`
  also works (`xo rtpack --cc=...`), see the decision's alternatives.
- Validated 2026-10-11: `internal/arm64` TestGoldenRun and TestRun with
  the pack on darwin and in the linux/arm64 container (no runtime
  compiled); the zig built pack linked by gcc 10 on glibc 2.31 (Debian
  bullseye, no clang) for a plain, a std/time, and an HTTPS program.
- Not in a pack: the LLVM backend's runtime (bitcode for LTO, compiled
  by the local clang), sanitizer builds, links with `use c` include
  flags. Go's zoneinfo.zip ships beside the pack instead
  (`lib/xo/zoneinfo.zip`, 2026-10-12), which xo1 reads before `go env
  GOROOT`; `XO_ZONEINFO` still overrides.
- Tests: `internal/build/rtpack_test.go` (TestRTPackVariant,
  TestRTPackKey, TestNormalizeAr, TestClangErrorRuntimeHint; without
  -short TestRTPackDeterministic: two packs from two work directories
  byte identical, and TestRTPackUse: used, off, another key, a bad
  checksum), `cmd/xo/rtpack_test.go`, `scripts/disttar/main_test.go`.

## TLS (native std/http, decision 0202)

TLS on the native backends is Xo: std/native/llrt/tlsio.xo (client),
tlssrv.xo (server) over tls13.xo, tls12.xo, tls13srv.xo, tls12srv.xo,
x509.xo, x509verify.xo, nistec.xo, mlkem.xo and the other crypto modules.
Nothing is compiled or cached for it; on macOS a TLS client loads the
Security framework (and with it CoreFoundation) with dlopen at its first
verification of the system's roots and calls it through dlsym and
rt.call_c2/rt.call_c4 (std/native/llrt/tlscert.xo; since 2026-10-16 no
program links the frameworks, which cost about 14M instructions of every
https client's launch). Seeds before 0.17.0 lack rt.call_c4 and cannot
build xo from these sources. Last verified: 2026-10-16. Mbed TLS and
fiat-crypto (decisions 0105, 0134; third_party/mbedtls,
third_party/fiat-crypto, std/llrt/xo_tls.c, the `<user cache>/xo/mbedtls`
library cache and `$XO_TLS_CACHE`, `XO_LLRT_C=tlsio` and `tlssrv`, the
prebuilt packs' libxotls.a) were removed on 2026-10-15. The 0.11.0 seed
links the frameworks only for a program that calls the Mbed C, so it
cannot build xo from these sources; seeds from 0.12.0 on can. Last
verified: 2026-10-15.

## Linux (containers)

Last verified: 2026-10-09. Results: `bench/results/linux-2026-10-09.md`.

- `scripts/linux-test.sh [arm64|amd64] [go test arguments...]` builds the
  image `xo-linux-test:<arch>` from `bench/linux/Dockerfile`
  (`golang:<host Go version>-trixie` plus Debian's clang 19, lld, and the
  compiler-rt sanitizer runtimes) and runs the tests in a container that
  is removed afterwards. The repository is mounted read only at `/src`;
  the Go cache, `HOME` (so the xo build directory), and `TMPDIR` live in
  the Docker volume `xo-linux-cache-<arch>` (`docker volume rm` it to
  start cold). `GOFLAGS=-buildvcs=false` because the worktree's `.git`
  points outside the mount.
- Default arm64 run: `go vet` then `go test -count=1 -p 2` over
  `go list -e ./... | grep -v /runs/`
  (about 6 min on an M series Mac): the Go backend on Linux, the LLVM and
  arm64 backends' goldens linked as ELF and run (epoll poller, futex
  parks, fibers, SIGTERM, TCP, HTTP), ThreadSanitizer, AddressSanitizer,
  and UBSan.
- Default amd64 run (emulated: Docker Desktop uses Rosetta, `/proc/cpuinfo`
  says `VirtualApple`): `go test -short -p 2 ./...` then the full
  `./internal/llvm` (x86-64 LLVM IR, the x86_64 fiber switch, epoll).
  ThreadSanitizer cannot start there; the TSan tests run their programs
  without it (`gotchas/tsan-under-x86-emulation.md`). The arm64 backend
  only emits assembly on amd64. The full amd64 suite also passes but
  needs `-p 1 -parallel 4` for internal/gen and internal/llvm (about
  4 and 5 min): translated compilers use more memory.
- Extra arguments replace the default, for example
  `scripts/linux-test.sh arm64 -count=1 -p 1 -run 'ASan|HTTP' ./internal/llvm`.
  Keep `-p` low: the Docker VM (8 GiB on 2026-10-09) runs out of memory
  with parallel Go builds of the generated runtime
  (`gotchas/docker-vm-memory-go-compile-killed.md`).
- Requires Docker; the container gets `SYS_PTRACE` and no seccomp
  profile (sanitizers). Containers are named `xo-linux-<arch>-<pid>`.

## Linux (EC2, infra/ci-aws)

Last verified: 2026-10-09. Results: `bench/results/ci-aws-2026-10-09.md`.

- `infra/ci-aws/ci.sh [short|full|race|sanitizers ...]` runs the CI jobs on
  real Linux machines: Terraform (local state in `infra/ci-aws/`) creates
  one key pair, one security group (tcp 22 from 203.0.113.10/32 only),
  and a `c7g.4xlarge` (arm64) and a `c7i.4xlarge` (amd64) with Ubuntu
  24.04 (Canonical's AMI by data source), all tagged `project=xo-ci`, in
  AWS account ACCOUNT_ID only (the provider's `allowed_account_ids`).
- It rsyncs the working tree (no `.git/`, `runs/`, `.tools/`, `.claude/`),
  installs the host's Go version from go.dev plus clang 18, lld, llvm,
  build-essential, and `libclang-rt-18-dev`
  (`gotchas/ubuntu-clang-without-sanitizer-runtimes.md`), runs the jobs on
  both machines in parallel, each job with a cold `GOCACHE`, and copies
  the logs to `infra/ci-aws/logs/<timestamp>/<arch>/` (gitignored).
- An EXIT trap always runs `terraform destroy -auto-approve`, deletes the
  generated key `infra/ci-aws/xo-ci-key.pem`, and runs
  `infra/ci-aws/verify-teardown.sh` (instances, key pair, security group,
  volumes: every list must be `[]`). By hand:
  `terraform -chdir=infra/ci-aws destroy -auto-approve`.
- A whole run takes about 20 minutes and costs about $0.45 on demand
  (2026-10-09). TSan runs natively on both (unlike amd64 under Rosetta).
- Needs terraform (1.16.2 used), the AWS CLI, rsync, ssh, python3.

## Modules and workspaces (decision 0086, steps 1 to 5; proxy 0126, checksum log 0124)

Last verified: 2026-10-09.

- Package `internal/mod`: `modfile.go` (xo.mod `require <path> <version>`
  parsing, `SetRequire`, semver `Compare`), `cache.go` (module cache, git
  fetch, `RepoURL`), `lock.go` (`HashDir`, `HashModFile`, xo.lock read and
  write), `load.go` (`Load`: MVS, verification, vendor mode, workspace),
  `work.go` (xo.work, `WorkspaceDirs`), `cmds.go` (`Get`, `Tidy`,
  `Vendor`). `internal/mod/modtest` builds local git repositories for
  tests from `internal/mod/testdata/repos/<module path>@<version>/`.
- Module cache: `$XO_MODCACHE`, else `<user cache dir>/xo/mod` (macOS
  `~/Library/Caches/xo/mod`, Linux `~/.cache/xo/mod`). A version is
  extracted to `<cache>/<escaped path>@<version>` (upper case letters
  escaped as `!` plus lower case) and made read only (files 0444, dirs
  0555); bare clones live in `<cache>/cache/vcs/<sha256 of URL>`. `xo mod
  clean` removes the cache (plain `rm -rf` fails on the read only dirs).
- Fetch: `git clone --bare --no-local`, `git fetch` of tags when a tag is
  missing, `git archive refs/tags/<v>` extracted in Go (no symlinks, no
  paths leaving the tree). git runs with `core.hooksPath=/dev/null`,
  `GIT_TERMINAL_PROMPT=0`, `GIT_CONFIG_NOSYSTEM=1`. The module must have
  its xo.mod in its directory declaring the same path. Directories below
  it that hold an xo.mod (nested modules) are removed after extraction,
  and `HashDir` skips them (a vendored root module holds its vendored
  subdirectory modules). Last verified: 2026-10-09.
- Locations: `github.com/a/b` is `https://github.com/a/b.git` (also
  gitlab.com, bitbucket.org), any other path `https://<path>`.
  `XO_GIT_BASE=<dir or URL>` maps every path to `<base>/<path>`;
  `XO_GIT_REWRITE=<prefix>=<url>,...` rewrites by longest prefix (wins
  over XO_GIT_BASE).
- Subdirectory modules (`internal/mod/repo.go` `Locate`): on the three
  known hosts the repository is the first three path elements and the
  rest (without `/vN`) is the directory; elsewhere `git ls-remote <url>
  HEAD` is tried for each prefix, longest first (a cached clone of the
  whole path skips it), memoized per process by git location. Versions
  are tags `<dir>/vX.Y.Z`; the tree is `git archive
  refs/tags/<dir>/<v>:<dir>`. `xo publish` finds the directory from `git
  rev-parse --show-toplevel`, requires the module path (without `/vN`) to
  end in it, lists `<dir>/v*` tags (ignoring majors above the path's), and
  prints `git tag <dir>/vX.Y.Z`. Last verified: 2026-10-09.
- MVS: walk every reachable `path version` from the main module (or every
  workspace module), keep the highest version per path. Every visited
  version is downloaded (its xo.mod is read from the cache tree).
- xo.lock: `<path> <version> h1:<tree hash>` for selected versions and
  `<path> <version>/xo.mod h1:<hash>` for every version read. Hash: base64
  SHA-256 of sorted `"<sha256 hex>  <slash path>\n"` lines (Go's h1
  shape over the raw tree). Every check and build rehashes the trees and
  compares (XO1304 mismatch, XO1305 missing). Only `xo get`, `xo mod
  tidy`, and `xo mod accept-effects` write it, always without workspace
  and vendor. Effect lines: see "Effect lock" below.
- Vendor: `xo vendor` copies selected trees to `vendor/<path>/` (writable
  copies) and writes `vendor/modules.txt` (`# <path> <version>` lines).
  When `vendor/modules.txt` exists (and no workspace is active), builds
  read only vendor/, check each direct require is vendored at its version
  or above (XO1307), and verify tree hashes against xo.lock. The checker
  skips a `vendor/` directory holding `modules.txt` when walking dirs.
- Offline: `XO_OFFLINE=1` or `--offline` (any command; removed from the
  arguments before dispatch in `cmd/xo/mod.go` `moduleFlags`, env
  restored after the command) never runs git; a missing version is
  XO1303; `xo get` resolves `latest` from the cached versions.
- Workspaces: `xo.work` (`xo preview`, `use <dir>` lines) found at or above
  the main module, or `XO_WORK=<file>`; `XO_WORK=off` or `--workspace=off`
  ignores it. It applies only when it uses the main module. Used modules
  override requirements (their requirements join MVS; xo.lock is the
  union of their locks for verification). `query.Load` (check, query,
  mcp), `xo edit`, and `xo test` add every workspace module's directory
  (`mod.WorkspaceDirs`); `xo doc` uses `query.LoadExact`. `xo build --release`
  and `xo build --ci` fail with XO1308 naming the overridden modules. Build info records
  `deps` (path to version, or `workspace <dir>`).
- Checker integration (`internal/check/deps.go`): `loadDirs` calls
  `mod.Load` once per main module and registers each selected module's
  directory as a root; imports resolve by the longest module path prefix.
  Problems become diagnostics on the xo.mod/xo.lock/xo.work line;
  imports of a module with a problem are not reported again. XO1306: a
  remote import no root covers, or a package of a module that is in the
  build only because another module requires it. Modules from the cache
  or vendor/ have `Module.Dependency` set; `xo edit` refuses to change
  them. `use go` in a dependency binds into the main module's cache.
- `xo serve` stamps xo.lock, vendor/modules.txt, and the nearest xo.work
  of each module root (used or not) and records which file that is
  (`mod.NearestWork`), so a new, removed, or edited workspace above the
  directories is noticed; `serve.Forward` checks in process when
  XO_OFFLINE, XO_WORK, XO_MODCACHE, XO_GIT_BASE, or XO_GIT_REWRITE is set.
- Effect ceilings (step 3): `internal/mod/ceiling.go` (`Ceiling`,
  `ParseCeiling`, `Ceilings(dir)`: xo.mod requires plus the workspace's
  `use ... uses` lines), `internal/check/ceiling.go` (`checkCeiling`,
  called from `callFunc` and `funcValue`; XO1309 with a note on the
  declaring line). `SetRequire` keeps a ceiling when `xo get` or `xo mod
  tidy` rewrites the version. `mod.EffectLabels` is kept equal to the
  checker's labels by `TestCeilingLabels`.
- Vouches (decision 0156, 2026-10-11): `internal/mod/vouch.go`
  (`Vouch`, `parseVouch`, `VouchesOf`, `FindVouch`, `SuggestVouch`);
  `internal/check/vouch.go` (`escapeHatch`, called from `loadCImport`
  for every `use c` and from `loadGoImport` for a `use go` of a package
  `gobind.PurePackage` rejects): a required module's (cache or vendor/)
  escape hatch needs a vouch of the main or a workspace module's xo.mod
  for the exact version, else XO1316 (`xo get` and `xo mod
  accept-effects` refuse such a version before writing, from a syntax
  scan, `mod.ModuleHatches`; `--accept-effects` does not override it;
  Last verified: 2026-10-11); an unvouched `use c` binds with no
  `uses` (ffi), a vouched one with the vouch's. `Result.EscapeHatches`
  and `Result.ConsumerVouches` feed `xo audit`. A dependency's `use c`
  binding goes to the main module's `.xo/cbind` (`goBindRoot`). Tests:
  `cmd/xo/vouch_test.go` (`TestVouches`, `TestVouchesC` with clang),
  `internal/check/vouch_test.go` (vendored). Last verified: 2026-10-11.
- Effect lock (decision 0156, 2026-10-11): xo.lock lines `<path> effects
  <labels|none>` (`Lock.Effects`), keyed by module path so they survive
  upgrades. `mod.ModuleEffects(dir)` is the union of the `uses` labels of
  the pub functions and methods of a module tree (syntax only, _test.xo
  files, nested modules, vendor/, testdata/ skipped). `Load` fills
  `BuildList.Effects` and `Locked` and reports XO1317 on the lock line
  for an effect the line does not admit (`checkEffects`, also for
  vendored builds), unless `Options.AcceptEffects`/`AcceptAll`.
  `writeLockFor` (get, tidy) records missing lines, narrows shrunk ones,
  and fails on widened ones (xo.mod restored). `xo mod accept-effects
  [--dir] [--json] [<module path>...]` (`mod.AcceptEffects`) and `xo get
  --accept-effects` accept. The self hosted checker parses effects lines
  (XO1301 when malformed) but does not compare them: XO1317 is not one
  of its staged diagnostics. Test: `cmd/xo/effectlock_test.go`. Last
  verified: 2026-10-11.
- Capability attenuation (decision 0156, 2026-10-11): `std/rt/attenuate.go`
  (`onlyNet`, `onlyEnv`, `onlyProc` wrapping any Net, Env, Proc, the
  fakes included; `httpDenied` called from `Http_Client.M_send`, a
  `CheckRedirect` in `http_net.go`, `Sql_open` refuses an `onlyNet`).
  The interfaces' `only` methods are in std/prelude.xo and std/testing.xo;
  `std/rt/std_gen_M.go` is regenerated with `go run ./scripts/stdgen`. Native builds get XO1201 from the SSA builder's
  fallback ("the method `Net.only`"). Tests: golden
  `internal/gen/testdata/golden/cap_attenuation`,
  `cmd/xo/attenuate_test.go`. Last verified: 2026-10-11.
- `xo audit` (decision 0156, 2026-10-11): `cmd/xo/audit.go`. Loads with
  `query.Load` (or `build.CheckNoGCDirs` with `--no-gc`), reads
  `Result.Deps()` (`Effects`, `Locked`), `mod.Ceilings`,
  `Result.EscapeHatches`, `Result.ConsumerVouches`, and walks the
  consuming modules' files (`Module.Dependency == ""`, not std, test
  files skipped) with `check.Inspect` and `Info.CallOf`: attenuation
  calls (`only`, `sub`, `read_only`, `write_only`, `cwd` on a capability
  type) with constant string or list-of-string arguments, and capability
  typed arguments of calls into dependency functions, attenuated when the
  argument is an attenuation call or a `let` local bound to one. JSON
  (`--json` or a non terminal stdout) or a text report; exit 1 on check
  errors. Test: `cmd/xo/audit_test.go`. Last verified: 2026-10-11.
- Attack replication (decision 0156, 2026-10-11): `cmd/xo`
  `TestSupplyChainAttacks` models the Backstabber's Knife Collection
  classes as small malicious Xo dependencies and consumers (benign,
  test-only payloads) and asserts the stage Xo blocks each (fetch,
  check, lock, runtime, or not blocked). `XO_SC_REPORT=1` writes the
  table to `bench/results/supply-chain-2026-10-11.generated.md`; the
  curated report is `bench/results/supply-chain-2026-10-11.md`. Last
  verified: 2026-10-11.
- API diffs and publish (step 4): package `internal/release`
  (`ModuleAPI`: the syntax level `xo query api` surface of every package
  via `query.DeclSurface`; `Diff`, `Bump`; `Publish`). `xo get` prints the
  diff of every changed requirement (JSON `api`), `xo get -u` upgrades
  every requirement; `xo publish [--check] [--version v] [--json]`
  (`cmd/xo/publish.go`) reads tags with `mod.LocalTags`, extracts the
  previous tag with `mod.ExtractTag` into a temp dir, and prints the tag
  command (never runs git tag or push). `xo init <path>` writes xo.mod and
  `/xo.work` in .gitignore.
- Major versions: `internal/mod/major.go` (`MajorSuffix`,
  `MajorMismatch`); `RepoURL` strips a `/vN` suffix; `Versions` keeps the
  tags of the path's major; `localName` (`internal/check/load.go`) skips
  the suffix.
- Tests: `internal/mod/mod_test.go` (MVS, lock goldens
  `testdata/golden/*.lock`, moved tag, fetch errors, offline, get, tidy,
  vendor, workspace), `internal/mod/ceiling_test.go` (ceilings, /vN),
  `cmd/xo/mod_test.go` (`TestModules`: the commands end to end; runs the
  built program unless -short), `cmd/xo/release_test.go`
  (`TestEffectCeilings`, `TestPublish`, `TestInit`),
  `cmd/xo/subdir_test.go` (`TestSubdirModules`: tidy, lock, run, get,
  vendor, cli/v2, publish in a subdirectory), `internal/mod/repo_test.go`
  (`TestLocate`: probing on another host, nested module pruning),
  `internal/release/api_test.go`, `internal/serve/serve_test.go`,
  `internal/check/rules_test.go` (XO1306), `internal/check/ceiling_test.go`
  (XO1309). No test touches the network.
- Module proxy (decision 0126, 2026-10-10): `internal/mod/fetch.go`
  (`ParseProxy`, `MatchPatterns`, `settingsFor`, `Versions`, `Download`,
  `download(root, ...)` with an explicit cache root, `gitFetch`),
  `internal/mod/proxy.go` (`proxyList`, `proxyFetch`, `unzipModule`,
  `ZipModule`, `ProxyHandler`). `XO_PROXY` default `direct`; `,` falls
  back on 404/410 only, `|` on any error; `XO_NOPROXY` globs go direct.
  Endpoints `<escaped path>/@v/list|<v>.mod|<v>.zip|<v>.lock`; zip
  entries under `<path>@<v>/`, 500 MB cap, regular files only. `xo mod
  proxy serve [--addr=127.0.0.1:7070] [--fetch] [<cache dir>]`
  (`cmd/xo/mod.go`) serves a module cache directory; `--fetch` fills it
  with direct git fetches (never through XO_PROXY). The repository
  helpers (`repoDir`, `vcsDir`, `locateIn`, `versionDirIn`) take the
  cache root so a proxy serves its own cache in process. Last verified:
  2026-10-10.
- Checksum log (decision 0124, 2026-10-10): package `internal/sumdb`
  (`tlog.go` RFC 6962 hashing and proofs, `note.go` keys and signed tree
  heads, `client.go`, `server.go` an in memory log with a lookup hook).
  `XO_SUMDB=off` (default) or `<name>+<id>+<key> [<url>]`; checked in
  `download` on every first fetch unless `XO_NOSUMDB` matches; the
  latest verified head is kept in `<cache>/cache/sumdb/<name>/latest`.
  `mod.LogRecord(root, path, v)` is what a log server records. There is
  no CLI to run a log or generate keys yet (`sumdb.GenerateKey`,
  `sumdb.NewServer` from Go). XO1313 security failure, XO1314
  unreachable or malformed.
- Definition hashes (decision 0125, step 5, 2026-10-10): package
  `internal/defhash` (`Module(dir, modPath)`, `Diff`, `Find`,
  `Nearest`), reflection over `internal/ast` with per field rules
  (`binders`, paths, `SelectorExpr`, `StructPat`); a new AST field is
  hashed by default, but a new binding or name field needs a rule.
  `xo query hash` runs without a check (`cmd/xo/hash.go`, before the
  query loader); `release.Publish` fills `Result.Definitions`.
- Tests that stand up servers use `httptest` on loopback in the test
  process (`internal/mod/proxy_test.go`, `log_test.go`,
  `cmd/xo/proxy_test.go`, `hash_test.go`); `modtest.Setup` pins
  XO_PROXY=direct and XO_SUMDB=off.
- Not yet: Go's "major subdirectory" layout (`/v2/` directories of one
  tag; major versions of a subdirectory module use the major branch way,
  tags `<dir>/v2.x.y`), `go-get` style HTML meta discovery of repository
  roots, `xo get --uses` to set a ceiling from the command line; proxy
  authentication and `file://` proxies; a log CLI (keygen, serve with
  persistence), tiles, proxying the log through a module proxy, and
  head gossip; definition hashes in xo.lock and type based (checked)
  definition hashes.

## Go interop (prototype)

Last verified: 2026-10-08. Decision 0041 is still Proposed; this is the
experiment of its work queue item 6. Results and coverage:
`bench/results/go-interop-2026-10-08.md`.

- Syntax: `use go "<Go import path>" [as name]` (`ast.UseDecl.Go`; the
  lexer reads `go` as an import path and the parser takes the string after
  it). Default local name: the Go package name. Not in `spec/core.md` yet.
- Binding (`internal/gobind`, `xo bind`): runs `go list -e -export -deps
  -json` with `CGO_ENABLED=0 GOWORK=off GOFLAGS=-mod=mod` in the binding
  workspace `<cache>/_mod` (a Go module `xogobind`; third party packages
  are added with `go get <path>@<pin or latest>`), reads export data with
  `go/importer.ForCompiler(fset, "gc", lookup)`, and writes
  `<cache>/<path>.xo` (declarations, signature mode, `use std/go`, every
  function `uses go`; skipped names listed as `// skip: name: reason`
  header lines) and `<cache>/<path>.go` (package `rt` adapters named with
  the prefix `Go_<path with _>_`). Files are written to a temporary name
  and renamed, so parallel builds never read a partial binding.
- Cache: `$XO_GOBIND_DIR`, else `<xo.mod dir or module dir>/.xo/gobind`
  (`.xo/` is in `.gitignore`; Go and the Xo module walk skip dot
  directories). A cached binding is reused unless its header has another
  `xo-bind: v<N>` (gobind.Version; 2 since 2026-10-08, when callbacks
  stopped carrying the request's arena, decision 0066; 3 since
  2026-10-08, decision 0067: adapters call `GoEnter(tk)` and defer
  `GoExit(tk, name)`, and callbacks go through `rt.NewGoCallback(tk)`
  with `cb.Begin()`/`cb.End(ctk)` per call; 4 since 2026-10-08, decision
  0073: callback arguments are converted first and passed as
  `cb.Begin(func() []any {...})`, and `*time.Timer`/`*time.Ticker` are
  handles, so `time.after_func` binds; 5 since 2026-10-09, decision 0093:
  adapters call `rt.GoFxValue`/`rt.GoFxRerun` when `rt.GoFxOn()`, by
  value calls live in `goCall_<prefix><name>`, declarations carry a
  `/// Replay:` line; 6 since 2026-10-27, Intel b15: a comparable struct
  handle the package hands out as a value, `T` in some bound signature or
  field (netip.Addr), compares and hashes by value; 7 since 2026-10-27,
  decision 0392: a type of another package is that package's binding's,
  named through a `use go "P" as alias` line of the declaration file,
  which Bind binds in the same run) or another module version
  than the pin, or another `replay rerun go` list (header line
  `// replay-rerun: <names>`). Pins: `require go <module> <version>`
  lines in xo.mod. A binding made with a rerun list is cached as
  `<cache>/<path>.rerun-<8 hex>.xo` and `.go`, beside the plain one.
  Last verified: 2026-10-09.
- Checker (`internal/check/load.go` `loadGoImport`): binds on demand, loads
  the declaration file as a module `go:<path>` with `IsStd` set and
  `Module.Go` holding the binding. XO1101 when binding fails, XO1102 when
  a name or `Type.method` is in the binding's skip list. Effect label `go`
  (effects.go).
- Codegen: `stdPrefix` returns the binding prefix, so calls lower as std
  calls. `gen.Generate` adds `rt/xo_gobind_<path>.go` per binding and
  writes the workspace's `require` lines into the build `go.mod` and its
  `go.sum`. Runtime helpers: `std/rt/gointerop.go` (`Go_Error`,
  `GoEnter`/`GoExit` around each Go call (panic to fault, `thread.inGo`),
  `GoBoundary` for field accessors, `GoCallback` (a call on another
  goroutine is a child task with its own thread, decision 0067),
  `GoContext`, `GoListOf`/`GoSliceOf`/`GoMapOf`/`GoMapToGo` copies,
  `GoCollect` for iterators), `std/rt/godeadlock.go` (XO-F001 through Go
  calls waiting for stuck callbacks, decision 0073), `std/rt/gosched.go`
  (foreign callbacks admitted at settled decisions and recorded, decision
  0073). `curG` (goroutine identity) is assembly in
  `std/rt/curg_amd64.s` and `curg_arm64.s` (the g register or TLS) with a
  `runtime.Stack` fallback in `curg_other.go`; `std/embed.go` embeds
  `rt/*.go rt/*.s`, so a new `.s` file in std/rt is copied into build
  directories. Last verified: 2026-10-08.
- Tests: `internal/gobind` (names, pins, mapping, cache staleness),
  `internal/check/rules_go_test.go`, `internal/gen/gobind_test.go`
  (`TestGoBindingSignatures`: 20 std packages, every declaration's Go
  signature as gen calls it equals the adapter's), goldens `go_*` and
  `test_go_interop`, `test_go_callback_deadlock` and
  `go_callback_deadlock_real` (decision 0073: a Go call waiting for a
  stuck callback is XO-F001, a sleeping one is not), `test_go_callback_task` (`-explore 40`, also under
  -race: a callback on Go's goroutine holds mutexes beside a test task and
  a request), `cmd/xo/bind_test.go`, `std/rt/gointerop_test.go`,
  `std/rt/gocallback_test.go`, `internal/gen/replay_gocallback_test.go`,
  `internal/gen/replay_gocalls_test.go` and `internal/gobind/replay_test.go`
  (Go calls recorded and replayed, decision 0093)
  (`TestReplayGoCallback`: seeds reproduce foreign callbacks, recordings
  replay them; runs under -short too).
  The golden `go_uuid` needs `github.com/google/uuid@v1.6.0` from the
  network or module cache and is skipped when `go mod download` fails. The
  golden and checker tests set `XO_GOBIND_DIR` to a temporary directory.

## C interop (`use c`, native builds)

Last verified: 2026-10-10. Decision 0116 (stage 2 of decision 0093);
spec core 4.5.

- Syntax: `use c "<header>" { link "<x>" } [as name] [uses none | uses
  e, ...]` (`ast.UseDecl.C`, `Links`, `Uses`; the lexer reads `c` as an
  import path like `go`). `xo fmt` keeps the clauses in that order.
- Binding (`internal/cbind`, `xo bind --c [--dir d] [--link x]...
  [--uses none|e,...] <header>...`): writes `#include "<abs header>"`
  (module header) or `#include <h>` (include path) into a temporary
  file, runs `clang -fsyntax-only -Xclang -ast-dump=json -x c` on it
  (`$XO_CLANG`, then `clang`; on macOS a failure is retried with
  `-isysroot $(xcrun --show-sdk-path)`), decodes the dump keeping key
  order, and binds the functions whose location is in the header.
  Writes `<cache>/c_<name>_<8 hex>.xo` (declarations, signature mode)
  and `.c` (the adapter, for reading; builds take it from memory). The
  hex is of the header path, links, and effects, so two `uses` ceilings
  of one header are two modules. Within one process a binding is reused
  while the header's contents are the same.
- Cache: `$XO_CBIND_DIR`, else `<xo.mod dir or module dir>/.xo/cbind`.
- Checker (`internal/check/cimport.go` `loadCImport`): XO1201 when the
  check is not native (`Options.Native` false: `xo check` without
  `--no-gc`, the Go backend); XO1104 when binding fails or a `uses`
  label is unknown; XO1105 for a skipped name; the module `c:<...>` is
  `IsStd` with `Module.C` set. C handles are handles for decision 0112.
- SSA (`internal/ssa/build_cffi.go`): a call becomes `rt.<prefix><fn>`
  (the C symbol `xo_<prefix><fn>`); a C handle type lowers to KInt (the
  pointer, not reference counted); a struct argument is passed field by
  field; a struct result is read through `<sym>_r<i>` getters; a handle
  result branches on NULL into `T?`.
- Build (`internal/build/cffi.go`): adapters go to `<build>/cffi/`, link
  sources and archives are added by absolute path, `-l` flags go after
  the objects (`ccBuild` splits them). No `-I` (it would turn off the
  runtime object cache): adapters include the header by absolute path.
  A build with module C files is not binary cached.
- Runtime: cabi.xo's `cffi_str` export (`xo_x_cffi_str`; it was
  std/llrt/xo_std.c's xo_cffi_str until 2026-10-15) makes the Str results.
- Tests: internal/cbind (tiny.h mapping and skips, zlib.h when present,
  the adapter compiles with -Wall -Werror), internal/check
  `TestUseC`, cmd/xo `TestBindC`, internal/llvm `TestUseC` (archive
  built in the test with clang and ar, and C source; arm64 and LLVM,
  leak check) and `TestUseCZlib` (skips without zlib.h).

## Real-time mode (`uses realtime`, decision 0133)

Last verified: 2026-10-10. Spec core 6.5 and 3.5 (`bound N`).

- Checker: `internal/check/realtime.go` (`checkRealtime` after
  `solveEffects`; `rtBounded` and `rtBlocking` name the prelude methods
  allowed and blocking; `constInt` evaluates loop bounds;
  `Info.LoopBound` for the backends). `resolveSig` removes `realtime`
  from the uses clause and sets `FuncObj.Realtime`. Codes XO1401 to
  XO1405. Goldens `internal/check/testdata/golden/realtime_*`, rule
  table `rules14_test.go` (TestRealtimeRules).
- `bound N`: `ast.ForStmt.Bound`, `ast.WhileStmt.Bound` (parser,
  formatter, compiler/parser where it is a `List[Expr]` of zero or one,
  see gotchas/arm64-rc-helper-frame.md). Lowered as an iteration counter
  checked at the start of each iteration: `internal/gen/fn.go`
  `loopBound` (calls `rt.LoopBound`, XO-F006) and
  `internal/ssa/build_stmt.go` `loopBound` (OpFault). `xo test` fuzzes a
  function with a bounded loop (`internal/gen/fuzz.go` `fuzzable`;
  golden `internal/gen/testdata/golden/test_loop_bound`).
- Sections: `internal/ssa/build_call.go` `callAsVia` brackets a call of a
  real-time function from code that is not with `rt.realtime_enter` and
  `rt.realtime_exit` (in `faultingRT`: it raises XO-F007). Runtime in
  `std/llrt/xo_rt.c` ("Real-time sections"): per-heap `rt_depth`,
  `rt_allocs` (counted in `alloc_in`), and a 256 entry `rt_q` filled by
  `xo_rc_free`, `xo_rl_free`, `xo_rc_defer`, `xo_h_free`; drained at the
  outermost exit and by `fault_raise`.
- Tests: internal/llvm `TestRealtimeNative` (LLVM and arm64:
  `testdata/realtime/alloc` XO-F007 cases caught by task.isolate, the
  `realtime_section` golden's XO_RTSTATS counts, the demo's three modes
  with the leak check) and `TestRuntimeRealtime`
  (`std/llrt/testdata/realtime.c` under ThreadSanitizer, then -O2);
  native goldens `realtime_section`, `realtime_bound_fault`.
- Demo: `examples/realtime-control` (native only: `use c "rtclock.h"`
  for a nanosecond clock): `xo build --backend=arm64
  examples/realtime-control`, then `./realtime-control rt|plain|alloc
  [iterations] [swap every]`. Numbers in
  `bench/results/realtime-2026-10-10.md`.

## Runtime (std/rt)

Last verified: 2026-10-08.

- `core.go` Option/Result/tuples/Box; `frames.go` error frames (2026-10-09,
  decision 0092): `Trace` with `Frame0` to `Frame6` (frame and values in
  one allocation) and `FrameN`, formatted only by `Trace.Lines`;
  `debugBounded` stops `Debug` formatting at a limit (`debugTo` checks a
  registry of bounded builders, `boundedActive`; `writeQuoted` quotes
  long strings only as far as needed); `FrameTextOf` for values formatted
  at capture; `arith.go` checked integer
  math; `fault.go` faults and contracts; `list.go`, `map.go` value
  semantics collections: List (a header over a backing array whose block
  has a claimed length and a sticky shared flag: append in place when the
  list ends at the claimed length; element writes, `pop`, `insert`,
  `remove_at` in place while the block is not shared, else copy first and
  mark the copied elements; own marks from generated code, soundness
  argument in the comment on `List`; since 2026-10-08) and Map/Set (append only entry log
  plus an atomic open addressing index; a write appends in place when the
  value's log length equals the claimed length, otherwise it forks a child
  table in O(1); soundness argument in the comment on `Map`, see
  `gotchas/map-copy-claims-log.md`; since 2026-10-08); `show.go` Eq/Hash/Cmp/Debug/Display; `str.go`, `time.go`,
  `option.go` prelude methods; `mono.go` typed helpers of the default
  lowering (`AddI`, `SubI`, `MulI`, `DivI`, `RemI` on int64 with panic
  valued faults, the constant operand forms `AddIUp`, `AddIDown`,
  `SubIUp`, `SubIDown`, `MulIK`, `MulKI`, `CmpO`, `List_AtM`, `List_AtU`;
  same results and fault messages as the generic forms, checked by
  `mono_test.go`); `core.go` `Iface` and `RegAdapter` (interface adapters
  registered by package main for generic code).
- `sched.go` (2026-10-08): threads, the deterministic scheduler, the
  deadlock monitor and XO-F001 reports, `xo dump` (see "Deterministic
  scheduling and live inspection").
- `task.go`, `scope.go`, `chan.go`, `sync.go`: the cancellation tree,
  scopes, tasks, channels and `select` with explicit wait queues, Mutex,
  Atomic, Semaphore, `task.*` (`once.go`: `task.Once`). `Mutex.with` records its acquisition site
  as program counters, resolved to `file.xo:line` only for XO-F004
  (2026-10-08).
- Channel loops, `task.Once`, `task.map_concurrent_until` (2026-10-08,
  decision 0043, core 3.5 and 7.3, std.md 7). Checker: `iterElem`
  (internal/check/stmt.go) gives `Chan[T]` the element type T; patterns
  are checked as in other `for` loops (irrefutable). Codegen
  (`forStmt`, internal/gen/fn.go): `for { rv := ch.M_recv(tk); if
  !rv.Some { break }; ... }`, so every receive is a cancellation point
  that unwinds. `once.go`: `Task_Once[T, E]` is a handle (a pointer to
  `onceState`; `XoEq` compares identity); `get[fx]` has no own non effect
  type parameter, so it is the Go method `M_get`
  (`gotchas/std-method-effect-only-go-method.md`). The first caller runs
  f in its own task, the others queue as waiters; the outcome (value,
  error, or fault) is stored and every waiter shares it. A runner that
  unwinds (cancellation, `proc.exit` in tests) hands the run to the first
  waiter still waiting, or clears the running flag when there is none. A
  fault in f is shared: later callers fault with the same message (the
  spec does not say; this is the choice of 2026-10-08).
  `Task_map_concurrent_until` (sync.go) runs the shared `mapConcurrent`
  loop in a child scope, waits with `awaitDeadline` (shared with
  `with_timeout`), and keeps a result only if it was stored before the
  timer callback set `expired` (one mutex, so the cut is the instant the
  timer fires on the real or virtual clock); the rest are canceled and
  give `Err(TimedOut)`. A fault in an item is raised in the caller.
  `mapConcurrent` rechecks cancellation after acquiring a permit
  (`gotchas/permit-handoff-after-cancel.md`; this also stops
  `map_concurrent` from starting items once canceled). Tests: checker
  `rules07_test.go`; goldens `chan_loops`, `test_once`,
  `test_map_until` (virtual clock), all three in `TestGoldenRace`.
- `arena.go` (2026-10-08): request scoped arenas (default, decision 0066). The HTTP
  server takes an `Arena` from a bounded channel free list (128 kept; not
  a sync.Pool, `gotchas/sync-pool-drops-on-gc.md`) per request, puts it on
  the request `Task` (`Task.arena`; scope bodies share it, spawned tasks
  never get it), and resets it after the response is written. It holds
  the request's header, parameter, and query Map tables, the body bytes,
  an arena response's JSON bytes and header table, and typed bump slabs
  for collection temporaries; buffers over 64 KB are not kept. Routes not
  marked confined, and every route behind middleware, get `heapCopy` of
  the request; a mounted router's middleware and `r.call` run with arena
  responses off (`noArenaResp`); the recorder gets heap copies of bodies
  (`recCopyReq`, `recCopyResp`); Go callbacks run with
  `GoCallbackTask` (no arena). Soundness argument: the comment on
  `Arena`; audit table: decision 0066. `world.go`: the virtual clock of tests (it
  advances when every task is blocked; wakers count the woken task as
  running first). `main.go`: `RunMain` (root scope, SIGINT/SIGTERM, exit
  status 0/1/70, crash bundles).
- `stop.go` (2026-10-08, decision 0042, core 7.5): two phase shutdown.
  `stopState` belongs to one program run (`RunMain` or
  `testing.run_main`) and every task of the run points to it
  (`Task.stop`, inherited by scope bodies, spawned tasks, and HTTP
  request tasks). Signals are counted: the first sets `requested`, closes
  the `task.stopping()` channel and an `event` for runtime waiters, and
  starts the grace timer (default `rt.DefaultGrace` 30s,
  `Proc.set_grace`; set after the stop it moves the deadline to stop
  time + d; `0s` cancels at once); the grace timer or the second signal
  cancels main's root scope; the third calls `os.Exit(130)` (real) or
  makes `run_main` return 130 and record it as `exit_code()`. Outside a
  run `task.stop_requested()` is false and `task.stopping()` never
  closes. `serve` (real): on the stop, `http.Server.Shutdown` with a
  context canceled when the serving task is canceled, then `Close` and
  cancellation of the request tasks (they hang below a per server node);
  in tests the in memory listener stops routing (new requests fail with
  `ClientErr.Connect`) and waits for requests in
  flight, which run with the server's stop state. Tests:
  `FakeProc.signal_after(d)` queues one signal per call (d from the start
  of the next `run_main`, or from now when the program calls it);
  `set_grace` before `run_main` sets the next run's grace. Timers of the
  virtual clock run the stop logic on a goroutine counted as a running
  task (`stopState.after`, `gotchas/virtual-timer-callback-holds-world-lock.md`).
  Golden `test_shutdown` (also in `TestGoldenRace`, which builds `test_*`
  directories in test mode), real process tests `TestSignal*` in
  `internal/gen/build_test.go` on `testdata/signal` (SIGTERM to the
  test's own child by PID). `TestSignalStopsReadLine` and
  `TestCancelStopsReadLine` (2026-10-09, decision 0088) run it with
  `STDIN=pipe` so standard input stays open and `read_line` blocks.
- `stdin.go` (2026-10-09, decision 0088): stoppable reads of the real
  standard input through one reader goroutine; golden `test_stdin_stop`
  covers the fake standard input and `sleep_or_cancel` on the fake clock,
  golden `codex_gaps` (Go and native: internal/llvm/testdata/golden)
  covers `Duration.parse`, tuple JSON, and `sleep_or_cancel`.
- `caps.go` real capabilities (fs rooting per core 6.3), `http.go`,
  `json.go` (`JsonW.str` escapes directly since 2026-10-08, matching
  encoding/json with HTML escaping off, `TestAppendJSONString`),
  `log.go`, `testing.go` (test runner, fakes, `run_main`),
  `std_gen_*.go` and `std_sigs_*.go` (generated), `gointerop.go` (Go interop boundary
  helpers and `go.Error`, 2026-10-08).
- Command line tool std (decisions 0160 to 0165, 2026-10-11):
  `cli.go` and `cli_completion.go` (std/cli; ported to Xo in
  std/native/cli.xo, byte for byte the same text), `proccmd.go` and
  `proccmd_caps.go` (`Proc.cmd`, `Cmd`, `Child`, `pipeline`, the real and
  fake backends), `term.go` with `term_unix.go`, `term_bsd.go`,
  `term_linux.go`, `term_windows.go`, `term_other.go` (terminal ioctls by
  build tag; std/term), `stdin.go`'s `stdinReadSome` (binary reads),
  `fsfile.go`, `fsfile_mem.go`, `flock_*.go` (File handles, links, globs,
  locks, user dirs), `csv.go`, `toml.go` (own parsers, ported to Xo in
  std/native/csv.xo and toml.xo), `regexp.go`, `archive.go` (Go's
  packages; no native port yet). internal/build's `stamp.go` is `xo build
  --stamp`.

## Agent interfaces (2026-10-08)

Last verified: 2026-10-08.

`xo mcp` (internal/mcp) is a Model Context Protocol server: JSON-RPC 2.0 on
stdin and stdout, one message per line, protocol revision 2025-06-18. It
answers `initialize`, `ping`, `tools/list`, and `tools/call`, and ignores
notifications. Tools, each with a JSON input schema and a compact JSON text
result (`isError` true for usage errors):

| Tool | Arguments | Result |
|---|---|---|
| `check` | `dirs`, `no_gc`, `fix` (decision 0096: apply the machine fixes first) | `{ok, errors, warnings, modules, diagnostics}` (diagnostics with notes and fixes, core 10.2), plus `applied` with `fix` |
| `query` | `kind` (type, def, callers, effects, holes, api, imports, deps), `arg`, `dir` | the `xo query --json` answer |
| `edit` | `op` (rename, add-field, change-sig), `args`, `dir`, `dry_run` | the `xo edit` summary `{op, target, ok, written, reason, files, edits, diagnostics, sites}` |
| `explain` | `code` | `{code, severity, title, summary, detail, example, fix}` |
| `fmt` | `paths`, `write` (default false: report only) | `{changed, unchanged, written, diagnostics}` |
| `test` | `dirs`, `run`, `seed`, `holes` | `{ok, passed, failed, skipped, failures}`; `errors` (human text) when the tests do not build |
| `build` | `dirs`, `targets`, `out` | `{ok, binaries, diagnostics}` |

Each tool runs the CLI command line in process with `--json` and reshapes the
output, so it answers what the CLI answers. Client config example:
`{"command": "xo", "args": ["mcp"]}`.

`xo serve` (internal/serve) listens on a unix socket
(`$XO_SERVE_SOCKET`, default `<user cache dir>/xo/serve.sock`, so
`~/Library/Caches/xo/serve.sock` on macOS) and answers `xo check` and
`xo query`. The CLI forwards those two commands when the socket answers
(dial timeout 300 ms) and checks in process otherwise; `XO_NO_SERVE=1`
turns forwarding off. `xo serve stop` shuts the server down through its own
request; `xo serve status` prints cache hits, misses, and entries.

Cache: whole check results (query.Session), keyed by the client's working
directory and the directories checked, at most 16 entries (least recently
used dropped). An entry is reused when every .xo and xo.mod file under the
directories, plus every file of the user modules it loaded, has the same
modification time and size, and no source file appeared. Any change rechecks
the whole program; there is no per module incremental recheck yet. Requests
run one at a time (the server changes to the client's directory for each).
A cached check of a small module took 0.26 ms against 3.3 ms uncached in
`TestServe` (2026-10-08).

Protocol: one JSON line each way, `{"op": "run"|"ping"|"stats"|"shutdown",
"args": [...], "cwd": "..."}` answered by `{"code", "stdout", "stderr",
"cached", "error", "stats"}`.

## Known gaps (2026-10-08)

- Command line std on the native backends (2026-10-11, decisions 0161,
  0163, 0164): `Proc.cmd` with `Cmd`, `Child`, and `Proc.pipeline`; the
  `Fs` handles, `append`, `copy`, modes, links, `glob`, and locks; and
  `std/regexp` and `std/archive` are XO1201 (planned) on `--no-gc`
  builds. std/cli, std/term and the Stdio terminal methods, std/csv,
  std/toml, and `Env.config_dir`/`cache_dir` build natively. Plans and
  next steps are in each decision. Last verified: 2026-10-11.

- Self hosted lexer (2026-10-08, decisions 0076 and 0081): `compiler/lexer`
  builds on the Go, arm64, and LLVM backends, and `TestSelfhostDifferential`
  compares all three with the Go lexer (the native ones skip without
  clang, and arm64 off arm64 hosts). The Xo tests of the module
  (`xo test`) run on the Go backend only: the native backends build
  programs, not tests. A change to `internal/lexer` or a lexical
  diagnostic must be ported to `compiler/lexer` in the same change.
  Remaining gaps with proposed fixes:
  `bench/results/selfhost-lexer-2026-10-08.md`. Last verified: 2026-10-08.

- Self hosted parser (2026-10-10, decision 0127): `compiler/parser`
  imports the lexer (`compiler/` is one module, `compiler/xo.mod`) and
  builds on all three backends; `TestSelfhostParserDifferential` requires
  identical `xo astdump` text for every file the Go parser accepts and a
  rejection for every file it rejects. It stops at its first error and
  prints only that diagnostic (no recovery, no habit fixes yet). A change
  to `internal/parser` or `internal/ast` must be ported to
  `compiler/parser` in the same change. Numbers and gaps:
  `bench/results/selfhost-parser-2026-10-10.md`. Last verified:
  2026-10-10.

- Self hosted checker (2026-10-10, decision 0141): `compiler/check`
  builds on all three backends and is compared stage by stage through
  the sections of the hidden `xo typedump`. Stages 1 and 2 (names, types
  and inference) are in: `TestSelfhostChecker` requires identical names
  and types sections on every repository run the Go checker accepts and
  the same diagnostics on runs rejected only for names and types. A
  change to name resolution or type checking in internal/check (or a new
  name record) must be ported to `compiler/check` in the same change.
  Runs with `use go` or `use c` are skipped (the Xo checker binds
  neither). Numbers and gaps: `bench/results/selfhost-checker-2026-10-10.md`.
  Last verified: 2026-10-10.

- LLVM backend prototype (2026-10-08): the subset above only (no Map,
  Set, Bytes, Rune, Duration, sized integers, interfaces, Display of
  structs, `defer`, contracts, tasks, std modules, `use go`, `xo test`,
  `var` parameters of function values, methods as values). Memory is
  reference counted; counts are not atomic. `==` on deeply nested
  recursive values recurses on the C stack. No error traces, crash
  bundles, or debug info; host only. What a full backend
  needs is listed in `bench/results/llvm-prototype-2026-10-08.md`.

- Deterministic scheduling (2026-10-08): a loop with no scheduling point
  keeps the turn forever (`gotchas/busy-loop-starves-deterministic-scheduler.md`);
  there is no preemption at loop back edges. `xo run --seed` reproduces
  interleavings only while real timers and I/O do not race with the
  program. `--explore` tries consecutive seeds with a uniform choice at
  each point, no smarter search (PCT, partial order reduction). Only
  plain tests are explored, not property tests or contract fuzzing.
  Deadlocks that involve a task blocked in I/O or a busy task are not
  reported, and a cycle through a Semaphore has no holder edge. The dump
  is not available on Windows (no SIGQUIT).

- Two phase shutdown in tests (2026-10-08): `run_main` runs main on the
  test's own goroutine, so a third signal cannot end it immediately as
  `os.Exit(130)` does; `run_main` returns 130 once main has unwound (a
  shielded `defer` that blocks keeps it waiting, on the virtual clock).
  Requests to an in memory server run on the client's task, so a stop
  (as opposed to cancellation) of the serving program does not cancel
  them; they only see its `task.stop_requested()`.

- HTTP request cost (2026-10-08, `bench/results/perf-arenas-2026-10-08.md`):
  task 01 under load allocates 13.1 KB and 97 objects per request (6.1 KB
  and 68 with `--arenas`) against Go's 3.0 KB and 31 (later, after other
  allocation cuts: 9.5 KB and 77 with `--no-arenas`, 5.5 KB and 49 with
  arenas, the default; decision 0066); throughput is 0.43x
  of Go (0.6x with arenas). Remaining sources: `JsonEnc` boxing fields
  through `any`, encoding/json token parsing of bodies, `Mutex.with`
  copying the value to the heap, the router splitting the path per route
  tried, re-parsing the request URL, and the per request Task.

- Generic code (2026-10-08): with the default lowering generic workloads
  run 0.49x to 1.08x of Go generics (`--no-mono`: 1.6x to 16.9x, `==`/`<`
  on a type parameter box both operands and constraint method calls
  convert to an interface). Polymorphic recursion is rejected by Go in
  both modes. An interface adapter for a concrete type that exists only
  inside generic code is not registered for the `--no-mono` lowering
  (`gotchas/generic-constraint-no-adapter.md`).
- Checked arithmetic (2026-10-08): the generic `rt.AddS` family
  (`--no-mono`, sized integer types such as Int32) still faults through an
  out of line call and does not inline; the range analysis has no
  relational facts beyond list lengths and does not follow values across
  calls (a function's result is unbounded at its callers).

- Regression tests (core 9.2): `xo test` passes `XO_REGRESSION_OUT` to the
  test binary; `std/rt/regress.go` records shrunk contract fuzz failures and
  `cmd/xo/regress.go` writes them to the module's `regressions_test.xo` after
  type checking a copy. Property test failures are not written.
- `Map.put` in update heavy loops is no longer a gap (2026-10-08): 3M
  puts over 50k keys plus 3M gets take 142 ms against Go's 45 ms (3.2x,
  was 48x); `Map.update` loops 4.4x. Numbers:
  `bench/results/perf-map-2026-10-08.md`, `perf-mono-2026-10-08.md`.
- List writes (2026-10-08, `bench/results/perf-list-2026-10-08.md`):
  element writes, `push`, `pop`, field updates of elements, `insert`,
  and `remove_at` are in place for the unique owner (sieve 1.4x of Go,
  nbody 1.6x, sort 1.0x, List workloads 1.0x to 1.5x). Remaining costs:
  the shared flag is sticky, so the first write after any copy (including
  `return xs` of a local) copies the list even if the other copy is dead;
  writes through a Map value, a box, `Map.update`, or `Mutex.with` copy
  the list every time (push through them stays O(1)); `--no-mono` marks
  type parameter values through `rt.Own`, which allocates; lists grow by
  doubling (sieve peak RSS 68 MB against Go's 48 MB).

## Go build cache (2026-10-08)

Per program growth (2026-10-09, after the fixes of
`gotchas/go-generic-method-sets-per-package.md`): hello world 2.9 MB,
examples/notes-service 6.9 MB, its test binary 8.4 MB (34, 54, and 68 MB
before); the runtime itself is about 13 MB per variant (with and without
`xo_nohttp`). Last verified: 2026-10-09.

Xo builds and bindings run Go with `GOCACHE` set to `$XO_GOCACHE` (default
`<user cache>/xo/gocache`, `gotool.CacheDir`), pruned as "Cache pruning"
below describes. See `gotchas/go-build-cache-growth.md`.

### Cache pruning (decision 0223)

One routine, `compiler/prune`, prunes xo's caches; internal/xotool
prunes its own `checkout/<key>` with the same stamp format. An entry's
time is its last use (refreshed on a hit when over an hour old); a
cache's last prune is the time of `DIR/prune-stamp`. A prune removes
entries unused for longer than the age limit, then the least recently
used until the cache is under its cap. Nothing used within the last two
hours is removed, so a cache can sit above its cap by that much.

Final outputs are staged (decision 0345, output paths; 2026-10-22): `xo
build -o OUT` and every other final write (the build directory's
`bin/NAME`, `NAME.dSYM`, `NAME.pdb`, `.wasm` and `.wat`, a copy out of
the binary cache, the Go backend's `go build`) is built in a hidden
`DIR/.NAME.tmp-TAG-RAND/` beside OUT under the final name (TAG random
per xo process) and renamed into place, debug information first. A
failure, or a SIGINT or SIGTERM during the build (xo exits 130), removes
the stage; `-o DIR` of an existing directory writes `DIR/NAME`. cmd/xo
TestBuildStagedOutputs (two concurrent builds of one output, a poll for
visible files, a SIGINT) and TestBuildOutPaths. Last verified:
2026-10-22.

`xo clean -cache` (2026-10-22, owner) empties them all at once, like
`go clean -cache`: every cache of `compiler/prune/clean.xo`'s
`caches()` table (build, backend, gocache, testbin, watch, dev,
lsp-std, checkout, testcc, testprog, testgobind, gobind, llvm-ir,
lifeline, xo1-test, llvm-sysroot), printing the bytes freed per cache; `-n` only
measures. It keeps each cache's `prune-*` and `*.lock` files, skips an
entry whose `build.lock` a live process holds (`Fs.try_lock`; a
checkout build in flight), and never touches the module cache (`mod`,
`xo mod clean` empties it), the seeds (`seeds`, `seed`), or anything
outside `<user cache>/xo` (a `$XO_BUILD_DIR` or `$XO_BIN_CACHE`
elsewhere, a project). A build running during a clean can fail with a
missing file (its build directory or a cached object removed under it);
run it again, and the next builds are cold. internal/repotest
TestCacheTable fails when the source names a new directory under
`<user cache>/xo` that the table lacks. Last verified: 2026-10-22.

| Cache | Entry | When | Age | Cap |
|---|---|---|---|---|
| `<user cache>/xo/build` (not a `$XO_BUILD_DIR`) | each `<hash>` directory; its `.s` files on their own | build_dir, at most hourly | `XO_CACHE_MAX_AGE`; a directory used within 30 minutes stays (not two hours: about 50 builds an hour held 31.7 GB under a 20 GB cap, 2026-10-21); its `.s` files go after 10 minutes unused whatever the directory's age (no build reads them after its link; they were 28 GB of 40); every file but those under a `bin/` goes after 2 hours unused (the part objects and split modules of 837 directories were 12 GB with nothing removed, 2026-10-24; the program in `bin/` is the user's and stays for the 24 hours / the cap) | `XO_BUILD_DIR_MAX` bytes (default 6 GiB, was 20), else `XO_CACHE_MAX_GB` |
| `$XO_GOCACHE` (`<user cache>/xo/gocache`) | each file below the top level | after a go build, at most hourly | `XO_CACHE_MAX_AGE`; a file used within 75 minutes stays (not two hours: Go refreshes a file's time once it is an hour old; 21.5 GB used within two hours held it over its 20 GB cap, 2026-10-21) | `XO_GOCACHE_MAX` bytes, else `XO_CACHE_MAX_GB` |
| `$XO_BIN_CACHE` (unset: `<user cache>/xo/testbin` for `xo test`, off for other builds, decision 0236; `off` turns it off; `run/` holds native `xo test`'s run cache, compiler/xo testcache.xo, and `xo build`'s and `xo run`'s entries in the same directory, on by default since 2026-10-23, `XO_WHOLE_BUILD=off` turns them off, decision 0360) | each file below the top level | after a store (a binary or a run entry), at most every 2 minutes | `XO_CACHE_MAX_AGE` | `XO_BIN_CACHE_MAX` bytes, else `XO_CACHE_MAX_GB`, else 4 GiB; entries used within 10 minutes stay (not two hours), marked used at most once a minute (last verified: 2026-10-15) |
| `<user cache>/xo/backend` (`XO_BACKEND_CACHE_DIR`; every top level directory, fn/, inl/, obj/, rc/, sum/, fe/, packs/, llobj/ and the rest, capped together, counted in allocated 4 KiB blocks since 2026-10-24: a 300 byte entry counts 4096) | each file below the top level (a pack segment is one entry), walked a slice of shards at a time (below) | after a store, every minute or once an eighth of the cap was stored since the last prune (`DIR/prune-stored`), in a detached `xo dev prune-backend DIR` | `XO_CACHE_MAX_AGE`, else a week | `XO_BACKEND_CACHE_MAX` bytes, else `XO_CACHE_MAX_GB`, else 16 GiB (8 until 2026-10-21: below the agents' working set, so prunes thrashed); entries used within 2 minutes stay (not two hours), marked used at most once a minute (last verified: 2026-10-16) |
| `<cache base>/checkout/<key>` (xotool) | each key directory | xotool Ensure, at most hourly | `XO_CACHE_MAX_AGE`, past the newest `XO_CHECKOUT_KEEP` (default 4); a key used within 30 minutes, or whose build.lock is held, stays (2026-10-21; was two hours) | `XO_CHECKOUT_MAX` bytes, default 8 GiB: the least recently used past the newest keys go (2026-10-21: 385 keys, 26 GB, none a day old, with no cap) |

- `XO_CACHE_MAX_AGE`: a Go duration (`6h`, `90m`), default `24h`; `0`
  turns age pruning off (and the checkout pruning).
- `XO_CACHE_MAX_GB`: GiB, a decimal, default `20`; `0` turns the size cap
  off for `build/` and `gocache/`.
- `XO_CACHE_PRUNE_BUDGET`: a Go duration, default `2s`; `0` for no
  limit. A build's prune of build/, gocache/, the bin cache and the
  back-end cache stops removing after it (the walk is not counted) and
  leaves the stamp due a minute later, so the next build goes on
  (2026-10-21: a build falling on an overdue prune of a busy day's cache
  spent minutes unlinking, and a timed test of one golden timed out;
  internal/arm64 TestPruneBudget).
- The back-end cache's prune (2026-10-21, xo-fa; last verified:
  2026-10-21): it walked the whole cache, 44 to 48 s for about 500,000
  files under load (96% of it lstat), a build that fell on it stalled
  (two dev goldens timed out at 240 s), and a killed build left a fresh
  stamp that blocked every prune for an hour (the cache reached 62.5 GB
  under its 16 GiB cap). Now a build that finds it due starts `xo dev
  prune-backend DIR` detached (`/bin/sh -c '... &'`, at most one a
  minute, `DIR/prune-spawned`; `XO_PRUNE_DETACH=off` or Windows: one
  slice in the build). Each run walks the shards (the directories two
  levels down: `fn/ab`, `packs/ab`; a top level directory's own files
  are one shard) from `DIR/prune-cursor` for 300 ms, keeps each
  shard's bytes in `DIR/prune-sizes`, applies the cap to their sum by
  evicting the least recently used entries of each walked shard down to
  its share of the cap, and appends a line to `DIR/prune-log` (walk
  time, bytes before and after, removed). The child runs slices until
  the cache is under the cap, for at most a minute; it walks every
  shard each run (60 s budget) and then evicts the least recently used
  entries across the whole cache, so it acts on current sizes (300 ms
  slices and per-shard shares left the cache at 49 GB, and a shard's
  share overflowed Int, 2026-10-21).
  An entry already gone (another prune removed it) counts as removed:
  counted as kept, concurrent runs each removed the next oldest in its
  place. `DIR/prune-version` (bcache.xo BACKEND_PRUNE_VERSION, 4) names
  the newest prune that managed the cache; an xo with an older version
  does not prune it (it still reads and writes entries): on 2026-10-21
  xos from that morning (8 GiB cap, no prune-log) beside today's took the
  cache from about 29 GB to 3.55 GB under its 16 GiB cap. A run holds
  `DIR/prune-lock`; one older than 10 minutes was left by a killed run
  and no longer blocks. A new cache directory gets its stamp, not a
  prune (a test's cache is never pruned while it is removed). Packs
  live under `packs/<xx>/<slot>/` (pack/ held 138,000 slots in one
  directory); the child renames the old `pack/` to `trash-pack-<ms>`
  and removes every `trash-*` directory (prune.retire, prune_trash),
  which the shards leave out: listing pack/'s slots took 4 to 5 s a run,
  more than the 300 ms walk budget, so a run walked one shard and the
  cache grew from 15 to 33 GB in 11 minutes (2026-10-21). The walk
  budget counts the shards' walk, not their listing. Tests: `xo test compiler/prune` (slices, convergence
  under the cap, the stale lock), internal/arm64 TestPruneBudget.
- `rtobj/` keeps its own 512 MiB size trim; `dev/`, `mbedtls/`, `seed/`,
  and `seeds/` are not pruned.
- Tests: `xo test compiler/prune` (TestSelfhostPruneXoTests, merge tier;
  MemFs and the fake clock), `go test -run Prune ./internal/xotool`.

Last verified: 2026-10-14.

Every go command the repo runs (tests, cmd/xo, scripts) uses Xo's cache
or has a listed reason not to (decision 0176, audit of 2026-10-12): tests
set `cmd.Env` from `build.GoEnv()` (GOWORK=off, GOFLAGS=-trimpath,
GOCACHE=Xo's) or `gotool.CacheEnv()`, and `TestGoCommandsUseXoCache`
(internal/build) fails on an `exec.Command("go", ...)` (or `goTool()`,
`gotool.Path()`) other than `go env` and `go version` without one; an
intended exception carries a `//xo:default-gocache` comment with the
reason. On the default cache by design: the repo's own `go test` and
`go vet` (bench/nightly.sh, bench/go/strict-check.sh, developers) and
the scripts that `go build` cmd/xo or one Go comparison program
(bench/perf, and before decision 0215 scripts/release.sh and bench/size):
they hold no Xo cache lock, so on Xo's cache a trim could delete an
entry they are linking. Go trims the default cache after 5 days unused.
The default cache held 20,461,608 KiB on 2026-10-12: 235 identical
24.8 MB `rt` archives from the gen tests' `go vet` before commit bc58195,
and since then mostly the repo's `std/rt` compiled once per worktree
path by `go test` (31 of the 46 files over 20 MB written after
2026-10-11 16:00). Last verified: 2026-10-12.

## Remote full runs (EC2)

`scripts/remote-full.sh [--dry-run] [--on-demand] [COMMIT]` runs the
full suite (`XO_SELFHOST_TIER=full`) of a commit on a temporary EC2
Graviton instance (default r8g.8xlarge, 32 vCPU, 256 GiB, on demand
only since 2026-10-13 (owner: spot instances were reclaimed mid-run three
times and spot capacity ran out; Last verified: 2026-10-13), us-east-1,
since 2026-10-12:
c8g.8xlarge's 64 GiB ran out in the full tier, where one Xo arm64 emitter
process on a self hosted component holds up to 13 GB,
gotchas/selfhost-heavy-runs-memory.md; XO_REMOTE_TYPE picks another;
the spot quota was raised from 32 to 128 vCPUs on 2026-10-12, so c8g.16xlarge, 64 vCPU, about $0.70 to $0.80 an hour spot, also fits; keep one instance per agent; a spot
r8g.8xlarge was reclaimed after 6 minutes on 2026-10-12, so use
--on-demand for a run that must finish), passes XO_SELFHOST_JOBS=$(nproc)
(or the caller's XO_SELFHOST_JOBS, and XO_SELFHOST_HEAVY_JOBS when set),
tests XO_REMOTE_PKGS (default every package but runs/) with
XO_REMOTE_TESTFLAGS (e.g. -v), appends the peak memory in use (free -m
every 10 s), copies
the log to bench/results/remote/<commit>.log (git ignored) and terminates
the instance. The instance also shuts itself down (terminating it) after
XO_REMOTE_MAX_MIN minutes (default 300). Setup made once, all named
xo-ci: an EC2 key pair (~/.ssh/xo-ci.pem) and a security group allowing
SSH from the owner host's IP only. Linux host: the Mac-only tests
(Mach-O, code signing, the darwin fixed point) do not run there and stay
on the development Mac. Approved by the owner on 2026-10-12 to move the
long combined run off the Mac. Last verified: 2026-10-12 (dry run).

Options (2026-10-12): `--cmd '<command>'` runs one shell command in the
checkout instead of the full suite (for a targeted reproduction, e.g.
`--cmd 'go test -count=1 -run TestGoldenThreads ./internal/llvm'`);
`--hold` clones the commit to ~/xo, writes `ec2-user@<ip>` to
$XO_REMOTE_HOLD (default bench/results/remote/hold.addr) and waits until
the watchdog or until the script is stopped, which terminates the
instance (debug over `ssh -i ~/.ssh/xo-ci.pem`, copy a working tree with
rsync). Every log now ends with the per-package durations, slowest
first. `--arch amd64` launches an x86_64 instance instead (default
c7i.8xlarge, the AL2023 x86_64 image, Go for linux-amd64); `--put FILE`
(repeatable) copies a local file to the instance's home directory
before `--cmd` runs (a release archive for scripts/release-verify.sh).
`--cmd` runs with /usr/local/go/bin on PATH; a step that must not see Go
drops it itself. Last verified: 2026-10-12 (release verification runs). The AMI comes from the SSM alias, else (the agent sandbox cannot
resolve ssm.us-east-1.amazonaws.com) from `ec2 describe-images`, the
newest `al2023-ami-2023.*-kernel-6.18-arm64`. The image has gcc as `cc`
(the arm64 backend's assembler and linker driver, CC()), clang 15.0.7,
and /tmp on a tmpfs apart from the home disk. Last verified: 2026-10-12
(--hold run on c8g.8xlarge).

## Intel Mac runs (darwin/amd64, decision 0196)

`scripts/intel-mac-test.sh [--clean|--purge] [COMMIT] [-- CMD...]` runs
CMD (default `go test -count=1 -run 'TestGolden$' ./internal/llvm`) for
a commit on the owner's Intel Mac, host `$XO_INTEL_HOST` (default
`user@intel-mac.local`, ssh key auth from the development Mac):
it bundles the commit (a temporary `refs/xo-intel/<commit>` ref), copies
the bundle with scp, fetches it into `~/xo-ci/xo`, and runs CMD under
`caffeinate -i` with go 1.26.1 from `~/xo-ci/go` (unpacked from go.dev
on first use), GOMAXPROCS=6, XO_SELF_DRIVER=0, and HOME, TMPDIR,
GOCACHE, GOPATH under `~/xo-ci/state` (so os.UserCacheDir and every xo
cache stay in ~/xo-ci). The log goes to
`$XO_INTEL_OUT` (default bench/results/remote/intel-<commit>.log, git
ignored) and the script exits with CMD's status. It refuses to start on
battery below `$XO_INTEL_MIN_BATT` percent (default 50). `--clean`
removes the checkout, state, and logs afterwards (keeps ~/xo-ci/go);
`--purge` removes ~/xo-ci. CMD runs in the ssh session's foreground, so
nothing outlives the script. The laptop is the owner's: no sudo, no
installs outside ~/xo-ci, keep it under 12 GB (the full internal/llvm
run left 2.9 GB in ~/xo-ci). Host: x86_64 i7-9750H, 12 threads, 16 GB,
macOS 26.5.2, Apple clang 17.0.0 (clang-1700.3.19.1), about 20 GB free.
The whole internal/llvm package passes there in 532 s
(bench/results/intel-mac-2026-10-12.md); ASan and TSan binaries cannot
start (gotchas/darwin-amd64-sanitizers-sigill.md). The development Mac
cannot run x86_64 binaries (no Rosetta) and the LLVM backend builds for
the host only, so darwin/amd64 is built and tested on that Mac.
Last verified: 2026-10-12.

## Windows runs (windows/amd64, decisions 0196 and 0197)

`scripts/remote-windows.sh [--dry-run] [--hold] [--cmd CMD] [COMMIT]`
launches an on demand Windows Server 2025 EC2 instance (default
c7i.4xlarge, 16 vCPU, 32 GiB, owner approved about $1.40 an hour;
`XO_REMOTE_TYPE`), us-east-1, key pair xo-ci, security group xo-ci-ssh
(SSH from this host's IP only, no RDP). EC2Launch v2 user data, as
SYSTEM: first the lifetime (`shutdown /s /f /t` and a scheduled task at
`XO_REMOTE_MAX_MIN` minutes, default 240; instance-initiated shutdown
terminates), then OpenSSH Server with the xo-ci public key in
`C:\ProgramData\ssh\administrators_authorized_keys` and Git Bash as the
default shell, then Git for Windows (latest release), Go 1.26.1 (msi),
LLVM (latest release msi: 23.1.3 on 2026-10-12, clang, lld-link,
llvm-ar), and the VS 2022 Build Tools (VCTools workload, x64 and ARM64
compilers, the recommended Windows SDK); `C:\xo-ready` marks the end
(about 11 minutes; the LLVM installer is an .msi since LLVM 23, an .exe
before). The script starts a local watchdog (PID printed) that
terminates the instance after the same limit and terminates it on exit.
It sends the commit as a git bundle and clones it to `~/xo` with
`core.autocrlf false` (gotchas/windows-git-crlf-goldens.md). `--hold`
writes `Administrator@<ip>` to `$XO_REMOTE_HOLD` and waits; `--cmd`
runs one command (Git Bash, in the checkout) and writes the log to
bench/results/remote/windows-<commit>.log (git ignored).

On the instance, xo1 (the shipped xo) first: the Go seed builds it with
the Go backend, `go build -o xo.exe ./cmd/xo && XO_SELF_DRIVER=0
./xo.exe build -o ../xo1.exe ./compiler/xo` (48 s), and it runs with
`XO_STD=C:/Users/Administrator/xo/std` (no release layout there) and
the `PWD` Git Bash passes (`C:/...`); `../xo1.exe build --backend=llvm
DIR` builds a Windows program, `--targets=windows/arm64` cross builds.
The golden count with xo1 building every program is
`XO_STD=... XO_GOLDEN_WANT=1 XO_GOLDEN_WANT_XO=C:/Users/Administrator/xo1.exe
go test -count=1 -run TestGoldenWant -v ./internal/llvm` (last
verified: 2026-10-12, i-EXAMPLE22); without
`XO_GOLDEN_WANT_XO` the Go driver builds in process
(`XO_SELF_DRIVER=0 ../xo.exe build --backend=llvm DIR` by hand). The
batch of Windows checks (decision 0197 stage 6) is
`scripts/remote-windows.sh --batch [COMMIT]` (a fresh instance, the
log's `batch:` lines summarize) or `bash scripts/windows-batch.sh` in a
held instance's checkout (4 min 13 s warm): xo1 built by the Go seed,
xo1's goldens for windows/amd64 (run) and windows/arm64 (built), the
LLVM runtime tests (threads, tasks, event loop, Ctrl-Break shutdown,
TLS and HTTP/2 diffs), and `xo1 test` LLVM against Go for the 16
directories of TestNoGCTests; `XO_BATCH_STEPS="2 4"` runs some steps.
Last verified: 2026-10-12 (i-EXAMPLE22). Runtime knobs there:
`XO_WINPOLL=wsapoll` (the WSAPoll poller instead of AFD polls). The
plain golden count is
`XO_GOLDEN_WANT=1 go test -count=1 -run TestGoldenWant -v ./internal/llvm`
(the summary line counts pass, fail, unsupported; with
`XO_GOLDEN_WANT_TARGET=windows/arm64` it cross builds instead and counts
the programs that build and link). clang targets
x86_64-pc-windows-msvc by default and finds the build tools itself; the
link is `-fuse-ld=lld` (LTO bitcode). Results:
bench/results/windows-2026-10-12.md. Last verified: 2026-10-12
(instance i-EXAMPLE19).

xo1 for Windows built by Xo (decision 0197 stage 7). Cross, from Linux
or macOS: an upstream LLVM (clang and lld-link; Apple's clang has no
lld-link; 23.1.3 measured) and a Windows sysroot,
`xwin --accept-license --arch x86_64,aarch64 splat --use-winsysroot-style
--preserve-ms-arch-notation --output DIR` (xwin 0.10.0, 19 s, 1.1 GB;
without the last flag lld-link finds no .lib,
gotchas/windows-cross-sysroot.md), then
`XO_CLANG=<llvm>/bin/clang XO_AR=<llvm>/bin/llvm-ar XO_WINSYSROOT=DIR
XO_LLVM_SPLIT=16 XO_LLVM_JOBS=16 XO_LLVM_LTO=thin xo build --backend=llvm
--targets=windows/amd64 -o xo.exe ./compiler/xo` (1,362 s on 32 vCPU and
64 GiB, 29.5 GB peak in use; the link alone about 22 GB). From d9f15415 until
the fix of 2026-10-19 this split build did not link
(gotchas/windows-llvm-split-link.md). On the arm64 Mac with
`XO_LLVM_OPT=O2 XO_LLVM_JOBS=4` and the split 16 ThinLTO recipe: 294 s
and 6.0 GB peak RSS, a 160 MB xo.exe that runs under Wine (one module
at O1 before the fix: 2,972 s, 5.2 GB). Last verified: 2026-10-19. Natively on
Windows: the same without XO_WINSYSROOT, XO_CLANG, XO_AR (1,544 s,
35 GB peak in use on c7i.8xlarge; use `XO_REMOTE_TYPE=c7i.8xlarge`, 32
GiB is too little for one module with full LTO and tight for this).
XO_LLVM_LTO=none is slower here (1,571 s cross: each -O2 module takes
764 to 1,426 s). Results: bench/results/windows-2026-10-12.md, "xo1
built by Xo for Windows". Last verified: 2026-10-13 (i-EXAMPLE07,
i-EXAMPLE03).


### Windows cross builds on the Mac

Set up 2026-10-15 (xo-fa; the owner accepted Microsoft's SDK license
for this Mac, for xwin). Homebrew could not install llvm, lld, or xwin
(it does not know macOS 27: "unknown or unsupported macOS version"), so
everything is in `~/xo-tools/winxc`, outside every cache directory (owner, 2026-10-28: the copy in `~/Library/Caches/xo/winxc` disappeared and was rebuilt here with the recipe below; Last verified: 2026-10-28, clang 23.1.3, xwin 0.10.0) (9.3 GB:
the LLVM release 7.6 GB unpacked, the sysroot 1.1 GB, a minimal Rust
toolchain and xwin the rest; rebuilt 2026-10-16 on the main drive with
the recipe below in 134 s, 9.3 GB after removing the 832 MB LLVM
tarball), no sudo, no shell profile change:

    X=~/xo-tools/winxc
    gh release download llvmorg-23.1.3 -R llvm/llvm-project -p 'LLVM-23.1.3-macOS-ARM64.tar.zst' -D $X
    zstd --long=30 -dc $X/LLVM-23.1.3-macOS-ARM64.tar.zst > $X/llvm.tar && tar -xf $X/llvm.tar -C $X
    rm $X/llvm.tar $X/LLVM-23.1.3-macOS-ARM64.tar.zst
    # (zstd without --long=30 refuses the 1 GiB window)
    curl -sSfL -o $X/rustup-init https://static.rust-lang.org/rustup/dist/aarch64-apple-darwin/rustup-init
    CARGO_HOME=$X/cargo RUSTUP_HOME=$X/rustup $X/rustup-init -y --no-modify-path --profile minimal
    CARGO_HOME=$X/cargo RUSTUP_HOME=$X/rustup $X/cargo/bin/cargo install xwin --version 0.10.0 --locked --root $X/xwin
    $X/xwin/bin/xwin --accept-license --arch x86_64,aarch64 --cache-dir $X/xwin-cache splat \
      --use-winsysroot-style --preserve-ms-arch-notation --output $X/winsysroot   # 58 s
    rm -rf $X/xwin-cache   # 1.2 GB of downloads

(xwin publishes no macOS binary, hence the build; the case insensitive
volume makes it splat without symlinks.) A cross build:

    XO_CLANG=$X/LLVM-23.1.3-macOS-ARM64/bin/clang XO_AR=$X/LLVM-23.1.3-macOS-ARM64/bin/llvm-ar \
    XO_WINSYSROOT=$X/winsysroot xo build --backend=llvm --target=windows/amd64 -o prog.exe .

(windows/arm64 the same). It links a PE32+ executable; nothing runs it
on the Mac. Last verified: 2026-10-15 (clang 23.1.3, xwin 0.10.0).

### Linux in Docker on the Mac

Since 2026-10-15 (xo-fa, the owner's request): Docker Desktop on this
Mac runs Debian containers natively as linux/arm64 and, emulated, as
linux/amd64 (slow: about 3x for a build).

Docker runs go outside the heavy lock (lead decision, 2026-10-15, after
long Docker runs held both heavy slots): docker-linux.sh, docker-wine.sh
and valgrind runs in a container are started directly, with `nice -n 10`
so they yield to the Mac's own work, not through scripts/lead/heavy.sh
(installed in ~/xo-lead/agents). Agents remove only containers
they started, by ID (`docker rm ID`), never by name or pattern. Last
verified: 2026-10-16.

Linux LLVM smoke test (2026-10-21, for hourly.sh):
`scripts/docker-llvm-smoke.sh XO_CLI` cross builds a linux/arm64 xo with
the darwin XO_CLI, then in the arm64 container builds hello and the
async_preempt_regs golden with --backend=llvm, linked by ld.bfd (with
the LLVM plugin, the image's default) and by ld.lld, and checks their
output against want.txt: exit 0 ok, 1 failed, 2 setup error, 0 with a
note when Docker is not running; about 4 minutes. Debian trixie has no
ld.gold. The darwin gates see neither Linux's linkers nor the no-libc
start (gotchas/llvm-linux-section-bounds-and-start.md). Last verified:
2026-10-21.

RSS guard for the fiber stack fix (2026-10-22, for hourly.sh):
`scripts/docker-rss-guard.sh XO_CLI [BOUND_MB]` cross builds the CRUD
server (bench/perf/http/xo) and its Go load generator for linux/arm64,
runs the server in the arm64 container (transparent huge pages "always"
there) under 1024 keep-alive connections for 8 s, and fails when its peak
RSS (VmHWM) is over BOUND_MB (default 150): exit 0 ok, 1 over, 2 setup
error, 0 with a note when Docker is not running. About 35 s. Measured: 60
MB with the fix, 2129 MB with the madvise in map_stack disabled
(gotchas/fiber-stacks-huge-pages.md). Last verified: 2026-10-22.

Three scripts:

    scripts/docker-linux.sh ARCH WORKDIR 'COMMAND'   # in xo-linux-test:ARCH-BASE (Debian with clang, lld, make, yara; $XO_DOCKER_BASE, default debian:trixie-slim, clang 19); the checkout at /xo read only, WORKDIR at /work, HOME=/work/home; --rm
    scripts/linux-goldens.sh XO BACKEND OUTDIR [NAME...]   # build each golden with XO and BACKEND and compare with want.txt (exit status and stdout); XO_GOLDEN_JOBS=N runs N at once (default 1; 2026-10-30), want_windows.txt under Git Bash
    scripts/docker-goldens.sh DARWIN_XO ARCH BACKEND WORK [NAME|FILE.txt...]   # a Linux xo of the checkout cross built here, then linux-goldens.sh in Docker (amd64 with no C toolchain); 2026-10-18. A .txt file (scripts/docker-amd64-sample.txt) gives names; # words and unknown names are skipped with a note, and no name left stops the run instead of running all 201 (2026-10-22)
    scripts/arm64-identity.sh BASE_XO NEW_XO [BACKEND]    # the arm64 (or amd64) output of every golden and compiler/xo, darwin and linux, byte compared; for changes meant for another target; 2026-10-18
    scripts/arm64-identity.sh --own-std BASE_XO NEW_XO [BACKEND]    # for a change to std and the compiler together: each xo on its own checkout's std, source positions and label-only lines normalized (gotchas/identity-std-and-compiler.md); 2026-10-20
    scripts/macho-link-compare.sh XO WORK                 # every golden for darwin/amd64 linked by Xo's linker and by Apple's ld, compared layout independently; 2026-10-18
    scripts/intel-goldens.sh XO HOST REMOTE_DIR WORK [BACKEND]   # every golden built for darwin/amd64 here, run on an Intel Mac over ssh in a NEW REMOTE_DIR (nothing deleted there; ask the owner first); 2026-10-18
    scripts/golden-run.sh BINARY GOLDEN_DIR                # one golden, on a writable copy of its directory

A Linux xo for the container: cross build it on the Mac with the Go
backend (the 8 GB Docker VM kills a self build inside,
gotchas/docker-vm-oom-self-build.md), with std/ next to bin/:

    D=WORKDIR/xomain; mkdir -p $D/bin && cp -R std $D/std
    bin/xo build --backend=go --targets=linux/arm64 --std=std -o $D/bin/xo compiler/xo
    scripts/docker-linux.sh arm64 WORKDIR 'export XO_ZONEINFO=/work/zoneinfo.zip; /xo/scripts/linux-goldens.sh /work/xomain/bin/xo arm64 /work/g-arm64'

(zoneinfo.zip: `cp "$(go env GOROOT)/lib/time/zoneinfo.zip" WORKDIR/`;
tz_table needs it with no Go in the container.) XO_NOLIBC goldens: prefix
the command with XO_NOLIBC=1 and pass the names, for example
`XO_NOLIBC=1 /xo/scripts/linux-goldens.sh /work/xomain/bin/xo arm64
/work/nl conc_scopes fault_unwind fault_tasks env_program json_text`
(llvm on amd64). Seeds unpack into WORKDIR and run as they are
(`/work/seed/xo/bin/xo version`).

Results on 2026-10-15 (main plus the stubs, the Go-built xo):

| Run | Result |
|---|---|
| seed 0.14.0 linux/arm64 and linux/amd64: version, hello with the default backend and llvm | pass |
| linux/arm64, arm64 backend, all 178 goldens | 178 pass (as the host's uid; as root fs_ops_edges' read only check fails) |
| linux/arm64, llvm backend, all 178 goldens | 178 pass since the thread local fix (gotchas/llvm-thread-local-constant.md); 157 before, 19 crashing |
| linux/amd64 (emulated), llvm backend, 15 goldens (faults, tasks, conc, net, proc, tls, http) | pass |
| XO_NOLIBC hello and five goldens, linux/arm64 (arm64) and linux/amd64 (llvm), built in the container | pass |

The Go tests in the container (the sanitizers, 2026-10-16): with Go's
image and the checkout mounted read write, from the checkout:

    XO_DOCKER_RW=1 XO_DOCKER_BASE=golang:1.26-trixie scripts/docker-linux.sh arm64 WORKDIR \
      'export PATH=/usr/local/go/bin:$PATH GOFLAGS=-buildvcs=false XO_CLI=/work/xomain/bin/xo GOCACHE=/work/home/gocache GOMODCACHE=/work/home/gomod; cd /xo && go test -run "TestSysCTSan|TestGoldenTasks|TestGoldenASan|TestRuntime|TestDeadlock" ./internal/llvm/'

TestSysCTSan, TestDeadlock, TestRuntime* (RawPoll after its unistd.h),
TestGoldenTasks (ThreadSanitizer) and TestGoldenASan pass on linux/arm64
but fs_ops_edges: the checkout is a macOS directory shared into the VM,
which keeps whole seconds, no read only mode, and no locks between
descriptors (scripts/linux-goldens.sh copies each golden into the
container's own file system, where it passes). Under load (every
sanitizer build at once) net_only and http_fault timed out once and pass
alone. Last verified: 2026-10-16.

Before the run, 27 arm64-backend builds failed to assemble with
bookworm's clang 14 (gotchas/clang14-arch-extension-crypto.md, fixed),
and bookworm's clang 14 cannot read the LLVM backend's IR (opaque
pointers need LLVM 15 or later). Last verified: 2026-10-15.

### windows/amd64 with the native amd64 backend (decision 0254 6b)

Since 2026-10-19 (step 1): `xo build --backend=amd64 --targets=windows/amd64`
emits COFF-flavored x86_64 assembly (unwind data from `.seh` directives;
Win64 only in thunks at the OS boundary, compiler/arm64 x86win.xo) and
links it with clang's assembler and lld-link (compiler/xo link.xo
win_amd64_link), from a Mac through winxc:

    X=~/xo-tools/winxc
    XO_CLANG=$X/LLVM-23.1.3-macOS-ARM64/bin/clang XO_WINSYSROOT=$X/winsysroot \
      xo build --backend=amd64 --targets=windows/amd64 -o prog.exe .

Under Wine: `XO_WINE_BACKEND=amd64 scripts/wine-goldens.sh XO WORK
[NAME...]` (the per-step subset: `$(grep -v '^#'
scripts/wine-goldens-subset.txt)`). All goldens on 2026-10-19: 190 of
201 (LLVM: 190 of 199), the 7 known Wine gaps and three Box64 flakes
that pass on a rerun. internal/arm64 TestWindowsAmd64Identity compares
the windows/amd64 and linux/amd64 code of the goldens' own functions
(the subset; XO_TEST_WIN_IDENTITY=all every golden). Since step 3 the
objects are Xo's (compiler/macho coff.xo; `--as=cc` assembles with
clang); `macho asm --windows-amd64 -o OUT.obj IN.s` runs the COFF writer
alone, and `scripts/coff-compare.sh XO MACHO LLVM_MC WORK [NAME...]`
compares its objects with llvm-mc's on the goldens (199 of 199 the same
on 2026-10-19). Last verified: 2026-10-19.

A windows/amd64 program of the native backend with no `use c` links by
default (decision 0254 6b step 4, owner 2026-10-19) with Xo's PE linker
(compiler/macho pelink.xo; `macho pelink -o OUT.exe --entry=SYM
[--import=SYM=DLL...] [--manifest] [--version=A.B.C.D] [--icon=F.ico]
IN.obj...`) and no C runtime (std/native/llrt/cabi_wincrt.xo: Xo's
memcpy, memmove, memset, memcmp, fmod, strlen, getpid and the process
entry xo_win_start): no clang, no lld-link, no Windows SDK.
`XO_WIN_NOCRT=0` links with the MSVC C runtime and lld-link instead.
Every such program gets a manifest (UTF-8 code page, long paths) and
version information (`--stamp VERSION=1.2.3` sets the version), as
.rsrc (compiler/macho peres.xo). Imports by
compiler/macho/peimports.xo, generated with `python3 -I
scripts/win-import-table.py SYSROOT > compiler/macho/peimports.xo` from
an SDK's import libraries when std/native/sys_windows.xo gains a
function. Wine under Box64 on 2026-10-19: hello 460 KB (1.4 MB with the
C runtime); every golden 194 of 202 (the seven known Wine gaps and
sys_c, since fixed). Last verified: 2026-10-19. A program with DWARF (not
--release or --strip) also gets PROG.pdb next to PROG.exe, named by the
image's CodeView debug directory (decision 0344, compiler/macho pdb.xo):
WinDbg, cdb, Visual Studio and llvm-symbolizer read its function names.
A new sys_windows.xo function is usable only once the seed's
peimports.xo has it (gotchas/seed-older-than-intrinsics.md). Check a PDB
in Docker: `llvm-pdbutil dump --all PROG.pdb` and
`llvm-pdbutil dump --globals --global-name=NAME PROG.pdb` (the
xo-linux-test image). Last verified: 2026-10-22.

### Windows programs under Wine in Docker

Since 2026-10-16 (xo-4): `scripts/docker-wine.sh MODE WORK COMMAND`
runs a command with Debian's Wine (10.0, trixie) in a Linux container,
like scripts/docker-linux.sh (WORK at /work, the checkout read only at
/xo, `--rm`, WINEPREFIX /work/wine-MODE). Use MODE box64: Debian's
amd64 Wine run by Box64 (0.3.4, Debian's) in a native linux/arm64
container, which runs structured exceptions; MODE amd64 (the emulated
amd64 container) runs ordinary programs but not exceptions (Rosetta).
A windows/amd64 program cross linked as above:

    printf '#!/bin/sh\nexec wine /work/win/P.exe "$@"\n' > $WORK/w/P; chmod +x $WORK/w/P
    scripts/docker-wine.sh box64 $WORK 'wineboot -u >/dev/null 2>&1; sh /xo/scripts/golden-run.sh /work/w/P /xo/internal/llvm/testdata/golden/P'

(golden-run.sh compares want.txt; use a copy with want_windows.txt in
its place where a golden has one). Every golden at once: `scripts/wine-goldens.sh XO
WORK [NAME...]` (2026-10-18, xo-fa) cross builds them on the Mac with
the winxc toolchain and runs them under box64, each in a fresh
wineserver, Box64's own lines dropped from standard output; use a new
WORK per run (a new prefix; gotchas/wine-box64-harness.md). About 75
minutes for the 192. Results 2026-10-16: 11 of the 17
http*, net*, fs*, proc*, stdin* goldens pass (every http_*, net_echo,
fs_cwd_path, fs_native); fs_paths_os needs the \\localhost\C$ share
Wine lacks, fs_files and fs_ops_edges symbolic links, proc_cmd and
net_only's proc line sh, echo and tr (no Xo bug among them; net_only's
env lines pass once the prefix exists: warm a new WINEPREFIX with
`wineboot -u` before measuring, as Wine's first run sets PATH up while
the program runs, Xo lang 5 2026-10-16). The Windows only goldens are in
internal/llvm/testdata/windows (want_windows.txt; stack_overflow: Windows'
stack overflow handler, Xo since decision 0234: passes under box64).
The amd64 results above are MODE amd64's. Limits: Rosetta stops Wine's
exception dispatch ("rosetta error: invalid gdt selector index 5"), so
use box64 for any structured exception; Debian's arm64 Wine runs no
ARM64 PE (a hello hangs), so windows/arm64 is cross linked only
(gotchas/wine-rosetta-no-exceptions.md). Last verified: 2026-10-16.

The box64 image is pinned (owner, 2026-10-22): debian:trixie-slim by
digest (sha256:62bd423a...), box64 0.3.4+dfsg-1, wine64 and libwine
10.0~repack-6, every package installed from Debian's snapshot archive
at 20261022T000000Z (http://snapshot.debian.org, Check-Valid-Until: no;
owner, 2026-10-22: so the pinned versions stay installable after the
mirrors replace them; no image is saved anywhere), tagged
`xo-wine-test:box64-trixie-62bd423-box64-0.3.4_dfsg-1-wine-10.0_repack-6-snap20261022T000000Z-prefix1`,
and it carries a prefix booted and checked (`wine cmd /c echo`) when
the image is built, which each run copies to WORK/wine-box64 when WORK
has none, so no run boots its own (Box64 stopped a run's first wineboot
part way under load: gotchas/wine-box64-harness.md). A run: `scripts/docker-wine.sh box64 WORK
'wine prog.exe; echo "exit $?"'` (the command one string: the script
runs it with `sh -c`), a new WORK or `rm -rf WORK/wine-box64` per run;
2 s for hello once the image exists (about 30 s to build it).
XO_DOCKER_BASE builds an unpinned image under its own tag. To move the
pins, change docker-wine.sh's pin_ lines (the snapshot time with them;
the tag follows) and run the goldens. Last verified: 2026-10-22 (hello, conc_mutex with
XO_THREADS=1, wine-goldens.sh conc_mutex conc_channels net_echo with
XO_WINE_BACKEND=amd64: 3 of 3).

### darwin/amd64 with Xo's amd64 backend

Since 2026-10-18 (xo-fa, decision 0254 stage 6a): `xo build
--backend=amd64 --target=darwin/amd64 -o prog .` on a Mac writes the
x86_64 text, assembles it with Xo's Mach-O x86_64 assembler and links it
with Xo's Mach-O linker, no C toolchain (`--as=cc`, or a `use c` program:
Apple's cc with `-arch x86_64`, which needs no Rosetta).
`scripts/macho-link-compare.sh XO WORK` compares the two links of every
golden (macho-symview.py). This arm64 Mac has no Rosetta ("Bad CPU type in
executable"), so the programs run only on the owner's Intel MacBook:
build them here, copy them to a new directory under ~/xo-ci there and
run them with scripts/golden-run.sh (macOS has no `timeout`: a perl
alarm shim on PATH gives it one), with the owner's OK for the run and
their rules (new files only, nothing deleted, no --clean). Last
verified: 2026-10-18 (193 of 193 goldens).

### XO_NOLIBC cross links on the Mac

Since 2026-10-15 (xo-fa, the backend entry stubs) an XO_NOLIBC=1 program
cross links on the Mac for linux/amd64 through the LLVM backend (the
x86_64 cross setup below; the sysroot's headers are only for xo_rt.c,
nothing of glibc is linked):

    XO_NOLIBC=1 XO_CLANG=$X/LLVM-23.1.3-macOS-ARM64/bin/clang XO_LINUXSYSROOT=$R \
      xo build --backend=llvm --target=linux/amd64 -o prog .

linux/arm64 through the arm64 backend writes only the assembly on the Mac
(a cross build with cc links nothing); link it by hand with arm64
headers (Debian's libc6-dev and linux-libc-dev for arm64, unpacked as
below into $RA) and the outline atomics file a Linux arm64 build writes
(xo_atomics.s, compiler/xo link.xo nolibc_atomics):

    XO_NOLIBC=1 xo build --backend=arm64 --target=linux/arm64 .   # prints the .s path
    $X/LLVM-23.1.3-macOS-ARM64/bin/clang --target=aarch64-linux-gnu --sysroot=$RA -fuse-ld=lld \
      -O2 -DXO_NOLIBC -ffunction-sections -fdata-sections -fno-stack-protector -U_FORTIFY_SOURCE \
      -mno-outline-atomics -static -nostdlib -Wl,--gc-sections -Wl,--eh-frame-hdr \
      -Wl,--defsym=xo_personality=xo_x_personality -o prog prog-linux.s std/llrt/xo_rt.c xo_atomics.s

The C process entry XO_NOLIBC_C_START=1 selected was deleted on
2026-10-15 once the stubs ran in Docker (below). Nothing runs either
binary on the Mac itself; scripts/docker-linux.sh runs them. Last verified: 2026-10-15.

### x86_64 cross builds on the Mac

Since 2026-10-15 (xo-fa, decision 0221 stage 2) the LLVM backend also
cross builds from darwin/arm64 to darwin/amd64 (the macOS SDK carries
both architectures; nothing else to set) and from macOS to linux/amd64
with a glibc sysroot in XO_LINUXSYSROOT and a clang that has lld (the
winxc LLVM above):

    R=<scratch>/lxsys/root
    for u in g/glibc/libc6-dev_2.31-13+deb11u11_amd64.deb g/glibc/libc6_2.31-13+deb11u11_amd64.deb \
             g/gcc-10/libgcc-10-dev_10.2.1-6_amd64.deb g/gcc-10/libgcc-s1_10.2.1-6_amd64.deb \
             l/linux/linux-libc-dev_5.10.223-1_amd64.deb; do
      curl -sfLO https://deb.debian.org/debian/pool/main/$u; done
    for d in *.deb; do mkdir -p x/$d && (cd x/$d && ar x ../../$d && tar -xf data.tar.* -C $R); done
    # Debian's links are absolute: make them relative (gotchas/debian-sysroot-absolute-links.md)
    ln -sfn ../lib/x86_64-linux-gnu/ld-2.31.so $R/lib64/ld-linux-x86-64.so.2
    find $R/usr/lib/x86_64-linux-gnu -maxdepth 1 -type l -lname '/lib/*' | while read l; do ln -sfn ../../..$(readlink $l) $l; done
    XO_CLANG=$X/LLVM-23.1.3-macOS-ARM64/bin/clang XO_LINUXSYSROOT=$R \
      xo build --backend=llvm --target=linux/amd64 -o prog .
    xo build --backend=llvm --target=darwin/amd64 -o prog .

Both link (an x86-64 PIE ELF against glibc 2.31; an x86_64 Mach-O);
neither runs on this Mac (no Rosetta). Last verified: 2026-10-15.

### xo1 on x86-64 hosts

    scripts/xo1-host-check.sh [OUTDIR]          # on the host, Go on PATH
    scripts/remote-xo1-x86.sh [--dry-run] [COMMIT]   # the same on EC2 linux/amd64
    scripts/intel-mac-test.sh --clean COMMIT -- 'XO_LLVM_JOBS=4 scripts/xo1-host-check.sh $HOME/out; cat $HOME/out/summary.txt'
    # (owner, 2026-10-17 and 2026-10-18: no --clean on the laptop any more, nothing deleted there)

xo1-host-check.sh builds xo1 as a checkout does (`go run
./scripts/xobuild --from-source` with a fresh cache, since 2026-10-13 opted in to the go-compiler-final root: arm64 backend on arm64, LLVM
split into 12 modules without LTO on x86-64; before decision 0215, as
switch point A's Go driver did), runs scripts/selfhost-goldens.py with
it (`XO1_BACKEND=llvm`),
runs bootstrap-xo.sh with it as the seed, and the want.txt goldens with
s3 and no Go; each step's wall clock and peak memory go to
OUTDIR/summary.txt. remote-xo1-x86.sh launches one on demand
m7i.4xlarge (Amazon Linux 2023 x86_64, clang 15.0.7; 240 minute self
poweroff, local watchdog, exit trap), runs it, and fetches the logs to
bench/results/remote/xo1-x86-<commit>.log and its directory (git
ignored). Result (8ac5652): xo1 build 2,037 s, goldens 169 of 169,
bootstrap s1 1,957 s, s2 1,923 s, s3 1,923 s, s1 = s2 = s3, s3 goldens
144 of 144 without Go. The Intel Mac run needs about 12 GB in ~/xo-ci
while the bootstrap's three build directories exist (more than the 12
GB guidance above; `--clean` removes it). Last verified: 2026-10-12.

## Bootstrapping without Go (decision 0178 stage 7, decision 0215)

Gates and the fixed point (owner, 2026-10-21): a push builds xo once
with the current xo and runs no fixed point; the full fixed point runs
hourly on main (`~/xo-lead/agents/hourly.sh`, which also runs -short: skips a main whose
compiler inputs did not change, lists the commits to bisect on a
failure) and in releases, seed-cut.sh and CI. `XO_BOOTSTRAP_SEED_IS_S1=1
scripts/bootstrap-xo.sh SEED` uses SEED as s1 when SEED is this tree's
compiler/xo just built by `go run ./scripts/xobuild` (hourly.sh and
gate.sh's fp step set it), so the fixed point builds xo twice (s2, s3).
Measured on a gate on 2026-10-21 under load: xobuild 112 s, s1 62 s,
s2 155 s, s3 10 s (s3 reused s2's backend cache entries). Of about 423
fixed points to that day, 16 failed, none an s2/s3 difference. Since
2026-10-27 every stage (and every seed chain hop) has its own back-end
cache under OUTDIR (`XO_BACKEND_CACHE_DIR=OUTDIR/cache-sN/backend`), as
it already had its own build directory and runtime object cache: the
stages shared the user's back-end cache, with each other and with every
xo on the machine, so s3 could take s2's objects instead of compiling
them, and an intermittent s2 != s3 seen under load that day could not be
told from a cache entry another xo wrote (gotchas/fixed-point-shared-cache.md).
A stage now compiles everything itself: s3 takes about as long as s2,
and OUTDIR holds the stages' caches too (delete it after a run). Last
verified: 2026-10-27.

The bootstrap chain: the first Xo `xo` on a new host is built by the
previous release's binary (or any self hosted xo1), never by the Go
compiler again: it was deleted (decision 0215). With no xo at all, the
seed is the xo1 of the `go-compiler-final` tag (87873aa5), then the hops of scripts/seed-chain.txt (decision 0216): Go builds
that commit's cmd/xo, which builds that commit's compiler/xo with its
own driver.

Bisecting a failure over commits (2026-10-22):

    HEAVY_CLASS=bulk ~/xo-lead/agents/heavy.sh NAME scripts/bisect.sh TestModuleCheckAutoGeneric c7d2445ff 2719f79a0
    scripts/bisect.sh --pkg=./internal/arm64 'TestGoldenRun/closures' GOOD BAD
    scripts/bisect.sh 'bin/xo build test/foo && ./foo' GOOD BAD

runs `git bisect run` in a shared clone of the checkout (in
`$XO_BISECT_WORK`, else `$TMPDIR/xo-bisect.PID`, removed at the end
unless `--keep`). Each step builds its xo with this checkout's
scripts/xobuild, so with today's seed choice (an ancestor's seed or
cached build, FetchSeedFor) even for a commit whose own xobuild chose
wrongly, into the shared checkout cache by source key; then runs the
test in the step's tree with `XO_CLI` that xo and `XO_CLI_STALE_OK=1`.
A go test pattern (no space; `TestA|TestB` runs as `^(TestA|TestB)$`,
one with a `/` as given) runs `go test -count=1 -timeout 30m` in the
package of its first name at BAD (`--pkg` overrides); a step where it
matches no test is skipped. A shell command exits 0 good, 125 skip,
anything else bad. A step whose xo does not build is skipped. xo's
back-end cache, build directory, Go cache, and test binaries are private
per step (`XO_BACKEND_CACHE_DIR`, `XO_BUILD_DIR`, `XO_GOCACHE`,
`XO_BIN_CACHE=off`), so an old xo's prune never reaches the shared ones.
Prints each step's verdict (a bad step with its test log's last lines),
the first bad commit, and its author line. Proved on 2026-10-22's
TestModuleCheckAutoGeneric failure: c7d2445ff good to 2719f79a0 bad,
508 commits, 9 steps in 8 minutes (each step's xo built from a cached
ancestor build or seed), first bad 129b19af7, the commit the hand bisect
found. Last verified: 2026-10-22.

Before bisecting it runs the step at GOOD and at BAD and stops with exit
3 when GOOD does not pass or BAD does not fail (either skipped counts):
on 2026-10-22 a bisect from a GOOD that already failed tested bad at every
step and named the commit right after GOOD. `--skip-verify` turns the
check off. scripts/bisect_test.sh tests the script on a throwaway
repository (`XO_BISECT_XOBUILD=true` replaces the xo build; seconds).

A checkout's xo (the one the Go tests run, internal/xotool):

    go run ./scripts/xobuild                  # prints <repo>/bin/xo
    XO_SEED=/path/to/xo go run ./scripts/xobuild

builds `bin/xo` (git ignored, with `bin/xo.key`) when the key of the
checkout's sources changed (the host, compiler/, and the std modules it
reaches through `use`; xotool.Inputs). The build goes into `<user
cache>/xo/checkout/<key>/xo` once per key, under a file lock, shared by
worktrees, and is copied to bin/xo. The seed is the first of `$XO_SEED`,
the checkout's earlier bin/xo (not when `bin/xo.commit` names a commit
that is not an ancestor of HEAD: a newer xo after checking out an older
commit), the newest earlier build in that cache
whose recorded commit (`<key>/commit`, written since 2026-10-14) is an
ancestor of HEAD, so another branch's xo on a shared CI runner is never
used, and only from a clean tree (`<key>/clean`, since 2026-10-16: a
build with uncommitted changes under compiler/ or std/ is never offered,
gotchas/xobuild-dirty-build-as-ancestor.md),
`xo` on PATH, and the newest prebuilt seed (below; needs the aws CLI
and read access to the seed bucket), or, when HEAD is older than that
seed's commit (a bisect step), the newest seed cut from an ancestor of
HEAD (FetchSeedFor, 2026-10-22: seed 0.24.0's output calls runtime
exports a commit before it lacks,
gotchas/xobuild-seed-newer-than-commit.md); a seed that fails, or whose
built xo does not run `xo version`, is reported and the next one tried, and the log names the path taken ("xotool: building
xo ... with the prebuilt seed of s3://..."). When none builds, xobuild
exits non-zero with every cause, the bucket path, and the opt in (owner,
2026-10-13: the chain costs 25 to 37 minutes and hid fetch failures).
Building from source is opt in: `XO_SEED_FALLBACK=chain` adds the
go-compiler-final seed (built once into `<user cache>/xo/seed/<commit>`,
from `git archive` of the tag, or of its commit in a clone without tags;
needs Go and git) through the seed chain after the others, and lets a
failing local xo try the chain; `go run ./scripts/xobuild --from-source`
(`XO_SEED_FALLBACK=from-source`) uses that root alone, for the replay
from source (decision 0216 item 3) and a new platform (a build of the
same key already in the cache is reused: point `XO_CHECKOUT_CACHE` at an
empty directory for a full replay); the log says "xotool: building the
go-compiler-final root and walking scripts/seed-chain.txt". The backend: arm64 on an arm64
host, amd64 on x86-64 hosts (since 2026-10-24, decision 0362: a
Docker linux/amd64 build of xo 212 s against LLVM -O0's 893 s, and that
xo builds compiler/xo in 211 s against 532 s; was LLVM split into 12
modules without LTO) and on Windows (since 2026-10-23, as release.sh
builds the release's xo.exe; the Go backend's xo.exe took 540 s a link
on CI's runner). The chain's hops keep LLVM on x86-64 and the Go
backend on Windows (internal/xotool chainBackend: those commits predate
both). `XO_CHECKOUT_CACHE` moves the cache. The Go tests
build it on first use unless `XO_CLI` names an xo. On the development
Mac a build with a cached xo1 as the seed took 391 s and 6.09 GB peak
RSS (2026-10-13, other agents running). Last verified: 2026-10-13.

### Prebuilt seeds (decision 0216 item 1)

Release N is built by release N-1 (owner, 2026-10-13). Seeds are not
public releases: they live in the private S3 bucket
`xo-seeds-ACCOUNT_ID` (us-east-1, all public access blocked,
versioned), readable by the development Mac's AWS user and by EC2
instances launched with the instance profile `xo-ci-seed-read`
(s3:GetObject and s3:ListBucket on that bucket only; remote-full.sh and
remote-windows.sh pass it; AL2023 ships the aws CLI, and
remote-windows.sh installs it when the Windows AMI lacks it). A seed's
name is its version and commit, `<version>-<commit, 8 hex>`. Layout (the
first seed, 0.9.0-5e21dc31 from main 5e21dc31, uploaded by the lead on
2026-10-13 for darwin-arm64 and linux-arm64):

    seeds/latest.txt                               the newest seed name, one line (0.10.0-23ce8ce2)
    seeds/<seed>/<goos>-<goarch>.tar.gz            the tree: xo/bin/xo, xo/std (xo finds std at bin/../std)
    seeds/<seed>/<goos>-<goarch>.tar.gz.sha256     "<hex>  <goos>-<goarch>.tar.gz" (sha256sum format)
    seeds/<seed>/manifest.txt                      "version V", "commit <10 hex>", "commit_full <40 hex>",
                                                   "date D", "layout ...", then "<goos>-<goarch> <sha256> (note)"

A seed carries no rtpack, xo-gobind, third_party, or zone data: it only
builds compiler/xo, against the checkout's `--std` (the TLS sources
then come from the checkout's third_party). Each seed bumps the minor
version (`compiler/cli/core` `VERSION` and `compiler/cli/servers`
`XO_VERSION`; 0.12.0 next).

Each version bump gets an annotated git tag `v<version>` on that commit
(owner, 2026-10-14, from 0.11.0 on), pushed to GitHub, whose message is
the summary of what the version brings over the previous one: a first
line `Xo v<version>`, then what it unblocks, what landed since the last
seed (zero-C items and the C lines deleted, compiler and backend
changes with figures from bench/results, build and tooling changes),
and whether the language or spec changed. `git tag -a v0.11.0 -F
notes.txt <commit> && git push origin v0.11.0` (seed-cut.sh does it).
A tag is not a GitHub release: no release is published (owner).

#### Cutting a seed

Only after the owner approves the version; then any session may run it
(owner, 2026-10-14), on the development Mac (darwin/arm64: Go, clang,
push access to main, and the AWS user that may write the bucket), from
a checkout of the repository, in two steps:

    # 1. write the tag's message: first line "Xo vX.Y.0", then the summary
    #    above (what it unblocks, what landed, tooling, language changes)
    $EDITOR /tmp/notes-X.Y.0.txt
    # 2. bump, gate, push, tag, build, pack (about 15 minutes); WORKDIR
    #    must be empty and outside the repository
    scripts/seed-cut.sh X.Y.0 /tmp/notes-X.Y.0.txt WORKDIR
    # 3. upload, latest.txt last (a minute)
    scripts/seed-upload.sh WORKDIR
    # 4. delete WORKDIR (about 4 GB: the boot stages, the cross and race
    #    builds, the packed tarballs, now in the bucket) and its tree
    #    worktree, by literal path
    git worktree remove --force /ABS/WORKDIR/tree && git worktree prune
    rm -rf /ABS/WORKDIR

Run the scripts from a worktree of origin/main, not the main checkout
(which stays at an old commit): seed-cut.sh refuses to run when its seed
scripts differ from origin/main's, since a stale copy once ran pre-rename
paths (2026-10-18). Let seed-cut.sh make the version commit; if main
already carries a `Version X.Y.0` commit at its head (a manual bump),
it cuts from that commit as is instead of refusing.

A patch seed (lead, 2026-10-28) cuts a tag that already exists, such as
a release branch's v0.33.1: `XO_SEED_TAG=v0.33.1 scripts/seed-cut.sh
0.33.1 /dev/null WORKDIR` (NOTES is unused; XO_SEED_TAG must be
`v` plus VERSION, X.Y.Z). Step 1 makes the tree a worktree of that tag
and refuses unless cli.xo, servers.xo and std/VERSION there say X.Y.Z; it
makes no bump commit, step 4 pushes and tags nothing, and no push lock is
taken. Steps 2, 3, 5 and 6 (-short, the gate and fixed point, the
darwin-arm64 build, the cross builds, race runtimes and packs) run as for
X.Y.0, from the tag's own scripts. The running seed-cut.sh must still
match origin/main's. Rehearse with `XO_SEED_DRY_RUN=1 XO_SEED_TAG=...`;
the X.Y.0 path is unchanged. Docker must be up for step 6.

With agents running (lead, 2026-10-27; Last verified: 2026-10-27):
freeze main and pause measure jobs first, and run the cut with
`XO_HEAVY="$HOME/xo-lead/agents/heavy.sh NAME" HEAVY_PRIORITY=1`: each
heavy step queues on its own, and the 0.33.0 cut (bulk class) took 70
minutes instead of 35 because its gate and its darwin build each waited
behind measure jobs queued before them.

seed-cut.sh stops at the first failure, before anything is pushed when
the failure is in the bump, `-short` (internal/xotool, cmd/xo), or the
gate (the bump's xo builds and runs hello, and the fixed point holds).
After the push and tag it builds darwin-arm64 natively at clang -O2,
builds ThreadSanitizer's runtime for native --race into the tree's
lib/xo/race (scripts/race-runtime.sh: LLVM compiler-rt 23.1.3, its
sources checked against pinned SHA-256 sums, built with clang, and cmake
and ninja from PATH or a Python virtual environment in WORKDIR/race;
the darwin-arm64 seed then holds xo/lib/xo/race, decision 0273; last
verified: 2026-10-20), and cross builds linux-arm64, linux-amd64, windows-amd64, and darwin-amd64
with that xo and the Go backend (about 25 s each; linux-arm64 also gets
ThreadSanitizer's static runtime, `scripts/race-runtime.sh
WORKDIR/race-linux DIR linux-arm64`, built in Docker, about 10 minutes,
so seed-cut refuses to start while Docker is not running; owner
2026-10-21), packs all five with
seed-pack.sh into WORKDIR/out, and writes WORKDIR/out/SEED. Every pack runs
with `XO_SEED_FORBID=WORKDIR`: seed-pack.sh refuses a pack when a packed
file contains a path it names (separated by `:`), and race-runtime.sh
maps its WORKDIR to `.` (`-ffile-prefix-map`, `-fdebug-prefix-map`), so
no runtime names the cutter's home directory
(gotchas/build-paths-in-shipped-runtimes.md; last verified: 2026-10-27).
Logs are in
WORKDIR/logs. A failure after the push (a build or pack) is fixed and
the packing redone by hand from WORKDIR/tree; the tag and the version
commit stay. `XO_SEED_DRY_RUN=1` rehearses every step without the push
and tag (seed-upload.sh refuses its output); `XO_HEAVY` prefixes the
heavy steps with a lock (the lead's agents/heavy.sh). With other
sessions pushing, set `XO_PUSH_LOCK` to the push lock pushq.sh takes
(`XO_PUSH_LOCK=$XO_LEAD_SCRATCH/locks/push`): seed-cut.sh takes it
before it reads origin/main and holds it through the gate to its push,
so main cannot move under the gate; without it the 0.24.0 cut passed its
gate and its push was refused (2026-10-21). Pushes from other sessions
wait for the gate (about 20 minutes). A lock whose holder's PID is gone
is taken over; a dry run takes none (last verified: 2026-10-21).

seed-upload.sh checks every tarball against its .sha256 and the
manifest, refuses a seed already in the bucket, uploads the five
tarballs, their .sha256, and manifest.txt, checks the listing (11
objects), then writes seeds/latest.txt and reads it back. Hosts pick the
new seed up on their next build that needs one (no local xo).

Verified: 0.11.0-ae2587fd was cut by the same steps (the lead's script
those were taken from) and uploaded by seed-upload.sh, and a dry run of
seed-cut.sh (0.12.0, no push) passed end to end on 2026-10-14. Last
verified: 2026-10-14.

Seeds uploaded: 0.9.0-5e21dc31 (2026-10-13, built natively and through
the chain, Windows cross built) and 0.10.0-23ce8ce2 (2026-10-14, the
version bump 23ce8ce2 on main after fast builds round 2: darwin-arm64
built natively on the Mac at clang -O2 after the fixed point; linux-arm64,
linux-amd64, windows-amd64, and darwin-amd64 cross built on the Mac by
that xo with `--backend=go --targets=<os>/<arch>`, 16 to 24 s each;
tarballs 14 to 16 MB, darwin-arm64 35 MB). The owner had 0.10.0 uploaded
without a separate verification run; the next builds from it are its
check (owner, 2026-10-14); 0.11.0-ae2587fd (2026-10-14, version bump
ae2587fd after B13 stage 2 and B14, tag v0.11.0; the same five
platforms the same way, cross builds 25 to 27 s). Last verified:
2026-10-14.

internal/xotool (seed.go) fetches a seed when no local xo is found:
seeds/latest.txt, then `<host>.tar.gz.sha256` and `<host>.tar.gz` with
`aws s3 cp` (`--region us-east-1`, or `$XO_SEED_REGION`), checks the
sha256, and unpacks into `<user
cache>/xo/seeds/<seed>/<goos>-<goarch>/tree` (once, under a file lock;
the xo is tree/xo/bin/xo); that xo builds the checkout's compiler/xo in
one hop. When latest.txt cannot be read, the newest seed already
unpacked is used. A failed fetch (no aws CLI, no access, no seed for the
host, a sha256 mismatch) or a failed build is an error naming the cause
and the bucket path; the chain runs only with `XO_SEED_FALLBACK=chain`
(TestFetchFailNoChain). Seeds are unpacked only into the user cache,
never into $HOME: the tarball's top directory is xo/, and on EC2 the
checkout is ~/xo (unpacking there overwrote its bin/xo and std,
2026-10-13). Variables: `XO_SEED_BUCKET` (bucket[/prefix]; no default since
decision 0249: unset, xo reads the first line of
`<user config dir>/xo/seed-bucket` (`~/Library/Application
Support/xo/seed-bucket` on macOS, `$XDG_CONFIG_HOME` or `~/.config` on
Linux), which sessions started before XO_SEED_BUCKET was set also see;
with neither, and no XO_SEED_URL, there is no prebuilt seed. The owner's
machines have both, their value in the untracked CLAUDE.local.md, and
CI reads the repository variable XO_SEED_BUCKET; last verified
2026-10-17), `XO_SEED_URL` (the location instead:
`s3://bucket[/prefix]`, `file:///dir`, or a directory laid out like the
bucket; `off` skips fetching). `XO_SEED` still comes first. The fetch
read the lead's darwin-arm64 tarball of 0.9.0-5e21dc31 from a local copy
and unpacked it in 0.5 s, and its xo printed `xo 0.9.0 darwin/arm64`
(2026-10-13). Last verified: 2026-10-13.

A fresh machine's xobuild with a seed, measured on the development Mac
(2026-10-13, commit 18ee6242, other agents running, through heavy.sh):
no bin/xo, no xo on PATH, empty caches (`XO_CHECKOUT_CACHE`,
`XO_BUILD_DIR`, `XO_RTOBJ_CACHE`, `XO_TLS_CACHE` fresh, `XO_BIN_CACHE=off`,
`XO_RTPACK=off`), and `XO_SEED_URL` a local directory holding a seed
packed by seed-pack.sh from the checkout's bin/xo (4 s, a 36,656,384
byte tarball, the earlier lib/xo layout): the seed unpacked in 0.6 s and
`go run ./scripts/xobuild` took 325 s end to end (one build of
compiler/xo; 9.16 GB maximum resident set size by /usr/bin/time -l),
against 1,491 to 1,936 s for the chain on a fresh EC2 machine. On EC2
the download from S3 is added; the lead measures it there.

`scripts/seed-pack.sh XO OUTDIR` packs a built xo in that shape: a
checkout's bin/xo (std at bin/../std; the tracked std/ files) or a
release's bin/xo (lib/xo/std beside it), into
OUTDIR/<goos>-<goarch>.tar.gz, `.tar.gz.sha256`, and `.info`; rebuilds
OUTDIR/manifest.txt from every platform packed there (they must agree on
version and commit; `seed-pack.sh --manifest OUTDIR` only rebuilds it);
and prints the seed name. The version and platform come from `XO
version`; an xo of another host needs `XO_SEED_VERSION` and
`XO_SEED_PLATFORM`. The commit is `$XO_SEED_COMMIT` (a full hash), else
the checkout's HEAD; `XO_SEED_NOTE` is the manifest note. A checkout
with changes under compiler/ or std/ is refused
(`XO_SEED_ALLOW_DIRTY=1` overrides). `XO_SEED_GZIP_LEVEL` (1 to 9,
default 6) is gzip's level; TestSeedPackHello packs at 1 (2.6 s instead
of 4.9 s; last verified: 2026-10-14). It uploads nothing. internal/xotool
TestSeedPackHello packs the checkout's bin/xo, fetches it from a local
directory, and runs hello with the unpacked xo (no XO_STD, a working
directory without std); it skips without a built xo.

Packing a seed by hand (a platform seed-cut.sh does not build, or a
redo after a failure): from a clean checkout of the version commit,
`scripts/seed-pack.sh bin/xo OUT` for the host's xo, and for an xo of
another platform `XO_SEED_VERSION=X.Y.0 XO_SEED_PLATFORM=<os>-<arch>
XO_SEED_COMMIT=<full hash> scripts/seed-pack.sh <dir>/bin/xo OUT` with
the checkout's tracked std/ beside its bin/; then
`scripts/seed-pack.sh --manifest OUT`. A natively built linux xo (`go
run ./scripts/xobuild` on an EC2 machine of that arch) packs the same
way.

Upload latest.txt last, after every platform of the seed is in place: a
host whose platform is missing from the newest seed fails (or, opted
in, takes the chain). release.sh's linux/amd64 xo is built by the Go
backend and its linux/arm64 xo by compiler/macho's ELF link (both passed
release-verify.sh on EC2, 2026-10-12). Seeds include windows-amd64 since
0.10.0. In an EC2
queue command the lead's prefix unpacks the seed into $HOME/xo-seed and
sets XO_SEED (UPDATE 13 of 2026-10-13); with this change `go run
./scripts/xobuild` fetches it into the user cache by itself on an
instance with the xo-ci-seed-read profile.

The seed chain (decision 0216 item 2): an xo older than an intrinsic
the sources use cannot build them (gotchas/seed-older-than-intrinsics.md).
scripts/seed-chain.txt records the hops, `<commit> [std=<commit>]`, in
order: a hop builds that commit's compiler/xo with the previous xo,
against the std of `std=` (default its own), and the last hop builds the
checkout. internal/xotool applies them only when opted in
(`XO_SEED_FALLBACK=chain` when a seed fails, or `--from-source`; each hop built
once into `<user cache>/xo/seed/hop-<commit>[-std-<commit>]`), and

    XO_SEED_CHAIN=file scripts/bootstrap-xo.sh SEED [OUTDIR]
    XO_SEED_CHAIN="COMMIT[:STDCOMMIT] ..." scripts/bootstrap-xo.sh SEED

runs them before s1. A change that makes xo's own sources use something
an older xo cannot compile records its hop in the same change. The one
hop today (syscalls2): main 4f792c55's compiler against the
go-compiler-final std, built by a pre-syscalls2 xo in 311 s, then that
xo built the branch's xo in 311 s (the Mac, 2026-10-13). On EC2 Linux
arm64 (r8g.8xlarge, no xo at all, 2026-10-13) `go run ./scripts/xobuild`
took 1,936 s: the go-compiler-final root seed (Go, then its xo1) about
685 s, then 1,251 s for a direct build from the root that failed
(undefined xo_call_c2), the hop (411 s), and the final build. The root
seed now goes through the recorded hops first (an existing xo is still
tried directly first), which saves the failed direct build, about one
build of compiler/xo (an estimate: the remainder of the 1,251 s after
the hop, split between the failed build and the final one). Last
verified: 2026-10-13.

    scripts/bootstrap-xo.sh SEED [OUTDIR]     # OUTDIR: .tools/bootstrap
    XO_BOOTSTRAP_RTPACK=DIR scripts/bootstrap-xo.sh SEED [OUTDIR]
                                              # with a pack root: the stages link with compiler/macho (darwin)

SEED builds compiler/xo with the host's native backend
(`XO_BOOTSTRAP_BACKEND`: arm64 on arm64 hosts, amd64 on x86-64 hosts
since 2026-10-24 (decision 0362; it was llvm with `XO_LLVM_SPLIT=12
XO_LLVM_LTO=none`, since ThinLTO's symbol names follow the build
directory, and llvm still is for other architectures)) (s1), s1 builds it (s2),
s2 builds it (s3); s2 and s3 must write the same assembly and the same
binary (the fixed point), and s3 must build and run a hello program.
Every stage runs with no `go` on PATH (a directory holding one is
replaced by a shim directory linking its other programs), GOROOT unset,
`XO_GO=/nonexistent/go`, and empty caches of its own (`XO_BUILD_DIR`,
`XO_RTOBJ_CACHE`, `XO_TLS_CACHE`; `XO_BIN_CACHE=off`, `XO_RTPACK=off`),
so the C runtime and the TLS library are compiled from source with the
C compiler, the one tool needed (`cc` drives the arm64 backend's
assembler and linker; clang compiles the runtime). The log and the
stage times land in OUTDIR/log.txt and OUTDIR/times.txt. What the seed
still takes from Go today: nothing for a native build; a program that
embeds time zones reads Go's zoneinfo.zip (a release's xo reads its
own lib/xo/zoneinfo.zip, 2026-10-12) unless `XO_ZONEINFO` names
one (compiler/xo tz.xo), and the Go backend needs the Go toolchain by
definition.

`scripts/warm-run.sh ARCH COMMIT CMD` (owner, 2026-10-13; Last verified:
2026-10-13) runs a targeted command on a warm, reusable on demand
instance per architecture (tag Name=xo-warm, role arm64 r8g.4xlarge or
amd64 m7i.4xlarge), created on first use and STOPPED (not terminated)
after 5 idle minutes (`scripts/warm-run.sh --stop-idle 5`, owner 2026-10-14, so idle warm machines free the vCPU quota; run by the
lead's EC2 queue runner), so its toolchain, Go and checkout survive: a
job on a running warm instance took 15 s end to end and one on a
stopped instance 32 s to 67 s, against 6 to 10 minutes for a fresh
remote-full.sh launch (about 5 to 6 minutes of setup). Every boot
schedules a poweroff after 8 hours; one job at a time per instance (a
local lock). Roles arm64-big (r8g.8xlarge) and amd64-big (c7i.8xlarge)
are 32 vCPU warm instances for runs that need the cores; the lead's
queue sends a KIND=cmd request with TYPE=r8g.8xlarge or c7i.8xlarge to
them, and one with no TYPE to the 16 vCPU role. Warm instances carry the
instance profile xo-ci-seed-read (read only on the seed bucket, decision
0216); `XO_REPO` names the checkout when the script runs from a copy.
Full suites still use remote-full.sh's fresh 32 vCPU instances.

On a clean machine: `scripts/remote-bootstrap.sh [--dry-run] [COMMIT]`
launches one EC2 Graviton instance (c8g.4xlarge on demand, remote-full.sh's
xo-ci key pair and security group, a 120 minute self poweroff plus a
local watchdog, terminated on exit), makes the seed there (`go run
./scripts/xobuild --from-source`: the go-compiler-final xo1 and the seed chain build the commit's
compiler/xo), deletes the Go toolchain and caches from the machine, and
runs bootstrap-xo.sh. The
log goes to bench/results/remote/bootstrap-<commit>.log (git ignored).

Result on 2026-10-12 (commit 30dfd24, Amazon Linux 2023 arm64, 16
vCPU, clang 15.0.7 and gcc, Go deleted): seed 630 s; s1 922 s, s2 965
s, s3 958 s; s2 and s3 assembly identical (375,303,244 bytes) and
binaries identical (73,492,408 bytes); s1 also equals s2; s3 reports
`xo 0.9.0 linux/arm64` and runs the hello program. The first run
(commit 4bdf716) failed at s1: xo1 with the default relative `--std`
could not build the TLS library on an empty cache
(gotchas/xo1-relative-std-tls-tree.md, fixed in 30dfd24). On the
development Mac (commit 7b3da52, Go installed but off PATH): seed 336
s; s1 556 s, s2 548 s, s3 555 s; s2 and s3 identical (assembly
384,779,554 bytes, binary 76,874,400 bytes), s1 too. Last verified:
2026-10-12.

## Publishing the public mirror (decision 0249)

Development stays in the private josephspurrier/xo; github.com/xo-lang/xo
is a scrubbed mirror of main and the version tags. Only the owner pushes
it. The tools live outside every repository, in
`../xo-publish-tools` next to the main checkout, because the
replacement rules hold the private values:

- `publish.sh prepare` (the publisher agent): rebuilds `../xo-publish` from
  the private origin/main and tags with `git filter-repo` (replacements,
  removed paths, commit hashes in files mapped to public commits), checks
  for leftovers, runs gitleaks, checks that every seed-chain.txt hop is a
  public commit, refuses a non fast-forward against what was last
  published, runs the purity check, and writes `last-prepare.txt`.
- `publish.sh push` (the owner, in a terminal): shows the summary, asks
  for a typed confirmation, and pushes with the deploy key
  (`~/.ssh/xo-lang-publish`, passphrase only the owner knows; the SSH
  agent is not used). It refuses without a terminal.

The private repository has a pre-push hook in its shared `.git/hooks`
(every worktree uses it; it is not versioned, so a fresh clone needs it
reinstalled) that refuses a push to any URL containing `xo-lang`. It does
not affect the owner's push, which runs from `../xo-publish`, a separate
repository without the hook. Last verified: 2026-10-17.

Also checked since 2026-10-18: every author and committer is the one
public identity (`mailmap.txt` maps any other), and replacement rules
cover the Intel Mac's host. A rule that matches already published
history changes its hashes, so prepare refuses it; before the mirror was
public, the fix was to delete and recreate xo-lang/xo (a deploy key can
belong to one repository, and a deleted one keeps it, so that took a new
key at the same path) and publish again from scratch.

What the public repository shows comes from the private one's files:
README.md, LICENSE (the plain MIT text, so GitHub detects it; the notes
about std/ and third-party code are in README.md), CONTRIBUTING.md,
SECURITY.md, CODE_OF_CONDUCT.md (Contributor Covenant 2.1), and the
issue forms in .github/ISSUE_TEMPLATE (bug, spec, use case; blank issues
allowed; the vulnerability link goes to private reporting). Settings live
on GitHub: Actions disabled, homepage https://xolang.dev, topics, the
rulesets in `../xo-publish-tools` (enforced once public), private
vulnerability reporting. Last verified: 2026-10-18.

go run ./scripts/xocheck [--no-gc] DIR...   # xo check of the tree's sources without building xo when a cached build has the same checker sources (internal/xotool CheckerXo: compiler/check, parser, lexer, cli/bind, cli/core/check.xo, xo.mod; --no-gc adds compiler/ssa), else after building bin/xo; the tree's std via XO_STD; prints which xo on stderr (last verified: 2026-10-21)
