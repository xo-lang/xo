#!/bin/sh
# Build bench/perf/xo with .tools/bin/xo (or $XO) and bench/perf/go with go, run each
# workload REPS times (default 5), and print the best time of each side in
# milliseconds with the ratio. Checksums must agree. Run from anywhere.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
out=${PERF_OUT:-$(mktemp -d)}
reps=${REPS:-5}
"${XO:-$root/.tools/bin/xo}" build -o "$out/perf-xo" "$here/xo" >/dev/null
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
printf '| Workload | Go ms | Xo ms | Ratio |\n|---|---|---|---|\n'
for w in ${WORKLOADS:-putget get push update iter copymut branch}; do
	set -- $(best "$out/perf-go" "$w")
	gs=$1 gt=$2
	set -- $(best "$out/perf-xo" "$w")
	xs=$1 xt=$2
	if [ "$gs" != "$xs" ]; then echo "checksum mismatch in $w: go $gs xo $xs" >&2; exit 1; fi
	awk -v w="$w" -v g="$gt" -v x="$xt" 'BEGIN { printf "| %s | %.1f | %.1f | %.1fx |\n", w, g/1000, x/1000, x/g }'
done
