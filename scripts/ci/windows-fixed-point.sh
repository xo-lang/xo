#!/bin/bash
# scripts/ci/windows-fixed-point.sh: the self-build fixed point on
# windows/amd64, on the xo-windows runner (owner-approved run, 2026-10-29;
# the box64 route on the Mac cannot hold the build's 8.2 GB peak,
# gotchas/docker-vm-oom-self-build.md):
#   gh workflow run ci.yml -f suite=script -f runner=xo-windows \
#     -f ref=BRANCH -f packages="scripts/ci/windows-fixed-point.sh"
# s1 is $XO_CLI (the checkout's xo.exe, built on the runner from the seed
# by scripts/xobuild); s2 = s1 builds compiler/xo with the amd64 backend,
# s3 = s2 builds it again, both with the same stamps and private caches
# (as scripts/bootstrap-xo.sh), XO_INLINE_STRICT=1; the fixed point is s2
# and s3 the same assembly and the same bytes. Then s2 builds the arith
# golden (internal/llvm/testdata/golden/arith) and runs it with
# XO_LEAKCHECK=1, and s3 builds and runs a hello program.
# $XO_CI_OUT/windows-fixed-point.txt is the summary, sN.log each stage's
# build output; when s2 and s3 differ, the first 200 differing assembly
# lines are in asm-diff.txt and both binaries go to the bucket
# (ci_offload). Exit 1 when a check fails.
set -uo pipefail
out=${XO_CI_OUT:?XO_CI_OUT}
xo=${XO_CLI:?XO_CLI}
np() { if command -v cygpath >/dev/null; then cygpath -w "$1"; else echo "$1"; fi; }
up() { if command -v cygpath >/dev/null; then cygpath -u "$1"; else echo "$1"; fi; }
o=$(up "$out")
xo=$(up "$xo")
. scripts/ci/ci-out.sh
w=$(ci_work windows-fp) || exit 2
trap 'rm -rf "${w:?}"' EXIT
sum="$o/windows-fixed-point.txt"
: > "$sum"
note() { echo "$1" | tee -a "$sum"; }
fail=0
check() { # NAME RESULT(0 ok) DETAIL
  if [ "$2" = 0 ]; then note "ok   $1"; else note "FAIL $1: $3"; fail=1; fi
}
sha() { sha256sum "$1" | cut -c1-64; }
export XO_STD=${XO_CI_STD:-$(dirname "$(dirname "$xo")")/std}
stdw=$(np "$XO_STD")
note "s1 (XO_CLI): $("$xo" version 2>&1 | head -1 | tr -d '\r'); std $XO_STD; $(nproc 2>/dev/null || echo ?) cpus"
commit=$(git rev-parse HEAD 2>/dev/null || echo unknown)
stamps="--stamp XO_COMMIT=$commit --stamp XO_DIRTY= --stamp XO_BUILT_BY=windows-fixed-point.sh"
export XO_GO=/nonexistent/go XO_BIN_CACHE=off XO_RTPACK=off

