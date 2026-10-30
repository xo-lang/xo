#!/bin/sh
# bootstrap-xo.sh: build the Xo `xo` (compiler/xo) of this checkout with a
# given xo binary and no Go toolchain, then check that it rebuilds itself
# to a fixed point (decision 0178, stage 7 step 3; toolchain.md
# "Bootstrapping without Go").
#
#   scripts/bootstrap-xo.sh SEED [OUTDIR]
#
# SEED is a self hosted xo that builds compiler/xo with the host's native
# backend: the previous release's bin/xo, an earlier build of the
# checkout's bin/xo, or, with no xo at all, the xo1 of the
# go-compiler-final tag (decision 0215: the last commit with the Go
# compiler; `go run ./scripts/xobuild` builds it once with Go and caches
# it, internal/xotool). Every stage gets the checkout's std (--std). The backend is
# $XO_BOOTSTRAP_BACKEND, by default the host's native backend (arm64,
# or amd64 on an x86-64 host since decision 0362), else llvm (the LLVM
# builds are split into $XO_LLVM_SPLIT modules, default 12, without LTO
# unless $XO_LLVM_LTO says thin, as internal/selfdriver builds xo1,
# decision 0168; --asm writes the LLVM IR). OUTDIR defaults to
# .tools/bootstrap. The stages, each with an empty build directory,
# runtime object cache, back-end cache and binary cache (so nothing is
# reused from an earlier build, another stage, or another xo; until
# 2026-10-27 the back-end cache was the user's, shared with every xo on
# the machine and between s2 and s3, gotchas/fixed-point-shared-cache.md):
#
#   s1: SEED builds compiler/xo
#   s2: s1 builds compiler/xo
#   s3: s2 builds compiler/xo
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
# builds each hop's compiler/xo in turn (decision 0216 item 2: a seed
# older than an intrinsic the sources use builds an intermediate first;
# "file" takes the hops recorded in scripts/seed-chain.txt).
#
# XO_BOOTSTRAP_SEED_IS_S1=1: the seed is this tree's own compiler/xo
# built by an earlier xo (a gate's `go run ./scripts/xobuild` output), so
# it is used as s1 instead of building s1 again; s2 and s3 and their
# comparison are unchanged. It saves one xo build (about 60 to 110 s and
# a 7.8 GB peak) per gate; 407 fixed points to 2026-10-21 never had s2
# and s3 differ, and every failure was a stage that could not build xo
# (owner, 2026-10-21). Only a caller that just built the seed from this
# tree may set it; seed-cut.sh and CI keep the full three stages.
#
# XO_BOOTSTRAP_RTPACK=DIR: use the runtime pack root DIR (a release's
# lib/xo/rtpack, made by `xo rtpack`) instead of XO_RTPACK=off, so xo1's
# default linker on darwin is compiler/macho (decision 0182) and the
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
origpath=$PATH
newpath=""
oldifs=$IFS
IFS=:
for d in $PATH; do
  [ -n "$d" ] || continue
  if [ -e "$d/go" ] || [ -e "$d/gofmt" ] || [ -e "$d/go.exe" ]; then
    for f in "$d"/*; do
      b=$(basename "$f")
      # go.exe and gofmt.exe too: Git Bash on Windows (it links as a copy)
      case "$b" in go|gofmt|go.exe|gofmt.exe|go[0-9]*) continue ;; esac
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
# the toolchain's own Linux builds keep the C library until a Linux fixed
# point passes with none (decision 0243; user programs default to none)
export XO_NOLIBC=${XO_NOLIBC:-0}
backend=${XO_BOOTSTRAP_BACKEND:-}
if [ -z "$backend" ]; then
  # amd64 on an x86-64 host too since 2026-10-24 (decision 0362: a
  # Docker linux/amd64 build of xo 212 s natively against 893 s by LLVM
  # -O0, and that xo builds compiler/xo in 211 s against 532 s)
  case "$(uname -m)" in arm64|aarch64) backend=arm64 ;; x86_64|amd64) backend=amd64 ;; *) backend=llvm ;; esac
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

# the build stamps (decision 0264, compiler/xo/provenance.xo): the
# commit and a hash of the uncommitted changes, the same for every stage
# so s2 and s3 can be identical; and the source key (internal/xotool
# Key) when it is known: $XO_SOURCE_KEY, else computed with the Go that
# PATH had before (only the key; no stage uses Go), so a test run that
# pins an output (XO_CLI) can tell it is stale (xotool StaleCLI,
# 2026-10-21)
srckey=${XO_SOURCE_KEY:-}
if [ -z "$srckey" ]; then
  srckey=$(PATH=$origpath go run ./scripts/xobuild --key 2>/dev/null || true)
fi
case "$srckey" in
  [0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]) ;;
  *) srckey="" ;;
esac
stamps=""
if [ -f "$root/compiler/xo/provenance.xo" ]; then
  commit=$(git -C "$root" rev-parse HEAD 2>/dev/null || true)
  dirty=""
  if [ -n "$commit" ] && [ -n "$(git -C "$root" diff HEAD 2>/dev/null | head -c 1)" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      dirty=$(git -C "$root" diff HEAD | sha256sum | cut -c1-12)
    else
      dirty=$(git -C "$root" diff HEAD | shasum -a 256 | cut -c1-12)
    fi
  fi
  stamps="--stamp XO_COMMIT=$commit --stamp XO_DIRTY=$dirty --stamp XO_BUILT_BY=bootstrap-xo.sh"
  [ -n "$srckey" ] && stamps="$stamps --stamp XO_SOURCE_KEY=$srckey"
fi

# stage N: binary $2 builds compiler/xo into $out/sN/xo (with the
# assembly beside it unless N is 1)
stage() {
  n=$1 bin=$2
  asm=""
  [ "$n" -gt 1 ] && asm="--asm=$out/s$n/xo.s"
  mkdir -p "$out/s$n" "$out/cache-s$n"
  say "s$n: $bin builds compiler/xo"
  t0=$(date +%s)
  # s2 and s3 (built by the new compiler) with XO_INLINE_STRICT=1: a
  # module that fails verification after inlining fails the stage
  # instead of building the compiler without inlining, which the fixed
  # point alone does not notice (gotchas/arm64-inline-verify-fallback.md);
  # s1's seed may be older than the sources and fall back
  strict=0
  [ "$n" -gt 1 ] && strict=1
  if XO_INLINE_STRICT=$strict XO_BUILD_DIR="$out/cache-s$n/build" XO_RTOBJ_CACHE="$out/cache-s$n/rtobj" \
    XO_BACKEND_CACHE_DIR="$out/cache-s$n/backend" "$bin" build --backend="$backend" --std="$root/std" -o "$out/s$n/xo" $asm $stamps compiler/xo >> "$log" 2>&1; then
    echo "s$n $(( $(date +%s) - t0 )) s" >> "$times"
  else
    say "FAIL: stage s$n (see $log)"
    exit 1
  fi
}
# the seed chain (decision 0216 item 2): an older seed first builds the
# intermediate xo of each hop, a commit's compiler/xo against that
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
    cdir=compiler; [ -d "$out/chain$n/tree/compiler/xo" ] || cdir=selfhost  # selfhost/ before decision 0249
    say "hop $n: $seed builds $c's $cdir/xo against $sc's std"
    t0=$(date +%s)
    # an old xo with a private back-end cache: its prune never reaches the
    # user's (gotchas/fixed-point-shared-cache.md)
    if (cd "$out/chain$n/tree" && XO_BUILD_DIR="$out/chain$n/build" XO_BACKEND_CACHE_DIR="$out/chain$n/backend" "$seed" build --backend="$backend" \
      --std="$out/chain$n/std/std" -o "$out/chain$n/xo" "./$cdir/xo") >> "$log" 2>&1; then
      echo "hop$n $(( $(date +%s) - t0 )) s" >> "$times"
      seed=$out/chain$n/xo
    else
      say "FAIL: hop $n ($c) (see $log)"
      exit 1
    fi
  done
fi
if [ "${XO_BOOTSTRAP_SEED_IS_S1:-}" = 1 ] && [ -z "${XO_SEED_CHAIN:-}" ]; then
  mkdir -p "$out/s1"
  cp "$seed" "$out/s1/xo"
  say "s1: the seed itself (XO_BOOTSTRAP_SEED_IS_S1=1: built from these sources)"
  echo "s1 0 s (the seed)" >> "$times"
else
  stage 1 "$seed"
fi
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
  XO_BACKEND_CACHE_DIR="$out/cache-s3/backend" "$out/s3/xo" run --backend="$backend" --std="$root/std" "$out/hello" > "$out/hello.txt" 2>> "$log"; then
  say "s3 builds and runs a hello program: $(head -1 "$out/hello.txt")"
else
  say "FAIL: s3 cannot build and run a hello program"
  rc=1
fi
cat "$times" | tee -a "$log" >&2
[ $rc -eq 0 ] && say "ok" || say "FAILED"
exit $rc
