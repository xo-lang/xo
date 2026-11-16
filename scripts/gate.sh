#!/bin/bash
# gate.sh: an agent's gate before a push, as named steps (CLAUDE.md
# "Tests"); run it under the heavy lock (scripts/lead/heavy.sh NAME
# scripts/gate.sh ...). It rebuilds WORKTREE's bin/xo, then runs each
# step with that xo and prints one line per step, then GATE-DONE.
#
#   scripts/gate.sh WORKTREE STEP...
#
# Steps:
#   repo     internal/repotest, which runs first in every gate anyway (named:
#            also with XO_GATE_REPO=0)
#   short    go test -short over the repository (runs/ excluded)
#   arm64    the full tests of internal/arm64
#   llvm     the full tests of internal/llvm, skipped when every file
#            the branch changes is native backend only (owner,
#            2026-10-18; see native_only below); XO_GATE_LLVM=1 runs it;
#            with TestGoldenGoParity (XO_TEST_GO_PARITY=1) when the
#            branch changes compiler/gen, std/rt, or a std module std/rt
#            also implements (go_backend), and else for the goldens the
#            branch adds or edits (llvmgo; 2026-11-12). Its
#            golden programs are only those the change touches (owner,
#            2026-10-22; llvm_goldens below): XO_GATE_LLVM=all runs
#            every one
#   corpora  internal/stdtest's differential corpora on the Go backend
#            (TestStdCorpora/go/) and internal/llvm's Test*Diff
#            differential tests: run when the branch changes std/rt,
#            compiler/gen, or a std module std/rt also implements
#            (go_backend, std_twin); skipped otherwise
#   cmd      the full tests of cmd/xo
#   irmanifest
#            HEAD's LLVM IR manifest alone, into the manifest cache, a clean
#            tree only (scripts/lead/irman.sh runs it for main's commits and
#            shares it with every offload host; no repo run before it)
#   gen      the full tests of internal/gen (-parallel 8, XO_GEN_PARALLEL)
#   fuzz     the private-module parser differentials of internal/stdtest
#            against Go's own parsers: http1's raw request head, its
#            response head, the request smuggling framing table, and
#            neturl's URL parse, non -short and at their default case
#            counts. 20 s alone and 42 s while corpora-all ran, both
#            measured 2026-10-26 (2000 URL cases on arm64 and
#            LLVM, 300 head and response cases, 41 smuggling rows) with a
#            warm xo; run it in every tier (lead, 2026-10-26: 0.33.0's
#            exit criterion 6 is that these run in the full tier)
#   corpora-all
#            every testdata corpus of internal/stdtest on every backend
#            (TestStdCorpora, no /go/ suffix: go, arm64 and LLVM; the http,
#            httpurl, mime, multipart and mail tables are the seeded
#            Go-oracle differentials among them). 10 min 32 s measured
#            2026-10-26, mostly clang -O2 thin LTO links of the LLVM
#            corpora; the 8-hourly full tier runs it every time (lead,
#            2026-10-26). The `corpora` step below is the cheaper Go
#            backend only subset a per-change gate runs.
#   corpuscheck
#            go run ./scripts/stdcorpus -check: every committed corpus
#            against its generator (26 s and 1.8 GB on the Mac,
#            2026-11-15); the full tier's Mac steps run it, since the
#            oracles need the module cache (lead, 2026-11-15)
#   alloff   internal/arm64's goldens with every optional pass off
#            (XO_PASSES, compiler/arm64 passes.xo)
#   fp       the self rebuild fixed point (scripts/bootstrap-xo.sh); its
#            output (about 3 GB) is removed after
#   warmseq  std goldens built through caches other programs filled, with
#            the default cache settings (internal/arm64
#            TestWarmBuildSequence, gate subset): run after the steps
#            named when the branch changes compiler/ (warmseq_change);
#            XO_WARMSEQ=modules or all for wider sets
#   chkcache the module check cache's auto mode against a cold check
#            (internal/arm64 TestModuleCheckAuto, ...Generic, ...Zones):
#            run after the steps named when the branch changes
#            compiler/check, compiler/ssa or compiler/xo (the build
#            driver; its output must be the same warm and cold), since XO_CHK_CACHE=auto is the
#            default of xo build and run (owner, 2026-10-22; two warm and
#            cold differences slipped past the per-push gates before);
#            XO_GATE_CHKCACHE=0 skips that
#   stdwasm  every std test module under `xo test --backend=wasm` with
#            Node's WASI as XO_TEST_EXEC (decision 0610); a module passes
#            when its summary line is ok, its listed wasm skips named
#            there. Needs node; about 4 minutes on host 2 (2026-11-11);
#            for the 8-hourly full tier
#   stdcov  internal/repotest's TestStdAPICovered with the resolved sweep
#            (`xo dev edges` over every program, XO_STDCOV_JOBS at once,
#            default 2; 3636 s in tier 69 on the Mac beside other steps
#            with NumCPU jobs; at 2 jobs 417 s alone on host 2, 2026-11-12,
#            host memory 19.2 to 22.9 GB of 24 GB). Only when named, and by the 8-hourly tier (tier.sh):
#            never after other steps by path, since per push only the bare
#            minimum runs (owner, via lead 2026-11-12; it auto-ran for std
#            pushes from 2026-11-11 and outran a 90 minute gate lease)
#   cross, codegen, wasm, selfbuild, seedbuild
#            pushq.sh's proving checks (below; wasm also runs
#            TestWasmCommandGoldens under wasmtime where the host has it): named, each runs after
#            the other steps even when the paths do not require it, and
#            is named in the PUSHQ_RAN line when it passes
#   (after the steps named)
#            internal/arm64 TestAmd64CrossBuilds (four programs built for
#            linux, windows and darwin amd64, "cross") when the branch
#            changes compiler/arm64 or ssa .xo or std/native/llrt
#            (XO_GATE_CROSS=0 skips that); and internal/arm64
#            TestSwitch65535Arms when it changes compiler/arm64/jumptab.xo
#            (the match lowering; XO_GATE_SCALE=0 skips that); and
#            cmd/xo TestCLIHelp (about 2 s) when it changes xo's help text
#            (compiler/cli/cmdhelp) or its goldens (cmd/xo/testdata/clihelp,
#            clihelp_test.go; XO_GATE_HELP=0 skips that); and
#            scripts/stdgen -check with the gate's xo (a few seconds) when
#            it changes compiler/gen, std/*.xo, std/native (the modules
#            std/go_compiled.txt lists) or std/rt, failing with
#            "run stdsync" (XO_GATE_STDGEN=0 skips that); and
#            internal/arm64's cache tests (-run 'Cache|BinCache',
#            TestMachoBinCache among them) when it changes compiler/xo's
#            bcache.xo, parallel.xo, packseg.xo or cache.xo
#            (XO_GATE_CACHE=0 skips that; lead, 2026-10-30); and
#            internal/arm64 TestStdStoreBuilds when it changes a std
#            module's source (std/*.xo, std/native/*.xo or a
#            std/native/MOD/ directory; XO_GATE_STDSTORE=0 skips that;
#            and internal/stdtest TestStdModules on arm64 for every std
#            module with tests that uses a changed std .xo file's module,
#            directly or through other std modules (std_mods, "stdmods";
#            lead, 2026-11-11: a Url field broke native/http's table
#            past its gate; XO_GATE_STDMODS=0 skips that);
#            lead, 2026-11-06); and
#            internal/llvm's TestRaceSyncTable and TestRaceSyncTableArm64
#            on darwin when it changes llrt's task, scheduler or race
#            runtime (XO_GATE_RACE=0 skips that; lead, 2026-10-31); and
#            the checks pushq.sh requires by path (gatelib.sh
#            gate_required_steps, the function pushq.sh asks with), `xo dev codegen` for
#            compiler/ssa, arm64 or llvm and TestSelfhostWasm for llrt or
#            std/native/rt*.xo, and one cold build of compiler/xo with
#            the new xo under XO_MAX_MEM=8G with --phase-times for what
#            compiler/xo builds from (compiler/ but macho and wasm,
#            std/native, std/llrt; "selfbuild") and, for the same paths,
#            one build of compiler/xo by the offload host's seed
#            (XO_SEED) under XO_INLINE_STRICT=1, the hourly fixed
#            point's s1 (on an offload host only; "seedbuild", lead,
#            2026-11-08), and internal/llvm
#            TestFieldGeneratorsInSync for scripts/gen or a file holding a
#            generated block (its table; "fieldgen", lead, 2026-11-08),
#            `xo test compiler/...` for a compiler/ .xo change ("xounit",
#            lead, 2026-11-12),
#            printing the PUSHQ_RAN
#            value to push with (XO_GATE_PUSHQ=0 skips them; lead,
#            2026-11-06)
#   warm    the backend caches across xo builds (decision 0275): the
#            parent commit's xo is built, then TestFrontCacheAcrossPrograms
#            fills a cache with it and builds every program again with
#            this xo, through the cache and with it off (XO_TEST_WARM_XO);
#            then internal/arm64's TestGoldenRun with XO_BACKEND_CACHE=verify
#            against the user cache, which every earlier xo warmed: a stale
#            entry of another xo fails the build
#
# The branch's changes are counted from $XO_GATE_BASE, else the
# merge-base of HEAD and origin/main.
#
# XO_GATE_CONCURRENT=1 runs the named steps at once, at most XO_GATE_JOBS
# (default 3) at a time (opt in; see "XO_GATE_CONCURRENT" below).
#
# Logs go to $XO_GATE_LOGS (default $TMPDIR/xo-gate-WORKTREE_NAME), one
# file per step. The first step that fails stops the gate (fail fast,
# lead, 2026-11-15): the steps still running are stopped, those not
# started are skipped, and a FAIL-FAST line names them;
# XO_GATE_FAILFAST=0 runs every step as before. The exit status is 1 when
# any failed, 2 for a refused gate, 3 for a stalled xobuild.
set -u
# a step name in WORKTREE's place (`gate.sh arm64`, 2026-10-30) ran the
# steps in the caller's directory and reported their failures as the
# gate's: refused
wt=$(cd "${1:?usage: gate.sh WORKTREE STEP...}" 2>/dev/null && pwd) || { echo "gate.sh: $1 is not a directory (usage: gate.sh WORKTREE STEP...)"; exit 2; }
shift
# WORKTREE must be an xo checkout (2026-11-11, gotcha
# heavy-cwd-runs-another-repos-gate): a gate given another repository has
# nothing of xo's to build or test
[ -d "$wt/compiler/xo" ] && [ -f "$wt/scripts/gate.sh" ] || { echo "gate.sh: $wt is not an xo checkout (no compiler/xo); give WORKTREE as the xo worktree you gate"; exit 2; }
logs=${XO_GATE_LOGS:-${TMPDIR:-/tmp}/xo-gate-$(basename "$wt")}
mkdir -p "$logs"
# one gate per log directory (lead, 2026-11-10: a dry run of tier.sh
# wrote into a running tier's xo-gate-lead-tierN): $logs/.gate.pid names
# the gate using it; a live gate.sh there refuses this one (exit 2). The
# pid is only checked for liveness and that it is a gate.sh, never signaled.
gp=$logs/.gate.pid
# (-f first: bash opens <"$gp" before 2>/dev/null applies, so a first
# gate printed "line 126: .gate.pid: No such file", lead, 2026-11-10)
if [ -f "$gp" ] && read -r op <"$gp" && [ -n "$op" ] && [ "$op" != $$ ] &&
  ps -p "$op" -o command= 2>/dev/null | grep -q 'gate\.sh'; then
  echo "gate.sh: $logs belongs to the running gate pid $op; set XO_GATE_LOGS to a directory of your own"
  exit 2
fi
echo $$ >"$gp"
trap 'rm -f "$gp"' EXIT
rm -f "$logs/times.tsv" "$logs/golden-sel.txt"
cd "$wt" || exit 1
# gate_required_steps and gate_plan: the proving checks, as pushq.sh asks
# for them (the tree's own copy, so a gate and its pushq agree)
. scripts/lead/gatelib.sh || exit 1
# a base that lacks main's test-input commits from over an hour ago fails
# on diffs main already fixed: refused (GATE_STALE_OK=1 gates anyway)
gate_stale_check . || exit 2
t0=$(date +%s)
rm -f bin/xo
# a build that prints nothing for XO_GATE_XOBUILD_IDLE_MIN minutes
# (default 10) is stopped and the gate exits 3, an infrastructure
# failure (lead, 2026-11-15: 18 offload gates in 2026-11 sat to the
# ceiling with no xobuild line, 15 of them while the hosts' seeds
# changed; every finished xobuild took 302 s or less)
go run ./scripts/xobuild >"$logs/xobuild.txt" 2>&1 &
gate_wait_progress $! "$logs/xobuild.txt" $((${XO_GATE_XOBUILD_IDLE_MIN:-10} * 60))
xbrc=$?
if [ "$xbrc" = 124 ]; then
  echo "XOBUILD-STALL: xobuild printed nothing for ${XO_GATE_XOBUILD_IDLE_MIN:-10} min after $(($(date +%s) - t0)) s and was stopped; an infrastructure failure, not the change (seed: ${XO_SEED:-none}; $logs/xobuild.txt ends:)"
  tail -5 "$logs/xobuild.txt" | cut -c1-300 | sed 's/^/  /'
  exit 3
fi
[ "$xbrc" = 0 ] || { echo "XOBUILD-FAIL ($logs/xobuild.txt)"; exit 1; }
echo "xobuild $(($(date +%s) - t0)) s"
# darwin: xo's own race runtime in lib/xo/race, so the race tests link
# the runtime xo ships, not clang's dylib (scripts/lead/race-rt.sh
# --local; lead, 2026-11-14); a failed build stops the gate. Off the
# lead's Mac (no ~/xo-lead) it says so and the tests use clang's.
if [ "$(uname -s)" = Darwin ]; then
  if [ -d "${XO_LEAD_DIR:-$HOME/xo-lead}" ]; then
    bash scripts/lead/race-rt.sh --local "$PWD" || { echo "RACE-RT-FAIL: no xo darwin race runtime"; exit 1; }
  else
    echo "gate: no lead directory: darwin race tests link clang's ThreadSanitizer runtime"
  fi
