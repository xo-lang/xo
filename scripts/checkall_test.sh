#!/bin/bash
# checkall_test.sh XO: scripts/checkall.sh lists an error a program has
# by mistake but not one its line expects (`// ERROR XOnnnn`, the
# errorcheck annotation; a new errorcheck test failed the gate's
# checkall step before, 2026-11-16).
set -u
xo=${1:?usage: checkall_test.sh XO}
here=$(cd "$(dirname "$0")" && pwd)
T=$(mktemp -d)
trap 'rm -rf "${T:?}"' EXIT
mkdir -p "$T/test/errorcheck/a" "$T/test/run/b"
printf 'fn main() {\n  let x: Int = "s" // ERROR XO0401 XO0110 "mismatch"\n}\n' >"$T/test/errorcheck/a/main.xo"
printf 'fn main() {\n  let y: Int = "s"\n}\n' >"$T/test/run/b/main.xo"
(cd "$T" && git init -q . && bash "$here/checkall.sh" "$xo" "$T/out.txt" >/dev/null)
fail=0
if grep -q 'errorcheck/a' "$T/out.txt"; then
  echo "FAIL: an expected error was listed:"; cat "$T/out.txt"; fail=1
fi
if ! grep -q 'run/b' "$T/out.txt"; then
  echo "FAIL: an unexpected error was not listed:"; cat "$T/out.txt"; fail=1
fi
[ $fail = 0 ] && echo "ok checkall_test"
exit $fail
