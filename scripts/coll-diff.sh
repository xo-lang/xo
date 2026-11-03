#!/bin/bash
# coll-diff.sh XO DIR...: builds each progen.Coll program (internal/progen/coll.go,
# dumped by XO_COLL_DUMP=DIR go test ./internal/progen -run TestCollDump) with the
# go and llvm backends, the host's native backend and native with every optional pass
# off, runs them, and compares exit status and stdout. A program that differs is
# swept with each pass off alone (XO_PASSES=-NAME) to name the pass whose removal
# makes native agree with go. Output: one line per difference, then totals.
#   scripts/coll-diff.sh $PWD/bin/xo /work/coll/*      (JOBS=4 programs at once)
# SKIP_LLVM=1 (a host without clang, the amd64 offload host) compares native with the go backend only.
# REF=llvm compares against llvm instead of go (goldens and corpora that the go backend
# cannot build); programs that print timing or read the environment differ by themselves.
# With REF=llvm a golden's want_go.txt (a pinned Go backend gap) is go's reference, and a
# go run that times out is skipped, not counted as a difference.
set -u
XO=${1:?usage: coll-diff.sh XO DIR...}; shift
JOBS=${JOBS:-4}
# without timeout (macOS has none) every run exits 127 with no output and
# all programs compare the same (2026-11-02)
command -v timeout > /dev/null || { echo "coll-diff.sh: no timeout command (coreutils); every run would read exit 127"; exit 2; }
case $(uname -m) in aarch64|arm64) NATIVE=arm64 ;; *) NATIVE=amd64 ;; esac
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
passes=$(XO_PASSES=-zz "$XO" build --backend=$NATIVE -o /dev/null "$1" 2>&1 | sed -n 's/.*the passes are //p' | head -1)
[ -n "$passes" ] || { echo "cannot list passes"; exit 2; }
alloff=$(echo "$passes" | tr -d ' ' | sed 's/,/\n/g' | sed 's/^/-/' | paste -sd, -)
export XO NATIVE T passes alloff
one() {
	d=$1; n=$(basename "$d"); o=$T/$n; mkdir -p "$o"
	run() { # label backend [passes]
		if XO_PASSES=${3:-} "$XO" build --backend=$2 -o "$o/$1" "$d" > "$o/$1.build" 2>&1; then
			timeout -k 5 ${RUN_TIMEOUT:-10} "$o/$1" > "$o/$1.out" 2> "$o/$1.err"; echo "exit $?" >> "$o/$1.out"
		else echo "BUILD FAILED" > "$o/$1.out"; fi
	}
	run go go; if [ -n "${SKIP_LLVM:-}" ]; then cp "$o/go.out" "$o/llvm.out"; else run llvm llvm; fi; run native $NATIVE; run alloff $NATIVE "$alloff"
	ref=${REF:-go}
	if grep -q -E "^BUILD FAILED|^exit (124|137)$" "$o/$ref.out"; then echo "REFFAIL $n: the $ref run failed or timed out (load?), not compared"; return; fi
	# a golden whose want_go.txt pins a Go backend gap (native_sql: no
	# Postgres driver, decision 0278 point 9) is compared on go against
	# that file's exit status and stdout; a go run that timed out when go
	# is not the reference is load, not a difference (map_log_model runs
	# about 7 s on go, 2026-11-02)
	skip=""
	if [ $ref != go ] && [ -f "$d/want_go.txt" ]; then
		awk 'NR==1{e=$0;next} /^--- stdout$/{s=1;next} /^--- stderr$/{s=0} s{print} END{print e}' "$d/want_go.txt" > "$o/want_go.out"
		if cmp -s "$o/want_go.out" "$o/go.out"; then skip=go; echo "  $n: go matches its want_go.txt (a pinned gap)"; fi
	fi
	if [ $ref != go ] && grep -q -E "^exit (124|137)$" "$o/go.out"; then skip=go; echo "  $n: go timed out (load?), not compared"; fi
	bad=""
	for b in go llvm native alloff; do
		[ $b = $ref ] || [ $b = "$skip" ] || cmp -s "$o/$ref.out" "$o/$b.out" || bad="$bad $b"
	done
	# a run that timed out (exit 124 or 137) is load, not a difference:
	# reported as TIMEOUT with its outputs kept, never as DIFF (a native
	# program that ran 10 s under load was a DIFF, 2026-11-02)
	to=""
	for b in go llvm native alloff; do
		grep -q -E "^exit (124|137)$" "$o/$b.out" && to="$to $b"
	done
	# a build that failed in a non-reference backend is a tool or host
	# problem (a cache file pruned under the run, a full disk), not a
	# difference: BUILDFAIL with the build logs kept (four aliasbig
	# programs read DIFF with "BUILD FAILED" output, 2026-11-03)
	bf=""
	for b in go llvm native alloff; do
		grep -q "^BUILD FAILED" "$o/$b.out" && bf="$bf $b"
	done
	if [ -n "$bf" ]; then
		echo "BUILDFAIL $n:$bf"
		mkdir -p "${KEEP:-/tmp/coll-diff-keep}/$n" && cp -r "$d/." "${KEEP:-/tmp/coll-diff-keep}/$n/" && cp "$o"/*.out "$o"/*.build "${KEEP:-/tmp/coll-diff-keep}/$n/" 2>/dev/null
		return
	fi
	if [ -n "$to" ]; then
		echo "TIMEOUT $n:$to"
		mkdir -p "${KEEP:-/tmp/coll-diff-keep}/$n" && cp -r "$d/." "${KEEP:-/tmp/coll-diff-keep}/$n/" && cp "$o"/*.out "${KEEP:-/tmp/coll-diff-keep}/$n/"
		return
	fi
	if [ -z "$bad" ]; then echo "same $n"; else
		echo "DIFF $n:$bad"
		if ! cmp -s "$o/$ref.out" "$o/native.out" && cmp -s "$o/$ref.out" "$o/llvm.out"; then
			for p in $(echo "$passes" | tr -d ' ' | sed 's/,/ /g'); do
				run sweep $NATIVE "-$p"
				cmp -s "$o/$ref.out" "$o/sweep.out" && echo "  $n: native agrees with $ref when $p is off"
			done
		fi
		# the source and every backend's output (stdout and exit), to compare
		mkdir -p "${KEEP:-/tmp/coll-diff-keep}" && cp -r "$d" "${KEEP:-/tmp/coll-diff-keep}/$n" && cp "$o"/*.out "$o"/*.err "${KEEP:-/tmp/coll-diff-keep}/$n/" 2>/dev/null
	fi
}
export -f one
printf '%s\n' "$@" | xargs -P "$JOBS" -I{} bash -c 'one {}' | tee "$T/log"
echo "reference failed: $(grep -c '^REFFAIL' "$T/log"), programs: $(grep -c '^\(same\|DIFF\|REFFAIL\|TIMEOUT\|BUILDFAIL\)' "$T/log"), same: $(grep -c '^same' "$T/log"), differ: $(grep -c '^DIFF' "$T/log"), timeout: $(grep -c '^TIMEOUT' "$T/log"), build failed: $(grep -c '^BUILDFAIL' "$T/log")"