fi
# the xo every step runs is the one this tree's sources key (owner,
# 2026-11-08): xobuild serves a cached build by key, so this confirms the
# cache entry, in a second or two
go run ./scripts/xoclicheck "$wt/bin/xo" || { echo "XOBUILD-STALE: $wt/bin/xo is not built from this tree's sources"; exit 1; }
# XO_INLINE_STRICT=1: an inlined module that does not verify fails the
# step instead of building slower with a warning (decision 0342)
export XO_CLI=$wt/bin/xo XO_BACKEND_CACHE=off XO_INLINE_STRICT=1
failed=0
# slowest JSON: the tests of a go test -json log by time, slowest first
# (lead, 2026-10-26: the full tier's 83 minutes needed per-subtest times)
slowest() {
  python3 - "$1" <<'PY'
import json, sys
rows = []
for line in open(sys.argv[1], errors="replace"):
    try:
        e = json.loads(line)
    except ValueError:
        continue
    if e.get("Test") and e.get("Action") in ("pass", "fail", "skip"):
        rows.append((e.get("Elapsed", 0), e["Action"], e["Test"]))
for el, act, t in sorted(rows, reverse=True)[:40]:
    print(f"{el:9.2f} {act:4} {t}")
PY
}
# gate_times NAME JSON (the sum of each parent's direct subtests too, as
# kind "subtests"): each top-level test of a go test -json log that
# took a second or more, as "NAME<TAB>TEST<TAB>SECONDS<TAB>ACTION" lines
# in $logs/times.tsv, and the package's own wall time as TEST "-"
# (lead, 2026-11-14, owner's test-time plan: the daily check
# scripts/lead/gatetime.sh reads every gate's lines to find a step or
# a test that grew)
gate_times() {
  python3 - "$1" "$2" >>"$logs/times.tsv" 2>/dev/null <<'PY'
import json, sys
n, rows, pk, sub = sys.argv[1], [], None, {}
for line in open(sys.argv[2], errors="replace"):
    try:
        e = json.loads(line)
    except ValueError:
        continue
    t, a = e.get("Test"), e.get("Action")
    if a in ("pass", "fail", "skip"):
        if t and "/" not in t and e.get("Elapsed", 0) >= 1:
            rows.append(f"{n}\t{t}\t{e.get('Elapsed', 0):.1f}\t{a}")
        elif t and "/" in t:
            # a parent's Elapsed leaves out its parallel subtests' time (a
            # subtest's leaves out its own): sum each depth, keep the most
            k = (t.split("/")[0], t.count("/"))
            sub[k] = sub.get(k, 0) + e.get("Elapsed", 0)
        elif not t:
            pk = e.get("Elapsed")
most = {}
for (p, _), v in sub.items():
    most[p] = max(most.get(p, 0), v)
for p, v in sorted(most.items()):
    if v >= 10:
        rows.append(f"{n}\t{p}\t{v:.1f}\tsubtests")
if pk is not None:
    rows.append(f"{n}\t-\t{pk:.1f}\tpackage")
print("\n".join(rows))
PY
}
# pkg_budget PKG: go test's -timeout for PKG's run in a step, from the
# continuous tier's actuals with every test on, about 1.5 times the
# slowest finished run on the slowest host (2026-11-15: host 4 runs every
# step about 1.7 times as long as host 3, and with 3 steps at once the
# llvm step's golden run (57,000 subtest seconds) passed 120m twice while
# the same work took 4,257 s on host 3; #51). XO_GATE_TIMEOUT overrides
# it for every package.
pkg_budget() {
  [ -n "${XO_GATE_TIMEOUT:-}" ] && { echo "$XO_GATE_TIMEOUT"; return; }
  case $1 in
  ./internal/llvm) echo 200m ;;  # 4,257 s host 3, over 7,200 s host 4
  ./internal/arm64) echo 120m ;; # 2,778 s host 3, 4,821 s host 4
  ./cmd/xo) echo 90m ;;          # 1,868 s host 3, 3,301 s host 4
  *) echo 120m ;;                # as before (gen 1,395 s, stdtest 1,140 s on host 4)
  esac
}
full() { # NAME PKG [go test args]: one package's full tests
  n=$1; p=$2; shift 2
  # -json keeps every test's time ($logs/NAME.json, the 40 slowest in
  # NAME.slow.txt); NAME.txt is its output as go test -v prints it
  go test -json -count=1 -timeout "$(pkg_budget "$p")" "$p" "$@" >"$logs/$n.json" 2>"$logs/$n.err"
  rc=$?
  python3 -c 'import json,sys
for l in open(sys.argv[1], errors="replace"):
    try: e = json.loads(l)
    except ValueError: sys.stdout.write(l); continue
    sys.stdout.write(e.get("Output", ""))' "$logs/$n.json" >"$logs/$n.txt"
  cat "$logs/$n.err" >>"$logs/$n.txt"
  slowest "$logs/$n.json" >"$logs/$n.slow.txt"
  gate_times "$n" "$logs/$n.json"
  if [ "$rc" = 0 ]; then
    echo "$n: $(tail -1 "$logs/$n.txt")"
  else
    failed=1
    echo "$n: FAIL ($logs/$n.txt)"
    grep -E '^(    )?--- FAIL' "$logs/$n.txt" | head -10
  fi
}
# gate_skip PKG: the -skip pattern for PKG's tests that scripts/gate-areas.txt
# scopes to areas the change does not touch (owner's test-time plan, lead,
# 2026-11-14: the llvm step's TLS, crypto and runtime tests and the arm64
# step's cache tests ran in every gate whatever it changed); empty for the
# full tier (XO_GATE_LLVM=all or XO_GATE_TESTS=all) and when nothing is
# scoped out. Writes a line for the log to $logs/PKG-areas.txt (PKG's
# slashes as dashes).
gate_skip() {
  local pkg=$1 note
  note="$logs/$(echo "$pkg" | tr / -)-areas.txt"
  rm -f "$note"
  [ "${XO_GATE_LLVM:-}" = all ] || [ "${XO_GATE_TESTS:-}" = all ] && return 0
  [ -f scripts/gate-areas.txt ] || return 0
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | sort -u |
    python3 scripts/gate-areas.py "$pkg" scripts/gate-areas.txt "$note"
}
# native_only: every changed file is one the LLVM backend never reads
# (compiler/arm64, which holds the amd64 emitter too, compiler/macho,
# internal/arm64's tests, documentation, bench results), so internal/llvm
# cannot see the change except through the xo it builds with: the arm64
# backend compiles compiler/llvm into that xo, and -short's LLVM goldens
# (shortGoldens) still run it. A change to compiler/ssa, the front end, compiler/xo,
# std or the runtime runs the step.
native_only() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  files=$(git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-)
  [ -n "$files" ] || return 1
  ! echo "$files" | grep -v -E '^(compiler/arm64/|compiler/macho/|internal/arm64/|knowledgebase/|bench/results/)|\.md$' | grep -q .
}
# go_backend: the branch changes the Go backend's code generator or its
# runtime, or a std module the Go backend implements in std/rt (std_twin),
# so internal/llvm's goldens run on the Go backend too
# (TestGoldenGoParity: TestGolden checks want.txt with the LLVM build
# only since 2026-10-18) and the corpora step runs.
go_backend() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  local files f
  files=$({ git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; })
  echo "$files" | grep -q -E '^(compiler/gen/|std/rt/)' && return 0
  for f in $(echo "$files" | grep -E '^std/(native/(llrt/)?)?[a-z0-9_]+\.xo$'); do
    std_twin "$f" && return 0
  done
  return 1
}
# std_twin: std file f is a module's that std/rt also implements, so a
# change to it can make the native backends differ from the Go backend:
# std/MOD.xo or std/native/MOD.xo (or its _test.xo, or rtMOD.xo, the
# module's runtime half: rtjson.xo) where std/rt has MOD.go or
# std_gen_MOD.go, or std/native/llrt/X.xo where X starts with
# such a module's name of four letters or more (jsontext is json's,
# http1 is http's; the runtime's own files, sched or tasks, are not a
# module's). Lead, 2026-11-02: a native json fix (f3524650c) passed its
# gate and turned main's TestGenGoldensNative/json_fast red, since only
# compiler/gen and std/rt changes ran the Go backend's checks.
std_twin() {
  local m x
  m=${1##*/}
  m=${m%.xo}
  m=${m%_test}
  case $1 in
  std/native/llrt/*)
    for x in std/*.xo; do
      x=${x##*/}
      x=${x%.xo}
      [ ${#x} -ge 4 ] || continue
      case $m in "$x"*) mod_twin "$x" && return 0 ;; esac
    done
    return 1 ;;
  esac
  mod_twin "$m" || { [ "${m#rt}" != "$m" ] && mod_twin "${m#rt}"; }
}
mod_twin() { [ -f "std/rt/$1.go" ] || [ -f "std/rt/std_gen_$1.go" ]; }
# std_mods: the std modules with tests (std/**/M_test.xo, the modules of
# internal/stdtest TestStdModules) that use a changed std file's module,
# directly or through other std modules (the reverse `use` closure), as
# a go test -run pattern on arm64; empty when none.
# Lead, 2026-11-11: 7501b8eb5 added Url.user and password and its gate
# ran only url's own checks; native/http's Go table (which shows Url's
# debug text) failed in the hourly run. A module and its native half are
# one unit (std/http.xo, std/native/http.xo); std/native/llrt/X is its own.
std_mods() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | python3 -c '
import os, re, sys
def unit(rel):
    rel = re.sub(r"(_test|_c)?\.xo$", "", rel)
    if rel.startswith("std/"):
        rel = rel[4:]
    if rel.startswith("native/") and not rel.startswith("native/llrt/"):
        rel = rel[7:]
    return rel.split("/")[0] if not rel.startswith("native/llrt/") else "/".join(rel.split("/")[:3])
uses, tests = {}, {}
for d, ds, fs in os.walk("std"):
    ds[:] = [x for x in ds if x not in ("rt", "testdata", "changes") and not x.startswith(".")]
    for f in fs:
        if not f.endswith(".xo"):
            continue
        p = os.path.join(d, f)
        u = unit(p)
        if f.endswith("_test.xo"):
            tests.setdefault(u, set()).add(p[4:-8])
        for l in open(p, errors="replace"):
            m = re.match(r"use (\S+)", l)
            if m and (m.group(1).startswith("std/") or m.group(1).startswith("native/")):
                uses.setdefault(u, set()).add(unit(m.group(1) + ".xo"))
changed = {unit(l.strip()) for l in sys.stdin if re.match(r"std/(?!rt/|changes/).*\.xo$", l.strip())}
hit, grew = set(changed), True
while grew:
    grew = False
    for u, deps in uses.items():
        if u not in hit and deps & hit:
            hit.add(u); grew = True
mods = sorted(m for u in hit for m in tests.get(u, ()))
# go test -run splits its pattern at each slash, a level per subtest
# name element: NAME, native/NAME and native/llrt/NAME as levels (a
# shorter name matches the levels it has)
one = sorted(m for m in mods if "/" not in m)
nat = sorted({m.split("/")[1] for m in mods if m.count("/") == 1})
llrt = sorted({m.split("/")[2] for m in mods if m.count("/") == 2})
if mods:
    lv = ["^TestStdModules$", "^arm64$", "^(" + "|".join(one + (["native"] if nat or llrt else [])) + ")$"]
    if nat or llrt:
        lv.append("^(" + "|".join(nat + (["llrt"] if llrt else [])) + ")$")
    if llrt:
        lv.append("^(" + "|".join(llrt) + ")$")
    print("/".join(lv))
'
}
# check_change: the branch changes the checker or the SSA builder, whose
# whole-program passes the module check cache's skipped modules must
# replay (gotchas/module-check-cache-skips-whole-program-passes.md), or
# the build driver (compiler/xo), whose output the tests compare byte for
# byte between a warm and a cold build (lead, 2026-10-24: 1ca32d5cc's
# random work-dir names reached the linked binary's N_OSO paths, its gate
# skipped this step, and main was red 11:50 to 14:15).
check_change() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | grep -q -E '^compiler/(check|ssa|xo)/'
}
chkcache_ran=0
# the cross step (internal/arm64 TestAmd64CrossBuilds) runs when the
# branch changes the native backend (compiler/arm64 holds the amd64
# emitter and the shared MIR passes; lead, 2026-10-23: daed94d4d, a mir.xo
# change, passed its arm64 gate and broke every amd64 build of three
# goldens on main for an hour) or std/native/llrt (the runtime xo's native
# code generator compiles with each program; lead, 2026-11-08: a762dc2e3's
# num.append_json_float32 left main red on amd64 for 2 hours):
# gatelib.sh gate_required_steps decides, as for pushq.sh. XO_GATE_CROSS=0
# skips it unless named.
named=""
# scale_change: the branch changes the match lowering (compiler/arm64
# jumptab.xo), so internal/arm64's TestSwitch65535Arms (about 100 s) runs
# after the steps named (lead, 2026-10-24: 9479f31cd's compare tree made
# that match quadratic, past XO_MAX_MEM, and only the 8 hourly full tier
# saw it). XO_GATE_SCALE=0 skips it.
scale_change() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | grep -q -E '^compiler/arm64/jumptab\.xo$'
}
# stdstore_change: the branch changes a std module's source, which std's
# store build (every std module at once, decision 0388) compiles, so
# internal/arm64's TestStdStoreBuilds (about 30 s) runs after the steps
# named (lead, 2026-11-06: f0b4e8523 added std/sqlite, the first std
# module with `use c`; the store build went down the cc link path, every
# build fell back to whole program, and main was red 11:23 to 14:22
# (gotchas/std-store-build-use-c-module.md)). XO_GATE_STDSTORE=0 skips it.
stdstore_change() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | grep -q -E '^std/([a-z0-9_]+\.xo|native/[a-z0-9_]+\.xo|native/[a-z0-9_]+/)'
}
# help_change: the branch changes xo's help text (compiler/cli/cmdhelp) or
# its goldens (cmd/xo/testdata/clihelp, cmd/xo/clihelp_test.go), so cmd/xo's
# TestCLIHelp (about 2 s) runs after the steps named (lead, 2026-11-14:
# 3c7227589 added --check to xo build, run and test's help but not the
# goldens, its gate ran no cmd/xo test, and main's -short check failed at
# 9dea0a645 until a4b46f321). XO_GATE_HELP=0 skips it.
help_change() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | grep -q -E '^(compiler/cli/cmdhelp/|cmd/xo/testdata/clihelp/|cmd/xo/clihelp_test\.go$)'
}
# stdgen_change: the branch changes what std/rt's generated Go comes from
# (compiler/gen, a std module, std/native and std/go_compiled.txt, std/rt), so scripts/stdgen -check runs with
# the gate's xo (lead, 2026-11-15: stale std_gen files reached main twice
# that day, d998e7187 and b88a5dc29, each found only by the next push's
# pushq). XO_GATE_STDGEN=0 skips it.
stdgen_change() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | grep -q -E '^(compiler/gen/|std/[a-z0-9_]+\.xo$|std/native/|std/go_compiled\.txt$|std/rt/)'
}
# race_change: the branch changes llrt's task, scheduler or TSan runtime
# (tasks.xo, sched.xo, racesym.xo, cabi_race*.xo), so on darwin, where the
# race tables run, internal/llvm's TestRaceSyncTable(Arm64) runs after the
# steps named (lead, 2026-10-31: 7b526e8ac's TSan edge hid the table's
# mutex race; it was verified on linux/arm64 only, where the tables skip,
# and main stayed red two hours). XO_GATE_RACE=0 skips it.
race_change() {
  [ "$(uname -s)" = Darwin ] || return 1
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | grep -q -E '^std/native/llrt/(tasks|sched|racesym|cabi_race[a-z]*)\.xo$'
}
# cache_change: the branch changes the build driver's cache files
# (compiler/xo bcache.xo, parallel.xo, packseg.xo, cache.xo), so
# internal/arm64's cache tests (Cache|BinCache: the backend and front
# caches, the binary cache, TestMachoBinCache) run after the steps named
# (lead, 2026-10-30: 50f08a853's spill files in the build directory were
# hashed into the binary cache key, its gate ran none of these, and main
# was red 01:04 to 02:30). XO_GATE_CACHE=0 skips it.
cache_change() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | grep -q -E '^compiler/xo/(bcache|parallel|packseg|cache)\.xo$'
}
# warmseq_change: the branch changes the compiler (compiler/, but not
# compiler/macho, compiler/wasm or a _test.xo file), so internal/arm64's
# TestWarmBuildSequence (its gate subset, about 50 s) runs after the steps
# named: std goldens built through caches other programs filled, with the
# default cache settings, each the same file as its cold build (0.51.0
# respin, 2026-11-13: building one std/log program after another failed
# with XO1201, and no gate built two programs through one default cache).
# XO_GATE_WARMSEQ=0 skips it; the 2-hourly main check runs its modules
# set (XO_WARMSEQ=modules, about 3 minutes).
warmseq_change() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | grep -v -E '^compiler/(macho|wasm)/|_test\.xo$' | grep -q -E '^compiler/'
}
warmseq_ran=0
# llvm_goldens: the golden programs the llvm step runs (owner,
# 2026-10-22: the whole package cost 46 to 56 minutes a gate), "all" or
# XO_LLVM_GOLDENS's list (internal/llvm golden_select_test.go): those
# whose LLVM IR differs between this xo and the merge base's
# (TestGoldenIRManifest: every golden's IR, no clang, hashed) or whose
# directory the branch changes; "none" when no golden did (the package's
# other tests still run; TestGoldenTasks and TestGoldenASan build with
# runtimes of their own and run every golden). Every golden for
# XO_GATE_LLVM=all, for a change to what builds or runs them past the IR
# (internal/llvm's tests, internal/xotool, compiler/xo: clang's flags, the
# link, the split; compiler/llvm debug.xo: TestDebugInfoSameOutputLLVM's
# --debug-info IR), and
# when a manifest cannot be made. The merge base's manifest is made once
# (its xo built in a worktree of its own) and kept in
# $XO_LLVM_IR_CACHE (default <user cache>/xo/llvm-ir) by commit and
# manifest code. Writes
# a line for the log to $logs/llvm-select.txt.
# ir_jobs: how many goldens' IR the selection builds at once: every CPU,
# as the available memory allows (llvm_goldens); ir_jobs_why says why
ir_avail_mb() {
  if [ -r /proc/meminfo ]; then
    awk '/^MemAvailable:/ {print int($2 / 1024)}' /proc/meminfo
  else
    # darwin: free, inactive and purgeable pages
    vm_stat | awk -v ps="$(pagesize 2>/dev/null || echo 16384)" '/Pages (free|inactive|purgeable)/ {gsub(/\./, "", $NF); n += $NF} END {print int(n * ps / 1048576)}'
  fi
}
ir_jobs() {
  local ncpu avail bymem
  ncpu=$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)
  avail=$(ir_avail_mb); avail=${avail:-0}
  bymem=$(((avail - ${XO_GATE_IR_RESERVE_MB:-8192}) / ${XO_GATE_IR_MB:-1600}))
  [ "$bymem" -lt "$ncpu" ] && ncpu=$bymem
  [ "$ncpu" -lt 2 ] && ncpu=2
  echo "$ncpu"
}
ir_jobs_why() {
  echo "$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4) CPUs, $(ir_avail_mb) MB available, ${XO_GATE_IR_RESERVE_MB:-8192} MB kept for the other steps, ${XO_GATE_IR_MB:-1600} MB a build"
}
# ir_mem_watch FILE: the summed resident size (MB) of the xo processes,
# every 2 s, until stopped (the selection's own memory, for sizing ir_jobs)
ir_mem_watch() {
  while :; do
    ps -A -o rss= -o comm= 2>/dev/null | awk '$2 ~ /(^|\/)xo$/ {n += $1} END {print int(n / 1024)}' >>"$1"
    sleep 2
  done
}
# ir_cache_setup: $cache, the IR manifests' cache directory, and $mcode,
# the manifest code's key (its normalization in golden_select_test.go)
ir_cache_setup() {
  if [ "$(uname)" = Darwin ]; then ucache=$HOME/Library/Caches; else ucache=${XDG_CACHE_HOME:-$HOME/.cache}; fi
  cache=${XO_LLVM_IR_CACHE:-$ucache/xo/llvm-ir}
  mkdir -p "$cache"
  mcode=$(git hash-object internal/llvm/golden_select_test.go | cut -c1-12)
}
# ir_manifest_for COMMIT: sets $irm to COMMIT's manifest: the cache's, else
# the one the lead's irman.sh shared into $XO_LLVM_IR_SHARED (an offload
# host's /work/in: main's manifest made once on one host for every
# host's gates, lead 2026-11-15), named by the host's OS and
# architecture since the IR is the target's; the cache's path when
# neither exists
ir_manifest_for() {
  irm="$cache/$1-$mcode.txt"
  [ -s "$irm" ] && return
  local sh="${XO_LLVM_IR_SHARED:-}/llvm-ir-$(uname -s | tr A-Z a-z)-$(uname -m)-$1-$mcode.txt"
  [ -n "${XO_LLVM_IR_SHARED:-}" ] && [ -s "$sh" ] && irm=$sh
}
# ir_head_manifest OUT JOBS [DEPS]: the IR manifest of this tree's goldens,
# built by its bin/xo with JOBS at once, into OUT (and each golden's std
# files into DEPS, for ir_scope); logs the time and the xo processes'
# resident peak. XO_LLVM_IR_ONLY=a,b builds only those goldens.
ir_head_manifest() {
  local irw t_ir rc
  echo "llvm IR manifest: -parallel $2 ($(ir_jobs_why))" >&2
  ir_mem_watch "$logs/llvm-ir-mem.txt" &
  irw=$!
  t_ir=$(date +%s)
  # niced (XO_GATE_IR_NICE, default 10): the tests that run beside it
  # keep the CPU first (lead, 2026-11-15: TestOneThreadSpawnJoin's steal
  # bound failed at steal=48 while twelve IR builds ran beside llvmpre)
  XO_LLVM_IR_MANIFEST=$1 XO_LLVM_IR_DEPS=${3:-} nice -n "${XO_GATE_IR_NICE:-10}" go test -count=1 -timeout 60m -parallel "$2" -run '^TestGoldenIRManifest$' ./internal/llvm >"$logs/llvm-ir-head.txt" 2>&1
  rc=$?
  kill "$irw" 2>/dev/null; wait "$irw" 2>/dev/null
  [ "$rc" = 0 ] && echo "llvm IR manifest: $(($(date +%s) - t_ir)) s, xo processes' resident peak $(sort -n "$logs/llvm-ir-mem.txt" 2>/dev/null | tail -1) MB" >&2
  return $rc
}
# ir_free_gb DIR: the GB free on DIR's file system (df -Pk), "" when
# unknown
ir_free_gb() {
  df -Pk "$1" 2>/dev/null | awk 'NR==2 {print int($4 / 1048576)}'
}
# ir_inputs_changed A B: whether the commits A..B change what a golden's
# LLVM IR is made from: the compiler (but its wasm and Mach-O linker
# parts) or std's sources (not tests, docs, the Go runtime std/rt), or the
# manifest's own code (its key: a manifest made before is not found, so
# irman.sh makes one)
ir_inputs_changed() {
  git diff --name-only "$1" "$2" -- compiler std internal/llvm/golden_select_test.go |
    grep -v -E '^compiler/(wasm|macho)/|_test\.xo$|\.md$|^std/(rt|changes|evidence)/' |
    grep -q -E '^compiler/|^std/.*\.xo$|^internal/llvm/golden_select_test\.go$'
}
# golden_inputs DIR: `$XO_CLI build --list-inputs` of every llvm golden
# into DIR/NAME.txt, at most ir_jobs at once; fails when this xo has no
# --list-inputs or any golden's list fails (then the scope falls back)
golden_inputs() {
  local d=$1
  rm -rf "${d:?}"; mkdir -p "$d"
  "$XO_CLI" build --list-inputs "internal/llvm/testdata/golden/$(ls internal/llvm/testdata/golden | head -1)" >/dev/null 2>&1 || return 1
  ls internal/llvm/testdata/golden | GI_DIR=$d xargs -P "$(ir_jobs)" -n 1 sh -c \
    '[ -d "internal/llvm/testdata/golden/$1" ] || exit 0; "$XO_CLI" build --list-inputs "internal/llvm/testdata/golden/$1" >"$GI_DIR/$1.txt" 2>/dev/null || { rm -f "$GI_DIR/$1.txt"; exit 1; }' _
}
# llvm_shard_goldens K N: the goldens of shard K (1..N), by CRC-32 of the
# golden's name mod N, a stable split that a new golden does not reshuffle
llvm_shard_goldens() {
  ls internal/llvm/testdata/golden | python3 -c '
import sys, zlib
k, n = int(sys.argv[1]), int(sys.argv[2])
print(",".join(g for g in sys.stdin.read().split() if zlib.crc32(g.encode()) % n == k - 1))' "$1" "$2"
}
llvm_goldens() {
  sel_note="$logs/llvm-select.txt"
  if [ "${XO_GATE_LLVM:-}" = all ] && [ -n "$llvm_shard_n" ]; then
    local s
    s=$(llvm_shard_goldens "$llvm_shard_k" "$llvm_shard_n")
    echo "shard $llvm_shard_k of $llvm_shard_n: $(echo "$s" | tr ',' '\n' | grep -c .) goldens (XO_GATE_LLVM=all, XO_GATE_LLVM_SHARD)" >"$sel_note"
    if [ -z "$s" ]; then echo none; else echo "$s"; fi
    return
  fi
  if [ "${XO_GATE_LLVM:-}" = all ]; then
    echo "every golden (XO_GATE_LLVM=all)" >"$sel_note"; echo all; return
  fi
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  files=$(git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-)
  if echo "$files" | grep -q -E '^(internal/llvm/[^/]+$|internal/xotool/|compiler/xo/|compiler/llvm/debug\.xo)'; then
    echo "every golden (the change touches internal/llvm's tests, internal/xotool, compiler/xo or the LLVM debug information)" >"$sel_note"; echo all; return
  fi
  ir_cache_setup
  # (B) of the gate-time plan (lead, 2026-11-15): the scope first, from
  # every golden's `xo build --list-inputs` (golden_inputs, seconds): no
  # golden reads a changed file, so no IR is built and only the goldens
  # whose own directory changed run; or a scope of at most
  # XO_GATE_IR_SCOPE_MAX goldens (300) has the base's and HEAD's IR built
  # for those goldens alone (a probe of a std/csv.xo comment: 3 goldens,
  # where the base's whole manifest took 385 to 509 s to build). A larger
  # scope, "all" (a compiler or runtime change), or XO_GATE_IR_INPUTS=0
  # goes on to the manifests below.
  local gi escope gdirs ntot pw2 bpart hpart
  gi="$logs/golden-inputs"
  escope=""
  if [ "${XO_GATE_IR_SCOPE:-1}" != 0 ] && [ "${XO_GATE_IR_INPUTS:-1}" != 0 ] && golden_inputs "$gi"; then
    escope=$(echo "$files" | python3 scripts/golden-std-deps.py "$PWD" "inputs:$gi" 2>>"$logs/llvm-ir-head.txt") || escope=""
  fi
  gdirs=$(echo "$files" | sed -n 's|^internal/llvm/testdata/golden/\([^/]*\)/.*|\1|p' | sort -u | grep .)
  ntot=$(ls internal/llvm/testdata/golden | wc -l | tr -d ' ')
  if [ "$escope" = none ]; then
    if [ -z "$gdirs" ]; then
      echo "0 of $ntot goldens (no golden reads a changed file, by xo build --list-inputs: no IR built)" >"$sel_note"; echo none; return
    fi
    echo "$(echo "$gdirs" | wc -l | tr -d ' ') of $ntot goldens (their directories changed; no golden reads another changed file, by xo build --list-inputs), and the -O2 sample: $(echo "$gdirs" | paste -sd' ' -)" >"$sel_note"
    echo "$gdirs" | paste -sd, -; return
  fi
  if [ -n "$escope" ] && [ "$escope" != all ] && [ "$(echo "$escope" | tr ',' '\n' | grep -c .)" -le "${XO_GATE_IR_SCOPE_MAX:-300}" ]; then
    echo "llvm IR: scoped by xo build --list-inputs to $(echo "$escope" | tr ',' '\n' | grep -c .) goldens; base and HEAD IR built for them alone" >&2
    pw2=$(mktemp -d "${TMPDIR:-/tmp}/xo-gate-ir.XXXXXX")
    bpart=$pw2/base.txt hpart=$pw2/head.txt
    if git worktree add -q --detach "$pw2/tree" "$base" >"$logs/llvm-ir-base.txt" 2>&1 &&
      cp internal/llvm/golden_select_test.go "$pw2/tree/internal/llvm/" &&
      (cd "$pw2/tree" && go run ./scripts/xobuild) >>"$logs/llvm-ir-base.txt" 2>&1 &&
      (cd "$pw2/tree" && XO_CLI=$pw2/tree/bin/xo XO_LLVM_IR_ONLY=$escope XO_LLVM_IR_MANIFEST="$bpart" go test -count=1 -timeout 60m -parallel "${XO_GATE_IR_PARALLEL:-$(ir_jobs)}" -run '^TestGoldenIRManifest$' ./internal/llvm) >>"$logs/llvm-ir-base.txt" 2>&1 &&
      XO_LLVM_IR_ONLY=$escope ir_head_manifest "$hpart" "${XO_GATE_IR_PARALLEL:-$(ir_jobs)}" && [ -s "$bpart" ] && [ -s "$hpart" ]; then
      names=$( { comm -3 <(sort "$bpart") <(sort "$hpart") | awk '{print $1}'; echo "$gdirs"; } | sort -u | grep . )
      git worktree remove --force "$pw2/tree" >/dev/null 2>&1; rm -rf "${pw2:?}"
      if [ -z "$names" ]; then
        echo "0 of $ntot goldens (no IR changed among the $(echo "$escope" | tr ',' '\n' | grep -c .) goldens that read a changed file, by xo build --list-inputs)" >"$sel_note"; echo none; return
      fi
      echo "$(echo "$names" | wc -l | tr -d ' ') of $ntot goldens (IR changed among the $(echo "$escope" | tr ',' '\n' | grep -c .) that read a changed file, by xo build --list-inputs), and the -O2 sample: $(echo "$names" | paste -sd' ' -)" >"$sel_note"
      echo "$names" | paste -sd, -; return
    fi
    echo "llvm IR: the scoped build failed ($logs/llvm-ir-base.txt, $logs/llvm-ir-head.txt): the manifests below" >&2
    git worktree remove --force "$pw2/tree" >/dev/null 2>&1; rm -rf "${pw2:?}"
  fi
  ir_manifest_for "$(git rev-parse "$base")"; bm=$irm
  # an ancestor's manifest in place of the base's (lead, 2026-11-14,
  # owner's test-time plan: main moves between almost every gate, so the
  # base's manifest was nearly always missing, and making it built the
  # base's xo and every golden's IR, about 7 to 12 minutes a gate): the
  # goldens whose IR differs between an ancestor and HEAD are the base's
  # plus those the commits between them changed, a superset, so nothing the
  # base would select is lost. Within XO_LLVM_IR_BACK (default 30) commits
  # of the base; an older one is replaced by the base's own.
  # Only an ancestor whose commits up to the base change no IR input
  # (ir_inputs_changed): a host 1 gate of a comment change took the
  # manifest 25 commits back, across compiler commits that changed
  # nearly every golden's IR, and ran 722 of 728 goldens (lead,
  # 2026-11-15; bench/results/gate-time-2026-11-14.md); the base's own
  # manifest is built then.
  ib=""
  if [ ! -s "$bm" ]; then
    for c in $(git rev-list --max-count="${XO_LLVM_IR_BACK:-30}" "$base"); do
      ir_manifest_for "$c"
      if [ -s "$irm" ]; then
        if ir_inputs_changed "$c" "$base"; then
          echo "llvm IR manifest: $(git rev-parse --short "$c")'s is $(git rev-list --count "$c..$base") commits before the base, across IR input changes: the base's own is built" >&2
        else
          bm=$irm; ib=" (manifest of $(git rev-parse --short "$c"), $(git rev-list --count "$c..$base") commits before the base, no IR input changed between)"
        fi
        break
      fi
    done
  fi
  case $bm in "${XO_LLVM_IR_SHARED:-/nonexistent}"/*) ib="$ib (shared by irman.sh)" ;; esac
  if [ ! -s "$bm" ]; then
    bm="$cache/$(git rev-parse "$base")-$mcode.txt"
    pw=$(mktemp -d "${TMPDIR:-/tmp}/xo-gate-ir.XXXXXX")
    # the branch's manifest test in the base's tree: both sides hashed
    # by the same code (a base older than the test has none)
    if git worktree add -q --detach "$pw/tree" "$base" >"$logs/llvm-ir-base.txt" 2>&1 &&
      cp internal/llvm/golden_select_test.go "$pw/tree/internal/llvm/" &&
      (cd "$pw/tree" && go run ./scripts/xobuild) >>"$logs/llvm-ir-base.txt" 2>&1 &&
      (cd "$pw/tree" && XO_CLI=$pw/tree/bin/xo XO_LLVM_IR_MANIFEST="$bm.tmp$$" XO_LLVM_IR_DEPS="${bm%.txt}.deps.tmp$$" nice -n "${XO_GATE_IR_NICE:-10}" go test -count=1 -timeout 60m -parallel "${XO_GATE_IR_PARALLEL:-$(ir_jobs)}" -run '^TestGoldenIRManifest$' ./internal/llvm) >>"$logs/llvm-ir-base.txt" 2>&1; then
      mv "${bm%.txt}.deps.tmp$$" "${bm%.txt}.deps" 2>/dev/null
      mv "$bm.tmp$$" "$bm"
    fi
    rm -f "$bm.tmp$$" "${bm%.txt}.deps.tmp$$"
    git worktree remove --force "$pw/tree" >/dev/null 2>&1
    rm -rf "${pw:?}"
  fi
  nm="$logs/llvm-ir.txt"
  rm -f "$nm"
  # HEAD's manifest is kept by commit too, for a gate of the same commit
  # again (a clean tree only), and built with more goldens at once: with
  # -parallel 4 it took 1,419 s of a host 2 gate that selected no golden
  # (bench/results/gate-time-2026-11-14.md). XO_GATE_IR_PARALLEL sets it.
  # The default is every CPU, as memory allows (lead, 2026-11-14: the
  # selection is the floor both the llvm and the arm64 golden tests wait
  # for; at -parallel 4 each golden's xo was blocked half its time, 204% CPU,
  # and every CPU took the selection from 988 to 292 s on host 1): at most
  # (MemAvailable - XO_GATE_IR_RESERVE_MB, default 8192) / XO_GATE_IR_MB
  # (default 1600, about one xo IR build's resident peak) at once, at least
  # 2 (ir_jobs).
  hm=""
  [ -z "$(git status --porcelain --untracked-files=no)" ] && hm="$cache/$(git rev-parse HEAD)-$mcode.txt"
  irp=${XO_GATE_IR_PARALLEL:-$(ir_jobs)}
  # ir_scope (lead, 2026-11-15, gate-time plan (b)): for a std only
  # change, HEAD's IR is built only for the goldens that can reach it
  # (scripts/golden-std-deps.py: their imports' closure and the std files
  # their base IR names), and the others keep the base's lines; every
  # golden for a compiler, std/rt, prelude or runtime change, or with no
  # deps file beside the base's manifest. XO_GATE_IR_SCOPE=0 builds every
  # golden.
  # (B) of the plan (lead, 2026-11-15): with every golden's
  # `xo build --list-inputs` (golden_inputs, about 11 s for 742 on the
  # Mac), the scope is the goldens one of whose input files changed, and
  # needs no deps file beside the base's manifest; XO_GATE_IR_INPUTS=0 uses
  # the deps file's key closure as before
  scope=all
  bd="${bm%.txt}.deps"
  if [ "${XO_GATE_IR_SCOPE:-1}" != 0 ] && [ -s "$bm" ] && { [ -z "$hm" ] || [ ! -s "$hm" ]; }; then
    gi="$logs/golden-inputs"
    if [ "${XO_GATE_IR_INPUTS:-1}" != 0 ] && golden_inputs "$gi"; then
      scope=$(echo "$files" | python3 scripts/golden-std-deps.py "$PWD" "inputs:$gi" 2>>"$logs/llvm-ir-head.txt") || scope=all
      echo "llvm IR scope from xo build --list-inputs of every golden" >&2
    elif [ -s "$bd" ]; then
      scope=$(echo "$files" | python3 scripts/golden-std-deps.py "$PWD" "$bd" 2>>"$logs/llvm-ir-head.txt") || scope=all
    fi
    [ -n "$scope" ] || scope=all
  fi
  if [ -n "$hm" ] && [ -s "$hm" ]; then
    cp "$hm" "$nm"; echo "HEAD's manifest from $hm" >"$logs/llvm-ir-head.txt"
  elif [ "$scope" = none ]; then
    cp "$bm" "$nm"; ib="$ib (no golden's imports reach the change: HEAD's IR not built)"
    echo "llvm IR manifest: scoped: no golden reaches the change" >&2
  elif [ "$scope" != all ] && [ -s "$bm" ]; then
    echo "llvm IR manifest: scoped to $(echo "$scope" | tr ',' '\n' | grep -c .) goldens the change can reach" >&2
    if XO_LLVM_IR_ONLY=$scope ir_head_manifest "$nm.part" "$irp"; then
      # the base's lines, those built replaced
      awk 'NR == FNR {h[$1] = $2; next} {print $1, ($1 in h) ? h[$1] : $2}' "$nm.part" "$bm" >"$nm"
      awk 'NR == FNR {b[$1] = 1; next} !($1 in b)' "$bm" "$nm.part" >>"$nm"
      sort -o "$nm" "$nm"
      ib="$ib (HEAD's IR built for the $(wc -l <"$nm.part" | tr -d ' ') goldens the change can reach)"
    else
      rm -f "$nm"
    fi
    rm -f "$nm.part"
  elif [ -s "$bm" ]; then
    ir_head_manifest "$nm" "$irp" || rm -f "$nm"
    [ -s "$nm" ] && [ -n "$hm" ] && cp "$nm" "$hm.tmp$$" && mv "$hm.tmp$$" "$hm"
  fi
  if [ ! -s "$bm" ] || [ ! -s "$nm" ]; then
    echo "every golden (no IR manifest: $logs/llvm-ir-base.txt, $logs/llvm-ir-head.txt)" >"$sel_note"; echo all; return
  fi
  names=$( { comm -3 "$bm" "$nm" | awk '{print $1}'; echo "$files" | sed -n 's|^internal/llvm/testdata/golden/\([^/]*\)/.*|\1|p'; } | sort -u | grep . )
  total=$(wc -l <"$nm" | tr -d ' ')
  if [ -z "$names" ]; then
    echo "0 of $total goldens (no golden's IR or directory changed$ib)" >"$sel_note"; echo none; return
  fi
  echo "$(echo "$names" | wc -l | tr -d ' ') of $total goldens$ib, and the -O2 sample: $(echo "$names" | paste -sd' ' -)" >"$sel_note"
  echo "$names" | paste -sd, -
}
# golden_dirs_changed: internal/llvm's golden programs the branch adds or
# edits (comma separated; empty when none)
golden_dirs_changed() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only --diff-filter=AMR "$base" HEAD; git status --porcelain | cut -c4-; } |
    sed -n 's|^internal/llvm/testdata/golden/\([^/]*\)/.*|\1|p' | sort -u | paste -sd, -
}
# XO_GATE_LLVM_SHARD=K/N (scripts/lead/ctier.sh; lead, 2026-11-15: one
# full llvm step held host 4's single slot 78 minutes while a 30 second
# lease waited 51): with XO_GATE_LLVM=all, shard 0 runs the package's
# tests that do not follow the golden selection, shards 1..N the selected
# tests on their share of the goldens (llvm_shard_goldens), so each is a
# lease of its own and waiting leases run between them.
llvm_shard_k="" llvm_shard_n=""
case ${XO_GATE_LLVM_SHARD:-} in
  "") ;;
  */*) llvm_shard_k=${XO_GATE_LLVM_SHARD%/*} llvm_shard_n=${XO_GATE_LLVM_SHARD#*/} ;;
  *) echo "gate.sh: XO_GATE_LLVM_SHARD wants K/N, got $XO_GATE_LLVM_SHARD"; exit 2 ;;
