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
#            also implements (go_backend). Its
#            golden programs are only those the change touches (owner,
#            2026-10-22; llvm_goldens below): XO_GATE_LLVM=all runs
#            every one
#   corpora  internal/stdtest's differential corpora on the Go backend
#            (TestStdCorpora/go/) and internal/llvm's Test*Diff
#            differential tests: run when the branch changes std/rt,
#            compiler/gen, or a std module std/rt also implements
#            (go_backend, std_twin); skipped otherwise
#   cmd      the full tests of cmd/xo
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
#   alloff   internal/arm64's goldens with every optional pass off
#            (XO_PASSES, compiler/arm64 passes.xo)
#   fp       the self rebuild fixed point (scripts/bootstrap-xo.sh); its
#            output (about 3 GB) is removed after
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
#            (`xo dev edges` over every program, a few minutes): also run
#            after the steps named when the branch changes std, a program
#            the sweep checks, internal/stdcoverage or `xo dev edges`
#            (XO_GATE_STDCOV=0 skips that; owner, 2026-11-11)
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
# file per step. A step's failure is reported and the next step runs;
# the exit status is 1 when any failed.
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
cd "$wt" || exit 1
# gate_required_steps and gate_plan: the proving checks, as pushq.sh asks
# for them (the tree's own copy, so a gate and its pushq agree)
. scripts/lead/gatelib.sh || exit 1
t0=$(date +%s)
rm -f bin/xo
go run ./scripts/xobuild >"$logs/xobuild.txt" 2>&1 || { echo "XOBUILD-FAIL ($logs/xobuild.txt)"; exit 1; }
echo "xobuild $(($(date +%s) - t0)) s"
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
full() { # NAME PKG [go test args]: one package's full tests
  n=$1; p=$2; shift 2
  # -json keeps every test's time ($logs/NAME.json, the 40 slowest in
  # NAME.slow.txt); NAME.txt is its output as go test -v prints it
  go test -json -count=1 -timeout 120m "$p" "$@" >"$logs/$n.json" 2>"$logs/$n.err"
  rc=$?
  python3 -c 'import json,sys
for l in open(sys.argv[1], errors="replace"):
    try: e = json.loads(l)
    except ValueError: sys.stdout.write(l); continue
    sys.stdout.write(e.get("Output", ""))' "$logs/$n.json" >"$logs/$n.txt"
  cat "$logs/$n.err" >>"$logs/$n.txt"
  slowest "$logs/$n.json" >"$logs/$n.slow.txt"
  if [ "$rc" = 0 ]; then
    echo "$n: $(tail -1 "$logs/$n.txt")"
  else
    failed=1
    echo "$n: FAIL ($logs/$n.txt)"
    grep -E '^(    )?--- FAIL' "$logs/$n.txt" | head -10
  fi
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
stdcov_ran=0
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
llvm_goldens() {
  sel_note="$logs/llvm-select.txt"
  if [ "${XO_GATE_LLVM:-}" = all ]; then
    echo "every golden (XO_GATE_LLVM=all)" >"$sel_note"; echo all; return
  fi
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  files=$(git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-)
  if echo "$files" | grep -q -E '^(internal/llvm/[^/]+$|internal/xotool/|compiler/xo/|compiler/llvm/debug\.xo)'; then
    echo "every golden (the change touches internal/llvm's tests, internal/xotool, compiler/xo or the LLVM debug information)" >"$sel_note"; echo all; return
  fi
  if [ "$(uname)" = Darwin ]; then ucache=$HOME/Library/Caches; else ucache=${XDG_CACHE_HOME:-$HOME/.cache}; fi
  cache=${XO_LLVM_IR_CACHE:-$ucache/xo/llvm-ir}
  mkdir -p "$cache"
  # by commit and by the manifest code (its normalization) that made it
  bm="$cache/$(git rev-parse "$base")-$(git hash-object internal/llvm/golden_select_test.go | cut -c1-12).txt"
  if [ ! -s "$bm" ]; then
    pw=$(mktemp -d "${TMPDIR:-/tmp}/xo-gate-ir.XXXXXX")
    # the branch's manifest test in the base's tree: both sides hashed
    # by the same code (a base older than the test has none)
    if git worktree add -q --detach "$pw/tree" "$base" >"$logs/llvm-ir-base.txt" 2>&1 &&
      cp internal/llvm/golden_select_test.go "$pw/tree/internal/llvm/" &&
      (cd "$pw/tree" && go run ./scripts/xobuild) >>"$logs/llvm-ir-base.txt" 2>&1 &&
      (cd "$pw/tree" && XO_CLI=$pw/tree/bin/xo XO_LLVM_IR_MANIFEST="$bm.tmp$$" go test -count=1 -timeout 60m -parallel 4 -run '^TestGoldenIRManifest$' ./internal/llvm) >>"$logs/llvm-ir-base.txt" 2>&1; then
      mv "$bm.tmp$$" "$bm"
    fi
    rm -f "$bm.tmp$$"
    git worktree remove --force "$pw/tree" >/dev/null 2>&1
    rm -rf "${pw:?}"
  fi
  nm="$logs/llvm-ir.txt"
  rm -f "$nm"
  if [ ! -s "$bm" ] || ! XO_LLVM_IR_MANIFEST=$nm go test -count=1 -timeout 60m -parallel 4 -run '^TestGoldenIRManifest$' ./internal/llvm >"$logs/llvm-ir-head.txt" 2>&1 || [ ! -s "$nm" ]; then
    echo "every golden (no IR manifest: $logs/llvm-ir-base.txt, $logs/llvm-ir-head.txt)" >"$sel_note"; echo all; return
  fi
  names=$( { comm -3 "$bm" "$nm" | awk '{print $1}'; echo "$files" | sed -n 's|^internal/llvm/testdata/golden/\([^/]*\)/.*|\1|p'; } | sort -u | grep . )
  total=$(wc -l <"$nm" | tr -d ' ')
  if [ -z "$names" ]; then
    echo "0 of $total goldens (no golden's IR or directory changed)" >"$sel_note"; echo none; return
  fi
  echo "$(echo "$names" | wc -l | tr -d ' ') of $total goldens, and the -O2 sample: $(echo "$names" | paste -sd' ' -)" >"$sel_note"
  echo "$names" | paste -sd, -
}
llvm_step() {
    if [ -z "${XO_GATE_LLVM:-}" ] && native_only; then
      echo "llvm: skipped (native backend only change; XO_GATE_LLVM=1 runs it)"
    else
      sel=$(llvm_goldens)
      echo "llvm goldens: $(cat "$logs/llvm-select.txt")"
      if [ "$sel" = all ]; then unset XO_LLVM_GOLDENS; else export XO_LLVM_GOLDENS=$sel; fi
      if go_backend; then
        # every 16th golden on the Go backend too: the selection above
        # follows the LLVM IR, which a Go backend change leaves alone, so
        # TestGoldenGoParity ran none and passed (2026-11-06,
        # gotchas/go-parity-selected-by-llvm-ir.md); it now fails when it
        # runs none
        XO_TEST_GO_PARITY=1 XO_GO_PARITY_SAMPLE=${XO_GO_PARITY_SAMPLE:-16} full llvm ./internal/llvm
      else
        full llvm ./internal/llvm
      fi
      unset XO_LLVM_GOLDENS
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
for s in "$@"; do
  [ "$s" = short ] && has_short=1
  [ "$s" = repo ] && has_repo=1
done
if { [ "${XO_GATE_REPO:-1}" != 0 ] || [ "$has_repo" = 1 ]; } && [ "$has_short" = 0 ]; then
  full repo ./internal/repotest
fi
# the named steps, in order: pushq's proving checks are collected into
# named (they run after the steps, with the ones the paths require), an
# unknown name fails the gate, and a named chkcache keeps it from running
# a second time after the steps
steps=""
for step in "$@"; do
  case $step in
  codegen|selfbuild|seedbuild|wasm|cross|fieldgen)
    # pushq's proving checks: run after the named steps with the ones the
    # change's paths require (gate_plan), each once, and named in the
    # PUSHQ_RAN line when they pass (lead, 2026-11-08: a named `cross
    # codegen` ran neither, then pushq refused the push for cross)
    named="$named $step" ;;
  repo) ;; # ran first, above
  short|arm64|llvm|corpora|cmd|gen|fuzz|corpora-all|alloff|fp|warm|chkcache|stdwasm|stdcov)
    [ "$step" = chkcache ] && chkcache_ran=1
    [ "$step" = stdcov ] && stdcov_ran=1
    steps="$steps $step" ;;
  *) echo "gate.sh: unknown step $step"; failed=1 ;;
  esac
