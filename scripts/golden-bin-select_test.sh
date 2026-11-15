#!/bin/bash
# golden-bin-select_test.sh: scripts/golden-bin-select.py on made-up goldens
# (decision 0657): an id-only difference is identical, a code difference,
# a side that did not build and a C golden are selected.
set -u
here=$(cd "$(dirname "$0")" && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/gbs-test.XXXXXX")
trap 'rm -rf "${T:?}"' EXIT
mk() { # SIDE NAME CONTENT
  mkdir -p "$T/$1/$2" && printf '%s' "$3" >"$T/$1/$2/prog"
}
for g in idonly code failed cgold same; do mkdir -p "$T/src/$g"; echo 'fn main(os: Os) {}' >"$T/src/$g/main.xo"; echo "$g"; done >"$T/names.txt"
printf 'use c "x.h"\nfn main(os: Os) {}\n' >"$T/src/cgold/main.xo"
mk a idonly 'X{"id":"0123456789abcdef01234567","dir":""}Y xo 0.51.1, arm64, linux/arm64, id 0123456789abcdef01234567, debug'
mk b idonly 'X{"id":"fedcba9876543210fedcba98","dir":""}Y xo 0.51.1, arm64, linux/arm64, id fedcba9876543210fedcba98, debug'
mk a code 'X{"id":"0123456789abcdef01234567"} add x0, x1'
mk b code 'X{"id":"0123456789abcdef01234567"} sub x0, x1'
mk a failed 'only the base built it'
mk a cgold 'same'
mk b cgold 'same'
mk a same 'same bytes'
mk b same 'same bytes'
got=$(python3 "$here/golden-bin-select.py" "$T/a" "$T/b" "$T/src" "$T/names.txt" 2>"$T/err" | paste -sd, -)
want=code,failed,cgold
if [ "$got" != "$want" ]; then
  echo "FAIL: selected $got, want $want ($(cat "$T/err"))"; exit 1
fi
grep -q '3 of 5: 1 differ, 1 did not build on a side, 1 use C; 2 identical' "$T/err" || { echo "FAIL: count line: $(cat "$T/err")"; exit 1; }
echo "PASS golden-bin-select_test.sh"