esac
llvm_step() {
    if [ -n "$llvm_shard_n" ] && [ "$llvm_shard_k" = 0 ]; then
      sel_tests='TestGolden|TestGoldenSplit|TestGoldenGoParity|TestDebugInfoSameOutputLLVM|TestGoldenTasks|TestGoldenASan'
      sk=$(gate_skip internal/llvm)
      presk="^($sel_tests)\$"
      [ -n "$sk" ] && presk="^($sel_tests|${sk#^(}"
      echo "llvm: shard 0 of $llvm_shard_n: the tests that do not follow the golden selection"
      full llvm ./internal/llvm -skip "$presk"
      echo none >"$logs/golden-sel.txt"
      return
    fi
    if [ -z "${XO_GATE_LLVM:-}" ] && native_only; then
      echo "llvm: skipped (native backend only change; XO_GATE_LLVM=1 runs it)"
      echo all >"$logs/golden-sel.txt"
    else
      # the package's tests that do not follow the golden selection run
      # while it is made (lead, 2026-11-14: the selection took 1,014 to
      # 2,186 s of a host 2 gate, and the 800 to 1,350 s of package tests
      # waited for it; bench/results/gate-time-2026-11-14.md); the tests
      # that read XO_LLVM_GOLDENS (goldenSelected) run after it.
      # XO_GATE_LLVM_OVERLAP=0 runs them all after it, as before.
      sel_tests='TestGolden|TestGoldenSplit|TestGoldenGoParity|TestDebugInfoSameOutputLLVM|TestGoldenTasks|TestGoldenASan'
      pre_pid=""
      sk=$(gate_skip internal/llvm)
      [ -s "$logs/internal-llvm-areas.txt" ] && echo "llvm areas: $(cat "$logs/internal-llvm-areas.txt")"
      # XO_GATE_IR_FIRST=1 (ir_first_wait): the selection runs alone, not
      # niced, and the package's other tests run after it
      if [ "${XO_GATE_IR_FIRST:-0}" = 1 ]; then
        export XO_GATE_IR_NICE=0
      fi
      # a golden shard (1..N) leaves those tests to shard 0
      if [ -z "$llvm_shard_n" ] && [ "${XO_GATE_LLVM_OVERLAP:-1}" != 0 ] && [ "${XO_GATE_IR_FIRST:-0}" != 1 ]; then
        presk="^($sel_tests)\$"
        [ -n "$sk" ] && presk="^($sel_tests|${sk#^(}"
        ( failed=0; full llvmpre ./internal/llvm -skip "$presk"; echo "$failed" >"$logs/llvmpre.failed" ) >"$logs/llvmpre.out" 2>&1 &
        pre_pid=$!
      fi
      t_sel=$(date +%s)
      sel=$(llvm_goldens)
      printf 'llvm\t(select)\t%s\tselect\n' "$(($(date +%s) - t_sel))" >>"$logs/times.tsv"
      case $sel in all) ng=all ;; none) ng=0 ;; *) ng=$(echo "$sel" | tr ',' '\n' | grep -c .) ;; esac
      printf 'llvm\t(goldens)\t%s\tcount\n' "$ng" >>"$logs/times.tsv"
      echo "llvm goldens: $(cat "$logs/llvm-select.txt") ($(($(date +%s) - t_sel)) s to select)"
      if [ "$sel" = all ]; then unset XO_LLVM_GOLDENS; else export XO_LLVM_GOLDENS=$sel; fi
      # for the arm64 step's golden tests (arm64_goldens)
      echo "$sel" >"$logs/golden-sel.txt.tmp" && mv "$logs/golden-sel.txt.tmp" "$logs/golden-sel.txt"
      # the sanitizer goldens (TestGoldenTasks, TestGoldenASan) follow the
      # selection too, except for a change in an area whose sanitizer
      # behavior the plain IR may not show (internal/llvm skipUnselectedSan)
      unset XO_LLVM_SAN_ALL
      if { git diff --name-only "${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}" HEAD; git status --porcelain | cut -c4-; } |
        python3 scripts/gate-areas.py --touches scripts/gate-areas.txt @rt @sched @race @http @llvmbe @driver &&
        [ -z "$llvm_shard_n" ]; then
        export XO_LLVM_SAN_ALL=1; echo "llvm sanitizer goldens: every one (the change touches the runtime, scheduler, race, HTTP or LLVM backend areas)"
      fi
      # with the overlap, the tests after the selection are only those
      # that read it
      if [ -n "$pre_pid" ] || [ -n "$llvm_shard_n" ]; then
        sk=""; set -- -run "^($sel_tests)\$"
      else
        set -- ${sk:+-skip "$sk"}
      fi
      # XO_GATE_GO_PARITY=1 (scripts/lead/ctier.sh, the continuous tier on main):
      # every golden on the Go backend too, whatever changed
      if [ "${XO_GATE_GO_PARITY:-0}" = 1 ]; then
        XO_TEST_GO_PARITY=1 XO_GO_PARITY_SAMPLE=${XO_GATE_GO_PARITY_SAMPLE:-1} full llvm ./internal/llvm "$@"
      elif go_backend; then
        # every 16th golden on the Go backend too: the selection above
        # follows the LLVM IR, which a Go backend change leaves alone, so
        # TestGoldenGoParity ran none and passed (2026-11-06,
        # gotchas/go-parity-selected-by-llvm-ir.md); it now fails when it
        # runs none
        XO_TEST_GO_PARITY=1 XO_GO_PARITY_SAMPLE=${XO_GO_PARITY_SAMPLE:-16} full llvm ./internal/llvm "$@"
      else
        full llvm ./internal/llvm "$@"
        # a golden the change adds or edits runs on the Go backend too,
        # whatever else changed: iface_wrap_eq reached main failing
        # TestGoldenGoParity because its change touched neither
        # compiler/gen nor std/rt (tier 71, 2026-11-12;
        # gotchas/go-adapter-equality.md)
        gdirs=$(golden_dirs_changed)
        if [ -n "$gdirs" ]; then
          echo "llvmgo goldens (Go parity, added or edited): $gdirs"
          XO_TEST_GO_PARITY=1 XO_LLVM_GOLDENS=$gdirs full llvmgo ./internal/llvm -run '^TestGoldenGoParity$'
        fi
      fi
      unset XO_LLVM_GOLDENS
      if [ -n "$pre_pid" ]; then
        wait "$pre_pid"
        cat "$logs/llvmpre.out"
        [ "$(cat "$logs/llvmpre.failed" 2>/dev/null)" = 0 ] || failed=1
      fi
    fi
}
# XO_GATE_CONCURRENT=1 (opt in; arm64 beside llvm since 2026-10-26,
# every independent step since 2026-11-10, owner's test-speed plan): the
# named steps run at once, at most XO_GATE_JOBS (default 3) at a time, each
# with its own logs as before and its result lines printed together when
# it ends ("gate.sh: STEP done in N s" after them). corpora and
# corpora-all write the same logs, so they share one job, in order. The
# checks run after the named steps (chkcache, scale, caches, stdstore,
# race, stdmods) are one more job, and the proving checks codegen, wasm, cross and
# fieldgen one job each; selfbuild and seedbuild run after every job ends
# (their wall time and peak heap are what they report). A failure is
# collected as in the serial order. Unset or 0 runs everything in order,
# as before. Background: in a full tier at most two tests ran for 72% of
# arm64's 1,904 s and 55% of llvm's 1,866 s on the 12 core Mac, and the
# llvm step left 69% of memory free; tier 64's Mac steps took about 100
# minutes one after another.
conc=0
[ "${XO_GATE_CONCURRENT:-0}" = 1 ] && conc=1
# fail fast (XO_GATE_FAILFAST, default 1; lead, owner approved
# 2026-11-15): ff=1 once a step has failed; ff_by names it, ff_stopped the
# jobs stopped while they ran, ff_skipped the steps never started
ff=0 ff_by="" ff_stopped="" ff_skipped="" ff_last=repo
ff_on() { [ "${XO_GATE_FAILFAST:-1}" = 1 ]; }
# ff_serial: in the serial order, true (stop) once anything failed
ff_serial() {
  ff_on && [ "$failed" = 1 ] || return 1
  [ "$ff" = 1 ] || { ff=1; ff_by=${ff_by:-${ff_last:-an earlier check}}; }
  return 0
}
gjobs=${XO_GATE_JOBS:-3}
case $gjobs in '' | *[!0-9]* | 0) echo "gate.sh: XO_GATE_JOBS=$gjobs is not a positive number"; exit 2 ;; esac
# repo: internal/repotest, the fast repo checks pushq runs before a push
# (about 20 s), first in every gate unless short (which includes it) is
# named; XO_GATE_REPO=0 skips it (lead, 2026-11-06 bottleneck review:
# pushq refused 14 pushes in 3 hours on TestStdAPICovered,
# TestThirdPartyOrigins, TestWasmUnsupportedListsNetGoldens,
# TestOutsideBytesValidated, TestStdGenCurrent and TestWaitsClassified,
# each found only after a whole gate had passed). It runs before the
# concurrent jobs too: it is short, and a repo failure is the first thing
# to fix.
# A named `repo` is that same first run (lead, 2026-11-11: offload-gate
# callers name it, and gate.sh failed the gate on "unknown step repo"
# after every step had passed); it runs even with XO_GATE_REPO=0.
has_short=0
has_repo=0
only_irman=1
for s in "$@"; do
  [ "$s" = short ] && has_short=1
  [ "$s" = repo ] && has_repo=1
  [ "$s" = irmanifest ] || only_irman=0
