#!/bin/bash
# scripts/ci/windows-paths.sh: the host-path fixes of 092f0f60e (Windows
# field report on 0.26.0, W1 to W4, W8; compiler/hostpath) on the Windows
# runner, the run Wine cannot stand in for. For suite=script on the
# xo-windows runner (one owner-approved run, 2026-10-23):
#   gh workflow run ci.yml -f suite=script -f runner=xo-windows \
#     -f ref=BRANCH -f packages="scripts/ci/windows-paths.sh"
# Checks, each under a timeout so a hang fails instead of stalling:
#   init      xo init in a fresh directory (nearest_work stopped only at
#             "/", so it looped at C:/)
#   test      xo test twice with TMPDIR a Windows path (C:/...), the
#             second run served from the test cache (the cached binary
#             keeps its .exe)
#   vet       xo vet with a C:/... absolute path (it was joined to the
#             working directory)
#   fuzz      a contract-fuzz failure writes regressions_test.xo (the
#             TMPDIR ?? "/tmp" fallback is gone), once, and it fails
#             when replayed
#   race      a --race report prints no doubled path (<dir>C:/<dir>);
#             needs the Windows TSan runtime, as windows-race.sh gets it
#             (skipped with a note if XO_SKIP_RACE=1)
# $XO_CI_OUT/windows-paths.txt has an ok/FAIL line per check and the tail
# of every failing command; exit 1 on any FAIL.
set -uo pipefail
RUN_TIMEOUT=${RUN_TIMEOUT:-300}
if command -v timeout >/dev/null; then tmo() { timeout -k 10 "$RUN_TIMEOUT" "$@"; }; else
  echo "windows-paths: no timeout command, runs are unbounded"; tmo() { "$@"; }; fi
out=${XO_CI_OUT:?XO_CI_OUT}
np() { if command -v cygpath >/dev/null; then cygpath -w "$1"; else echo "$1"; fi; }
up() { if command -v cygpath >/dev/null; then cygpath -u "$1"; else echo "$1"; fi; }
# a path as xo takes it on Windows: C:/dir (drive letter, forward slashes)
fp() { np "$1" | tr '\\' '/'; }
o=$(up "$out")
. scripts/ci/ci-out.sh
w=$(ci_work windows-paths) || exit 2
trap 'rm -rf "${w:?}"' EXIT
sum="$o/windows-paths.txt"
: > "$sum"
fail=0
"$XO_CLI" version --json | tee -a "$sum"
# run NAME LOG CMD...: ok when CMD exits 0 within the timeout
run() {
  local name=$1 log=$2; shift 2
  tmo "$@" > "$log" 2>&1
  local rc=$?
  if [ $rc = 0 ]; then echo "ok   $name" >> "$sum"; else
    echo "FAIL $name (exit $rc$([ $rc = 124 ] || [ $rc = 137 ] && echo ", timeout"))" | tee -a "$sum"
    tail -20 "$log" | sed 's/^/     /' >> "$sum"; fail=1
  fi
  return $rc
}
# expect NAME LOG PATTERN: LOG has PATTERN
expect() {
  if grep -q -- "$3" "$2"; then echo "ok   $1" >> "$sum"; else
    echo "FAIL $1 (no \"$3\")" | tee -a "$sum"; tail -20 "$2" | sed 's/^/     /' >> "$sum"; fail=1; fi
}

# init
d="$w/init"; mkdir -p "$d"
( cd "$d" && run "init" "$w/init.log" "$XO_CLI" init github.com/acme/demo )
find "$d" -type f >> "$sum" 2>&1
[ -n "$(find "$d" -name xo.mod)" ] && echo "ok   init wrote xo.mod" >> "$sum" || { echo "FAIL init: no xo.mod" | tee -a "$sum"; fail=1; }

# test twice under a Windows TMPDIR
tm="$w/tmp"; mkdir -p "$tm"
m="$w/mod"; mkdir -p "$m"
cat > "$m/xo.mod" <<'M'
module winpathsdemo

