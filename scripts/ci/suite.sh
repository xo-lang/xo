#!/bin/bash
# scripts/ci/suite.sh SUITE [PACKAGE...]: the test suites of the on demand
# CI workflow (.github/workflows/ci.yml, decision 0225). Runs the same on
# any machine with Go and clang, from anywhere in the checkout.
#
#   auto         short, then the touched Go packages' full tests (a
#                changed file belongs to its nearest directory with .go
#                files), the internal/selfhost merge tier when selfhost/ or
#                std/ changed, internal/gen, llvm, and arm64 when std/
#                changed, and the fixed point when the change touches what
#                the self build compiles (the CLAUDE.md test policy)
#   short        go vet and go test -short over every package but runs/
#   vet          go vet only
#   merge        the full tests of PACKAGE... (default: the touched ones)
#   fixed-point  scripts/bootstrap-xo.sh with the checkout's xo (s2 == s3)
#   full         every package with XO_SELFHOST_TIER=full
#   windows      scripts/windows-batch.sh (xo1 from the Windows seed, the
#                LLVM goldens for windows/amd64 and arm64, the runtime
#                tests, xo test LLVM against Go); on the xo-windows runner
#   xcross DEST  cross builds xo.exe for windows/amd64 with the Go backend
#                from the checkout and copies it to DEST (an s3:// URL):
#                the plan job does this for a windows job, since the
#                windows-amd64 seed cannot build xo on Windows (XO1306,
#                gotchas/windows-seed-xo1306-module-not-required.md)
#   script PATH [ARG...]
#                runs PATH from the checkout (.sh with bash, .ps1 with
#                Windows PowerShell, anything else as an executable) with
#                ARG..., on the runner the workflow picks (xo-arm64 unless
#                -f runner=...). Its environment: XO_CLI (the checkout's
#                xo; on Windows the plan job's cross built xo.exe),
#                XO_CI_STD (the checkout's std), XO_CI_BASE_STD (std of
#                $XO_CI_BASE, main by default, from a worktree), XO_CI_OUT
#                (a directory the workflow uploads as the run's artifact
#                script-out), all native paths on Windows
#   measure DIR  bench/perf/DIR/measure.sh with main's xo and std, then
#                with the checkout's; both outputs and new/base ratios
#   plan SUITE [PACKAGE...]
#                the workflow's job list for SUITE as JSON (one line,
#                {"include": [{"name", "runner", "args"}]}): auto fans out
#                to `short` on the small runner and the full tests, split
#                by weight, plus the fixed point, on the big runners
#   job ARGS     runs one planned job (ARGS: short, merge PACKAGE...,
#                fixed-point, or merge-fp PACKAGE...: merge then the fixed
#                point)
#
# auto's full tests: the touched packages, ./internal/selfhost and
# ./cmd/xo when selfhost/ or std/ changed, ./internal/gen, llvm, and
# arm64 when std/ changed, and PACKAGE... (the workflow's packages input)
# on top.
#
# "Touched" means changed between $XO_CI_BASE (default origin/main) and
# HEAD, from their merge base.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
suite=${1:?usage: scripts/ci/suite.sh SUITE [PACKAGE...]}
shift
base=${XO_CI_BASE:-origin/main}

step() { echo; echo "::group::$*"; }
end() { echo "::endgroup::"; }

all_pkgs() { go list -e ./... | grep -v /runs/; }
changed() { git diff --name-only "$base...HEAD"; }

# The changed paths, listed once: a pipeline into `grep -q` would fail
# under pipefail on a long list (gotchas/grep-q-pipefail-long-input.md).
files=""
load_changed() { files=$(changed); }
has() { grep -qE "$@" <<<"$files"; }