done
# (not before an irmanifest alone: it checks nothing, it makes main's
# manifest)
[ "$#" -gt 0 ] && [ "$only_irman" = 1 ] && [ "$has_repo" = 0 ] && has_short=1
if { [ "${XO_GATE_REPO:-1}" != 0 ] || [ "$has_repo" = 1 ]; } && [ "$has_short" = 0 ]; then
  full repo ./internal/repotest
fi
# The pre-check (lead, owner approved 2026-11-15, gate-failures-2026-11.md
# (F): 11 of 28 failed gates in 2026-11 failed a check of seconds, then
# ran their llvm and arm64 suites 30 to 90 minutes before reporting):
# after repo and before the steps, one after another, the host's free
# disk (hostcheck) and the quick checks the paths call for: stdgen
# -check, TestCLIHelp and TestStdChecks, which otherwise run after the
# steps. With fail fast, a failure here ends the gate in minutes.
# XO_GATE_PRECHECK=0 leaves them after the steps; each keeps its own
# switch (XO_GATE_STDGEN, XO_GATE_HELP, XO_GATE_STDCHECKS).
pre_done=" "
pre_ran() { case "$pre_done" in *" $1 "*) return 0 ;; esac; return 1; }
stdgen_check() {
  if go run ./scripts/stdgen -check >"$logs/stdgen.txt" 2>&1; then
    echo "stdgen: current"
  else
    failed=1
    echo "stdgen: FAIL: std/rt's generated Go is stale: run go run ./scripts/stdsync and commit ($logs/stdgen.txt)"
    grep -E 'stale|error' "$logs/stdgen.txt" | head -5
  fi
}
stdchecks_check() {
  if go test -count=1 -run '^TestStdChecks$' ./cmd/xo >"$logs/stdchecks.txt" 2>&1; then
    echo "stdchecks: ok"
  else
    failed=1; echo "stdchecks: FAIL ($logs/stdchecks.txt)"; grep -E 'stdcheck_test.go' "$logs/stdchecks.txt" | head -8
  fi
}
# hostcheck: XO_GATE_MIN_FREE_GB (default 10) free on the file systems of
# TMPDIR, the logs and the worktree, else HOSTCHECK-FAIL (a full disk
# failed jsondec's gate in 2026-11 after its steps had run); many
# TIME_WAIT sockets (Linux ss) only print a warning
hostcheck() {
  local d free tw min=${XO_GATE_MIN_FREE_GB:-10}
  for d in "${TMPDIR:-/tmp}" "$logs" "$wt"; do
    free=$(df -Pk "$d" 2>/dev/null | awk 'NR == 2 { print int($4 / 1048576) }')
    [ -n "$free" ] || continue
    if [ "$free" -lt "$min" ]; then
      failed=1
      echo "HOSTCHECK-FAIL: $free GB free on the file system of $d, under $min (XO_GATE_MIN_FREE_GB): infrastructure, not the change; free space or gate elsewhere"
      return
    fi
  done
  if command -v ss >/dev/null 2>&1; then
    tw=$(ss -Htan state time-wait 2>/dev/null | wc -l | tr -d ' ')
    [ "${tw:-0}" -gt 20000 ] && echo "hostcheck: warning: $tw TCP sockets in TIME_WAIT (the HTTP diff tests can run out of ephemeral ports)"
  fi
  echo "hostcheck: ok"
}
if [ "${XO_GATE_PRECHECK:-1}" != 0 ] && ! ff_serial; then
  tpre=$(date +%s)
  pch=$({ git diff --name-only "${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}" HEAD; git status --porcelain | cut -c4-; } | grep -v '_test\.xo$')
  ff_last=hostcheck; hostcheck; pre_done="$pre_done hostcheck "
  if ! ff_serial && [ "${XO_GATE_STDGEN:-1}" != 0 ] && stdgen_change; then ff_last=stdgen; stdgen_check; pre_done="$pre_done stdgen "; fi
  if ! ff_serial && [ "${XO_GATE_HELP:-1}" != 0 ] && help_change; then ff_last=clihelp; full clihelp ./cmd/xo -run '^TestCLIHelp$'; pre_done="$pre_done clihelp "; fi
  if ! ff_serial && [ "${XO_GATE_STDCHECKS:-1}" != 0 ] && echo "$pch" | grep -qE '^std/.*\.xo$'; then ff_last=stdchecks; stdchecks_check; pre_done="$pre_done stdchecks "; fi
  printf 'precheck\t=\t%s\tstep\n' "$(($(date +%s) - tpre))" >>"$logs/times.tsv"
  echo "gate.sh: pre-check: $(echo $pre_done) in $(($(date +%s) - tpre)) s"
