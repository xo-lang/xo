#!/bin/sh
# Build bench/perf/xo with .tools/bin/xo (or $XO) and bench/perf/go with go, run each
# workload REPS times (default 5), and print the best time of each side in
# milliseconds with the ratio. Checksums must agree. Run from anywhere.
#
# MONO=1 also builds the Xo program with `xo build --mono` and adds a column
# for it (and its ratio to Go). XOFLAGS adds flags to the default Xo build.
# Workloads: collection (putget get push update iter copymut branch) and
# generic (sortint sortpt hof maxby iface); WORKLOADS selects some.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
out=${PERF_OUT:-$(mktemp -d)}
reps=${REPS:-5}
mono=${MONO:-0}
xo=${XO:-$root/.tools/bin/xo}
# shellcheck disable=SC2086
"$xo" build ${XOFLAGS:-} -o "$out/perf-xo" "$here/xo" >/dev/null
if [ "$mono" = 1 ]; then
	"$xo" build --mono -o "$out/perf-xo-mono" "$here/xo" >/dev/null
fi
(cd "$here/go" && go build -o "$out/perf-go" .)
best() { # binary workload -> "checksum best_us"
	b=""
	sum=""
	i=0
	while [ "$i" -lt "$reps" ]; do
		set -- $("$1" "$2") "$1" "$2"
		sum=$2
		if [ -z "$b" ] || [ "$3" -lt "$b" ]; then b=$3; fi
		shift 3
		i=$((i + 1))
	done
	echo "$sum $b"
}
if [ "$mono" = 1 ]; then
	printf '| Workload | Go ms | Xo ms | Xo --mono ms | Xo/Go | Xo --mono/Go |\n|---|---|---|---|---|---|\n'
else
	printf '| Workload | Go ms | Xo ms | Ratio |\n|---|---|---|---|\n'
fi
for w in ${WORKLOADS:-putget get push update iter copymut branch sortint sortpt hof maxby iface}; do
	set -- $(best "$out/perf-go" "$w")
	gs=$1 gt=$2
	set -- $(best "$out/perf-xo" "$w")
	xs=$1 xt=$2
	if [ "$gs" != "$xs" ]; then echo "checksum mismatch in $w: go $gs xo $xs" >&2; exit 1; fi
	if [ "$mono" = 1 ]; then
		set -- $(best "$out/perf-xo-mono" "$w")
		ms=$1 mt=$2
		if [ "$gs" != "$ms" ]; then echo "checksum mismatch in $w: go $gs xo --mono $ms" >&2; exit 1; fi
		awk -v w="$w" -v g="$gt" -v x="$xt" -v m="$mt" 'BEGIN { printf "| %s | %.1f | %.1f | %.1f | %.2fx | %.2fx |\n", w, g/1000, x/1000, m/1000, x/g, m/g }'
	else
		awk -v w="$w" -v g="$gt" -v x="$xt" 'BEGIN { printf "| %s | %.1f | %.1f | %.1fx |\n", w, g/1000, x/1000, x/g }'
	fi
done
