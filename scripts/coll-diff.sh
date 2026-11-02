#!/bin/bash
# coll-diff.sh XO DIR...: builds each progen.Coll program (internal/progen/coll.go,
# dumped by XO_COLL_DUMP=DIR go test ./internal/progen -run TestCollDump) with the
# go and llvm backends, the host's native backend and native with every optional pass
# off, runs them, and compares exit status and stdout. A program that differs is
# swept with each pass off alone (XO_PASSES=-NAME) to name the pass whose removal
# makes native agree with go. Output: one line per difference, then totals.
#   scripts/coll-diff.sh $PWD/bin/xo /work/coll/*      (JOBS=4 programs at once)
# REF=llvm compares against llvm instead of go (goldens and corpora that the go backend
# cannot build); programs that print timing or read the environment differ by themselves.
set -u
XO=${1:?usage: coll-diff.sh XO DIR...}; shift
JOBS=${JOBS:-4}
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
			timeout 60 "$o/$1" > "$o/$1.out" 2> "$o/$1.err"; echo "exit $?" >> "$o/$1.out"
		else echo "BUILD FAILED" > "$o/$1.out"; fi
	}
	run go go; run llvm llvm; run native $NATIVE; run alloff $NATIVE "$alloff"
	ref=${REF:-go}
	bad=""
	for b in go llvm native alloff; do
		[ $b = $ref ] || cmp -s "$o/$ref.out" "$o/$b.out" || bad="$bad $b"
	done
	if [ -z "$bad" ]; then echo "same $n"; else
		echo "DIFF $n:$bad"
		if ! cmp -s "$o/$ref.out" "$o/native.out"; then
			for p in $(echo "$passes" | tr -d ' ' | sed 's/,/ /g'); do
				run sweep $NATIVE "-$p"
				cmp -s "$o/$ref.out" "$o/sweep.out" && echo "  $n: native agrees with $ref when $p is off"
			done
		fi
		mkdir -p "${KEEP:-/tmp/coll-diff-keep}" && cp -r "$d" "${KEEP:-/tmp/coll-diff-keep}/$n"
	fi
}
export -f one
printf '%s\n' "$@" | xargs -P "$JOBS" -I{} bash -c 'one {}' | tee "$T/log"
echo "programs: $(grep -c '^\(same\|DIFF\)' "$T/log"), same: $(grep -c '^same' "$T/log"), differ: $(grep -c '^DIFF' "$T/log")"
