#!/bin/sh
# Measures bench/perf/tls (TLS performance, decision 0202): builds the
# client with each backend named, with the Xo TLS (tls13.xo, tls12.xo)
# and with Mbed TLS (XO_LLRT_C=tlsio), starts serve/ (its own PID,
# stopped at the end), and runs every workload 5 times under
# /usr/bin/time -l. Prints "<workload> <backend> <tls> <min instructions
# retired of the client> <best wall ms>". Handshake workloads make N
# requests (default 200), each its own connection; bulk workloads read
# SIZE bytes (default 64 MiB) once. A side that fails a workload prints
# its error in place of the numbers.
#
# usage: bench/perf/tls/measure.sh <xo binary> <out dir> <backend...>
# WORKLOADS overrides the list; TLSES the TLS sides ("xo mbed"); PORT
# the first listener's port (default 18120, the next 9 are taken too).
set -e
XO=$1
OUT=$2
shift 2
SRC=$(cd "$(dirname "$0")" && pwd)
N=${N:-200}
SIZE=${SIZE:-67108864}
PORT=${PORT:-18120}
mkdir -p "$OUT"
(cd "$SRC/serve" && go build -o "$OUT/serve" .)
for be in "$@"; do
	for t in ${TLSES:-xo mbed}; do
		rm -rf "$OUT/src-$be-$t"
		mkdir -p "$OUT/src-$be-$t"
		cp "$SRC/main.xo" "$SRC/xo.mod" "$OUT/src-$be-$t/"
		if [ "$t" = mbed ]; then
			(cd "$OUT/src-$be-$t" && XO_LLRT_C=tlsio "$XO" build --backend="$be" -o "$OUT/c-$be-$t" . >/dev/null)
		else
			(cd "$OUT/src-$be-$t" && "$XO" build --backend="$be" -o "$OUT/c-$be-$t" . >/dev/null)
		fi
	done
done
rm -f "$OUT/ca.pem"
"$OUT/serve" -port "$PORT" -ca "$OUT/ca.pem" >"$OUT/serve.txt" 2>&1 &
spid=$!
trap 'kill $spid 2>/dev/null || true' EXIT
i=0
until grep -q ready "$OUT/serve.txt" 2>/dev/null; do
	i=$((i + 1))
	if [ $i -gt 1000 ]; then
		echo "server did not start" >&2
		exit 1
	fi
	sleep 0.02
done
for w in ${WORKLOADS:-x25519 x25519only p256 p384 rsa2048 ed25519 tls12 aes128 aes256 chacha aes128tls13}; do
	n=$N
	# x25519only: the x25519 handshake with no ML-KEM share offered
	# (XO_TLS_MLKEM=0, std.md), as Mbed TLS offers none; x25519 is the
	# client's default, X25519MLKEM768 and X25519 as Go 1.26
	mlkem=""
	case $w in x25519only) mlkem=0 ;; esac
	case $w in aes128 | aes256 | chacha | aes128tls13) n=1 ;; esac
	for be in "$@"; do
		for t in ${TLSES:-xo mbed}; do
			best=""
			bt=""
			for k in 1 2 3 4 5; do
				XO_TLS_MLKEM=$mlkem W=$w N=$n SIZE=$SIZE PORT=$PORT CA="$OUT/ca.pem" /usr/bin/time -l "$OUT/c-$be-$t" >"$OUT/out.txt" 2>"$OUT/time.txt"
				if grep -q error "$OUT/out.txt"; then
					# a side that cannot do the workload (Mbed's TLS has no
					# Ed25519 certificates): its error instead of numbers
					best="error: $(head -1 "$OUT/out.txt")"
					bt=""
					break
				fi
				c=$(awk '/instructions retired/{print $1}' "$OUT/time.txt")
				ms=$(awk '/real/{printf "%d", $1 * 1000}' "$OUT/time.txt")
				if [ -z "$best" ] || [ "$c" -lt "$best" ]; then best=$c; fi
				if [ -z "$bt" ] || [ "$ms" -lt "$bt" ]; then bt=$ms; fi
			done
			echo "$w $be $t $best $bt"
		done
	done
done
