#!/bin/sh
# Node (TypeScript, compiled by tsc, Node's own http, JSON and worker_threads)
# against Xo, and Go for the server rows: the twins of bench/perf's rows in
# Node's own idiom, versions pinned by package-lock.json (npm ci).
#
#   compute   dist/compute/fib.js against llvm/xo/fib
#   conc      dist/conc/conc.js against conc/xo: spawn fanout pingpong (async
#             tasks on the event loop), mutex pmap (worker_threads, Atomics)
#   json      dist/json/json.js against jsontwin/xo, Go's jsontwin as a reference
#   http      dist/http/server.js (node:http, one process: the store is in
#             memory) against the Go and Xo CRUD servers, through
#             bench/perf/http/run.sh (the Go load generator, keep alive)
#
# See bench/perf/twins/lib.sh for how the `--ab` rows are measured and
# bench/perf/rust/run.sh for the environment (XO, ROUNDS, REPS, ROWS, OUT,
# RESULT). Writes bench/results/node-vs-xo-DATE.md.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
xo=${XO:-$root/.tools/bin/xo}
rounds=${ROUNDS:-10}
maxload=${MAXLOAD:-5}
rows=${ROWS:-compute conc json http}
out=${OUT:-$(mktemp -d)}
mkdir -p "$out"
out=$(cd "$out" && pwd)
date=$(date +%F)
result=${RESULT:-$root/bench/results/node-vs-xo-$date.md}
. "$root/bench/perf/xocheck.sh" "$xo" "$root"
. "$here/../twins/lib.sh"
(cd "$here" && npm ci --no-audit --no-fund --silent && npx tsc -p .)
header "Node (TypeScript)" "node $(node --version), typescript $(cd "$here" && npx tsc --version | sed 's/Version //'), @types/node and typescript pinned in package-lock.json" > "$out/header.md"
: > "$out/body.md"
for r in $rows; do
	case $r in
	compute)
		(cd "$here/../llvm/xo/fib" && "$xo" build -o "$out/fib-xo" . >/dev/null)
		wrapper "$out/fib-node" node "$here/dist/compute/fib.js"
		echo "## Compute (recursive fib 40)" >> "$out/body.md"; echo >> "$out/body.md"
		agree fib "$out/fib-node" "$out/fib-xo"
		ab fib "$out/fib-node" "$out/fib-xo" >> "$out/body.md" ;;
	conc)
		wrapper "$out/conc-node" node "$here/dist/conc/conc.js"
		echo >> "$out/body.md"; echo "## Concurrency (async tasks, worker_threads; no rwmutex row: Node has no read write lock)" >> "$out/body.md"; echo >> "$out/body.md"
		for w in spawn fanout pingpong mutex pmap; do
			(cd "$here/../conc/xo/$w" && "$xo" build -o "$out/conc-xo-$w" . >/dev/null)
			agree "$w" "$out/conc-node" "$out/conc-xo-$w" "$w"
			ab "$w" "$out/conc-node" "$out/conc-xo-$w" "$w" >> "$out/body.md"
		done ;;
	json)
		wrapper "$out/json-node" node "$here/dist/json/json.js"
		(cd "$here/../jsontwin/go" && go build -o "$out/json-go" .)
		(cd "$here/../jsontwin/xo" && "$xo" build -o "$out/json-xo" . >/dev/null)
		echo >> "$out/body.md"; echo "## JSON (JSON.parse and JSON.stringify against std/json; 1000 users, 100 rounds)" >> "$out/body.md"; echo >> "$out/body.md"
		for w in decode encode; do
			agree "json $w" "$out/json-node" "$out/json-xo" "$w"
			ab "json $w, Node" "$out/json-node" "$out/json-xo" "$w" >> "$out/body.md"
			ab "json $w, Go" "$out/json-go" "$out/json-xo" "$w" >> "$out/body.md"
		done ;;
	http)
		wait_load
		echo >> "$out/body.md"; echo "## HTTP: the users CRUD API (node:http, one process; Go net/http; Xo std/http)" >> "$out/body.md"; echo >> "$out/body.md"
		echo '```' >> "$out/body.md"
		PERF_OUT="$out/crud" CONFIGS="go xo-arenas node" XO="$xo" sh "$here/../http/run.sh" >> "$out/body.md"
		echo '```' >> "$out/body.md" ;;
	*) echo "unknown row group $r" >&2; exit 2 ;;
	esac
done
mkdir -p "$(dirname "$result")"
cat "$out/header.md" "$out/body.md" > "$result"
echo "wrote $result"
