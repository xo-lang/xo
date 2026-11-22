#!/bin/sh
# lib_test.sh: bench/perf/twins/lib.sh's load() on a linux host, where
# there is no vm.loadavg sysctl. The old load() piped a failing sysctl into
# awk, so the pipeline succeeded with no output, the /proc/loadavg fallback
# never ran, and wait_load's test read "Illegal number" (host 1, task #687,
# 2026-11-22). Run: sh bench/perf/twins/lib_test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
t=$(mktemp -d)
trap 'rm -rf "$t"' EXIT
# a sysctl that fails as linux's does for vm.loadavg, and a fake loadavg
printf '#!/bin/sh\necho "sysctl: cannot stat /proc/sys/vm/loadavg" >&2\nexit 255\n' > "$t/sysctl"; chmod +x "$t/sysctl"
echo "2.50 1.00 0.50 1/100 42" > "$t/loadavg"
fail=0
got=$(PATH="$t:$PATH" LOADAVG_FILE="$t/loadavg" sh -c '. "$1"; load' sh "$here/lib.sh" 2>/dev/null)
[ "$got" = "2.50" ] && echo "ok load: $got from the loadavg file" || { echo "FAIL load: got '$got', want 2.50"; fail=1; }
got=$(PATH="$t:$PATH" LOADAVG_FILE="$t/loadavg" sh -c 'maxload=5; . "$1"; wait_load && echo done' sh "$here/lib.sh" 2>&1)
[ "$got" = "done" ] && echo "ok wait_load: under maxload, no error" || { echo "FAIL wait_load: '$got'"; fail=1; }
exit $fail
