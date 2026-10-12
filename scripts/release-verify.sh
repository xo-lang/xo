#!/bin/bash
# release-verify.sh: check a release archive of scripts/release.sh on the
# host it was built for (decision 0178, switch point B):
#
#   scripts/release-verify.sh ARCHIVE [OUTDIR]
#
# Run from a checkout of the commit the archive was built from (the cmd/xo
# tests come from it). OUTDIR defaults to .tools/release-verify. Steps:
#
#   1. unpack ARCHIVE into OUTDIR (a clean layout: bin/xo, lib/xo/...).
#   2. without Go: every directory holding `go` is dropped from PATH,
#      GOROOT unset, XO_GO pointing at nothing, empty caches. `xo
#      version`, `xo rtpack --status`, `xo check`, and a hello program
#      with `xo run` on each native backend: arm64 (cc link), arm64 with
#      --ld=macho (no cc), llvm (clang). On a host with no native backend
#      (linux/amd64) these are expected to fail and are reported as
#      "unsupported", not as failures; `xo build` with the default (Go)
#      backend must say Go is missing.
#   3. with Go: a hello program on the Go backend, a `use go` program
#      (bin/xo-gobind), then cmd/xo's whole suite (no -short) with XO_CLI
#      = the archive's bin/xo, no XO_STD (the switch point B gate). With
#      BASELINE=1 the suite also runs against the Go cmd/xo of the
#      checkout, so a failure of the host itself is told apart.
#
# Prints "PASS name", "FAIL name", or "UNSUPPORTED name" per check and a
# summary line; exit status 0 when nothing failed.
set -u
[ $# -ge 1 ] || { echo "usage: $0 ARCHIVE [OUTDIR]" >&2; exit 2; }
archive=$1
case "$archive" in /*) ;; *) archive=$(pwd)/$archive ;; esac
cd "$(dirname "$0")/.."
root=$(pwd)
out=${2:-.tools/release-verify}
case "$out" in /*) ;; *) out=$root/$out ;; esac
rm -rf "$out"
mkdir -p "$out/unpack" "$out/shim" "$out/hello" "$out/usego/app"
tar -xzf "$archive" -C "$out/unpack" || { echo "FAIL unpack"; exit 1; }
rel=$(echo "$out"/unpack/*)
xo=$rel/bin/xo
host="$(uname -s | tr A-Z a-z)/$(uname -m | sed 's/aarch64/arm64/; s/x86_64/amd64/')"
fails=0
pass() { echo "PASS $1"; }
fail() { echo "FAIL $1"; fails=$((fails + 1)); }
native=1
[ "$host" = linux/amd64 ] && native=0
# check NAME CMD...: runs CMD, its output in OUTDIR/NAME.txt
check() {
  local name=$1; shift
  if "$@" > "$out/$name.txt" 2>&1; then pass "$name"; else
    if [ "$native" = 0 ] && [[ "$name" == native-* ]]; then echo "UNSUPPORTED $name ($(tail -1 "$out/$name.txt"))"
    else fail "$name"; tail -5 "$out/$name.txt" | sed 's/^/  /'; fi
  fi
}
echo "host $host; archive $(basename "$archive"); $(uname -sr)"
printf 'fn main(os: Os) uses (stdio) {\n  os.stdio.println("hello from the released xo")\n}\n' > "$out/hello/main.xo"
printf 'module ex\n' > "$out/usego/xo.mod"
printf 'use go "strings"\n\nfn main(os: Os) uses (stdio) {\n  os.stdio.println(strings.to_upper("hello from use go"))\n}\n' > "$out/usego/app/main.xo"

# 2. without Go
gopath_env=$PATH
newpath=""
IFS=: read -ra dirs <<< "$PATH"
for d in "${dirs[@]}"; do
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
(
  mkdir -p "$out/home"; export PATH=$newpath XO_GO=/nonexistent/go HOME="$out/home"
  export XO_BUILD_DIR="$out/cache-nogo/build" XO_RTOBJ_CACHE="$out/cache-nogo/rtobj" XO_TLS_CACHE="$out/cache-nogo/tls" XO_BIN_CACHE=off
  unset GOROOT GOPATH GOFLAGS GOTOOLCHAIN XO_STD XO_RTPACK XO_CLI XDG_CACHE_HOME
  if command -v go >/dev/null 2>&1; then echo "FAIL no-go-on-path ($(command -v go))"; exit 1; fi
  echo "PASS no-go-on-path"
  check version "$xo" version
  cat "$out/version.txt"
  check rtpack-status "$xo" rtpack --status
  cat "$out/rtpack-status.txt"
  check check "$xo" check "$out/hello"
  check native-arm64-run "$xo" run --backend=arm64 "$out/hello"
  check native-arm64-ld-macho-run "$xo" run --backend=arm64 --ld=macho "$out/hello"
  check native-llvm-run "$xo" run --backend=llvm "$out/hello"
  for f in native-arm64-run native-arm64-ld-macho-run native-llvm-run; do
    grep -q "hello from the released xo" "$out/$f.txt" 2>/dev/null && echo "  $f: $(head -1 "$out/$f.txt")"
  done
  # the default backend is Go: with no Go it must say so, not crash
  if "$xo" build -o "$out/hello-go" "$out/hello" > "$out/go-missing.txt" 2>&1; then
    fail go-missing-reported
  elif grep -qi "go" "$out/go-missing.txt" && ! grep -qi "fault\|panic" "$out/go-missing.txt"; then
    pass go-missing-reported; sed 's/^/  /' "$out/go-missing.txt" | head -3
  else
    fail go-missing-reported; sed 's/^/  /' "$out/go-missing.txt" | head -5
  fi
  exit $fails
)
fails=$((fails + $?))

# 3. with Go
export PATH=$gopath_env
unset XO_STD XO_RTPACK XO_CLI XO_GO
if command -v go >/dev/null 2>&1; then
  check go-backend-run "$xo" run --backend=go "$out/hello"
  check use-go-run "$xo" run --backend=go "$out/usego/app"
  grep -h "hello" "$out/go-backend-run.txt" "$out/use-go-run.txt" 2>/dev/null | sed 's/^/  /'
  export XO_SELF_DRIVER=0 XO_NO_SERVE=1 GOMAXPROCS=${GOMAXPROCS:-4}
  ( cd "$root" && XO_CLI="$xo" go test -count=1 -p 1 -parallel 2 -timeout 120m ./cmd/xo -v ) > "$out/cmdxo.log" 2>&1
  grep -E '^(--- |ok|FAIL|panic)' "$out/cmdxo.log" > "$out/cmdxo.txt"
  p=$(grep -c '^--- PASS' "$out/cmdxo.txt"); f=$(grep -c '^--- FAIL' "$out/cmdxo.txt"); s=$(grep -c '^--- SKIP' "$out/cmdxo.txt")
  echo "cmd/xo against the archive's xo: pass $p fail $f skip $s"
  grep -E '^--- (FAIL|SKIP)' "$out/cmdxo.txt" | sed 's/^/  /'
  if [ "$f" = 0 ] && grep -q '^ok' "$out/cmdxo.txt"; then pass cmdxo-gate; else fail cmdxo-gate; fi
  if [ "${BASELINE:-}" = 1 ]; then
    ( cd "$root" && go test -count=1 -p 1 -parallel 2 -timeout 120m ./cmd/xo -v ) > "$out/cmdxo-go.log" 2>&1
    grep -E '^(--- |ok|FAIL|panic)' "$out/cmdxo-go.log" > "$out/cmdxo-go.txt"
    echo "cmd/xo against the Go cmd/xo (baseline): pass $(grep -c '^--- PASS' "$out/cmdxo-go.txt") fail $(grep -c '^--- FAIL' "$out/cmdxo-go.txt") skip $(grep -c '^--- SKIP' "$out/cmdxo-go.txt")"
    grep -E '^--- (FAIL|SKIP)' "$out/cmdxo-go.txt" | sed 's/^/  /'
  fi
else
  echo "no go on PATH: the Go backend checks and cmd/xo's tests are not run"
  fail go-present
fi
echo "release-verify: $fails failed"
[ "$fails" = 0 ]
