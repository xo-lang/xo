#!/bin/bash
# b21-gdb.sh (temporary, B21 diagnosis): the fixed point's stage s1 under
# gdb (every thread's stack at the crash), then with XO_EMIT_JOBS 2 and 4,
# twice each.
set -uo pipefail
cd "$(dirname "$0")/../.."
root=$PWD
command -v gdb >/dev/null || sudo -n dnf install -y -q gdb >/dev/null 2>&1 || true
command -v gdb || echo "no gdb"
s1() {
  local d
  d=$(mktemp -d)
  (cd "$d" && XO_BUILD_DIR="$d/b" "$@" build --backend=arm64 --std="$root/std" -o "$d/xo" "$root/selfhost/xo" > "$d/log" 2>&1)
  local rc=$?
  rm -rf "$d"
  return $rc
}
if command -v gdb >/dev/null; then
  d=$(mktemp -d)
  cd "$d"
  t0=$(date +%s)
  XO_BUILD_DIR="$d/b" gdb -q -batch -ex "set pagination off" -ex "handle SIGUSR1 SIGUSR2 SIGPIPE SIGPROF nostop noprint pass" \
    -ex run -ex "info registers pc sp x0 x1 x2" -ex "x/6i \$pc - 12" -ex "thread apply all bt 14" \
    --args "$XO_CLI" build --backend=arm64 --std="$root/std" -o "$d/xo" "$root/selfhost/xo" 2>&1 | grep -v "^\[New Thread\|^\[Thread .* exited\|^\[Detaching" | tail -400
  echo "== gdb secs=$(( $(date +%s) - t0 ))"
  cd "$root"
  rm -rf "$d"
fi
for j in 2 2 4 4; do
  t0=$(date +%s)
  XO_EMIT_JOBS=$j s1 "$XO_CLI"
  echo "== jobs=$j rc=$? secs=$(( $(date +%s) - t0 ))"
done
