#!/bin/bash
# scripts/ci/tzlocal-windows.sh: std/time's "Local" on Windows (work item
# B27, std/native/llrt/tzhost.xo windows_local: the system's setting from
# GetDynamicTimeZoneInformation made into TZif data) against the Go
# backend's time.Local, under the runner's zone and each zone given as
# an argument (set with tzutil, the runner's zone restored at the end).
#   gh workflow run ci.yml -f ref=BRANCH -f suite=script -f runner=xo-windows \
#     -f packages="scripts/ci/tzlocal-windows.sh 'Pacific Standard Time' 'India Standard Time'"
set -euo pipefail
w=$(mktemp -d)
printf 'module example.com/tzlocal\nedition preview\n' > "$w/xo.mod"
cat > "$w/main.xo" <<'XO'
use std/time

fn main(os: Os) {
  match time.local(os.env) {
    Ok(z) => {
      for s in [-2000000000, 0, 1000000000, 1700000000, 1720000000, 4000000000] {
        let t = Time.UNIX_EPOCH + Duration.from_secs_int(s)
        os.stdio.println("${s} ${z.offset_at(t)} ${z.abbreviation_at(t)}")
      }
    }
    Err(e) => os.stdio.println("${e}")
  }
}
XO
orig=$(tzutil //g)
trap 'tzutil //s "$orig"' EXIT
fail=0
check() {
  (cd "$w" && unset TZ && "$XO_CLI" run --backend=go . > "$w/go.txt" 2>&1 && "$XO_CLI" run --backend=llvm . > "$w/xo.txt" 2>&1) || { echo "run failed"; cat "$w"/*.txt; fail=1; return; }
  if diff "$w/go.txt" "$w/xo.txt" > "$w/diff.txt"; then
    echo "same ($(tzutil //g)): $(head -1 "$w/xo.txt")"
  else
    echo "DIFFERS ($(tzutil //g)):"; cat "$w/diff.txt"; fail=1
  fi
  cat "$w/xo.txt" >> "$XO_CI_OUT/tzlocal.txt"
}
check
for z in "$@"; do
  tzutil //s "$z"
  check
done
exit $fail