# Go packages with a changed file: each changed path maps to its nearest
# directory holding .go files (so a testdata golden or .xo fixture tests
# its package); deleted directories are skipped.
touched_pkgs() {
  [ -n "$files" ] || return 0
  while read -r f; do
    d=$(dirname "$f")
    while [ "$d" != . ] && ! ls "$d"/*.go >/dev/null 2>&1; do d=$(dirname "$d"); done
    if [ "$d" != . ]; then echo "./$d"; fi
  done <<<"$files" | sort -u
}

# The CLAUDE.md fixed point rule: what the self build compiles or runs.
needs_fixed_point() {
  grep -E '^(selfhost/|std/native/|std/llrt/|internal/xotool/(xotool|chain|seed|lock_[^/]*)\.go$|scripts/xobuild/|scripts/seed-chain\.txt$)' <<<"$files" |
    grep -vE '^selfhost/(macho|wasm)/|_test\.xo$' >/dev/null
}

run_short() {
  step "go vet"; go vet $(all_pkgs); end
  step "go test -short"; go test -short $(all_pkgs); end
}

run_merge() {
  [ $# -gt 0 ] || { echo "merge: no packages"; return 0; }
  step "go test (full) $*"; go test -timeout 60m "$@"; end
}

run_fixed_point() {
  step "xo for the fixed point (scripts/xobuild)"; xo=$(go run ./scripts/xobuild); echo "$xo"; end
  step "fixed point (scripts/bootstrap-xo.sh)"
  rc=0; scripts/bootstrap-xo.sh "$xo" .tools/bootstrap || rc=$?
  cat .tools/bootstrap/times.txt 2>/dev/null || true
  [ $rc -eq 0 ] || tail -40 .tools/bootstrap/log.txt
  end; return $rc
}

# auto's package list for the full tests (PACKAGE... added).
auto_pkgs() {
  local pkgs
  pkgs="$(touched_pkgs) $*"
  if has '^(selfhost|std)/'; then pkgs="$pkgs ./internal/selfhost ./cmd/xo"; fi
  # std/ changes reach every backend's program output goldens
  if has '^std/'; then pkgs="$pkgs ./internal/gen ./internal/llvm ./internal/arm64"; fi
  echo $pkgs | tr ' ' '\n' | grep . | sort -u | tr '\n' ' '
}

# Seconds a package's full tests take on a big runner (2026-10-14
# measurements, ci.md "Measurements"); the planner balances jobs by it.
weight() {
  case "$1" in
    ./internal/llvm) echo 320 ;; ./internal/selfhost) echo 190 ;;
    ./cmd/xo) echo 100 ;; ./internal/gen | ./internal/arm64) echo 90 ;;
    ./internal/xotool) echo 45 ;; *) echo 30 ;;
  esac
}

# plan_auto [PACKAGE...]: short on the small runner; the full tests
# spread over the three big runners by weight (longest first, each to
# the least loaded), the fixed point (150 s) counted as one more load.
plan_auto() {
  load_changed
  local fp=0 p args=""
  if needs_fixed_point; then fp=1; fi
  for p in $(auto_pkgs "$@"); do args="$args $p=$(weight "$p")"; done
  python3 - "$fp" $args <<'PY'
import json, sys
fp = sys.argv[1] == "1"
w = {a.split("=")[0]: int(a.split("=")[1]) for a in sys.argv[2:]}
pkgs = list(w)
bins = [{"load": 0, "pkgs": [], "fp": False} for _ in range(3)]
items = sorted(pkgs, key=lambda p: -w[p])
if fp:
    items = ["@fp"] + items
for it in items:
    b = min(bins, key=lambda b: b["load"])
    if it == "@fp":
        b["fp"] = True; b["load"] += 150
    else:
        b["pkgs"].append(it); b["load"] += w[it]
jobs = [{"name": "short", "runner": "xo-arm64", "args": "short"}]
for b in bins:
    if not b["pkgs"] and not b["fp"]:
        continue
    if b["pkgs"] and b["fp"]:
        jobs.append({"name": "full " + " ".join(p[2:] for p in b["pkgs"]) + " + fixed point", "runner": "xo-arm64-big", "args": "merge-fp " + " ".join(b["pkgs"])})
    elif b["fp"]:
        jobs.append({"name": "fixed point", "runner": "xo-arm64-big", "args": "fixed-point"})
    else:
        jobs.append({"name": "full " + " ".join(p[2:] for p in b["pkgs"]), "runner": "xo-arm64-big", "args": "merge " + " ".join(b["pkgs"])})
print(json.dumps({"include": jobs}))
PY
}

# measure DIR: main's xo (built from a worktree of $base) against the
# checkout's, each with its own std, through bench/perf/DIR/measure.sh
# <xo> <out> <backend...> (arm64 and llvm).
run_measure() {
  local dir=${1:?measure: bench/perf directory}
  local m=bench/perf/$dir/measure.sh
  [ -f "$m" ] || { echo "no $m"; return 2; }
  head -20 "$m" | grep -q 'usage: .*<xo binary> <out dir> <backend' || { echo "$m does not take <xo> <out> <backend...>"; return 2; }
  if [ "$(uname)" = Linux ] && ! command -v perf >/dev/null; then
    # measure.sh counts with perf stat on Linux; the runners' AL2023 image
    # has no perf until it is installed (once per machine, it stays)
    step "install perf"; sudo dnf install -y -q perf; end
  fi
  local out=$PWD/.tools/measure; rm -rf "$out"; mkdir -p "$out"
  step "xo of $base"
  git worktree remove --force "$out/base-tree" 2>/dev/null || true
  git worktree add -q --detach "$out/base-tree" "$base"
  local bx; bx=$(cd "$out/base-tree" && go run ./scripts/xobuild); echo "$bx"; end
  step "xo of the checkout"; local nx; nx=$(go run ./scripts/xobuild); echo "$nx"; end
  # a directory new on the branch has no base copy: the branch's
  # measure.sh builds its own workload with main's xo and std
  local bm="$out/base-tree/$m"; [ -f "$bm" ] || bm=$m
  step "base"; sh "$bm" "$bx" "$out/base" arm64 llvm | tee "$out/base.txt"; end
  step "new"; sh "$m" "$nx" "$out/new" arm64 llvm | tee "$out/new.txt"; end
  echo "new/base (instructions:u, minimum of the runs):"
  awk 'FNR==NR{b[$1" "$2]=$3; next} ($1" "$2) in b && b[$1" "$2] > 0 {printf "  %s %s base %s new %s ratio %.3f\n", $1, $2, b[$1" "$2], $3, $3/b[$1" "$2]}' "$out/base.txt" "$out/new.txt"
  git worktree remove --force "$out/base-tree"
}

case "$suite" in
  short) run_short ;;
  vet) step "go vet"; go vet $(all_pkgs); end ;;
  merge) if [ $# -gt 0 ]; then run_merge "$@"; else load_changed; run_merge $(touched_pkgs); fi ;;
  fixed-point) run_fixed_point ;;
  measure) run_measure "$@" ;;
  xcross)
    dest=${1:?xcross: DEST}
    step "xo.exe for windows/amd64 (Go backend, cross built)"
    xo=$(go run ./scripts/xobuild)
    out=$PWD/.tools/xcross; rm -rf "$out"; mkdir -p "$out"
    "$xo" build --backend=go --targets=windows/amd64 --std="$PWD/std" -o "$out/xo.exe" selfhost/xo
    aws s3 cp --only-show-errors "$out/xo.exe" "$dest"
    end ;;
  script)
    sc=${1:?script: PATH}; shift
    [ -f "$sc" ] || { echo "script: no $sc in the checkout"; exit 2; }
    np() { if command -v cygpath >/dev/null; then cygpath -w "$1"; else echo "$1"; fi; }
    if [ -n "${XO_CI_XO_EXE:-}" ]; then
      step "fetch xo.exe"; mkdir -p bin; aws s3 cp --only-show-errors "$XO_CI_XO_EXE" bin/xo.exe
      XO_CLI=$(np "$PWD/bin/xo.exe"); end
    else
      step "xo (scripts/xobuild)"; XO_CLI=$(go run ./scripts/xobuild); end
    fi
    "$XO_CLI" version
    step "std of $base"
    bt=$PWD/.tools/base-tree
    git worktree remove --force "$bt" 2>/dev/null || rm -rf "$bt"
    git worktree add -q --detach "$bt" "$base"; end
    out=$PWD/.tools/script-out; rm -rf "$out"; mkdir -p "$out"
    export XO_CLI XO_CI_STD XO_CI_BASE_STD XO_CI_OUT
    XO_CI_STD=$(np "$PWD/std") XO_CI_BASE_STD=$(np "$bt/std") XO_CI_OUT=$(np "$out")
    step "$sc $*"; rc=0
    case "$sc" in
      *.ps1) powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(np "$PWD/$sc")" "$@" || rc=$? ;;
      *.sh) bash "$sc" "$@" || rc=$? ;;
      *) "./$sc" "$@" || rc=$? ;;
    esac
    end
    git worktree remove --force "$bt"
    exit $rc ;;
  windows)
    # the plan job's cross built xo.exe ($XO_CI_XO_EXE), in bin/ so it
    # finds std at bin/../std; XO_CLI makes xobuild use it as is
    if [ -n "${XO_CI_XO_EXE:-}" ]; then
      step "fetch xo.exe"; mkdir -p bin; aws s3 cp --only-show-errors "$XO_CI_XO_EXE" bin/xo.exe
      export XO_CLI=$PWD/bin/xo.exe; "$XO_CLI" version; end
    fi
    step "scripts/windows-batch.sh"; rc=0; scripts/windows-batch.sh || rc=$?; end; exit $rc ;;
  plan)
    what=${1:?plan: SUITE}; shift
    case "$what" in
      auto) plan_auto "$@" ;;
      full) echo '{"include":[{"name":"full tier","runner":"xo-arm64-big","args":"full"}]}' ;;
      measure) echo "{\"include\":[{\"name\":\"measure $*\",\"runner\":\"xo-arm64-big\",\"args\":\"measure $*\"}]}" ;;
      windows) echo '{"include":[{"name":"windows","runner":"xo-windows","args":"windows"}]}' ;;
      script) echo "{\"include\":[{\"name\":\"script $*\",\"runner\":\"xo-arm64\",\"args\":\"script $*\"}]}" ;;
      merge) echo "{\"include\":[{\"name\":\"merge $*\",\"runner\":\"xo-arm64-big\",\"args\":\"merge $*\"}]}" ;;
      *) echo "{\"include\":[{\"name\":\"$what\",\"runner\":\"xo-arm64\",\"args\":\"$what\"}]}" ;;
    esac ;;
  job)
    kind=${1:?job: ARGS}; shift
    case "$kind" in
      merge-fp) run_merge "$@"; run_fixed_point ;;
      *) exec "$0" "$kind" "$@" ;;
    esac ;;
  full) step "full tier"; XO_SELFHOST_TIER=full go test -timeout 120m $(all_pkgs); end ;;
  auto)
    load_changed
    echo "changed since $base:"; sed 's/^/  /' <<<"$files"
    run_short
    pkgs=$(auto_pkgs "$@")
    echo "packages for the full tests:" $pkgs
    run_merge $pkgs
    if needs_fixed_point; then run_fixed_point; else echo "fixed point: not needed for these paths"; fi
    ;;
  *) echo "unknown suite $suite" >&2; exit 2 ;;
esac
