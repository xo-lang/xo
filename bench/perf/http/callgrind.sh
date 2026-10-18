#!/bin/sh
# callgrind.sh: the server's instructions per request of bench/perf/http
# (the users CRUD API, xo/) on Linux under valgrind's callgrind, a count
# that does not swing with the machine's load as wall clock rates do.
# Runs inside a Linux container or host with valgrind;
# callgrind-docker.sh builds everything and runs it from a Mac.
#
#   bench/perf/http/callgrind.sh OUT LOADGEN NAME=SERVER...
#
# For each server and scenario the server runs twice under callgrind,
# loaded by LOADGEN (loadgen/, a separate process) with a fixed count of
# serial requests (-route cycle: POST, GET, PATCH, GET, DELETE of a new
# user, and a list every 8th cycle) after 8 preloaded users: N cycles, then
# 2N. It is stopped with SIGTERM (it exits, so callgrind writes its count)
# and the instructions per request are (Ir(2N) - Ir(N)) / (requests(2N) -
# requests(N)): startup, preload, and shutdown cancel. Ir counts every
# thread. Scenarios (SCENARIOS): plain (HTTP/1.1, one keep-alive
# connection), plain-newconn (a connection per request), tls (one
# keep-alive TLS connection), tls-newconn (a full TLS handshake per
# request), h2 (HTTP/2 over TLS, the requests streams of one connection;
# loadgen's -h2det writes each request's frames in one TLS record and -gap
# pauses 20 ms before each request so the server is waiting when it comes:
# without them its count varied 5% between repeats, as the client's
# flushes and the server's read attempts raced, 2026-10-17).
# REPS (2) repeats each measurement; N (100) cycles. ANN=1 also keeps each
# run's per function counts (callgrind_annotate) in OUT/ann.SCENARIO.CYCLES
# with its requests in OUT/req.SCENARIO.CYCLES (the last repeat's), for
# bench/perf/http/cgcompare.py and xo dev compare. KEEP=1 keeps each
# run's callgrind output too, as OUT/cg.SCENARIO.CYCLES, for
# callgrind_annotate --tree=caller (who calls the allocator, 2026-10-18).
#
# Prints one line per measurement: `<scenario> <name> <instructions per
# request> <requests>`, and the raw counts to OUT/raw.txt.
set -eu
out=$1
lg=$2
shift 2
n=${N:-100}
reps=${REPS:-2}
scenarios=${SCENARIOS:-"plain plain-newconn tls tls-newconn h2"}
mkdir -p "$out"
"$lg" -gencert "$out"
port=19100
: > "$out/raw.txt"

# run SERVER SCENARIO CYCLES: prints "<Ir> <requests>"
run() {
	srv=$1
	sc=$2
	cycles=$3
	port=$((port + 1))
	scheme=http
	flags=""
	case $sc in
	plain) ;;
	plain-newconn) flags="-newconn" ;;
	tls) scheme=https ;;
	tls-newconn) scheme=https; flags="-newconn" ;;
	h2) scheme=https; flags="-h2 -h2det -gap 20ms" ;;
	*) echo "callgrind.sh: unknown scenario $sc" >&2; exit 2 ;;
	esac
	cg="$out/cg.$port"
	if [ "$scheme" = https ]; then
		TLS_CERT=$(cat "$out/cert.pem") TLS_KEY=$(cat "$out/key.pem") PORT=$port \
			valgrind --tool=callgrind --callgrind-out-file="$cg" "$srv" > "$out/srv.$port.log" 2> "$out/vg.$port.log" &
	else
		PORT=$port valgrind --tool=callgrind --callgrind-out-file="$cg" "$srv" > "$out/srv.$port.log" 2> "$out/vg.$port.log" &
	fi
	vp=$!
	reqs=$("$lg" -url "$scheme://127.0.0.1:$port" -preload 8 -route cycle -n "$cycles" $flags | sed 's/[^0-9]//g')
	kill -TERM "$vp"
	wait "$vp" || true
	ir=$(sed -n 's/.*I *refs: *//p' "$out/vg.$port.log" | tr -d ',')
	if [ "${ANN:-0}" = 1 ]; then
		callgrind_annotate --auto=no --threshold=100 "$cg" > "$out/ann.$sc.$cycles" 2>/dev/null || true
		echo "$reqs" > "$out/req.$sc.$cycles"
	fi
	if [ "${KEEP:-0}" = 1 ]; then
		mv "$cg" "$out/cg.$sc.$cycles"
		echo "$reqs" > "$out/req.$sc.$cycles"
	fi
	rm -f "$cg"
	echo "$ir $reqs"
}

for spec in "$@"; do
	name=${spec%%=*}
	srv=${spec#*=}
	for sc in $scenarios; do
		for r in $(seq 1 "$reps"); do
			set -- $(run "$srv" "$sc" "$n")
			ir1=$1
			q1=$2
			set -- $(run "$srv" "$sc" $((2 * n)))
			ir2=$1
			q2=$2
			echo "$sc $name $r: $ir1 Ir for $q1 requests, $ir2 Ir for $q2" >> "$out/raw.txt"
			echo "$sc $name $(( (ir2 - ir1) / (q2 - q1) )) $((q2 - q1))"
		done
	done
done