done
run_step() {
  case $1 in
  short)
    # -p and -parallel capped (2026-11-07): uncapped, -short's parallel
    # packages and tests passed 16 GB on the Mac (six packages at once,
    # each with parallel xo builds); XO_SHORT_P and XO_SHORT_PARALLEL set them
    go test -short -count=1 -p="${XO_SHORT_P:-2}" -parallel="${XO_SHORT_PARALLEL:-4}" $(go list ./... | grep -v /runs/) >"$logs/short.txt" 2>&1
    k=$(grep -c '^--- FAIL' "$logs/short.txt")
    [ "$k" -gt 0 ] && failed=1
    echo "short fails: $k" ;;
  arm64) full arm64 ./internal/arm64 ;;
  llvm) llvm_step ;;
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
  cmd) full cmd ./cmd/xo ;;
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
# stdcov_change: the branch changes what the resolved std coverage reads:
# a std module's source, a program the sweep checks (the corpora, the
# goldens, the benchmark programs, the scripts' programs), the allowlist
# and its checker, or `xo dev edges`. XO_GATE_STDCOV=0 skips it.
stdcov_change() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | grep -q -E '^(std/|internal/stdtest/testdata/corpus/|internal/llvm/testdata/golden/|internal/arm64/testdata/|internal/gen/testdata/|cmd/xo/testdata/|bench/perf/|scripts/.*\.xo$|internal/stdcoverage/|compiler/xo/edgecache\.xo$|compiler/cli/servers/edges\.xo$)'
}
# stdcov_step: internal/repotest's TestStdAPICovered with the resolved
# sweep (stdcoverage.Sweep: `xo dev edges --own` of every program and the
# std graph once a bindings value), so a std declaration counts as covered
# only when a call resolves to it (owner, 2026-11-11). A few minutes, so
# not in the fast set: std pushes run it by path and the 8-hourly tier by
# name.
stdcov_step() {
  XO_STDCOV_SWEEP=1 full stdcov ./internal/repotest -run '^TestStdAPICovered$'
}
# the checks after the named steps, by path
after_steps() {
  if [ "$chkcache_ran" = 0 ] && [ "${XO_GATE_CHKCACHE:-1}" != 0 ] && check_change; then
    full chkcache ./internal/arm64 -run '^TestModuleCheckAuto(Generic|Zones)?$'
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
  if [ "$stdcov_ran" = 0 ] && [ "${XO_GATE_STDCOV:-1}" != 0 ] && stdcov_change; then
    # not a darwin check: on the Mac it ran beside selfbuild gates and
    # starved them (2026-11-11); named, stdcov still runs anywhere
    if [ "$(uname -s)" = Darwin ]; then
      echo "stdcov: skipped on darwin, runs in the tier / an offload gate"
    else
      stdcov_step
    fi
  fi
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
  if XO_MAX_MEM=8G XO_BACKEND_CACHE=off XO_BIN_CACHE=off XO_WHOLE_BUILD=off "$XO_CLI" build --phase-times -o "$logs/selfbuild-xo" ./compiler/xo >"$logs/selfbuild.txt" 2>&1; then
    echo "selfbuild: $(($(date +%s) - t1)) s, $(grep -o 'peak [0-9]* MB' "$logs/selfbuild.txt" | sort -k2 -n | tail -1)"; ran="$ran selfbuild"
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
    if XO_NOLIBC=$nolibc XO_INLINE_STRICT=1 XO_MAX_MEM=8G XO_BACKEND_CACHE=off XO_BIN_CACHE=off XO_BUILD_DIR="$sb/build" XO_RTOBJ_CACHE="$sb/rtobj" \
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
# The concurrent jobs (conc=1): job KEY CMD... starts CMD in the
# background once fewer than gjobs jobs run, with its output buffered in
# $logs/.job-KEY.out and printed when it ends; its failed and ran go to
# files that pool_wait folds in, in the order the jobs started.
jobkeys=""
job() {
  local k=$1; shift
  while [ "$(jobs -pr | wc -l)" -ge "$gjobs" ]; do sleep 2; done
  rm -f "$logs/.job-$k".*
  (
    failed=0 ran="" t1=$(date +%s)
    "$@" >"$logs/.job-$k.out" 2>&1
    echo "$failed" >"$logs/.job-$k.failed"
    echo "$ran" >"$logs/.job-$k.ran"
    cat "$logs/.job-$k.out"
    echo "gate.sh: $k done in $(($(date +%s) - t1)) s"
  ) &
  jobkeys="$jobkeys $k"
}
pool_wait() {
  local k
  wait
  for k in $jobkeys; do
    # a job that left no status died before it finished: a failure
    [ "$(cat "$logs/.job-$k.failed" 2>/dev/null || echo 1)" = 0 ] || failed=1
    ran="$ran $(cat "$logs/.job-$k.ran" 2>/dev/null)"
    rm -f "$logs/.job-$k".*
  done
  jobkeys=""
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
  for step in $steps; do run_step "$step"; done
  after_steps
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
plan=" $(gate_plan "$req" $named) "
echo "gate.sh: proving checks:${plan% }${plan:+ }(required by the paths: ${req:-none}; named:${named:- none})"
if [ "$conc" = 1 ]; then
  for p in codegen wasm cross fieldgen; do
    [[ "$plan" == *" $p "* ]] && job "$p" "prove_$p"
  done
  pool_wait
  [[ "$plan" == *" selfbuild "* ]] && prove_selfbuild
  [[ "$plan" == *" seedbuild "* ]] && prove_seedbuild
  # PUSHQ_RAN in the serial order
  r2=""
  for p in codegen wasm cross selfbuild seedbuild fieldgen; do
    [[ " $ran " == *" $p "* ]] && r2="$r2 $p"
  done
  ran=$r2
  echo "gate.sh: steps took $(($(date +%s) - tsteps)) s (concurrent, at most $gjobs jobs)"
else
  [[ "$plan" == *" codegen "* ]] && prove_codegen
  [[ "$plan" == *" wasm "* ]] && prove_wasm
  [[ "$plan" == *" cross "* ]] && prove_cross
  [[ "$plan" == *" selfbuild "* ]] && prove_selfbuild
  [[ "$plan" == *" seedbuild "* ]] && prove_seedbuild
  [[ "$plan" == *" fieldgen "* ]] && prove_fieldgen
  echo "gate.sh: steps took $(($(date +%s) - tsteps)) s"
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
echo GATE-DONE
exit $failed
