#!/bin/bash
# b21-s1.sh (temporary, B21 diagnosis): the fixed point's stage s1 (the
# branch's xo building selfhost/xo with the arm64 backend) with one emit
# job and with the default, core dumps on, a backtrace when it crashes.
set -uo pipefail
cd "$(dirname "$0")/../.."
ulimit -c unlimited || true
echo "core_pattern: $(cat /proc/sys/kernel/core_pattern 2>/dev/null)"
command -v gdb coredumpctl || true
nproc
for jobs in 1 default; do
  d=$(mktemp -d)
  cd "$d"
  t0=$(date +%s)
  if [ "$jobs" = 1 ]; then
    XO_EMIT_JOBS=1 XO_BUILD_DIR="$d/b" "$XO_CLI" build --backend=arm64 --std="$OLDPWD/std" -o "$d/xo" "$OLDPWD/selfhost/xo" > "$d/log" 2>&1
  else
    XO_BUILD_DIR="$d/b" "$XO_CLI" build --backend=arm64 --std="$OLDPWD/std" -o "$d/xo" "$OLDPWD/selfhost/xo" > "$d/log" 2>&1
  fi
  rc=$?
  cd "$OLDPWD"
  echo "== jobs=$jobs rc=$rc secs=$(( $(date +%s) - t0 ))"
  tail -15 "$d/log"
  if [ "$rc" -ge 128 ]; then
    ls -la "$d" | head
    core=$(ls "$d"/core* 2>/dev/null | head -1)
    if [ -n "$core" ] && command -v gdb >/dev/null; then
      gdb -batch -ex "bt 30" -ex "info registers" -ex "x/8i \$pc - 16" "$XO_CLI" "$core" 2>&1 | tail -60
    elif command -v coredumpctl >/dev/null; then
      coredumpctl info --no-pager -1 2>&1 | tail -60
    fi
  fi
  cp "$d/log" "$XO_CI_OUT/s1-$jobs.log" 2>/dev/null || true
done
