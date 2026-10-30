#!/bin/bash
# gate.sh: an agent's gate before a push, as named steps (CLAUDE.md
# "Tests"); run it under the heavy lock (scripts/lead/heavy.sh NAME
# scripts/gate.sh ...). It rebuilds WORKTREE's bin/xo, then runs each
# step with that xo and prints one line per step, then GATE-DONE.
#
#   scripts/gate.sh WORKTREE STEP...
#
# Steps:
#   short    go test -short over the repository (runs/ excluded)
#   arm64    the full tests of internal/arm64
#            (with XO_GATE_CONCURRENT=1 and llvm named too, both run
#            at once, arm64 in the background; opt in, 2026-10-26)
#   llvm     the full tests of internal/llvm, skipped when every file
#            the branch changes is native backend only (owner,
#            2026-10-18; see native_only below); XO_GATE_LLVM=1 runs it;
#            with TestGoldenGoParity (XO_TEST_GO_PARITY=1) when the
#            branch changes compiler/gen or std/rt (go_backend). Its
#            golden programs are only those the change touches (owner,
#            2026-10-22; llvm_goldens below): XO_GATE_LLVM=all runs
#            every one
#   corpora  internal/stdtest's differential corpora on the Go backend
#            (TestStdCorpora/go/) and internal/llvm's Test*Diff
#            differential tests: run when the branch changes std/rt or
#            compiler/gen (go_backend); skipped otherwise
#   cmd      the full tests of cmd/xo
#   gen      the full tests of internal/gen
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
#   cross    internal/arm64 TestAmd64CrossBuilds (four programs built for
#            linux, windows and darwin amd64): run after the steps named
#            when the branch changes compiler/arm64 or compiler/ssa;
#            XO_GATE_CROSS=0 skips that; and internal/arm64
#            TestSwitch65535Arms when it changes compiler/arm64/jumptab.xo
#            (the match lowering; XO_GATE_SCALE=0 skips that); and
#            internal/arm64's cache tests (-run 'Cache|BinCache',
#            TestMachoBinCache among them) when it changes compiler/xo's
#            bcache.xo, parallel.xo, packseg.xo or cache.xo
#            (XO_GATE_CACHE=0 skips that; lead, 2026-10-30)
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
# Logs go to $XO_GATE_LOGS (default $TMPDIR/xo-gate-WORKTREE_NAME), one
# file per step. A step's failure is reported and the next step runs;
# the exit status is 1 when any failed.
set -u
wt=$(cd "${1:?usage: gate.sh WORKTREE STEP...}" && pwd)
shift
logs=${XO_GATE_LOGS:-${TMPDIR:-/tmp}/xo-gate-$(basename "$wt")}
mkdir -p "$logs"
cd "$wt" || exit 1
t0=$(date +%s)
rm -f bin/xo
go run ./scripts/xobuild >"$logs/xobuild.txt" 2>&1 || { echo "XOBUILD-FAIL ($logs/xobuild.txt)"; exit 1; }
echo "xobuild $(($(date +%s) - t0)) s"
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
# runtime, so internal/llvm's goldens run on the Go backend too
# (TestGoldenGoParity: TestGolden checks want.txt with the LLVM build
# only since 2026-10-18).
go_backend() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | grep -q -E '^(compiler/gen/|std/rt/)'
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
# native_change: the branch changes the native backend (compiler/arm64
# holds the amd64 emitter and the shared MIR passes), so the cross step
# runs after the steps named (lead, 2026-10-23: daed94d4d, a mir.xo
# change, passed its arm64 gate and broke every amd64 build of three
# goldens on main for an hour; the x86-only Docker sample rule did not
# apply to a shared file). XO_GATE_CROSS=0 skips it.
native_change() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | grep -q -E '^compiler/(arm64|ssa)/'
}
cross_ran=0
# scale_change: the branch changes the match lowering (compiler/arm64
# jumptab.xo), so internal/arm64's TestSwitch65535Arms (about 100 s) runs
# after the steps named (lead, 2026-10-24: 9479f31cd's compare tree made
# that match quadratic, past XO_MAX_MEM, and only the 8 hourly full tier
# saw it). XO_GATE_SCALE=0 skips it.
scale_change() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | grep -q -E '^compiler/arm64/jumptab\.xo$'
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
        XO_TEST_GO_PARITY=1 full llvm ./internal/llvm
      else
        full llvm ./internal/llvm
      fi
      unset XO_LLVM_GOLDENS
    fi
}
# XO_GATE_CONCURRENT=1 (2026-10-26, opt in): with both arm64 and llvm
# named, the two run at once, arm64 in the background: in a full tier at
# most two tests ran for 72% of arm64's 1,904 s and 55% of llvm's
# 1,866 s on the 12 core Mac, and the llvm step left 69% of memory free
llvm_done=0
has_llvm=0
for s in "$@"; do [ "$s" = llvm ] && has_llvm=1; done
for step in "$@"; do
  case $step in
  short)
    go test -short -count=1 $(go list ./... | grep -v /runs/) >"$logs/short.txt" 2>&1
    k=$(grep -c '^--- FAIL' "$logs/short.txt")
    [ "$k" -gt 0 ] && failed=1
    echo "short fails: $k" ;;
  arm64)
    if [ "${XO_GATE_CONCURRENT:-}" = 1 ] && [ "$has_llvm" = 1 ] && [ "$llvm_done" = 0 ]; then
      ( failed=0; full arm64 ./internal/arm64; echo "$failed" >"$logs/arm64.failed" ) &
      apid=$!
      llvm_step
      llvm_done=1
      wait "$apid"
      [ "$(cat "$logs/arm64.failed" 2>/dev/null || echo 1)" = 1 ] && failed=1
    else
      full arm64 ./internal/arm64
    fi ;;
  llvm)
    if [ "$llvm_done" = 1 ]; then
      continue
    fi
    llvm_step ;;
  corpora)
    if go_backend; then
      full corpora ./internal/stdtest -run '^TestStdCorpora$/^go/'
      # the native-against-Go differential tests of internal/llvm
      # (TestTimeDiff, TestNumDiff, TestJSONTextDiff, ...: lead,
      # 2026-10-24: 7822046d4 made the Go backend's RFC 3339 parse strict,
      # passed its gate, and TestTimeDiff, a -short test the gate never
      # ran, kept main red for 1 h 30 min)
      full diffs ./internal/llvm -run 'Diff$'
    else
      echo "corpora: skipped (no std/rt or compiler/gen change)"
    fi ;;
  cmd) full cmd ./cmd/xo ;;
  gen) full gen ./internal/gen ;;
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
      echo "fp: FAIL ($logs/fp.txt)"
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
    chkcache_ran=1
    full chkcache ./internal/arm64 -run '^TestModuleCheckAuto(Generic|Zones)?$' ;;
  cross)
    cross_ran=1
    full cross ./internal/arm64 -run '^TestAmd64CrossBuilds$' ;;
  *) echo "gate.sh: unknown step $step"; failed=1 ;;
  esac
done
if [ "$chkcache_ran" = 0 ] && [ "${XO_GATE_CHKCACHE:-1}" != 0 ] && check_change; then
  chkcache_ran=1
  full chkcache ./internal/arm64 -run '^TestModuleCheckAuto(Generic|Zones)?$'
fi
if [ "$cross_ran" = 0 ] && [ "${XO_GATE_CROSS:-1}" != 0 ] && native_change; then
  cross_ran=1
  full cross ./internal/arm64 -run '^TestAmd64CrossBuilds$'
fi
if [ "${XO_GATE_SCALE:-1}" != 0 ] && scale_change; then
  full scale ./internal/arm64 -run '^TestSwitch65535Arms$'
fi
if [ "${XO_GATE_CACHE:-1}" != 0 ] && cache_change; then
  full caches ./internal/arm64 -run 'Cache|BinCache'
fi
echo GATE-DONE
exit $failed