fi
# the named steps, in order: pushq's proving checks are collected into
# named (they run after the steps, with the ones the paths require), an
# unknown name fails the gate, and a named chkcache keeps it from running
# a second time after the steps
steps=""
for step in "$@"; do
  case $step in
  codegen|selfbuild|seedbuild|wasm|cross|fieldgen|xounit|stdfast|checkall)
    # pushq's proving checks: run after the named steps with the ones the
    # change's paths require (gate_plan), each once, and named in the
    # PUSHQ_RAN line when they pass (lead, 2026-11-08: a named `cross
    # codegen` ran neither, then pushq refused the push for cross)
    named="$named $step" ;;
  repo) ;; # ran first, above
  short|arm64|llvm|irmanifest|corpora|cmd|gen|fuzz|corpora-all|corpuscheck|alloff|fp|warm|chkcache|warmseq|stdwasm|stdcov)
    [ "$step" = chkcache ] && chkcache_ran=1
    [ "$step" = warmseq ] && warmseq_ran=1
    steps="$steps $step" ;;
  selftest-fail|selftest-sleep|selftest-ok)
    if [ "${XO_GATE_SELFTEST:-0}" = 1 ]; then steps="$steps $step"; else echo "gate.sh: $step is a test step (XO_GATE_SELFTEST=1)"; failed=1; fi ;;
  *) echo "gate.sh: unknown step $step"; failed=1 ;;
  esac
done
# arm64's golden selection (lead, 2026-11-14, owner's test-time plan:
# TestGoldenRun was 3,523 s of subtests in a host 2 gate): the arm64 tests
# that build every golden of internal/llvm follow the llvm step's selection
# (the goldens whose LLVM IR or directory the change alters) when the
# change leaves the native backend's own code alone: the LLVM IR is made
# from the same SSA module, so a golden whose IR is unchanged has the same
# SSA, and the arm64 output changes only with the native backend, the
# shared SSA passes, the driver or the arm64 tests. They need the llvm step
# in the same concurrent gate (its selection is written to
# $logs/golden-sel.txt); otherwise, or with XO_GATE_ARM64_SELECT=0, every
# golden runs.
arm64_sel_tests='TestGoldenRun|TestGoldenRunSpill|TestGoldenRunHeapGuard|TestGoldenPassesOff|TestDebugInfoSameCode'
arm64_select_ok() {
  [ "${XO_GATE_ARM64_SELECT:-1}" != 0 ] || return 1
  [ "$conc" = 1 ] || return 1
  case " $steps " in *" llvm "*) ;; *) return 1 ;; esac
  [ "${XO_GATE_LLVM:-}" != all ] && [ "${XO_GATE_TESTS:-}" != all ] || return 1
  ! { git diff --name-only "${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}" HEAD; git status --porcelain | cut -c4-; } |
    grep -q -E '^(compiler/(arm64|macho|ssa|xo)/|internal/arm64/|internal/xotool/|scripts/xobuild/|go\.mod$|std/native/llrt/[a-z0-9_]*(arm64|a64|darwin|macho|elf|linux)[a-z0-9_]*\.xo$)'
}
# arm64_goldens: the llvm step's selection, once it is written (at most
# XO_GATE_ARM64_SEL_WAIT seconds, default 3600; then every golden)
arm64_goldens() {
  local w=0
  while [ ! -s "$logs/golden-sel.txt" ] && [ "$w" -lt "${XO_GATE_ARM64_SEL_WAIT:-3600}" ]; do
    sleep 5; w=$((w + 5))
  done
  if [ -s "$logs/golden-sel.txt" ]; then cat "$logs/golden-sel.txt"; else echo all; fi
}
# bin_select BACKEND: decision 0657 (owner through the lead, 2026-11-15):
# the goldens to run, from the binaries themselves. Every golden is built
# by the merge base's xo with the base's std and by this xo with this std
# (each std copied to one path in turn, this branch's golden sources at
# one path, every cache off: the std path is in the debug information and
# strings, so two paths differ everywhere), and scripts/golden-bin-select.py
# names those whose binaries differ with the build-info id masked, those
# a side could not build, and those that use C (their C step records a
# path of its own). A compiler change left 98% of the arm64 binaries
# byte-identical (E: 14 of 743 differ; openenum: 8 of 742, all C;
# ~/xo-lead/notes/lang2/artifact-keyed-tests.md), where its LLVM IR differed
# in nearly every golden. Prints "all", "none" or the comma separated
# names, with a line for the log in $logs/bin-select.txt; every golden when
# the change touches the golden tests, the test harness or the selection,
# or when a build of the base fails. The full tier on main (ctier.sh) runs
# every golden. Off by default (lead, 2026-11-15: on E it made the arm64
# step 1,006 s against 912 s on host 2; a selection mode is on by default
# only once it measures a net gate-time win on its host):
# XO_GATE_BIN_SELECT=1 turns it on, force also skips the touched-files check.
bin_select() {
  local be=$1 base files T bx t0 sel n ntot
  local note="$logs/bin-select.txt"
  t0=$(date +%s)
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  files=$({ git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; })
  # XO_GATE_BIN_SELECT=force skips this check (for proving the selection
  # on a branch that carries a change to it)
  if [ "${XO_GATE_BIN_SELECT:-0}" != force ] && echo "$files" | grep -q -E '^(internal/(arm64|llvm)/[^/]+$|internal/xotool/|scripts/xobuild/|scripts/golden-bin-select\.py$|go\.(mod|sum)$)'; then
    echo "every golden (the change touches the golden tests, internal/xotool, scripts/xobuild or the selection)" >"$note"; echo all; return
  fi
  T=$(mktemp -d "${TMPDIR:-/tmp}/xo-gate-bin.XXXXXX")
  if ! { git worktree add -q --detach "$T/tree" "$base" && (cd "$T/tree" && go run ./scripts/xobuild); } >"$logs/bin-select-base.txt" 2>&1 || [ ! -x "$T/tree/bin/xo" ]; then
    echo "every golden (the merge base's xo did not build: $logs/bin-select-base.txt)" >"$note"
    git worktree remove --force "$T/tree" >/dev/null 2>&1; rm -rf "${T:?}"; echo all; return
  fi
  cp "$T/tree/bin/xo" "$T/xo-base"
  mkdir -p "$T/src" && cp -R internal/llvm/testdata/golden "$T/src/golden"
  ls "$T/src/golden" | sort >"$T/names.txt"
  ntot=$(wc -l <"$T/names.txt" | tr -d ' ')
  # both sides build to one output path, renamed after: a Mach-O's code
  # signature and UUID take the output's path
  # (gotchas/darwin-binary-identity-output-name.md)
  bin_build_side() { # XO STDDIR OUT
    rm -rf "$T/std" "$T/o" && cp -R "$2" "$T/std" && mkdir -p "$T/o"
    XO_BIN_CACHE=off XO_BACKEND_CACHE=off XO_FE_CACHE=off XO_PART_CACHE=off XO_CHK_CACHE=off XO_CLANG_CACHE=off XO_BACKEND_CACHE_DIR="$T/bcache" \
      BX="$1" XSTD="$T/std" SRC="$T/src/golden" OUT="$T/o" BE="$be" \
      xargs -P "${XO_GATE_BIN_JOBS:-$(ir_jobs)}" -n 1 sh -c 'mkdir -p "$OUT/$0" && cd "$SRC/$0" && XO_STD="$XSTD" "$BX" build --backend="$BE" -o "$OUT/$0/prog" . >"$OUT/$0/log" 2>&1; :' <"$T/names.txt"
    mv "$T/o" "$3"
  }
  bin_build_side "$T/xo-base" "$T/tree/std" "$T/a"
  bin_build_side "$XO_CLI" "$PWD/std" "$T/b"
  sel=$(python3 scripts/golden-bin-select.py "$T/a" "$T/b" "$T/src/golden" "$T/names.txt" 2>"$T/count.txt")
  # the goldens whose own directory the branch changes or adds: both sides
  # built this branch's sources
  sel=$( { echo "$sel"; echo "$files" | sed -n 's|^internal/llvm/testdata/golden/\([^/]*\)/.*|\1|p'; } | sort -u | grep . | grep -x -F -f "$T/names.txt")
  n=$(echo "$sel" | grep -c .)
  echo "$n of $ntot goldens by their $be binaries ($(sed 's/^golden-bin-select: //' "$T/count.txt"); $(($(date +%s) - t0)) s): $(echo "$sel" | paste -sd' ' -)" >"$note"
  printf '%s\t(binselect)\t%s\tselect\n' "$be" "$(($(date +%s) - t0))" >>"$logs/times.tsv"
  git worktree remove --force "$T/tree" >/dev/null 2>&1; rm -rf "${T:?}"
  if [ "$n" = 0 ]; then echo none; else echo "$sel" | paste -sd, -; fi
}
bin_select_ok() {
  [ "${XO_GATE_BIN_SELECT:-0}" != 0 ] || return 1
  [ "${XO_GATE_LLVM:-}" != all ] && [ "${XO_GATE_TESTS:-}" != all ]
}
# ir_first_wait STEP: with XO_GATE_IR_FIRST=1 in a concurrent gate that
# runs llvm, every other step but repo starts once the llvm step's golden
# selection is made, so HEAD's IR builds have the host to themselves at
# normal priority (lead, 2026-11-15: 1,253 s niced beside arm64 and cmd
# on host 1, 288 s alone; bench/results/gate-time-2026-11-14.md). The
# wait is not counted in the step's time; times.tsv records it.
ir_first_wait() {
  [ "${XO_GATE_IR_FIRST:-0}" = 1 ] && [ "$conc" = 1 ] || return 0
  case " $steps " in *" llvm "*) ;; *) return 0 ;; esac
  case $1 in llvm | repo) return 0 ;; esac
  local t0 w=0
  t0=$(date +%s)
  while [ ! -s "$logs/golden-sel.txt" ] && [ "$w" -lt "${XO_GATE_ARM64_SEL_WAIT:-3600}" ]; do
    sleep 5; w=$((w + 5))
  done
  printf '%s	(ir-first wait)	%s	wait