stage() { # N BIN: BIN builds compiler/xo into $w/sN/xo.exe with the assembly beside it
  local n=$1 bin=$2 t0 rc
  mkdir -p "$w/s$n" "$w/cache-s$n"
  t0=$(date +%s)
  XO_INLINE_STRICT=1 XO_BUILD_DIR=$(np "$w/cache-s$n/build") XO_RTOBJ_CACHE=$(np "$w/cache-s$n/rtobj") \
    XO_BACKEND_CACHE_DIR=$(np "$w/cache-s$n/backend") \
    "$bin" build --backend=amd64 --std="$stdw" -o "$(np "$w/s$n/xo.exe")" --asm="$(np "$w/s$n/xo.s")" $stamps compiler/xo > "$o/s$n.log" 2>&1
  rc=$?
  note "s$n: exit $rc in $(( $(date +%s) - t0 )) s; xo.exe $(wc -c < "$w/s$n/xo.exe" 2>/dev/null || echo 0) bytes, xo.s $(wc -c < "$w/s$n/xo.s" 2>/dev/null || echo 0) bytes; $(tail -2 "$o/s$n.log" | tr -d '\r' | tr '\n' ' ' | cut -c1-200)"
  return $rc
}
stage 2 "$xo"; check "s2 (s1 builds compiler/xo)" $? "see s2.log"
[ -x "$w/s2/xo.exe" ] || { note "FAILED: no s2"; exit 1; }
note "s2 version: $("$w/s2/xo.exe" version 2>&1 | head -1 | tr -d '\r')"
stage 3 "$w/s2/xo.exe"; check "s3 (s2 builds compiler/xo)" $? "see s3.log"
[ -x "$w/s3/xo.exe" ] || { note "FAILED: no s3"; exit 1; }
h2s=$(sha "$w/s2/xo.s"); h3s=$(sha "$w/s3/xo.s"); h2=$(sha "$w/s2/xo.exe"); h3=$(sha "$w/s3/xo.exe")
note "sha256 s2 xo.s $h2s; s3 xo.s $h3s; s2 xo.exe $h2; s3 xo.exe $h3; s1 xo.exe $(sha "$xo")"
[ "$h2s" = "$h3s" ]; check "fixed point: s2 and s3 assembly identical" $? "differ"
[ "$h2" = "$h3" ]; check "fixed point: s2 and s3 binaries identical" $? "differ"
if [ "$h2s" != "$h3s" ] || [ "$h2" != "$h3" ]; then
  diff "$w/s2/xo.s" "$w/s3/xo.s" | head -200 > "$o/asm-diff.txt"
  note "asm diff: $(diff "$w/s2/xo.s" "$w/s3/xo.s" | grep -c '^[<>]') differing lines (asm-diff.txt has the first 200)"
  mkdir -p "$o/bin" && cp "$w/s2/xo.exe" "$o/bin/s2-xo.exe" && cp "$w/s3/xo.exe" "$o/bin/s3-xo.exe"
  ci_offload "$o/bin" "$sum"
fi
# arith by s2, run with the leak check
a="$w/arith"; mkdir -p "$a"
cp internal/llvm/testdata/golden/arith/main.xo "$a/main.xo" && printf 'module arith\n' > "$a/xo.mod"
XO_BACKEND_CACHE_DIR=$(np "$w/cache-s2/backend") "$w/s2/xo.exe" build --backend=amd64 --std="$stdw" -o "$(np "$a/arith.exe")" "$(np "$a")" > "$o/arith-build.log" 2>&1
check "s2 builds arith" $? "$(tail -3 "$o/arith-build.log" | tr -d '\r' | tr '\n' ' ')"
if [ -x "$a/arith.exe" ]; then
  XO_LEAKCHECK=1 "$a/arith.exe" > "$o/arith-out.txt" 2> "$o/arith-err.txt"; rc=$?
  note "arith by s2 with XO_LEAKCHECK=1: exit $rc, stdout $(wc -l < "$o/arith-out.txt") lines, stderr: $(head -3 "$o/arith-err.txt" | tr -d '\r' | tr '\n' ' ' | cut -c1-200)"
  want=internal/llvm/testdata/golden/arith/want.txt
  if grep -q '^--- stdout' "$want" 2>/dev/null; then
    awk '/^--- stdout/ {p=1; next} /^--- / {p=0} p' "$want" > "$w/want-out.txt"
    diff -q <(tr -d '\r' < "$o/arith-out.txt") "$w/want-out.txt" > /dev/null; check "arith output as want.txt" $? "see arith-out.txt"
  fi
  [ $rc = 0 ]; check "arith exit 0 under XO_LEAKCHECK=1" $? "exit $rc"
fi
# hello by s3
h="$w/hello"; mkdir -p "$h"
printf 'module hello\n' > "$h/xo.mod"
printf 'fn main(os: Os) uses (stdio) {\n  os.stdio.println("hello from the bootstrapped xo")\n}\n' > "$h/main.xo"
got=$(XO_BACKEND_CACHE_DIR=$(np "$w/cache-s3/backend") "$w/s3/xo.exe" run --backend=amd64 --std="$stdw" "$(np "$h")" 2> "$o/hello.err" | tr -d '\r')
[ "$got" = "hello from the bootstrapped xo" ]; check "s3 builds and runs hello" $? "got '$got': $(tail -2 "$o/hello.err" | tr '\n' ' ')"
[ $fail = 0 ] && note "ok" || note "FAILED"
exit $fail
