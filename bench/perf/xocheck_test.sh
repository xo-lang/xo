#!/bin/sh
# xocheck_test.sh: bench/perf/xocheck.sh under dash, the /bin/sh of the
# linux offload hosts. dash's `.` passes no arguments to the sourced file,
# so `. xocheck.sh "$xo" "$root"` reached it with $1 unset and every
# #!/bin/sh caller (rust/run.sh, node/run.sh, ...) stopped at its usage
# line on host 1 (task #687, 2026-11-22). Run: sh bench/perf/xocheck_test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
dash=$(command -v dash || true)
[ -n "$dash" ] || { echo "xocheck_test: SKIP (no dash)"; exit 0; }
t=$(mktemp -d)
trap 'rm -rf "$t"' EXIT
# a fake xo with no build stamp: xocheck must get past its usage line and
# refuse it for the stamp (or measure it under XO_BENCH_ANY=1)
printf '#!/bin/sh\necho {}\n' > "$t/xo"; chmod +x "$t/xo"
fail=0
out=$(cd "$t" && XO_BENCH_ANY=1 "$dash" -c 'xo=$1; root=$2; x=$3; set --; . "$x" "$xo" "$root"; echo "stamp=$XO_STAMP"' sh "$t/xo" "$t" "$here/xocheck.sh" 2>&1)
case $out in
*usage:*) echo "FAIL dash: usage error: $out"; fail=1 ;;
*"stamp=xo "*"NOT THIS CHECKOUT"*) echo "ok dash: XO_BENCH_ANY measures the unstamped xo" ;;
*) echo "FAIL dash: unexpected: $out"; fail=1 ;;
esac
out=$(cd "$t" && "$dash" -c 'xo=$1; root=$2; x=$3; set --; . "$x" "$xo" "$root"; echo reached' sh "$t/xo" "$t" "$here/xocheck.sh" 2>&1)
case $out in
*"is not a build of this checkout"*) echo "ok dash: refuses the unstamped xo" ;;
*) echo "FAIL dash: no refusal: $out"; fail=1 ;;
esac
exit $fail