' "$1" "$(($(date +%s) - t0))" >>"$logs/times.tsv"
}
# run_step: one named step, its wall time recorded in times.tsv as
# "STEP<TAB>=<TAB>SECONDS<TAB>step"
run_step() {
  local t1 rc
  ir_first_wait "$1"
  t1=$(date +%s)
  run_step_body "$1"
  rc=$?
  printf '%s\t=\t%s\tstep\n' "$1" "$(($(date +%s) - t1))" >>"$logs/times.tsv"
  return $rc
}
run_step_body() {
  case $1 in
  selftest-*)
    # scripts/lead/gate_failfast_test.sh's steps (XO_GATE_SELFTEST=1):
    # fail after 2 s, sleep 60 s, or pass at once
    case $1 in
    selftest-fail) sleep 2; failed=1; echo "selftest-fail: FAIL (on purpose)" ;;
    selftest-sleep)
      echo '--- PASS: TestSelf/first (0.01s)' >"$logs/$1.txt"
      sleep 60; echo "selftest-sleep: ok" ;;
    *) echo "$1: ok" ;;
    esac ;;
  short)
    # -p and -parallel capped (2026-11-07): uncapped, -short's parallel
    # packages and tests passed 16 GB on the Mac (six packages at once,
    # each with parallel xo builds); XO_SHORT_P and XO_SHORT_PARALLEL set them
    go test -short -count=1 -p="${XO_SHORT_P:-2}" -parallel="${XO_SHORT_PARALLEL:-4}" $(go list ./... | grep -v /runs/) >"$logs/short.txt" 2>&1
    nf=$(grep -c '^--- FAIL' "$logs/short.txt")
    [ "$nf" -gt 0 ] && failed=1
    echo "short fails: $nf" ;;
  arm64)
    sk=$(gate_skip internal/arm64)
    [ -s "$logs/internal-arm64-areas.txt" ] && echo "arm64 areas: $(cat "$logs/internal-arm64-areas.txt")"
    if bin_select_ok; then
      # decision 0657: the goldens whose arm64 binaries differ, selected
      # while the package's other tests run
      ( bin_select arm64 >"$logs/arm64-binsel.txt.tmp" 2>>"$logs/bin-select-base.txt"; mv "$logs/arm64-binsel.txt.tmp" "$logs/arm64-binsel.txt" ) &
      bpid=$!
      presk="^($arm64_sel_tests)\$"
      [ -n "$sk" ] && presk="^($arm64_sel_tests|${sk#^(}"
      full arm64 ./internal/arm64 -skip "$presk"
      wait "$bpid"
      asel=$(cat "$logs/arm64-binsel.txt" 2>/dev/null); [ -n "$asel" ] || asel=all
      echo "arm64 goldens: $(cat "$logs/bin-select.txt" 2>/dev/null)"
      case $asel in
        all) full arm64sel ./internal/arm64 -run "^($arm64_sel_tests)\$" ${sk:+-skip "$sk"} ;;
        *) XO_ARM64_GOLDENS=$asel full arm64sel ./internal/arm64 -run "^($arm64_sel_tests)\$" ${sk:+-skip "$sk"} ;;
      esac
      # what ran, so a selection that matched no golden shows
      echo "arm64 golden subtests: $(grep -c -E -- '--- PASS: Test[A-Za-z]+/[^ ]+' "$logs/arm64sel.txt") passed, $(grep -c -E -- '--- FAIL: Test[A-Za-z]+/[^ ]+' "$logs/arm64sel.txt") failed, $(grep -c -E -- '--- SKIP: Test[A-Za-z]+/[^ ]+' "$logs/arm64sel.txt") skipped"
    elif arm64_select_ok; then
      # the tests that build every golden run after the others, on the
      # llvm step's selection (arm64_goldens)
      presk="^($arm64_sel_tests)\$"
      [ -n "$sk" ] && presk="^($arm64_sel_tests|${sk#^(}"
      full arm64 ./internal/arm64 -skip "$presk"
      asel=$(arm64_goldens)
      echo "arm64 goldens: $asel"
      case $asel in
        all) full arm64sel ./internal/arm64 -run "^($arm64_sel_tests)\$" ${sk:+-skip "$sk"} ;;
        *) XO_ARM64_GOLDENS=$asel full arm64sel ./internal/arm64 -run "^($arm64_sel_tests)\$" ${sk:+-skip "$sk"} ;;
      esac
    else
      full arm64 ./internal/arm64 ${sk:+-skip "$sk"}
    fi ;;
  llvm) llvm_step ;;
  irmanifest)
    # HEAD's IR manifest alone, into the cache (scripts/lead/irman.sh
    # runs it for main's commits and shares the file with every host)
    ir_cache_setup
    hm="$cache/$(git rev-parse HEAD)-$mcode.txt"
    # skipped, not failed, when the disk is short (lead, 2026-11-15: under
    # XO_GATE_IRMAN_MIN_GB, default 25, free after offload.sh's prune)
    irfree=$(ir_free_gb .)
    if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
      echo "irmanifest: FAIL (uncommitted changes: a manifest is kept by commit)"; failed=1
    elif [ -n "$irfree" ] && [ "$irfree" -lt "${XO_GATE_IRMAN_MIN_GB:-25}" ]; then
      echo "irmanifest: skipped: $irfree GB free here, under ${XO_GATE_IRMAN_MIN_GB:-25}"
    elif [ -s "$hm" ] || {
      # at ir_jobs' full count and not niced, so it finishes (lead,
      # 2026-11-15: niced at half width on a shared host 3 it took over 5
      # times test1's 277 s; irman.sh runs it on host 2 and limits how
      # often instead); its wall and CPU time are logged for irman.sh
      irj=${XO_GATE_IR_PARALLEL:-$(ir_jobs)}
      # this shell's own and its waited children's CPU ticks (its PID read
      # here: inside $( ) BASHPID names the substitution's subshell)
      me=$BASHPID
      t0=$(date +%s); c0=$(awk '{print $14 + $15 + $16 + $17}' /proc/$me/stat 2>/dev/null)
      XO_GATE_IR_NICE=0 ir_head_manifest "$hm.tmp$$" "$irj" "${hm%.txt}.deps.tmp$$" && mv "${hm%.txt}.deps.tmp$$" "${hm%.txt}.deps" && mv "$hm.tmp$$" "$hm" &&
        echo "irmanifest: built in $(($(date +%s) - t0)) s wall, $(( ($(awk '{print $14 + $15 + $16 + $17}' /proc/$me/stat 2>/dev/null || echo 0) - ${c0:-0}) / $(getconf CLK_TCK) )) s CPU, -parallel $irj"
    }; then
      echo "irmanifest: $hm ($(wc -l <"$hm" | tr -d ' ') goldens)"
    else
      rm -f "$hm.tmp$$" "${hm%.txt}.deps.tmp$$"; echo "irmanifest: FAIL ($logs/llvm-ir-head.txt)"; failed=1
    fi ;;
  stdcov) stdcov_step ;;
  corpora)
    if go_backend; then
      full corpora ./internal/stdtest -run '^TestStdCorpora$/^go/'
      # the native-against-Go differential tests of internal/llvm
      # (TestTimeDiff, TestNumDiff, TestJSONTextDiff, ...: lead,
      # 2026-10-24: 7822046d4 made the Go backend's RFC 3339 parse strict,
      # passed its gate, and TestTimeDiff, a -short test the gate never
      # ran, kept main red for 1 h 30 min)
      # -timeout 20m (later than full's 120m, so it wins): the step takes
      # 161 s on the second host, and a hang prints its stacks at 20 min
      # instead of holding a host for two hours (lead, 2026-11-07:
      # TestHTTP1ParseDiff hung at afb174dc1 and three agents chased the
      # wrong cause for 1 h 30 min before a stack named it)
      full diffs ./internal/llvm -run 'Diff$' -timeout 20m
    else
      echo "corpora: skipped (no std/rt, compiler/gen or std module with a std/rt twin changed)"
    fi ;;
  cmd)
    sk=$(gate_skip cmd/xo)
    [ -s "$logs/cmd-xo-areas.txt" ] && echo "cmd areas: $(cat "$logs/cmd-xo-areas.txt")"
    full cmd ./cmd/xo ${sk:+-skip "$sk"} ;;
  # -parallel capped (2026-11-12, aws-43's tier): uncapped it is the
  # machine's CPU count, and internal/gen's tests each build a Go program
  # (about 1.3 GB apiece), so the Mac's 30 cores peaked at 39 GB; at 8 the
  # step took 336 s and 13.3 GB on the Mac (hosts with 12 to 18 CPUs: 242
  # to 469 s, 11 to 18 GB). XO_GEN_PARALLEL sets it.
  gen) full gen ./internal/gen -parallel "${XO_GEN_PARALLEL:-8}" ;;
  fuzz)
    # the private-module probes only (the http parsers and neturl), at
    # their default counts, non -short: they skip under -short. 20 s alone,
    # 42 s while corpora-all ran (both measured 2026-10-26).
    full fuzz ./internal/stdtest -run '^(TestHttpHeadFuzz|TestHttpResponseFuzz|TestHttpSmuggle|TestHttpUrlFuzz)$' ;;
  corpora-all)
    # every corpus of internal/stdtest on every backend, non -short (under
    # -short the corpora run the host backend alone). 10 min 32 s measured
    # 2026-10-26, mostly the LLVM corpora's clang links.
    full corpora ./internal/stdtest -run '^TestStdCorpora$' ;;
  corpuscheck)
    # stdcorpus -check: every committed corpus against its generator, so a
    # change to Go, an oracle or a generator shows (lead, 2026-11-15: a
    # check that always reports stale hides real drift). 26 s and 1.8 GB
    # on the Mac, 2026-11-15 (generators run one at a time). The oracles
    # read their Go modules from the module cache (GOPROXY=off), so the
    # step runs where they are: the Mac's tier steps.
    if go run ./scripts/stdcorpus -check >"$logs/corpuscheck.txt" 2>&1; then
      echo "corpuscheck: every corpus is current"
    else
      failed=1
      echo "corpuscheck: FAIL: a corpus differs from its generator ($logs/corpuscheck.txt)"
      grep -E 'stale|panic|error' "$logs/corpuscheck.txt" | head -8
    fi ;;
  alloff)
    off=$(sed -n '/^pub fn passes()/,/^}/p' compiler/arm64/passes.xo | grep -o '"[a-z0-9_]*"' | tr -d '"' | sed 's/^/-/' | paste -sd, -)
    XO_PASSES=$off full alloff ./internal/arm64 -run '^TestGoldenRun$' ;;
  fp)
    tmp=$(mktemp -d "${TMPDIR:-/tmp}/xo-gate-fp.XXXXXX")
    cp bin/xo "$tmp/seed"
    # bin/xo was just built from this tree, so it is s1 (one build less)
    if XO_BOOTSTRAP_SEED_IS_S1=1 scripts/bootstrap-xo.sh "$tmp/seed" "$tmp/out" >"$logs/fp.txt" 2>&1; then
      echo "fp: $(tail -1 "$logs/fp.txt")"
    else
      failed=1
      # the bootstrap's own logs, before the cleanup below deletes them (an
      # s3 failure on 2026-11-03 left only "see .../out/log.txt", and three
      # reruns passed); XO_GATE_FP_KEEP=1 keeps the whole output (s2 and
      # s3 with their assembly for scripts/identity, a few GB)
      for f in log.txt times.txt hello.txt; do
        [ -f "$tmp/out/$f" ] && cp "$tmp/out/$f" "$logs/fp-$f"
      done
      echo "fp: FAIL ($logs/fp.txt, $logs/fp-log.txt)"
      if [ "${XO_GATE_FP_KEEP:-}" = 1 ]; then
        mv "$tmp/out" "$logs/fp-out" && echo "fp: output kept in $logs/fp-out"
      fi
    fi
    rm -rf "$tmp" ;;
  warm)
    base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
    pw=$(mktemp -d "${TMPDIR:-/tmp}/xo-gate-warm.XXXXXX")
    if git worktree add -q --detach "$pw/tree" "$base" >"$logs/warm-base.txt" 2>&1 &&
      (cd "$pw/tree" && go run ./scripts/xobuild) >>"$logs/warm-base.txt" 2>&1; then
      XO_TEST_WARM_XO=$pw/tree/bin/xo full warm-across ./internal/arm64 -run '^TestFrontCacheAcrossPrograms$'
      XO_BACKEND_CACHE=verify full warm-verify ./internal/arm64 -run '^TestGoldenRun$'
    else
      failed=1
      echo "warm: the base's xo did not build ($logs/warm-base.txt)"
    fi
    git worktree remove --force "$pw/tree" >/dev/null 2>&1
    rm -rf "${pw:?}" ;;
  chkcache)
    full chkcache ./internal/arm64 -run '^TestModuleCheckAuto(Generic|Zones)?$' ;;
  warmseq)
    full warmseq ./internal/arm64 -run '^TestWarmBuildSequence$' ;;
  stdwasm)
    # every std test module under `xo test --backend=wasm` (decision 0610),
    # Node's WASI as the runner: a module passes when its summary line is
    # ok (its listed wasm skips name their reasons there); for the lead's
    # tier, about 4 minutes on host 2 (2026-11-11)
    if ! command -v node >/dev/null 2>&1; then
      failed=1; echo "stdwasm: FAIL (node not found: the wasm test modules run under Node's WASI)"
    else
      : >"$logs/stdwasm.txt"
      bad=0; nmod=0
      for m in $(find std -name '*_test.xo' | sed 's#^std/##; s#_test\.xo$##' | sort); do
        nmod=$((nmod + 1))
        out=$(XO_TEST_EXEC="node --no-warnings $PWD/compiler/wasm/run.mjs" timeout 900 bin/xo test --backend=wasm --count=1 --std-module="$m" 2>&1)
        sum=$(printf '%s\n' "$out" | grep -E '^(ok|FAIL): [0-9]+ passed' | tail -1)
        echo "$m: ${sum:-no summary}" >>"$logs/stdwasm.txt"
        case $sum in
          ok:*) ;;
          *) bad=$((bad + 1)); printf '%s\n' "$out" | tail -20 | sed "s#^#  [$m] #" >>"$logs/stdwasm.txt" ;;
        esac
      done
      [ "$bad" -gt 0 ] && failed=1
      echo "stdwasm: $((nmod - bad)) of $nmod std test modules ok on wasm ($logs/stdwasm.txt)"
    fi ;;
  esac
}
# stdcov_step: internal/repotest's TestStdAPICovered with the resolved
# sweep (stdcoverage.Sweep: `xo dev edges --own` of every program and the
# std graph once a bindings value), so a std declaration counts as covered
# only when a call resolves to it (owner, 2026-11-11). Not a few minutes:
# about an hour in tier 69 (NumCPU jobs, 32 GB peak on a 24 GB Mac beside
# selfbuild gates); with XO_STDCOV_JOBS (default 2) 417 s alone on host 2
# (18 CPUs, 2026-11-12), host memory 19.2 GB at the start to a 22.9 GB
# peak, so about 4 GB its own. Not in the fast set: it runs when named and
# in the 8-hourly tier, never by path.
stdcov_step() {
  XO_STDCOV_SWEEP=1 full stdcov ./internal/repotest -run '^TestStdAPICovered$'
}
# the checks after the named steps, by path
after_steps() {
  ir_first_wait after
  if [ "$chkcache_ran" = 0 ] && [ "${XO_GATE_CHKCACHE:-1}" != 0 ] && check_change; then
    full chkcache ./internal/arm64 -run '^TestModuleCheckAuto(Generic|Zones)?$'
  fi
  if [ "$warmseq_ran" = 0 ] && [ "${XO_GATE_WARMSEQ:-1}" != 0 ] && warmseq_change; then
    full warmseq ./internal/arm64 -run '^TestWarmBuildSequence$'
  fi
  if [ "${XO_GATE_SCALE:-1}" != 0 ] && scale_change; then
    full scale ./internal/arm64 -run '^TestSwitch65535Arms$'
  fi
  if [ "${XO_GATE_CACHE:-1}" != 0 ] && cache_change; then
    full caches ./internal/arm64 -run 'Cache|BinCache'
  fi
  if [ "${XO_GATE_STDSTORE:-1}" != 0 ] && stdstore_change; then
    full stdstore ./internal/arm64 -run '^TestStdStoreBuilds$'
  fi
  pre_ran clihelp || { [ "${XO_GATE_HELP:-1}" != 0 ] && help_change && full clihelp ./cmd/xo -run '^TestCLIHelp$'; }
  pre_ran stdgen || { [ "${XO_GATE_STDGEN:-1}" != 0 ] && stdgen_change && stdgen_check; }
  if [ "${XO_GATE_RACE:-1}" != 0 ] && race_change; then
    full race ./internal/llvm -run '^TestRaceSyncTable(Arm64)?$'
  fi
  local sm
  if [ "${XO_GATE_STDMODS:-1}" != 0 ] && sm=$(std_mods) && [ -n "$sm" ]; then
    full stdmods ./internal/stdtest -run "$sm"
  fi
}
# The proving checks, one function each: a passing one adds its name to
# ran. Serial order (codegen, wasm, cross, selfbuild, seedbuild, fieldgen)
# is the PUSHQ_RAN order.
prove_codegen() {
  if "$XO_CLI" dev codegen --backend=arm64,llvm test/codegen >"$logs/codegen.txt" 2>&1; then
    echo "codegen: $(tail -1 "$logs/codegen.txt")"; ran="$ran codegen"
  else
    failed=1; echo "codegen: FAIL ($logs/codegen.txt)"
  fi
}
prove_wasm() {
  # the goldens the branch touches, and the concurrency, atomics and map
  # sets pushq.sh names
  local g f0
  g=$(echo "$ch" | sed -n 's|^internal/[a-z0-9]*/testdata/golden/\([^/]*\)/.*|\1|p' | sort -u | paste -sd, -)
  f0=$failed; failed=0
  XO_WASM_GOLDENS="${g:+$g,}conc_*,atomic_ops,map_*" full wasm ./internal/selfhost -run '^TestSelfhostWasm$'
  # the same goldens and the file ones as wasip2/wasm commands under
  # wasmtime (TestWasmCommandGoldens): the Node runner above runs the
  # preview 1 build, which hid a component-only fd_seek stub until the
  # tier (gotchas/wasip2-p1-call-stubbed-enosys.md; lead, 2026-11-11).
  # The offload hosts keep wasmtime in /work/tools/bin; where it is
  # missing (the Mac) the test would skip, so say so instead
  local wp=$PATH
  [ -d /work/tools/bin ] && wp=/work/tools/bin:$PATH
  if PATH=$wp command -v wasmtime >/dev/null && PATH=$wp command -v wasm-tools >/dev/null; then
    PATH=$wp XO_TEST_WASM_TOOLS=1 XO_WASM_GOLDENS="${g:+$g,}conc_*,atomic_ops,map_*,file_*,fs_*" full wasmcmd ./internal/selfhost -run '^TestWasmCommandGoldens$'
  else
    echo "wasmcmd: skipped (no wasmtime or wasm-tools here; an offload host runs it)"
  fi
  [ "$failed" = 0 ] && ran="$ran wasm"
  [ "$f0" = 1 ] && failed=1
}
prove_cross() {
  local f0=$failed; failed=0
  full cross ./internal/arm64 -run '^TestAmd64CrossBuilds$'
  [ "$failed" = 0 ] && ran="$ran cross"
  [ "$f0" = 1 ] && failed=1
}
# what compiler/xo builds from: one cold self build with the new xo under
# an 8 GB budget (owner, 2026-11-06: 11a08a448's inliner runaway passed
# 22 GB building xo and reached main, found only when builders failed)
prove_selfbuild() {
  local t1
  t1=$(date +%s)
  # XO_LOCKCHECK=1: a heap region window (decision 0633) that switches
  # fibers stops the build; the release must have run with no thread left
  # in region mode (2026-11-14: a preempted lazy check left the mode on,
  # the release was skipped in 3 of 4 self builds, and one gate's RSS
  # did not show it; gotchas/region-mode-preempted.md)
  if XO_MAX_MEM=8G XO_BACKEND_CACHE=off XO_BIN_CACHE=off XO_WHOLE_BUILD=off XO_LOCKCHECK=1 XO_HEAP_REGION_STATS=1 "$XO_CLI" build --phase-times -o "$logs/selfbuild-xo" ./compiler/xo >"$logs/selfbuild.txt" 2>&1; then
    local rel
    rel=$(grep -m1 '^xo heap region: release' "$logs/selfbuild.txt")
    if [ -z "$rel" ] || ! printf '%s' "$rel" | grep -q ' 0 threads in region mode' || printf '%s' "$rel" | grep -q 'pooled -1 units'; then
      failed=1; echo "selfbuild: FAIL (the heap region release did not run: ${rel:-no release line}; $logs/selfbuild.txt)"
    else
      echo "selfbuild: $(($(date +%s) - t1)) s, $(grep -o 'peak [0-9]* MB' "$logs/selfbuild.txt" | sort -k2 -n | tail -1)"; ran="$ran selfbuild"
    fi
  else
    failed=1; echo "selfbuild: FAIL ($logs/selfbuild.txt: $(grep -m1 -E 'memory budget|error' "$logs/selfbuild.txt"))"
  fi
  rm -f "$logs/selfbuild-xo"
}
# seedbuild: compiler/xo built by the offload host's seed (XO_SEED,
# which offload-gate-remote.sh exports) under XO_INLINE_STRICT=1, the
# hourly fixed point's s1, with caches of its own so no cached build of
# an ancestor stands in (lead, 2026-11-08: db70f6030 passed its gate,
# whose xo came from the newest cached ancestor, and failed
# XO_INLINE_STRICT from the 0.44 seed on the hosts). The Mac has no
# XO_SEED: run it there with offload-gate.sh WORKTREE seedbuild.
prove_seedbuild() {
  if [ -z "${XO_SEED:-}" ] || [ ! -x "${XO_SEED:-}" ]; then
    failed=1; echo "seedbuild: FAIL (no XO_SEED: it runs on an offload host, ~/xo-lead/agents/offload-gate.sh WORKTREE seedbuild)"
  else
    local t1 sb nolibc
    t1=$(date +%s) sb=$logs/seedbuild nolibc=${XO_NOLIBC:-}
    # as xobuild (internal/xotool hostBackend): xo links with the C
    # library on Linux
    [ "$(uname -s)" = Linux ] && [ -z "$nolibc" ] && nolibc=0
    rm -rf "${sb:?}" && mkdir -p "$sb"
    # first the seed's check of compiler/xo with this std, as xotool
    # CheckXo runs before every build of xo from a seed: it reads all of
    # std/native, the build below only compiler/xo's closure (#57)
    if ! gate_seed_check "$XO_SEED" "$wt" "$logs/seedcheck.txt"; then
      failed=1; echo "seedbuild: FAIL (the seed's check, XO_STD=$wt/std $XO_SEED check compiler/xo: $logs/seedcheck.txt: $(grep -m1 -E ' error |^error|: error' "$logs/seedcheck.txt" || tail -1 "$logs/seedcheck.txt")) with $("$XO_SEED" version 2>/dev/null)"
    elif XO_NOLIBC=$nolibc XO_INLINE_STRICT=1 XO_MAX_MEM=8G XO_BACKEND_CACHE=off XO_BIN_CACHE=off XO_BUILD_DIR="$sb/build" XO_RTOBJ_CACHE="$sb/rtobj" \
      XO_BACKEND_CACHE_DIR="$sb/backend" "$XO_SEED" build --std="$wt/std" -o "$sb/xo" ./compiler/xo >"$logs/seedbuild.txt" 2>&1; then
      echo "seedbuild: $(($(date +%s) - t1)) s with $("$XO_SEED" version 2>/dev/null)"; ran="$ran seedbuild"
    else
      failed=1; echo "seedbuild: FAIL ($logs/seedbuild.txt: $(grep -m1 -E 'XO[0-9]{4}|error|verif|panic' "$logs/seedbuild.txt")) with $("$XO_SEED" version 2>/dev/null)"
    fi
    rm -rf "${sb:?}/build" "${sb:?}/rtobj" "${sb:?}/backend" "${sb:?}/xo"
  fi
}
# a generated block (scripts/gen's output in the runtime) against its
# generator, run by this xo (lead, 2026-11-08: a hand edit of sha3.xo's
# Keccak reached main)
prove_fieldgen() {
  local f0=$failed; failed=0
  full fieldgen ./internal/llvm -run '^TestFieldGeneratorsInSync$'
  if [ "$failed" = 0 ] && ! grep -q -- '--- PASS: TestFieldGeneratorsInSync' "$logs/fieldgen.txt"; then
    failed=1; echo "fieldgen: FAIL (TestFieldGeneratorsInSync did not run: $logs/fieldgen.txt)"
  fi
  [ "$failed" = 0 ] && ran="$ran fieldgen"
  [ "$f0" = 1 ] && failed=1
}
# prove_checkall: `xo check` over every Xo program of the repository,
# test files included, with this xo and the merge base's
# (scripts/checkall.sh: test/, cmd/xo/testdata, internal/*/testdata and
# each std module with tests of its own), failing on an error only this
# xo reports; check only, about a minute a side (lead, 2026-11-15, #54:
# decision 0662's checker change broke std/sql's corpus tests, cmd/xo
# TestCheckFlag's program and the native_sqlite golden, which its gate
# did not check). Required for compiler/check (gatelib
# gate_required_steps).
prove_checkall() {
  local base T
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  T=$(mktemp -d "${TMPDIR:-/tmp}/xo-gate-checkall.XXXXXX")
  if ! { git worktree add -q --detach "$T/tree" "$base" && (cd "$T/tree" && go run ./scripts/xobuild); } >"$logs/checkall-base.txt" 2>&1 || [ ! -x "$T/tree/bin/xo" ]; then
    failed=1; echo "checkall: FAIL (the merge base's xo did not build: $logs/checkall-base.txt)"
  elif scripts/checkall.sh "$T/tree/bin/xo" "$logs/checkall-base-errors.txt" >>"$logs/checkall.txt" 2>&1 &&
    scripts/checkall.sh "$XO_CLI" "$logs/checkall-head-errors.txt" >>"$logs/checkall.txt" 2>&1; then
    LC_ALL=C comm -13 "$logs/checkall-base-errors.txt" "$logs/checkall-head-errors.txt" >"$logs/checkall-new.txt"
    if [ -s "$logs/checkall-new.txt" ]; then
      failed=1; echo "checkall: FAIL ($(wc -l <"$logs/checkall-new.txt" | tr -d ' ') errors the merge base's xo does not report: $logs/checkall-new.txt)"
      head -10 "$logs/checkall-new.txt" | cut -c1-200
    else
      echo "checkall: ok ($(tail -1 "$logs/checkall.txt"); none new)"; ran="$ran checkall"
    fi
  else
    failed=1; echo "checkall: FAIL ($logs/checkall.txt)"
  fi
  git worktree remove --force "$T/tree" >/dev/null 2>&1; rm -rf "${T:?}"
}
# the compiler's own xo tests (compiler/..._test.xo) with this xo (lead,
# 2026-11-12: a parser change broke compiler/parser's test, which only
# hourly.sh ran); a few seconds
prove_xounit() {
  if "$XO_CLI" test --count=1 compiler/... >"$logs/xounit.txt" 2>&1; then
    echo "xounit: $(tail -1 "$logs/xounit.txt")"; ran="$ran xounit"
  else
    failed=1; echo "xounit: FAIL ($logs/xounit.txt: $(grep -m1 -- '--- FAIL' "$logs/xounit.txt"))"
  fi
}
# prove_stdfast: the std fast lane (owner, 2026-11-15, CLAUDE.md "Std
# Proven push"): for each std module off compiler/xo's closure the change
# touches (gatelib gate_stdfast_mods), `xo test --std-module=NAME`, its
# corpora, and `xo vet --cost --std-module=NAME` (every function of it
# lowered; a cost note TestVetCostStd would refuse fails: gatelib
# gate_vetcost_new; seconds), then stdchecks and stdgen -check (the
# pre-check's when it ran them); minutes, no golden suites, the tier on
# main runs the rest
prove_stdfast() {
  local sfb mods m bad=0
  sfb=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  mods=$({ git diff --name-only "$sfb" HEAD; git status --porcelain | cut -c4-; } | sort -u | gate_stdfast_mods)
  local cdirs="" d t
  for m in $mods; do
    # the module's own tests (std/M_test.xo, std/M/*_test.xo): most std
    # modules have none, their tests are internal/stdtest's corpora below
    # (the probe of 2026-11-15: "xo test: the std module csv has no tests")
    t=0
    for d in std/"$m"_test.xo std/"$m"/*_test.xo; do [ -f "$d" ] && t=1; done
    if [ "$t" = 0 ]; then
      echo "stdfast $m: no xo tests of its own"
    elif "$XO_CLI" test --count=1 --std-module="$m" >"$logs/stdfast-$m.txt" 2>&1; then
      echo "stdfast $m: $(tail -1 "$logs/stdfast-$m.txt")"
    else
      bad=1; echo "stdfast $m: FAIL ($logs/stdfast-$m.txt: $(grep -m1 -- '--- FAIL' "$logs/stdfast-$m.txt"))"
    fi
    # its cost notes against cmd/xo/testdata/vetcost-std-allow.txt
    # (TestVetCostStd's rule, which the tier runs over bench/perf)
    # (std/tzdata holds data, no .xo file: nothing to vet)
    t=0
    for d in std/"$m".xo std/"$m"/*.xo; do [ -f "$d" ] && t=1; done
    if [ "$t" = 1 ]; then
      "$XO_CLI" vet --json --cost --std-module="$m" >"$logs/stdfast-vet-$m.json" 2>"$logs/stdfast-vet-$m.err"
      t=$?
    else
      t=skip
    fi
    if [ "$t" = skip ]; then
      echo "stdfast vet $m: no .xo files"
    elif [ "$t" != 0 ]; then
      bad=1; echo "stdfast vet $m: FAIL (xo vet exited $t: $logs/stdfast-vet-$m.err: $(head -1 "$logs/stdfast-vet-$m.err"))"
    elif gate_vetcost_new "$logs/stdfast-vet-$m.json" std cmd/xo/testdata/vetcost-std-allow.txt >"$logs/stdfast-vet-$m.txt" 2>&1; then
      echo "stdfast vet $m: no new cost notes"
    else
      bad=1; echo "stdfast vet $m: FAIL, fix the code (knowledgebase/std-performance-guide.md) or allow it with a measured reason in cmd/xo/testdata/vetcost-std-allow.txt (only for a line a bench/perf program reaches: TestVetCostStd refuses an allow line no note there matches):"; sed 's/^/  /' "$logs/stdfast-vet-$m.txt"
    fi
    # its corpora: internal/stdtest/testdata/corpus/M and M* (csv, csvfuzz)
    for d in internal/stdtest/testdata/corpus/"$m" internal/stdtest/testdata/corpus/"$m"*; do
      [ -d "$d" ] && cdirs="$cdirs ${d##*/}"
    done
  done
  cdirs=$(echo $cdirs | tr ' ' '\n' | sort -u | paste -sd'|' -)
  if [ -n "$cdirs" ]; then
    if XO_CLI="$XO_CLI" go test -count=1 -timeout 30m ./internal/stdtest -run "^TestStdCorpora\$/.*/^($cdirs)\$" >"$logs/stdfast-corpora.txt" 2>&1; then
      echo "stdfast corpora ($cdirs): $(tail -1 "$logs/stdfast-corpora.txt")"
    else
      bad=1; echo "stdfast corpora ($cdirs): FAIL ($logs/stdfast-corpora.txt: $(grep -m1 -- '--- FAIL' "$logs/stdfast-corpora.txt"))"
    fi
  else
    echo "stdfast: no corpora for $mods"
  fi
  pre_ran stdchecks || { [ "${XO_GATE_STDCHECKS:-1}" != 0 ] && stdchecks_check; }
  pre_ran stdgen || { [ "${XO_GATE_STDGEN:-1}" != 0 ] && stdgen_change && stdgen_check; }
  if [ "$bad" = 1 ]; then failed=1; else ran="$ran stdfast"; fi
}
# The concurrent jobs (conc=1): job KEY CMD... starts CMD in the
# background once fewer than gjobs jobs run, with its output buffered in
# $logs/.job-KEY.out and printed when it ends; its failed and ran go to
# files that pool_wait folds in, in the order the jobs started.
jobkeys="" jobpids=""
job() {
  local k=$1; shift
  ff_check
  while [ "$ff" = 0 ] && [ "$(jobs -pr | wc -l)" -ge "$gjobs" ]; do sleep 2; ff_check; done
  if [ "$ff" = 1 ]; then ff_skipped="$ff_skipped $k"; return 0; fi
  rm -f "$logs/.job-$k".*
  (
    failed=0 ran="" t1=$(date +%s)
    "$@" >"$logs/.job-$k.out" 2>&1
    echo "$failed" >"$logs/.job-$k.failed"
    echo "$ran" >"$logs/.job-$k.ran"
    cat "$logs/.job-$k.out"
    echo "gate.sh: $k done in $(($(date +%s) - t1)) s"
  ) &
  jobkeys="$jobkeys $k" jobpids="$jobpids $k:$!"
}
# ff_check: with fail fast on, once a job has ended failed (or a check of
# this shell, such as repo, failed), every job still running is stopped
# (gate_tree_kill: the jobs' own $! only) and ff is set, so job starts
# no more; a stopped job's output so far is kept as $logs/KEY.stopped.txt
ff_check() {
  ff_on || return 0
  local kp k p
  if [ "$failed" = 1 ] && [ "$ff" = 0 ]; then ff=1; ff_by=${ff_by:-${ff_last:-repo}}; fi
  for kp in $jobpids; do
    [ "$ff" = 1 ] && break
    k=${kp%%:*}
    [ "$(cat "$logs/.job-$k.failed" 2>/dev/null)" = 1 ] && { ff=1; ff_by=$k; }
  done
  [ "$ff" = 1 ] || return 0
  for kp in $jobpids; do
    k=${kp%%:*} p=${kp#*:}
    [ -e "$logs/.job-$k.failed" ] && continue
    case " $ff_stopped " in *" $k "*) continue ;; esac
    kill -0 "$p" 2>/dev/null || continue
    gate_tree_kill "$p" 2>/dev/null
    ff_stopped="$ff_stopped $k"
    cp "$logs/.job-$k.out" "$logs/$k.stopped.txt" 2>/dev/null
    echo "gate.sh: $k stopped (fail fast: $ff_by failed; its output so far: $logs/$k.stopped.txt)"
  done
}
# pool_beat: every XO_GATE_HEARTBEAT_S seconds (default 600; 0: off)
# while jobs run, one line per running job with its test log's size and
# last finished test, so a long step is not silent and a hung one shows
# "no new lines" (lead, 2026-11-15: the tier's llvm step printed nothing
# for 77 minutes, and heavyd marked the lease STALLED after 45, though
# no test was slower than 387 s)
pool_beat() {
  local kp k f n prev last
  for kp in $jobpids; do
    k=${kp%%:*}
    kill -0 "${kp#*:}" 2>/dev/null || continue
    [ -e "$logs/.job-$k.failed" ] && continue
    f=$logs/$k.txt n=0 last=""
    if [ -f "$f" ]; then
      n=$(wc -l <"$f" | tr -d ' ')
      last=$(grep -E -- '--- (PASS|FAIL|SKIP): ' "$f" | tail -1 | sed 's/^ *//')
    fi
    prev=$(cat "$logs/.job-$k.beat" 2>/dev/null)
    echo "$n" >"$logs/.job-$k.beat"
    echo "gate.sh: $k running ($(($(date +%s) - tsteps)) s since the steps began); $k.txt $n lines$([ "$prev" = "$n" ] && echo ', no new lines since the last beat')${last:+; last: $last}"
  done
}
pool_wait() {
  local k kp alive hb=${XO_GATE_HEARTBEAT_S:-600} beat
  beat=$(date +%s)
  while :; do
    ff_check
    alive=0
    for kp in $jobpids; do kill -0 "${kp#*:}" 2>/dev/null && { alive=1; break; }; done
    [ "$alive" = 0 ] && break
    if [ "$hb" -gt 0 ] && [ $(($(date +%s) - beat)) -ge "$hb" ]; then
      pool_beat
      beat=$(date +%s)
    fi
    sleep 2
  done
  ff_check
  wait 2>/dev/null
  for k in $jobkeys; do
    # a job that left no status died before it finished: a failure
    [ "$(cat "$logs/.job-$k.failed" 2>/dev/null || echo 1)" = 0 ] || failed=1
    ran="$ran $(cat "$logs/.job-$k.ran" 2>/dev/null)"
    rm -f "$logs/.job-$k".*
  done
  jobkeys="" jobpids=""
}
corpora_group() { # corpora then corpora-all, as named: one job (shared logs)
  local s
  for s in $steps; do
    case $s in corpora | corpora-all) run_step "$s" ;; esac
  done
}
tsteps=$(date +%s)
if [ "$conc" = 1 ]; then
  echo "gate.sh: concurrent, at most $gjobs jobs:$steps"
  cg=0
  for step in $steps; do
    case $step in
    corpora | corpora-all)
      [ "$cg" = 1 ] && continue
      cg=1; job corpora corpora_group ;;
    *) job "$step" run_step "$step" ;;
    esac
  done
  job after after_steps
