#!/bin/sh
# Heap allocations per request of the bench/perf/http servers, per route:
# a measure that does not depend on host load (run.sh measures req/s).
#
# Each server is started fresh with its allocation counters on
# (XO_MEMSTATS for Xo, MEMSTATS for Go: mallocs, bytes, GCs of the whole
# run, written when the server is stopped with `kill <its own PID>`),
# preloaded with PRELOAD users, and sent N1 then (in a second fresh
# server) N2 units of one route by `loadgen -route` on one connection.
# The difference of the two runs divided by the difference in requests
# cancels startup and preload. Routes: get, list (PRELOAD users), patch,
# createdel (one POST and one DELETE; per request is the mean of the two),
# cycle (the run.sh mix: POST, GET, PATCH, GET, DELETE, a list every
# eighth cycle).
#
# Prints a Markdown table: allocations per request, bytes per request,
# and GCs per 10k requests (GOGC=100; at one connection the live heap is
# smaller than under run.sh's 32, so this GC count is a lower bound).
#
# Environment: CONFIGS ("go xo xo-arenas"; xo builds with --no-arenas,
# xo-arenas with the default request arenas of decision 0066; xo-base with
# the binary in $XO_BASE and its default, xo-base-noarenas the same with
# --no-arenas),
# ROUTES ("get list patch createdel cycle"), N1 (500), N2 (5500), PRELOAD
# (64), PORT (18190, apart from run.sh's 18080), XO, PERF_OUT.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
out=${PERF_OUT:-$(mktemp -d)}
mkdir -p "$out"
configs=${CONFIGS:-go xo xo-arenas}
routes=${ROUTES:-get list patch createdel cycle}
n1=${N1:-500}
n2=${N2:-5500}
preload=${PRELOAD:-64}
port=${PORT:-18190}
xo=${XO:-$root/.tools/bin/xo}

(cd "$here/loadgen" && go build -o "$out/loadgen" .)
for c in $configs; do
	case $c in
	go) (cd "$here/go" && go build -o "$out/srv-go" .) ;;
	xo) "$xo" build --no-arenas -o "$out/srv-xo" "$here/xo" >/dev/null ;;
	xo-arenas) "$xo" build -o "$out/srv-xo-arenas" "$here/xo" >/dev/null ;;
	xo-base) XO_BUILD_DIR="$out/build-base" "${XO_BASE:?set XO_BASE to an xo binary}" build -o "$out/srv-xo-base" "$here/xo" >/dev/null ;;
	xo-base-noarenas) XO_BUILD_DIR="$out/build-base" "${XO_BASE:?set XO_BASE to an xo binary}" build --no-arenas -o "$out/srv-xo-base-noarenas" "$here/xo" >/dev/null ;;
	*) echo "unknown config $c" >&2; exit 2 ;;
	esac
done

# runone config route n -> "requests mallocs bytes gcs"
runone() {
	stats="$out/$1-$2-$3.memstats"
	rm -f "$stats"
	PORT=$port MEMSTATS=$stats XO_MEMSTATS=$stats "$out/srv-$1" >/dev/null 2>&1 &
	pid=$!
	res=$("$out/loadgen" -url "http://127.0.0.1:$port" -preload "$preload" -route "$2" -n "$3") || {
		kill "$pid" 2>/dev/null || true
		wait "$pid" 2>/dev/null || true
		echo "loadgen failed for $1 $2" >&2
		exit 1
	}
	kill "$pid" 2>/dev/null || true
	wait "$pid" 2>/dev/null || true
	reqs=$(echo "$res" | tr -dc '0-9')
	awk -v r="$reqs" '{ v[$1] = $2 } END { print r, v["mallocs"], v["total_alloc"], v["num_gc"] }' "$stats"
}

echo "| Config | Route | Allocs/req | Bytes/req | GCs/10k req |"
echo "|---|---|---|---|---|"
for c in $configs; do
	for r in $routes; do
		a=$(runone "$c" "$r" "$n1")
		b=$(runone "$c" "$r" "$n2")
		echo "$a $b" | awk -v c="$c" -v r="$r" '{
			d = $5 - $1
			printf "| %s | %s | %.1f | %.0f | %.1f |\n", c, r, ($6 - $2) / d, ($7 - $3) / d, ($8 - $4) * 10000 / d
		}'
	done
done
