#!/bin/bash
# golden-std-deps_test.sh: scripts/golden-std-deps.py on a small tree
set -u
here=$(cd "$(dirname "$0")" && pwd)
T=$(mktemp -d)
trap 'rm -rf "${T:?}"' EXIT
mk() { mkdir -p "$(dirname "$T/$1")"; printf '%b' "$2" >"$T/$1"; }
mk std/native/prelude.xo 'use native/llrt/atomic\nuse native/rt\n'
mk std/native/rt.xo ''
mk std/native/llrt/atomic.xo 'use std/netip\n'
mk std/netip.xo ''
mk std/csv.xo 'use std/bufio\n'
mk std/native/csv.xo ''
mk std/bufio.xo ''
mk std/png.xo 'use std/compress\n'
mk std/compress.xo ''
mk std/json.xo ''
mk std/url_native.xo ''
g=internal/llvm/testdata/golden
mk $g/a/main.xo 'use std/csv\n'
mk $g/b/main.xo 'use std/png\n'
mk $g/c/main.xo 'fn main() {}\n'
mk $g/d/main.xo 'use github.com/acme/kv\n'
mk $g/e/main.xo 'use std/url\n'
printf 'a std/csv.xo\nb std/png.xo\nc std/json.xo\nd\ne\n' >"$T/deps"
fails=0
t() {
  local want=$1 why=$2; shift 2
  got=$(printf '%s\n' "$@" | python3 "$here/golden-std-deps.py" "$T" "$T/deps")
  if [ "$got" = "$want" ]; then echo "PASS $why"; else echo "FAIL $why: got $got, want $want"; fails=$((fails + 1)); fi
}
t a,d "a module's twin and an import's closure" std/native/csv.xo
t a,d "a module its import reaches" std/bufio.xo
t b,d "two hops" std/compress.xo
t c,d "a module only the base IR names (a derive's codec)" std/json.xo
t d,e "a _native twin by its module" std/url_native.xo
t all "the prelude" std/native/prelude.xo
t all "a module the runtime reaches" std/netip.xo
t all "the runtime" std/native/llrt/atomic.xo
t all "the compiler" compiler/ssa/x.xo
t none "std/rt, the Go backend's runtime (#275)" std/rt/x.go std/rt/curg_arm64.s
t all "a std file that is not a module source" std/VERSION
t none "tests and docs" std/csv_test.xo std/changes/csv.md compiler/wasm/x.xo
t none "files that make no IR" scripts/x.sh
got=$(echo std/csv.xo | python3 "$here/golden-std-deps.py" "$T" -)
[ "$got" = a,b,c,d,e ] && echo "PASS no deps file: every golden" || { echo "FAIL no deps file: $got"; fails=$((fails + 1)); }
# inputs:DIR: exact input files from xo build --list-inputs (gate.sh golden_inputs)
mkdir -p "$T/in"
printf 'internal/llvm/testdata/golden/a/main.xo\nstd/csv.xo\nstd/native/csv.xo\nstd/bufio.xo\n' >"$T/in/a.txt"
printf 'std/png.xo\nstd/compress.xo\n' >"$T/in/b.txt"
printf 'std/json.xo\n' >"$T/in/c.txt"
printf 'std/url_native.xo\n' >"$T/in/e.txt"
ti() {
  local want=$1 why=$2; shift 2
  got=$(printf '%s\n' "$@" | python3 "$here/golden-std-deps.py" "$T" "inputs:$T/in")
  if [ "$got" = "$want" ]; then echo "PASS inputs: $why"; else echo "FAIL inputs: $why: got $got, want $want"; fails=$((fails + 1)); fi
}
ti a,d "a file a golden reads (d has no list: built)" std/bufio.xo
ti b,d "two hops are in the list" std/compress.xo
ti c,d "a derive's codec is in the list" std/json.xo
ti all "a removed std file" std/gone.xo
ti all "the compiler" compiler/ssa/x.xo
ti none "docs and scripts" std/changes/csv.md scripts/x.sh
ti a,d "std/rt beside a std file: that file's goldens (#275)" std/rt/http.go std/bufio.xo
[ "$fails" = 0 ] && echo "all passed"
exit $((fails > 0))
