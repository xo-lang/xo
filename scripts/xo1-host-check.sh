#!/bin/bash
# xo1-host-check.sh: build the self hosted xo (xo1, selfhost/xo) on this
# host as a checkout does (`go run ./scripts/xobuild`, internal/xotool:
# the arm64 backend on an arm64 host, the LLVM backend split into modules
# on an x86-64 host; decision 0215),
# run the LLVM goldens with it, then check that it rebuilds itself to a
# fixed point with no Go toolchain (scripts/bootstrap-xo.sh). Written for
# the x86-64 hosts (decision 0178 stage 7, decision 0196); run in a
# checkout with Go on PATH, by scripts/remote-xo1-x86.sh on EC2 and by
# scripts/intel-mac-test.sh on the Intel Mac:
#
#   scripts/xo1-host-check.sh [OUTDIR]     (default .tools/xo1-host)
#
# Steps, each logged with its wall clock time and peak memory
# (/usr/bin/time: the largest process's maximum resident set; on Linux
# also the machine's peak memory in use, sampled every 5 s):
#   1. go run ./scripts/xobuild with a fresh XO_CHECKOUT_CACHE: the xo1
#      of the go-compiler-final commit (built with Go) builds the
#      checkout's selfhost/xo
#   2. that xo1 builds a hello program with the native backend; the
#      hello program must print its line
#   3. scripts/selfhost-goldens.py with that xo1 and XO1_BACKEND=llvm
#      (the want.txt goldens and internal/arm64/testdata/run against the
#      recorded Go backend outputs)
#   4. scripts/bootstrap-xo.sh with that xo1 as the seed (no Go on PATH):
#      s1, s2, s3 and the fixed point
#   5. the goldens with s3 (no Go on PATH)
# Exit status 0 when every step passes. XO_HOST_CHECK_SKIP_GOLDENS=1
# skips steps 3 and 5.
set -uo pipefail
cd "$(dirname "$0")/.."
root=$(pwd)
out=${1:-.tools/xo1-host}
case "$out" in /*) ;; *) out=$root/$out ;; esac
rm -rf "$out"
mkdir -p "$out"
log=$out/summary.txt
: > "$log"
say() { echo "xo1-host: $*" | tee -a "$log" >&2; }
os=$(uname -s)
arch=$(uname -m)
backend=llvm
case "$arch" in arm64|aarch64) backend=arm64 ;; esac
# as internal/xotool builds xo1 on an x86-64 host (hostBackend)
export XO_LLVM_SPLIT=${XO_LLVM_SPLIT:-12} XO_LLVM_LTO=${XO_LLVM_LTO:-none}

sampler=""
start_sampler() {
  [ "$os" = Linux ] || return 0
  : > "$out/mem-$1.txt"
  ( while sleep 5; do free -m | awk '/^Mem:/ {print $3}'; done ) > "$out/mem-$1.txt" 2>/dev/null &
  sampler=$!
}
stop_sampler() {
  [ -n "$sampler" ] || return 0
  kill "$sampler" 2>/dev/null; wait "$sampler" 2>/dev/null
  sampler=""
  echo "machine peak memory in use $(sort -n "$out/mem-$1.txt" | tail -1) MB"
}
trap '[ -n "$sampler" ] && kill "$sampler" 2>/dev/null' EXIT

# timed NAME CMD...: runs CMD under /usr/bin/time, logs its wall clock
# time and peak memory
timed() {
  local name=$1 rc t0 dt rss foot mach; shift
  start_sampler "$name"
  t0=$(date +%s)
  if [ "$os" = Darwin ]; then
    /usr/bin/time -l "$@" > "$out/$name.log" 2> "$out/$name.err"; rc=$?
    rss=$(awk '/maximum resident set size/ {printf "%.2f GB", $1/1e9}' "$out/$name.err")
    foot=$(awk '/peak memory footprint/ {printf ", peak footprint %.2f GB", $1/1e9}' "$out/$name.err")
  else
    /usr/bin/time -v "$@" > "$out/$name.log" 2> "$out/$name.err"; rc=$?
    rss=$(awk -F': ' '/Maximum resident set size/ {printf "%.2f GB", $2/1e6}' "$out/$name.err")
    foot=""
  fi
  dt=$(( $(date +%s) - t0 ))
  mach=$(stop_sampler "$name")
  say "$name: exit $rc, ${dt} s, max RSS $rss$foot${mach:+, $mach}"
  return $rc
}

say "$(uname -a)"
say "cpus $( (sysctl -n hw.ncpu || nproc) 2>/dev/null), memory $( (sysctl -n hw.memsize | awk '{printf "%.1f GB", $1/1e9}') 2>/dev/null || free -g | awk '/^Mem:/ {print $2 " GB"}'), $(go version), $(clang --version | head -1), backend $backend, XO_LLVM_SPLIT=$XO_LLVM_SPLIT XO_LLVM_JOBS=${XO_LLVM_JOBS:-default} XO_LLVM_LTO=${XO_LLVM_LTO:-default}"
say "commit $(git rev-parse --short HEAD 2>/dev/null)"

export XO_CHECKOUT_CACHE=$out/cache
timed xo1build go run ./scripts/xobuild || { tail -30 "$out/xo1build.err" >&2; exit 1; }
xo1=$(tail -1 "$out/xo1build.log")
[ -x "$xo1" ] || { say "FAIL: no xo1 ($xo1)"; exit 1; }
mkdir -p "$out/hello"
printf 'fn main(os: Os) uses (stdio) {\n  os.stdio.println("hello from xo1 on %s")\n}\n' "$arch" > "$out/hello/main.xo"
timed hello "$xo1" build --backend=$backend -o "$out/hello-bin" "$out/hello" || { tail -30 "$out/hello.err" >&2; exit 1; }
say "xo1: $xo1 ($(wc -c < "$xo1" | tr -d ' ') bytes, $(file -b "$xo1" | cut -c1-60))"
say "hello: $("$out/hello-bin" 2>&1 | head -1)"
grep -q "hello from xo1" <("$out/hello-bin" 2>&1) || { say "FAIL: the hello program"; exit 1; }
cp "$xo1" "$out/xo1"

rc=0
if [ "${XO_HOST_CHECK_SKIP_GOLDENS:-}" != 1 ]; then
  XO1_BACKEND=llvm XO_GOLD_DIR="$out/gold" \
    timed goldens python3 scripts/selfhost-goldens.py "$out/xo1" || rc=1
  say "goldens (xo1 built by $backend, programs by llvm): $(head -1 "$out/goldens.log")"
  grep -q ' 0 fail$' "$out/goldens.log" || rc=1
fi

timed bootstrap scripts/bootstrap-xo.sh "$out/xo1" "$out/bootstrap" || rc=1
grep -E '^bootstrap: (fixed point|s1 equals|s3 |FAIL|ok|FAILED)' "$out/bootstrap/log.txt" | tee -a "$log" >&2
sed 's/^/stage time: /' "$out/bootstrap/times.txt" | tee -a "$log" >&2

if [ "${XO_HOST_CHECK_SKIP_GOLDENS:-}" != 1 ] && [ -x "$out/bootstrap/s3/xo" ]; then
  # without Go: only the system directories on PATH (clang, cc, python3)
  if PATH=/usr/bin:/bin command -v go >/dev/null; then say "FAIL: go in /usr/bin or /bin"; exit 1; fi
  PATH=/usr/bin:/bin XO_GO=/nonexistent/go XO1_BACKEND=llvm XO_GOLD_DIR="$out/gold-s3" \
    timed goldens-s3 python3 scripts/selfhost-goldens.py "$out/bootstrap/s3/xo" || rc=1
  say "goldens with s3 (no Go): $(head -1 "$out/goldens-s3.log")"
  grep -q ' 0 fail$' "$out/goldens-s3.log" || rc=1
fi
[ $rc -eq 0 ] && say "ok" || say "FAILED"
exit $rc
