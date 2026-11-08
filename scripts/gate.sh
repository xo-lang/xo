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
#            (XO_GATE_CACHE=0 skips that; lead, 2026-10-30); and
#            internal/arm64 TestStdStoreBuilds when it changes a std
#            module's source (std/*.xo, std/native/*.xo or a
#            std/native/MOD/ directory; XO_GATE_STDSTORE=0 skips that;
#            lead, 2026-11-06); and
#            internal/llvm's TestRaceSyncTable and TestRaceSyncTableArm64
#            on darwin when it changes llrt's task, scheduler or race
#            runtime (XO_GATE_RACE=0 skips that; lead, 2026-10-31); and
#            the checks pushq.sh requires by path, `xo dev codegen` for
#            compiler/ssa, arm64 or llvm and TestSelfhostWasm for llrt or
#            std/native/rt*.xo, and one cold build of compiler/xo with
#            the new xo under XO_MAX_MEM=8G with --phase-times for what
#            compiler/xo builds from (compiler/ but macho and wasm,
#            std/native, std/llrt; "selfbuild"), printing the PUSHQ_RAN
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
# Logs go to $XO_GATE_LOGS (default $TMPDIR/xo-gate-WORKTREE_NAME), one
# file per step. A step's failure is reported and the next step runs;
# the exit status is 1 when any failed.
set -u
# a step name in WORKTREE's place (`gate.sh arm64`, 2026-10-30) ran the
# steps in the caller's directory and reported their failures as the
# gate's: refused
wt=$(cd "${1:?usage: gate.sh WORKTREE STEP...}" 2>/dev/null && pwd) || { echo "gate.sh: $1 is not a directory (usage: gate.sh WORKTREE STEP...)"; exit 2; }
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
# apply to a shared file). std/native/llrt (the runtime xo's native code
# generator compiles with each program, cpu.xo and num.xo among it) counts
# too (lead, 2026-11-08: a762dc2e3's num.append_json_float32 fused a
# multiply and an add that the amd64 emitter had no template for, and main
# was red on amd64 for 2 hours). XO_GATE_CROSS=0 skips it.
native_change() {
  base=${XO_GATE_BASE:-$(git merge-base HEAD origin/main)}
  { git diff --name-only "$base" HEAD; git status --porcelain | cut -c4-; } | grep -q -E '^(compiler/(arm64|ssa)/|std/native/llrt/)'
}
cross_ran=0
cross_ok=0
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
# XO_GATE_CONCURRENT=1 (2026-10-26, opt in): with both arm64 and llvm
# named, the two run at once, arm64 in the background: in a full tier at
# most two tests ran for 72% of arm64's 1,904 s and 55% of llvm's
# 1,866 s on the 12 core Mac, and the llvm step left 69% of memory free
llvm_done=0
has_llvm=0
for s in "$@"; do [ "$s" = llvm ] && has_llvm=1; done
# repo: internal/repotest, the fast repo checks pushq runs before a push
# (about 20 s), first in every gate unless short (which includes it) is
# named; XO_GATE_REPO=0 skips it (lead, 2026-11-06 bottleneck review:
# pushq refused 14 pushes in 3 hours on TestStdAPICovered,
# TestThirdPartyOrigins, TestWasmUnsupportedListsNetGoldens,
# TestOutsideBytesValidated, TestStdGenCurrent and TestWaitsClassified,
# each found only after a whole gate had passed)
has_short=0
for s in "$@"; do [ "$s" = short ] && has_short=1; done
if [ "${XO_GATE_REPO:-1}" != 0 ] && [ "$has_short" = 0 ]; then
  full repo ./internal/repotest
fi
for step in "$@"; do
  case $step in
  short)
    # -p and -parallel capped (2026-11-07): uncapped, -short's parallel
    # packages and tests passed 16 GB on the Mac (six packages at once,
    # each with parallel xo builds); XO_SHORT_P and XO_SHORT_PARALLEL set them
    go test -short -count=1 -p="${XO_SHORT_P:-2}" -parallel="${XO_SHORT_PARALLEL:-4}" $(go list ./... | grep -v /runs/) >"$logs/short.txt" 2>&1
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
  f0=$failed; failed=0
  full cross ./internal/arm64 -run '^TestAmd64CrossBuilds$'
  [ "$failed" = 0 ] && cross_ok=1
  [ "$f0" = 1 ] && failed=1
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
if [ "${XO_GATE_RACE:-1}" != 0 ] && race_change; then
  full race ./internal/llvm -run '^TestRaceSyncTable(Arm64)?$'
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
if [ "${XO_GATE_PUSHQ:-1}" != 0 ] && echo "$ch" | grep -qE '^compiler/(ssa|arm64|llvm)/.*\.xo$'; then
  if "$XO_CLI" dev codegen --backend=arm64,llvm test/codegen >"$logs/codegen.txt" 2>&1; then
    echo "codegen: $(tail -1 "$logs/codegen.txt")"; ran="$ran codegen"
  else
    failed=1; echo "codegen: FAIL ($logs/codegen.txt)"
  fi
fi
if [ "${XO_GATE_PUSHQ:-1}" != 0 ] && echo "$ch" | grep -qE '^std/native/(llrt/|rt[^/]*\.xo$)'; then
  # the goldens the branch touches, and the concurrency, atomics and map
  # sets pushq.sh names
  g=$(echo "$ch" | sed -n 's|^internal/[a-z0-9]*/testdata/golden/\([^/]*\)/.*|\1|p' | sort -u | paste -sd, -)
  f0=$failed; failed=0
  XO_WASM_GOLDENS="${g:+$g,}conc_*,atomic_ops,map_*" full wasm ./internal/selfhost -run '^TestSelfhostWasm$'
  [ "$failed" = 0 ] && ran="$ran wasm"
  [ "$f0" = 1 ] && failed=1
fi
# what compiler/xo builds from: one cold self build with the new xo under
# an 8 GB budget (owner, 2026-11-06: 11a08a448's inliner runaway passed
# 22 GB building xo and reached main, found only when builders failed)
if [ "${XO_GATE_PUSHQ:-1}" != 0 ] && echo "$ch" | grep -E '^(compiler/|std/native/|std/llrt/)' | grep -qvE '^compiler/(macho|wasm)/'; then
  t1=$(date +%s)
  if XO_MAX_MEM=8G XO_BACKEND_CACHE=off XO_BIN_CACHE=off XO_WHOLE_BUILD=off "$XO_CLI" build --phase-times -o "$logs/selfbuild-xo" ./compiler/xo >"$logs/selfbuild.txt" 2>&1; then
    echo "selfbuild: $(($(date +%s) - t1)) s, $(grep -o 'peak [0-9]* MB' "$logs/selfbuild.txt" | sort -k2 -n | tail -1)"; ran="$ran selfbuild"
  else
    failed=1; echo "selfbuild: FAIL ($logs/selfbuild.txt: $(grep -m1 -E 'memory budget|error' "$logs/selfbuild.txt"))"
  fi
  rm -f "$logs/selfbuild-xo"
fi
[ "$cross_ok" = 1 ] && ran="$ran cross"
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
