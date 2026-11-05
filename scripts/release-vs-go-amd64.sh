#!/bin/bash
# release-vs-go-amd64.sh DIR XO ROUNDS: the amd64 section of release-vs-go.sh,
# on the linux/amd64 offload host (lead, 2026-11-04: json10kx20 was 1.78 times
# slower on linux/amd64 in 0.38.1 than in 0.35.0 and the arm64 only report
# did not show it). DIR holds rvg-micro13.base, rvg-micro13.new and rvg-micro13.go
# (bench/perf/micro13 built for linux/amd64 with the release before, the
# release and Go); XO is an xo on the host that runs `xo bench --ab`. Per
# workload: the release before against the release (A = base, B = new), then
# Go against the release (A = go, B = new). Never pattern-kills.
set -u -o pipefail
dir=${1:?usage: release-vs-go-amd64.sh DIR XO ROUNDS}
xo=${2:?usage: release-vs-go-amd64.sh DIR XO ROUNDS}
rounds=${3:-5}
chmod +x "$dir"/rvg-micro13.base "$dir"/rvg-micro13.new "$dir"/rvg-micro13.go
echo "host $(uname -sm), $(nproc) CPUs, load $(cut -d' ' -f1 /proc/loadavg)"
for w in fib32 map1M strbuild1M sort1M sort1M_copy json10kx20 spawn100k; do
	for a in base go; do
		echo "### $w: $a against new"
		"$xo" bench --ab --rounds="$rounds" --metrics=wall_ms,cpu_ms,max_rss_kb "$dir/rvg-micro13.$a" "$dir/rvg-micro13.new" -- "$w" 2>&1 | grep -v '^round ' | sed "s#$dir/##g"
	done
done