else
  ff_last=repo
  for step in $steps; do
    if ff_serial; then ff_skipped="$ff_skipped $step"; continue; fi
    ff_last=$step
    run_step "$step"
  done
  if ff_serial; then ff_skipped="$ff_skipped after"; else ff_last=after; after_steps; fi
fi
# The checks pushq.sh requires by path (its "Proving-test reminders"), run
# here so the push is not refused after the gate (lead, 2026-11-06
# bottleneck review: l2-tape at 03:42 and simd at 06:04 were refused for
# a wasm run their gates never did, each a second gate). The same paths
# as pushq.sh; the last line before GATE-DONE names what to pass as
# PUSHQ_RAN. XO_GATE_PUSHQ=0 skips both.
ran=""
base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
ch=$({ git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | grep -v '_test\.xo$')
# the checks: those the paths require (none with XO_GATE_PUSHQ=0; no
# cross with XO_GATE_CROSS=0) and every one named
req=""
if [ "${XO_GATE_PUSHQ:-1}" != 0 ]; then
  req=$(echo "$ch" | gate_required_steps)
  [ "${XO_GATE_CROSS:-1}" = 0 ] && req=$(echo " $req " | sed 's/ cross / /')
fi
# std's own checks (TestStdChecks: every native and runtime body checked
# with no diagnostic, so an XO0804 or other warning in std fails here, not
# first at pushq; owner, 2026-11-15) for a change to a std .xo file, a few
# seconds; XO_GATE_STDCHECKS=0 skips it
ff_serial
ff_last=stdchecks
if [ "$ff" = 1 ]; then
  echo "$ch" | grep -qE '^std/.*\.xo$' && ff_skipped="$ff_skipped stdchecks"
elif ! pre_ran stdchecks && [ "${XO_GATE_STDCHECKS:-1}" != 0 ] && echo "$ch" | grep -qE '^std/.*\.xo$'; then
  stdchecks_check
fi
plan=" $(gate_plan "$req" $named) "
echo "gate.sh: proving checks:${plan% }${plan:+ }(required by the paths: ${req:-none}; named:${named:- none})"
if [ "$conc" = 1 ]; then
  for p in codegen wasm cross fieldgen xounit stdfast checkall; do
    [[ "$plan" == *" $p "* ]] && job "$p" "prove_$p"
  done
  pool_wait
  for p in selfbuild seedbuild; do
    [[ "$plan" == *" $p "* ]] || continue
    if ff_serial; then ff_skipped="$ff_skipped $p"; continue; fi
    ff_last=$p
    "prove_$p"
  done
  # PUSHQ_RAN in the serial order
  r2=""
  for p in codegen wasm cross selfbuild seedbuild fieldgen xounit stdfast checkall; do
    [[ " $ran " == *" $p "* ]] && r2="$r2 $p"
  done
  ran=$r2
  echo "gate.sh: steps took $(($(date +%s) - tsteps)) s (concurrent, at most $gjobs jobs)"
else
  for p in codegen wasm cross selfbuild seedbuild fieldgen xounit stdfast checkall; do
    [[ "$plan" == *" $p "* ]] || continue
    if ff_serial; then ff_skipped="$ff_skipped $p"; continue; fi
    ff_last=$p
    "prove_$p"
  done
  echo "gate.sh: steps took $(($(date +%s) - tsteps)) s"
fi
if [ "$ff" = 1 ]; then
  echo "FAIL-FAST: $ff_by failed, so the gate stopped there; stopped while running:${ff_stopped:- none}; not run:${ff_skipped:- none} (XO_GATE_FAILFAST=0 runs every step)"
fi
[ -n "$ran" ] && echo "pushq: PUSHQ_RAN=\"$(echo $ran)\""
# pushq-ran (lead, 2026-11-06 bottleneck review: 6 pushes in 3 hours were
# refused for a PUSHQ_RAN their gate had printed but the agent did not
# pass): a passing gate records the checks it ran with the patch id of
# its change, in the worktree's git dir; pushq.sh uses them when
# PUSHQ_RAN is unset and the change's patch id still matches
pr=$(git rev-parse --git-dir 2>/dev/null)/pushq-ran
if [ "$failed" = 0 ] && [ -n "$ran" ]; then
  pid=$(git diff "$(git merge-base HEAD origin/main)" HEAD | git patch-id --stable | cut -d' ' -f1)
  [ -n "$pid" ] && echo "$pid $(echo $ran)" >"$pr"
fi
# the gate's test times (gate_times): kept for the daily check in the
# lead's directory when this machine has one, else printed as GATE-TIME
# lines (an offload host; offload-gate.sh files them on the Mac)
if [ -s "$logs/times.tsv" ]; then
  gt_head="# $(date -u +%FT%TZ) $(uname -sm) $(git rev-parse --short HEAD) $(basename "$wt") failed=$failed"
  gtd=${XO_LEAD_DIR:-$HOME/xo-lead}
  if [ -d "$gtd" ] && [ -z "${XO_GATE_TIMES_PRINT:-}" ]; then
    mkdir -p "$gtd/notes/gate-times"
    { echo "$gt_head"; cat "$logs/times.tsv"; } >"$gtd/notes/gate-times/$(date -u +%Y%m%dT%H%M%S)-$(basename "$wt")-$$.tsv"
  else
    { echo "$gt_head"; cat "$logs/times.tsv"; } | sed 's/^/GATE-TIME	/'
  fi
fi
echo GATE-DONE
exit $failed
