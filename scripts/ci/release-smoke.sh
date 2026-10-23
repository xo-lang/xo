#!/bin/bash
# scripts/ci/release-smoke.sh: a release archive of scripts/release.sh run
# on the host it is for, as a user would: unpacked alone, no XO_STD, no
# XO_CLI. For suite=script, e.g. the windows/amd64 archive on the
# xo-windows runner:
#   gh workflow run ci.yml -f ref=BRANCH -f suite=script -f runner=xo-windows \
#     -f packages="scripts/ci/release-smoke.sh ARCHIVE SHA256 [COMMIT]"
# ARCHIVE is a path in the checkout, else a name under ci/release/ of the
# seed bucket ($XO_SEED_BUCKET, the runner's read only instance profile),
# checked against SHA256. COMMIT, when given, is the commit `xo version
# --json` must report (release.sh stamps it). Checks: version and its
# stamp; on Windows, the Authenticode signatures of bin/xo.exe and
# bin/xo-gobind.exe (Get-AuthenticodeSignature: Valid, with a timestamp;
# XO_VERIFY_UNSIGNED=1 skips them); check, build and run of a hello
# program with the default (native)
# backend; goldens with threads (channels, mutexes, scopes, a reader and
# writer handoff) and the time zone golden (zone data from the archive's
# lib/xo/std/tzdata) through scripts/golden-run.sh; `xo test` of a small
# module; and the released xo building compiler/xo (its own sources,
# threads and memory under load) and that xo's version (XO_SMOKE_SELF=0
# skips it: the owner's Intel laptop builds no xo). $XO_CI_OUT/
# release-smoke.txt has a line per check; exit 1 on any failure. Crash
# bundles go to $XO_CI_OUT/crashes (XO_CRASH_DIR, and any *.xocrash the
# temporary directories gained while it ran, as suite.sh keeps them);
# only Go backend programs write them, so a native xo's crash leaves its
# exit status and output alone (CI 39800969338: every build exit 139, no
# output). Actions artifacts carry text only (ci-out.sh): the archive, its
# unpacked tree and every build product stay in a work directory outside
# XO_CI_OUT, and crash bundles over the text limit go to the bucket's
# dev/ci-out/$GITHUB_RUN_ID/.
set -uo pipefail
archive=${1:?usage: release-smoke.sh ARCHIVE SHA256 [COMMIT]}
want_sum=${2:?usage: release-smoke.sh ARCHIVE SHA256 [COMMIT]}
want_commit=${3:-}
out=${XO_CI_OUT:-$PWD/.tools/release-smoke}
np() { if command -v cygpath >/dev/null; then cygpath -w "$1"; else echo "$1"; fi; }
up() { if command -v cygpath >/dev/null; then cygpath -u "$1"; else echo "$1"; fi; }
o=$(up "$out")
mkdir -p "$o"
root=$PWD
. "$root/scripts/ci/ci-out.sh"
w=$(ci_work release-smoke) || exit 2
trap 'rm -rf "${w:?}"' EXIT
sum="$o/release-smoke.txt"
: > "$sum"
fails=0
pass() { echo "PASS $1" | tee -a "$sum"; }
fail() { echo "FAIL $1" | tee -a "$sum"; fails=$((fails + 1)); }

a=$archive
if [ ! -f "$a" ]; then
  a="$w/$(basename "$archive")"
  aws s3 cp --only-show-errors "s3://${XO_SEED_BUCKET:?XO_SEED_BUCKET}/ci/release/$(basename "$archive")" "$(np "$a")" ||
    { echo "release-smoke: $archive is not in the checkout or the bucket (ci/release/)"; exit 2; }
fi
got=$(sha256sum "$a" | cut -d' ' -f1)
[ "$got" = "$want_sum" ] || { echo "release-smoke: SHA-256 $got, want $want_sum"; exit 2; }
mkdir -p "$w/unpack"
tar -xzf "$a" -C "$w/unpack" || { echo "release-smoke: unpack failed"; exit 2; }
rel=$(echo "$w"/unpack/xo-*)
exe=""
[ -f "$rel/bin/xo.exe" ] && exe=.exe
xo="$rel/bin/xo$exe"
unset XO_STD XO_CLI XO_RTPACK
mkdir -p "$o/crashes"
touch "$o/crashes/.start"
export XO_CRASH_DIR
XO_CRASH_DIR=$(np "$o/crashes")
keep_crashes() {
  local d
  for d in "${TEMP:-}" "${TMP:-}" "${TMPDIR:-}" /tmp /c/Windows/SystemTemp "${LOCALAPPDATA:-}/Temp"; do
    d=$(up "$d")
    [ -n "$d" ] && [ -d "$d" ] && find "$d" -maxdepth 1 -name '*.xocrash' -newer "$o/crashes/.start" -exec cp {} "$o/crashes/" \; 2>/dev/null
  done
  true
}
echo "archive $(basename "$a") ($got) on $(uname -s) $(uname -m)" | tee -a "$sum"

# run NAME CMD...: CMD's output in OUT/NAME.txt; its last lines on failure
run() {
  local name=$1; shift
  if "$@" > "$o/$name.txt" 2>&1; then pass "$name"; else
    fail "$name (exit $?)"; tail -8 "$o/$name.txt" | sed 's/^/  /' | tee -a "$sum"
  fi
}

