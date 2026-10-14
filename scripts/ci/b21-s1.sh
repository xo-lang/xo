#!/bin/bash
# b21-s1.sh (temporary, B21 diagnosis): the fixed point's stage s1 (the
# branch's xo building selfhost/xo with the arm64 backend, default emit
# jobs); when it crashes, systemd-coredump's report of that process
# (every thread's stack).
set -uo pipefail
cd "$(dirname "$0")/../.."
root=$PWD
d=$(mktemp -d)
cd "$d"
t0=$(date +%s)
XO_BUILD_DIR="$d/b" "$XO_CLI" build --backend=arm64 --std="$root/std" -o "$d/xo" "$root/selfhost/xo" > "$d/log" 2>&1 &
pid=$!
wait $pid
rc=$?
echo "== pid=$pid rc=$rc secs=$(( $(date +%s) - t0 ))"
tail -15 "$d/log"
if [ "$rc" -ge 128 ]; then
  sleep 5
  coredumpctl info --no-pager "$pid" 2>&1 | head -400
fi
