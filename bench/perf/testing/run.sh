#!/bin/bash
# The std/testing twin (ADR 0655 rule 7): a suite of N tests (1,000 by
# default) with the same trivial body, run by `xo test` and by `go test`.
# Both commands run warm (a first run fills the build caches) with the
# result caches off (--count=1, -count=1), so each round is the harness:
# finding the tests, starting the test binary, running each test and
# reporting. Prints one line per round and side, then the medians and
# the ratio (Xo over Go). Go's whole run links its test binary each time
# (its build cache keeps compiled packages, not the linked test), so a
# second row times Go's prebuilt test binary alone (go test -c, then
# pkg.test): the lower bound of Go's harness against Xo's whole command
# (xo test finds its cached binary and runs the tests in batches).
#
# usage: bench/perf/testing/run.sh <xo binary> <scratch dir> [N] [rounds]
set -eu
XO=${1:?xo binary}; OUT=${2:?scratch dir}; N=${3:-1000}; R=${4:-11}
mkdir -p "$OUT/xo" "$OUT/go"
OUT=$(cd "$OUT" && pwd)
printf 'module example.com/testingtwin\nedition preview\n' >"$OUT/xo/xo.mod"
printf 'module example.com/testingtwin\n\ngo 1.22\n' >"$OUT/go/go.mod"
printf 'package testingtwin\n' >"$OUT/go/t.go"
{
	for i in $(seq 1 "$N"); do
		printf 'test "t%d" {\n  let n = %d\n  expect(n * 2) == %d\n}\n\n' "$i" "$i" $((i * 2))
	done
} >"$OUT/xo/t_test.xo"
{
	printf 'package testingtwin\n\nimport "testing"\n\n'
	for i in $(seq 1 "$N"); do
		printf 'func TestT%d(t *testing.T) {\n\tn := %d\n\tif n*2 != %d {\n\t\tt.Fatalf("got %%d", n*2)\n\t}\n}\n\n' "$i" "$i" $((i * 2))
	done
} >"$OUT/go/t_test.go"
xo_run() { (cd "$OUT/xo" && "$XO" test --count=1 . >"$OUT/xo.txt" 2>&1); }
go_run() { (cd "$OUT/go" && go test -count=1 . >"$OUT/go.txt" 2>&1); }
gobin_run() { (cd "$OUT/go" && ./testingtwin.test >"$OUT/gobin.txt" 2>&1); }
ms() { local t0 t1; t0=$(date +%s%N 2>/dev/null || python3 -c 'import time;print(time.time_ns())'); "$@"; t1=$(date +%s%N 2>/dev/null || python3 -c 'import time;print(time.time_ns())'); echo $(((t1 - t0) / 1000000)); }
xo_run || { cat "$OUT/xo.txt"; exit 1; }
go_run || { cat "$OUT/go.txt"; exit 1; }
(cd "$OUT/go" && go test -c -o testingtwin.test .) || exit 1
# the passed counts, so a run that found fewer tests shows
grep -q -E "(^| )$N passed" "$OUT/xo.txt" || { echo "xo test did not report $N passed:"; tail -3 "$OUT/xo.txt"; exit 1; }
: >"$OUT/xo.ms"; : >"$OUT/go.ms"; : >"$OUT/gobin.ms"
for r in $(seq 1 "$R"); do
	a=$(ms xo_run); b=$(ms go_run); c=$(ms gobin_run)
	echo "round $r: xo $a ms, go $b ms, go binary $c ms"
	echo "$a" >>"$OUT/xo.ms"; echo "$b" >>"$OUT/go.ms"; echo "$c" >>"$OUT/gobin.ms"
done
med() { sort -n "$1" | awk '{a[NR]=$1} END {print a[int((NR+1)/2)]}'; }
x=$(med "$OUT/xo.ms"); g=$(med "$OUT/go.ms"); gb=$(med "$OUT/gobin.ms")
echo "tests $N rounds $R: xo test ${x} ms, go test ${g} ms, ratio $(awk -v x="$x" -v g="$g" 'BEGIN{printf "%.2f", x/g}')"
echo "tests $N rounds $R: xo test ${x} ms, go's test binary alone ${gb} ms, ratio $(awk -v x="$x" -v g="$gb" 'BEGIN{printf "%.2f", x/g}')"
