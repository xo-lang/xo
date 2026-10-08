#!/bin/sh
# Load benchmark of the task 01 users CRUD API (bench/perf/http): the
# plain Go solution (go/, a copy of bench/go/plain/01-crud-api) against the
# Xo solution (xo/, a copy of bench/xo/v0.4/01-crud-api), built with
# .tools/bin/xo (or $XO) with and without --arenas. Both copies listen on
# 127.0.0.1 only. Run from anywhere.
#
# Each server is started fresh with GODEBUG=gctrace=1, loaded by loadgen/
# for WARMUP then DURATION, and stopped with `kill <its own PID>`. Configs
# are interleaved REPS times (default 3) because laptop timing drifts.
# Prints one raw line per run and a Markdown table of the medians:
#   rps, p50/p99/p99.9 latency (measured window only), GC count, total STW
#   pause (sum of the two stop the world phases of every GC), peak heap
#   (largest heap at GC start, MB, from gctrace), GC CPU percent, peak RSS
#   (MB, sampled every 100 ms), and GCs per 10k requests (whole run).
#
# Environment: REPS (3), DURATION (10s), WARMUP (2s), CONC (32), PRELOAD
# (64), PORT (18080), CONFIGS ("go xo xo-arenas"; "xo-base" builds the Xo
# server with the xo binary in $XO_BASE, for before and after runs), XO,
# PERF_OUT (keep binaries and logs there), GOGC passed through to the
# servers.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
out=${PERF_OUT:-$(mktemp -d)}
mkdir -p "$out"
reps=${REPS:-3}
dur=${DURATION:-10s}
warm=${WARMUP:-2s}
conc=${CONC:-32}
preload=${PRELOAD:-64}
port=${PORT:-18080}
configs=${CONFIGS:-go xo xo-arenas}
xo=${XO:-$root/.tools/bin/xo}

(cd "$here/loadgen" && go build -o "$out/loadgen" .)
for c in $configs; do
	case $c in
	go) (cd "$here/go" && go build -o "$out/srv-go" .) ;;
	xo) "$xo" build -o "$out/srv-xo" "$here/xo" >/dev/null ;;
	xo-arenas) "$xo" build --arenas -o "$out/srv-xo-arenas" "$here/xo" >/dev/null ;;
	xo-base) XO_BUILD_DIR="$out/build-base" "${XO_BASE:?set XO_BASE to an xo binary}" build -o "$out/srv-xo-base" "$here/xo" >/dev/null ;;
	*) echo "unknown config $c" >&2; exit 2 ;;
	esac
done

# gcstats log -> "gcs pause_ms peak_heap_mb gc_cpu_pct"
gcstats() {
	awk '/^gc [0-9]+ @/ {
		n++
		for (i = 1; i <= NF; i++) {
			if ($(i+1) == "ms" && $(i+2) == "clock,") { split($i, a, "+"); p += a[1] + a[3] }
			if ($i ~ /^[0-9]+->[0-9]+->[0-9]+$/) { split($i, h, "->"); if (h[1] + 0 > m) m = h[1] + 0 }
			if ($i ~ /%:$/) { cpu = $i; sub(/%:/, "", cpu) }
		}
	} END { printf "%d %.3f %d %s\n", n, p, m, (cpu == "" ? 0 : cpu) }' "$1"
}

runone() { # config rep -> raw line
	bin="$out/srv-$1"
	log="$out/$1-$2.gctrace"
	PORT=$port GODEBUG=gctrace=1 "$bin" >/dev/null 2>"$log" &
	pid=$!
	res=$("$out/loadgen" -url "http://127.0.0.1:$port" -c "$conc" -d "$dur" -warmup "$warm" -preload "$preload" -pid "$pid") || {
		kill "$pid" 2>/dev/null || true
		wait "$pid" 2>/dev/null || true
		echo "loadgen failed for $1" >&2
		exit 1
	}
	kill "$pid" 2>/dev/null || true
	wait "$pid" 2>/dev/null || true
	set -- "$1" "$2" $(gcstats "$log")
	echo "$res" | awk -v cfg="$1" -v gcs="$3" -v pause="$4" -v heap="$5" -v cpu="$6" '{
		gsub(/[{}"]/, ""); n = split($0, kv, ",")
		for (i = 1; i <= n; i++) { split(kv[i], p, ":"); v[p[1]] = p[2] }
		printf "%s %.0f %.0f %.0f %.0f %d %d %.2f %d %s %.1f %.2f\n", cfg, v["rps"], v["p50_us"], v["p99_us"], v["p999_us"], v["errors"], gcs, pause, heap, cpu, v["peak_rss_kb"] / 1024, gcs * 10000 / v["all_requests"]
	}'
}

raw="$out/raw.txt"
: >"$raw"
echo "# config rps p50_us p99_us p999_us errors gcs pause_ms peak_heap_mb gc_cpu_pct peak_rss_mb gcs_per_10k"
i=1
while [ "$i" -le "$reps" ]; do
	for c in $configs; do
		line=$(runone "$c" "$i")
		echo "$line"
		echo "$line" >>"$raw"
		sleep 1
	done
	i=$((i + 1))
done

echo
echo "Medians of $reps runs, $conc workers, ${dur} measured after ${warm} warmup, loopback."
echo
echo "| Config | Req/s | p50 us | p99 us | p99.9 us | Errors | GCs | GC pause ms | Peak heap MB | GC CPU % | Peak RSS MB | GCs/10k req |"
echo "|---|---|---|---|---|---|---|---|---|---|---|---|"
for c in $configs; do
	row="| $c"
	col=2
	while [ "$col" -le 12 ]; do
		med=$(awk -v c="$c" -v k="$col" '$1 == c { print $k }' "$raw" | sort -g | awk '{ a[NR] = $1 } END { print a[int((NR + 1) / 2)] }')
		row="$row | $med"
		col=$((col + 1))
	done
	echo "$row |"
done
