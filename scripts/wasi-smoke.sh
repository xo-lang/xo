#!/bin/bash
# wasi-smoke.sh XO_WASM STD_DIR [WORKDIR]: smoke test of xo built for
# wasip1/wasm (decision 0405). Runs the module with RUNNER (default:
# node --no-warnings compiler/wasm/run.mjs; for wasmtime set
# RUNNER="wasmtime run --dir=/ --dir=. --env XO_STD --"), then checks:
#   1. `xo version` prints "xo VERSION wasip1/wasm"
#   2. `xo check` accepts a hello program
#   3. `xo build` makes hello for darwin/arm64 and linux/arm64 (Xo's own
#      assemblers and linkers, no processes); on a darwin/arm64 host the
#      darwin binary runs natively and prints its line
#   4. `xo run .` and `xo test .` are refused: exit 2, XO1513 in the message
# WANT_PLATFORM (default wasip1/wasm) is the platform `version` must name
# (to try this script on a native xo: RUNNER=env WANT_PLATFORM=darwin/arm64).
# It stops at the first failure. WORKDIR (default a new temp directory, removed
# on success) holds the scratch program.
set -euo pipefail
wasm=${1:?usage: wasi-smoke.sh XO_WASM STD_DIR [WORKDIR]}
std=${2:?usage: wasi-smoke.sh XO_WASM STD_DIR [WORKDIR]}
root=$(cd "$(dirname "$0")/.." && pwd)
runner=${RUNNER:-node --no-warnings $root/compiler/wasm/run.mjs}
work=${3:-$(mktemp -d "${TMPDIR:-/tmp}/wasi-smoke.XXXXXX")}
mkdir -p "$work/hello"
cd "$work"
printf 'module example.com/hello\nedition preview\n' > hello/xo.mod
printf 'fn main(os: Os) {\n  os.stdio.println("hello from xo-wasi")\n}\n' > hello/main.xo
export XO_STD=$std
x() { $runner "$wasm" "$@"; }
step() { echo "== $*"; }
fail() { echo "wasi-smoke: $*" >&2; exit 1; }

step version
v=$(x version) || fail "version failed"
case $v in "xo "*" ${WANT_PLATFORM:-wasip1/wasm}") ;; *) fail "version printed: $v" ;; esac

step check
x check hello > check.log 2>&1 || { cat check.log; fail "check failed"; }

for t in darwin/arm64 linux/arm64; do
  step "build $t"
  out=hello-${t//\//-}
  x build --targets=$t -o "$out" hello > "build-$out.log" 2>&1 || { cat "build-$out.log"; fail "build $t failed"; }
  [ -s "$out" ] || fail "build $t wrote no $out"
done
if [ "$(uname -s)-$(uname -m)" = Darwin-arm64 ]; then
  step "run the darwin binary natively"
  chmod +x hello-darwin-arm64
  [ "$(./hello-darwin-arm64)" = "hello from xo-wasi" ] || fail "the darwin binary printed something else"
fi

for c in run test; do
  step "$c is refused"
  set +e
  x $c hello > refuse.log 2>&1
  code=$?
  set -e
  [ "$code" = 2 ] || { cat refuse.log; fail "$c exited $code, want 2"; }
  grep -q XO1513 refuse.log || { cat refuse.log; fail "$c did not name XO1513"; }
done
echo "wasi-smoke: ok ($wasm)"
[ -n "${3:-}" ] || rm -rf "$work"
