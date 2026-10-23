#!/bin/sh
# Rust against Xo (and Go, for the server rows): the twins of bench/perf's
# rows, each in Rust's own idiom (tokio and axum for the servers, serde for
# JSON, tokio tasks and std threads for the concurrency rows, plain loops
# for compute), pinned by Cargo.lock (every build uses --locked).
#
#   compute   bench/perf/llvm/rust (llperf) against llvm/xo/<row>: fib sieve
#             nbody sort interp collatz strings
#   conc      bench/perf/conc/rust (concperf) against conc/xo/<row>: spawn
#             fanout pingpong mutex rwmutex pmap
#   json      bench/perf/rust/json against jsontwin/xo (decode, encode), with
#             Go's encoding/json (jsontwin/go) as a reference column
#   http      bench/perf/rust/http (axum, tokio, serde_json) against the Go
#             and Xo CRUD servers of bench/perf/http, through its run.sh (the
#             Go load generator, keep alive, interleaved reps)
#
# Compute, conc and json rows use `xo bench --ab` (A = Rust, B = Xo; the
# ratio is B / A, so below 1 is Xo faster): wall, cpu, instructions and
# max RSS over ROUNDS rounds (default 10), the checksums compared first.
# The http rows print rps, p50/p99/p99.9 and server CPU per request (REPS,
# DURATION, WARMUP, CONC as bench/perf/http/run.sh). Every measurement waits
# for the 1 minute load average to drop under MAXLOAD (default 5).
#
# Writes bench/results/rust-vs-xo-DATE.md (DATE=$(date +%F)) with the method,
# the machine, the versions and every table. Environment: XO (default
# .tools/bin/xo), ROUNDS, REPS, ROWS (some of: compute conc json http),
# OUT (a directory for binaries and logs), RESULT (the file to write).
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
result=${RESULT:-$root/bench/results/rust-vs-xo-$date.md}
cargo=${CARGO:-$(command -v cargo || echo "$HOME/.cargo/bin/cargo")}
. "$root/bench/perf/xocheck.sh" "$xo" "$root"

. "$here/../twins/lib.sh"
build_rust() { # DIR BIN OUT
	(cd "$1" && CARGO_TARGET_DIR="$out/target-$2" "$cargo" build --release --locked -q) && cp "$out/target-$2/release/$2" "$3"
}
crates=$(for d in http json; do awk '/^name = "(axum|tokio|hyper|serde|serde_json)"$/{n=$3} /^version/{if(n){printf "%s %s; ", n, $3; n=""}}' "$here/$d/Cargo.lock"; done | tr -d '"')
header Rust "$("$cargo" --version), $(rustc --version 2>/dev/null || true)" "crates (Cargo.lock): $crates" "release profile of every crate: opt-level 3, codegen-units 1, thin LTO, panic abort" > "$out/header.md"
: > "$out/body.md"

for r in $rows; do
	case $r in
	compute)
		build_rust "$here/../llvm/rust" llperf "$out/llperf-rust"
		echo "## Compute (llvm/rust against llvm/xo, \`xo bench --ab\`, $rounds rounds)" >> "$out/body.md"
		echo >> "$out/body.md"
		for w in fib sieve nbody sort interp collatz strings; do
			(cd "$here/../llvm/xo/$w" && "$xo" build -o "$out/llp-xo-$w" . >/dev/null)
			agree "$w" "$out/llperf-rust" "$out/llp-xo-$w" "$w" 2>/dev/null || { "$out/llperf-rust" "$w"; "$out/llp-xo-$w"; exit 1; }
			ab "$w" "$out/llperf-rust" "$out/llp-xo-$w" "$w" >> "$out/body.md"
		done ;;
	conc)
		build_rust "$here/../conc/rust" concperf "$out/concperf-rust"
		echo >> "$out/body.md"
		echo "## Concurrency (conc/rust: tokio tasks, std threads; against conc/xo)" >> "$out/body.md"
		echo >> "$out/body.md"
		for w in spawn fanout pingpong mutex rwmutex pmap; do
			(cd "$here/../conc/xo/$w" && "$xo" build -o "$out/conc-xo-$w" . >/dev/null)
			ab "$w" "$out/concperf-rust" "$out/conc-xo-$w" "$w" >> "$out/body.md"
		done ;;
	json)
		build_rust "$here/json" jsontwin "$out/json-rust"
		(cd "$here/../jsontwin/go" && go build -o "$out/json-go" .)
		(cd "$here/../jsontwin/xo" && "$xo" build -o "$out/json-xo" . >/dev/null)
		echo >> "$out/body.md"
		echo "## JSON (serde_json against std/json; 1000 users, 100 rounds)" >> "$out/body.md"
		echo >> "$out/body.md"
		for w in decode encode; do
			agree "json $w" "$out/json-rust" "$out/json-xo" "$w"
			agree "json $w" "$out/json-go" "$out/json-xo" "$w"
			ab "json $w, Rust" "$out/json-rust" "$out/json-xo" "$w" >> "$out/body.md"
			ab "json $w, Go" "$out/json-go" "$out/json-xo" "$w" >> "$out/body.md"
		done ;;
	http)
		wait_load
		echo >> "$out/body.md"
		echo "## HTTP: the users CRUD API (axum 0.8 on tokio with serde_json; Go net/http; Xo std/http)" >> "$out/body.md"
		echo >> "$out/body.md"
		echo '```' >> "$out/body.md"
		PERF_OUT="$out/crud" CONFIGS="go xo-arenas rust" XO="$xo" CARGO="$cargo" sh "$here/../http/run.sh" >> "$out/body.md"
		echo '```' >> "$out/body.md" ;;
	*) echo "unknown row group $r" >&2; exit 2 ;;
	esac
done
mkdir -p "$(dirname "$result")"
cat "$out/header.md" "$out/body.md" > "$result"
echo "wrote $result"