run version "$xo" version
"$xo" version --json > "$o/version.json" 2>&1
cat "$o/version.json" | tee -a "$sum"
commit=$(sed -n 's/.*"commit":"\([0-9a-f]*\)".*/\1/p' "$o/version.json")
if [ -z "$commit" ]; then fail "stamp: no commit in xo version --json"
elif [ -n "$want_commit" ] && [ "$commit" != "$want_commit" ]; then fail "stamp: commit $commit, want $want_commit"
else pass "stamp ($commit)"; fi
# Windows' own check of the release's Authenticode signatures
# (scripts/sign-windows.sh); XO_VERIFY_UNSIGNED=1 for an unsigned trial
if [ -n "$exe" ]; then
  for f in xo.exe xo-gobind.exe; do
    if [ "${XO_VERIFY_UNSIGNED:-}" = 1 ]; then
      echo "UNSUPPORTED signed-$f (XO_VERIFY_UNSIGNED=1)" | tee -a "$sum"
      continue
    fi
    sig=$(powershell.exe -NoProfile -NonInteractive -Command "\$s = Get-AuthenticodeSignature -LiteralPath '$(np "$rel/bin/$f")'; '' + \$s.Status + ' ' + \$s.SignerCertificate.Subject + ' timestamp ' + [bool]\$s.TimeStamperCertificate" 2>&1 | tr -d '\r')
    case "$sig" in
      "Valid "*"timestamp True") pass "signed-$f ($sig)" ;;
      *) fail "signed-$f ($sig)" ;;
    esac
  done
fi

mkdir -p "$w/hello" "$w/modt/calc"
printf 'fn main(os: Os) uses (stdio) {\n  os.stdio.println("hello from the released xo")\n}\n' > "$w/hello/main.xo"
run check "$xo" check "$(np "$w/hello")"
run build-hello "$xo" build -o "$(np "$w/hello-bin$exe")" "$(np "$w/hello")"
if [ -f "$w/hello-bin$exe" ] && [ "$("$w/hello-bin$exe" 2>&1 | tr -d '\r')" = "hello from the released xo" ]; then pass run-hello; else fail run-hello; fi
run run-hello-xo-run "$xo" run "$(np "$w/hello")"

# Git Bash (the Windows runner's shell) rewrites a POSIX-looking value in
# a program's environment or arguments into a Windows path when it starts
# a Windows exe (env_dirs' XDG_CONFIG_HOME=/xdg/config became C:/Program
# Files/Git/xdg/config, CI 39999938410): the goldens' env.txt values are
# meant as written
if [ -n "$exe" ]; then
  export MSYS_NO_PATHCONV=1 MSYS2_ENV_CONV_EXCL='*' MSYS2_ARG_CONV_EXCL='*'
fi
# goldens: threads, the zone database from the archive, and the goldens
# whose output differs on Windows (want_windows.txt there: environment,
# files, processes, signals, the clock, method values)
for g in internal/llvm/testdata/golden/conc_channels internal/llvm/testdata/golden/conc_mutex \
  internal/llvm/testdata/golden/conc_scopes internal/llvm/testdata/golden/conc_rwlock_handoff \
  internal/gen/testdata/golden/time_zones \
  internal/llvm/testdata/golden/env_dirs internal/llvm/testdata/golden/fs_files \
  internal/llvm/testdata/golden/fs_ops_edges internal/llvm/testdata/golden/method_values \
  internal/llvm/testdata/golden/proc_cmd internal/llvm/testdata/golden/sys_files_clock \
  internal/llvm/testdata/golden/unix_kill; do
  n=$(basename "$g")
  if ! "$xo" build -o "$(np "$w/$n$exe")" "$(np "$root/$g")" > "$o/build-$n.txt" 2>&1; then
    fail "build-$n"; tail -5 "$o/build-$n.txt" | sed 's/^/  /' | tee -a "$sum"
    continue
  fi
  d="$root/$g"
  if [ -f "$d/want_windows.txt" ] && [ -n "$exe" ]; then
    d="$w/golden-$n"; rm -rf "$d"; cp -R "$root/$g" "$d"; cp "$d/want_windows.txt" "$d/want.txt"
  fi
  if sh "$root/scripts/golden-run.sh" "$w/$n$exe" "$d" > "$o/golden-$n.txt" 2>&1; then pass "golden-$n"; else
    fail "golden-$n"; tail -8 "$o/golden-$n.txt" | sed 's/^/  /' | tee -a "$sum"
  fi
done

# xo test of a small module
printf 'module calc\n' > "$w/modt/xo.mod"
printf 'pub fn add(a: Int, b: Int) -> Int {\n  a + b\n}\n' > "$w/modt/calc/calc.xo"
printf 'test "add" {\n  expect(add(2, 3)) == 5\n}\n' > "$w/modt/calc/calc_test.xo"
run xo-test sh -c 'cd "$1" && "$2" test ./calc' _ "$w/modt" "$xo"

# the released xo builds its own compiler (several minutes)
if [ "${XO_SMOKE_SELF:-1}" != 0 ]; then
  run self-build "$xo" build -o "$(np "$w/xo2$exe")" "$(np "$root/compiler/xo")"
  [ -f "$w/xo2$exe" ] && run self-build-version "$w/xo2$exe" version
fi

keep_crashes
n=$(find "$o/crashes" -name '*.xocrash' | wc -l | tr -d ' ')
echo "crash bundles: $n in $o/crashes" | tee -a "$sum"
ci_offload "$o" "$sum"
echo "release-smoke: $fails failed" | tee -a "$sum"
[ "$fails" = 0 ]
