#!/bin/bash
# Binary size, Go build cache growth, peak RSS, and wall time of `xo build`
# (or `xo test` with MODE=test) for each program directory, against a fresh
# Go cache whose runtime is already compiled (with and without std/http).
# Results: bench/results/binary-size-2026-10-09.md.
#
#   XO=/path/to/xo bench/size/measure.sh <dir>...
#
# XO defaults to the checkout's bin/xo (go run ./scripts/xobuild). RSS is the largest child
# process (compile or link). Temporary directories are removed at the end.
set -eu
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d)
trap 'chmod -R u+w "$tmp" 2>/dev/null; rm -rf "$tmp"' EXIT
xo=${XO:-}
if [ -z "$xo" ]; then
	# the checkout's bin/xo, built by an earlier xo (decision 0215)
	xo=$(cd "$root" && go run ./scripts/xobuild)
fi
export XO_GOCACHE=$tmp/gocache XO_BUILD_DIR=$tmp/build
mkdir -p "$tmp/warm" "$tmp/warmhttp"
printf 'fn main(os: Os) { os.stdio.println("warm") }\n' > "$tmp/warm/main.xo"
printf 'use std/http\n\nfn main(os: Os) { os.stdio.println(http.text(200, "warm").text() ?? "") }\n' > "$tmp/warmhttp/main.xo"
"$xo" build -o "$tmp/warmbin" "$tmp/warm" >/dev/null
"$xo" build -o "$tmp/warmbin" "$tmp/warmhttp" >/dev/null
for d in "$@"; do
	before=$(du -sk "$XO_GOCACHE" | cut -f1)
	if [ "${MODE:-build}" = test ]; then
		/usr/bin/time -l "$xo" test "$d" 2> "$tmp/time.txt" >/dev/null || true
		size=-
	else
		/usr/bin/time -l "$xo" build -o "$tmp/out" "$d" 2> "$tmp/time.txt" >/dev/null
		size=$(wc -c < "$tmp/out" | tr -d ' ')
	fi
	after=$(du -sk "$XO_GOCACHE" | cut -f1)
	rss=$(awk '/maximum resident/ {print int($1 / 1048576)}' "$tmp/time.txt")
	real=$(awk '/ real/ {print $1}' "$tmp/time.txt")
	echo "$(basename "$d") binary=$size cache_kb=$((after - before)) rss_mb=$rss real_s=$real"
done
