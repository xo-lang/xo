#!/usr/bin/env bash
# First native (arm64) build time with every cache cold: the C runtime
# compiled locally (XO_RTPACK=off) against a prebuilt pack (decision 0157).
#
#   bench/perf/rtpack/first-build.sh <xo binary> [runs]
#
# Use the xo of a release archive (bin/xo, its pack in lib/xo/rtpack) or set
# XO_RTPACK. Prints one line per build: program, mode, run, exit status, and
# wall seconds. Works on macOS and Linux (perl for the clock).
set -u
xo="$1"
runs="${2:-3}"
repo="$(cd "$(dirname "$0")/../../.." && pwd)"
work="$(mktemp -d "${TMPDIR:-/tmp}/xo-first-build.XXXXXX")"
trap 'rm -rf "$work"' EXIT
now() { perl -MTime::HiRes=time -e 'printf "%.3f", time'; }
echo "$("$xo" rtpack --status)"
for p in internal/arm64/testdata/run/boxes internal/gen/testdata/golden/time_zones bench/perf/http/xo; do
	for mode in off pack; do
		for i in $(seq 1 "$runs"); do
			rm -rf "$work/m"
			mkdir -p "$work/m/build" "$work/m/rtobj" "$work/m/tls" "$work/m/out"
			pack="${XO_RTPACK:-}"
			[ "$mode" = off ] && pack=off
			start=$(now)
			XO_RTPACK=$pack XO_BUILD_DIR="$work/m/build" XO_BIN_CACHE=off \
				XO_RTOBJ_CACHE="$work/m/rtobj" XO_TLS_CACHE="$work/m/tls" \
				"$xo" build --backend=arm64 -o "$work/m/out/prog" "$repo/$p" >"$work/log" 2>&1
			rc=$?
			end=$(now)
			echo "$p $mode run$i rc=$rc $(perl -e "printf '%.2f', $end - $start") s"
			[ "$rc" = 0 ] || tail -5 "$work/log"
		done
	done
done
