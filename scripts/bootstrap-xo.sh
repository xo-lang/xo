#!/bin/sh
# bootstrap-xo.sh: build the Xo `xo` (selfhost/xo) of this checkout with a
# given xo binary and no Go toolchain, then check that it rebuilds itself
# to a fixed point (decision 0178, stage 7 step 3; toolchain.md
# "Bootstrapping without Go").
#
#   scripts/bootstrap-xo.sh SEED [OUTDIR]
#
# SEED is a self hosted xo that builds selfhost/xo with the host's native
# backend: the previous release's bin/xo, an earlier build of the
# checkout's bin/xo, or, with no xo at all, the xo1 of the
# go-compiler-final tag (decision 0215: the last commit with the Go
# compiler; `go run ./scripts/xobuild` builds it once with Go and caches
# it, internal/xotool). Every stage gets the checkout's std (--std). The backend is
# $XO_BOOTSTRAP_BACKEND, by default arm64 on an arm64 host and llvm on
# an x86-64 host (where xo1 is built by the LLVM backend; the LLVM
# builds are split into $XO_LLVM_SPLIT modules, default 12, without LTO
# unless $XO_LLVM_LTO says thin, as internal/selfdriver builds xo1,
# decision 0168; --asm writes the LLVM IR). OUTDIR defaults to
# .tools/bootstrap. The stages, each with an empty build directory,
# runtime object cache, and binary cache (so nothing is reused from an
# earlier build):
#
#   s1: SEED builds selfhost/xo
#   s2: s1 builds selfhost/xo
#   s3: s2 builds selfhost/xo
#
# and the fixed point: s2 and s3 write the same assembly and the same
# binary (both are built by the new compiler from the same sources; s1 is
# built by SEED and may differ). Every stage runs with no `go` on PATH
# (the directories holding one are replaced by a shim directory linking
# their other programs), GOROOT unset, and XO_GO pointing at nothing, so
# a step that still needs Go fails instead of finding it. The C compiler
# (clang, or $XO_CC) is the one tool needed. Last: s3 builds and runs a
# hello program. s1 is built without --asm (the seed's assembly is not
# compared), s2 and s3 with it. Exit status 0 when every step passes; the log is
# OUTDIR/log.txt, the timings OUTDIR/times.txt.
#
# XO_SEED_CHAIN="COMMIT[:STDCOMMIT] ..." or "file": before s1, the seed
# builds each hop's selfhost/xo in turn (decision 0216 item 2: a seed
# older than an intrinsic the sources use builds an intermediate first;
# "file" takes the hops recorded in scripts/seed-chain.txt).
#
# XO_BOOTSTRAP_RTPACK=DIR: use the runtime pack root DIR (a release's
# lib/xo/rtpack, made by `xo rtpack`) instead of XO_RTPACK=off, so xo1's
# default linker on darwin is selfhost/macho (decision 0182) and the
# stages need no C compiler at all; set PATH without one to show it.
set -eu
[ $# -ge 1 ] || { echo "usage: $0 SEED [OUTDIR]" >&2; exit 2; }
seed=$1
case "$seed" in /*) ;; *) seed=$(pwd)/$seed ;; esac
cd "$(dirname "$0")/.."
root=$(pwd)
out=${2:-.tools/bootstrap}
case "$out" in /*) ;; *) out=$root/$out ;; esac
rm -rf "$out"
mkdir -p "$out/shim"
log=$out/log.txt
times=$out/times.txt
: > "$log"
: > "$times"
say() { echo "bootstrap: $*" | tee -a "$log" >&2; }

# PATH without Go: a directory holding `go` is replaced by a shim
# directory with links to its other programs.
newpath=""
oldifs=$IFS
IFS=:
for d in $PATH; do
  [ -n "$d" ] || continue
  if [ -e "$d/go" ] || [ -e "$d/gofmt" ]; then
    for f in "$d"/*; do
      b=$(basename "$f")
      case "$b" in go|gofmt|go[0-9]*) continue ;; esac
      [ -x "$f" ] && [ ! -e "$out/shim/$b" ] && ln -s "$f" "$out/shim/$b"
    done
    d=$out/shim
    case ":$newpath:" in *":$d:"*) continue ;; esac
  fi
  newpath=${newpath:+$newpath:}$d
done
IFS=$oldifs
export PATH=$newpath
unset GOROOT GOPATH GOFLAGS GOTOOLCHAIN 2>/dev/null || true
export XO_GO=/nonexistent/go
export XO_BIN_CACHE=off
export XO_RTPACK=${XO_BOOTSTRAP_RTPACK:-off}
backend=${XO_BOOTSTRAP_BACKEND:-}
if [ -z "$backend" ]; then
  case "$(uname -m)" in arm64|aarch64) backend=arm64 ;; *) backend=llvm ;; esac
fi
if [ "$backend" = llvm ]; then
  # clang -O0 for the development fixed point (decision 0220); the
  # fixed point holds within one level
  export XO_LLVM_SPLIT=${XO_LLVM_SPLIT:-12} XO_LLVM_LTO=${XO_LLVM_LTO:-none} XO_LLVM_OPT=${XO_LLVM_OPT:-O0}
fi
if command -v go >/dev/null 2>&1; then
  say "FAIL: go is still on PATH ($(command -v go))"
  exit 1
fi
say "no go on PATH; seed $seed; backend $backend; $(uname -sm); $(${XO_CC:-clang} --version 2>/dev/null | head -1); XO_RTPACK=$XO_RTPACK"

# stage N: binary $2 builds selfhost/xo into $out/sN/xo (with the
# assembly beside it unless N is 1)
stage() {
  n=$1 bin=$2
  asm=""
  [ "$n" -gt 1 ] && asm="--asm=$out/s$n/xo.s"
  mkdir -p "$out/s$n" "$out/cache-s$n"
  say "s$n: $bin builds selfhost/xo"
  t0=$(date +%s)
  if XO_BUILD_DIR="$out/cache-s$n/build" XO_RTOBJ_CACHE="$out/cache-s$n/rtobj" XO_TLS_CACHE="$out/cache-s$n/tls" \
    "$bin" build --backend="$backend" --std="$root/std" -o "$out/s$n/xo" $asm selfhost/xo >> "$log" 2>&1; then
    echo "s$n $(( $(date +%s) - t0 )) s" >> "$times"
  else
    say "FAIL: stage s$n (see $log)"
    exit 1
  fi
}
# the seed chain (decision 0216 item 2): an older seed first builds the
# intermediate xo of each hop, a commit's selfhost/xo against that
# commit's std or, with COMMIT:STDCOMMIT, another commit's; the last hop
# is the seed of s1. XO_SEED_CHAIN="c1 c2:std2 ..." lists the hops, or
# "file" takes scripts/seed-chain.txt's (internal/xotool reads the same).
if [ -n "${XO_SEED_CHAIN:-}" ]; then
  hops=$XO_SEED_CHAIN
  if [ "$hops" = file ]; then
    hops=$(sed -e 's/#.*//' -e 's/ std=/:/' scripts/seed-chain.txt | tr -s ' \n' ' ')
  fi
  n=0
  for hop in $hops; do
    n=$((n + 1))
    c=${hop%%:*} sc=${hop#*:}
    [ "$sc" = "$hop" ] && sc=$c
    mkdir -p "$out/chain$n/tree" "$out/chain$n/std"
    git archive "$c" | tar -x -C "$out/chain$n/tree" || { say "FAIL: git archive $c"; exit 1; }
    git archive "$sc" std | tar -x -C "$out/chain$n/std" || { say "FAIL: git archive $sc std"; exit 1; }
    say "hop $n: $seed builds $c's selfhost/xo against $sc's std"
    t0=$(date +%s)
    if (cd "$out/chain$n/tree" && XO_BUILD_DIR="$out/chain$n/build" "$seed" build --backend="$backend" \
      --std="$out/chain$n/std/std" -o "$out/chain$n/xo" ./selfhost/xo) >> "$log" 2>&1; then
      echo "hop$n $(( $(date +%s) - t0 )) s" >> "$times"
      seed=$out/chain$n/xo
    else
      say "FAIL: hop $n ($c) (see $log)"
      exit 1
    fi
  done
fi
stage 1 "$seed"
stage 2 "$out/s1/xo"
stage 3 "$out/s2/xo"
rc=0
if cmp -s "$out/s2/xo.s" "$out/s3/xo.s"; then
  say "fixed point: s2 and s3 assembly ($backend) identical ($(wc -c < "$out/s3/xo.s" | tr -d ' ') bytes)"
else
  say "FAIL: s2 and s3 assembly differ"
  rc=1
fi
if cmp -s "$out/s2/xo" "$out/s3/xo"; then
  say "fixed point: s2 and s3 binaries identical ($(wc -c < "$out/s3/xo" | tr -d ' ') bytes)"
else
  say "FAIL: s2 and s3 binaries differ"
  rc=1
fi
if cmp -s "$out/s1/xo" "$out/s2/xo"; then
  say "s1 equals s2 too (the seed compiles as the new compiler does)"
fi
(cd "$out" && { shasum -a 256 s1/xo s2/xo s3/xo s2/xo.s s3/xo.s 2>/dev/null || sha256sum s1/xo s2/xo s3/xo s2/xo.s s3/xo.s; }) >> "$log" 2>&1
say "s3 version: $("$out/s3/xo" version 2>&1 | head -1)"
mkdir -p "$out/hello"
printf 'fn main(os: Os) uses (stdio) {\n  os.stdio.println("hello from the bootstrapped xo")\n}\n' > "$out/hello/main.xo"
if XO_BUILD_DIR="$out/cache-s3/build" XO_RTOBJ_CACHE="$out/cache-s3/rtobj" \
  "$out/s3/xo" run --backend="$backend" --std="$root/std" "$out/hello" > "$out/hello.txt" 2>> "$log"; then
  say "s3 builds and runs a hello program: $(head -1 "$out/hello.txt")"
else
  say "FAIL: s3 cannot build and run a hello program"
  rc=1
fi
cat "$times" | tee -a "$log" >&2
[ $rc -eq 0 ] && say "ok" || say "FAILED"
exit $rc