xo 0.26
M
cat > "$m/main.xo" <<'M'
pub fn add(a: Int, b: Int) -> Int { a + b }
M
cat > "$m/main_test.xo" <<'M'
test "add" {
  expect(add(1, 2)) == 3
}
M
export TMPDIR
TMPDIR=$(fp "$tm")
echo "TMPDIR=$TMPDIR" >> "$sum"
run "test first" "$w/t1.log" "$XO_CLI" test "$(fp "$m")"
run "test second (cached)" "$w/t2.log" "$XO_CLI" test "$(fp "$m")"
expect "test second ran the test" "$w/t2.log" "1 passed, 0 failed"
unset TMPDIR

# vet with a C:/ absolute path
run "vet C:/..." "$w/vet.log" "$XO_CLI" vet "$(fp "$m")"

# contract fuzz writes regressions_test.xo
f="$w/halver"; mkdir -p "$f"
printf 'module halver\n\nxo 0.26\n' > "$f/xo.mod"
printf 'fn half(n: Int) -> Int\n  ensures result * 2 == n\n{\n  n / 2\n}\n\npub fn twice(n: Int) -> Int { half(n) * 4 }\n' > "$f/main.xo"
TMPDIR=$(fp "$tm") tmo "$XO_CLI" test "$(fp "$f")" > "$w/fuzz.log" 2>&1
rc=$?
if [ $rc != 0 ] && [ $rc != 124 ] && [ $rc != 137 ]; then echo "ok   fuzz fails (exit $rc)" >> "$sum"; else
  echo "FAIL fuzz: want a failing contract (exit $rc)" | tee -a "$sum"; tail -20 "$w/fuzz.log" | sed 's/^/     /' >> "$sum"; fail=1; fi
if [ -f "$f/regressions_test.xo" ] && grep -q 'regression: halver.half' "$f/regressions_test.xo"; then
  echo "ok   fuzz wrote regressions_test.xo" >> "$sum"
else
  echo "FAIL fuzz: no regressions_test.xo" | tee -a "$sum"; ls "$f" >> "$sum"; fail=1
fi
tmo "$XO_CLI" test "$(fp "$f")" > "$w/fuzz2.log" 2>&1
expect "fuzz regression replays" "$w/fuzz2.log" "FAIL: regression: halver.half"
[ "$(grep -c 'regression: halver.half' "$f/regressions_test.xo")" = 1 ] || { echo "FAIL fuzz: regression written twice" | tee -a "$sum"; fail=1; }

# --race report paths
if [ "${XO_SKIP_RACE:-}" = 1 ]; then echo "skip race (XO_SKIP_RACE=1)" >> "$sum"; else
  rt=${XO_RACE_RUNTIME:-}
  if [ -z "$rt" ]; then
    rt="$w/race_windows_amd64.syso"
    aws s3 cp --only-show-errors "s3://${XO_SEED_BUCKET:?XO_SEED_BUCKET}/ci/race/race_windows_amd64.syso" "$(np "$rt")" || { echo "FAIL race: no runtime in the bucket" | tee -a "$sum"; fail=1; rt=; }
  fi
  if [ -n "$rt" ]; then
    export XO_RACE_RUNTIME
    XO_RACE_RUNTIME=$(np "$(up "$rt")")
    if run "race build" "$w/rb.log" "$XO_CLI" build --race --backend=amd64 -o "$(np "$w/tsan.exe")" internal/llvm/testdata/tsan_race; then
      tmo "$w/tsan.exe" > "$w/race.out" 2> "$w/race.err"
      rc=$?
      [ $rc = 66 ] && echo "ok   race reported (exit 66)" >> "$sum" || { echo "FAIL race exit $rc (want 66)" | tee -a "$sum"; fail=1; }
      cp "$w/race.err" "$o/race.err.txt"
      # a doubled path: a second drive or "/" root inside one path
      if grep -E '[A-Za-z]:[/\\][^ ]*[A-Za-z]:[/\\]' "$w/race.err" > "$w/dbl.txt"; then
        echo "FAIL race: doubled paths" | tee -a "$sum"; head -5 "$w/dbl.txt" | sed 's/^/     /' >> "$sum"; fail=1
      else echo "ok   race paths not doubled" >> "$sum"; fi
    fi
  fi
fi

cp "$w"/*.log "$o/" 2>/dev/null
ci_offload "$o" "$sum"
echo "windows-paths: $(grep -c '^ok' "$sum") ok, $(grep -c '^FAIL' "$sum") failed"
exit $fail
