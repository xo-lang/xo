#!/bin/sh
# Measures bench/perf/tls (TLS performance, decision 0202): builds the
# client with each backend named, with the Xo TLS (tls13.xo, tls12.xo;
# Mbed TLS, the other side until 2026-10-15, is removed), starts serve/ (its own PID,
# stopped at the end), and runs every workload 5 times under
# /usr/bin/time -l. Prints "<workload> <backend> <tls> <min instructions
# retired of the client> <best wall ms>". Handshake workloads make N
# requests (default 200), each its own connection; bulk workloads read
# SIZE bytes (default 64 MiB) once. A side that fails a workload prints
# its error in place of the numbers.
#
# Server workloads (srv-rsa2048, srv-rsa4096, srv-p256) measure the Xo
# server of server/ instead: its key and certificate from serve -keys,
# serve -dial as the client (N handshakes, Go's TLS 1.3 with X25519),
# the instructions those of the server process, which starts, serves the
# N connections, and stops when its stdin closes. srv-<key>-0 rows are
# the same run with no connection (start, key loading, stop).
#
# usage: bench/perf/tls/measure.sh <xo binary> <out dir> <backend...>
# WORKLOADS overrides the list; TLSES the TLS sides ("xo"); PORT
# the first listener's port (default: from the PID, see below; the next
# 10 and PORT+20 are taken too).
set -e
XO=$1
OUT=$2
shift 2
SRC=$(cd "$(dirname "$0")" && pwd)
N=${N:-200}
SIZE=${SIZE:-67108864}
# a base of its own unless PORT is set: one of 400 30-port buckets from
# 20000 by this shell's PID (it takes PORT to PORT+10 and PORT+20), so
# two sessions measuring at once do not reach each other's servers
# (gotchas/tls-measure-port-collision.md)
PORT=${PORT:-$((20000 + ($$ % 400) * 30))}
mkdir -p "$OUT"
(cd "$SRC/serve" && go build -o "$OUT/serve" .)
for be in "$@"; do
	for t in ${TLSES:-xo}; do
		rm -rf "$OUT/src-$be-$t"
		mkdir -p "$OUT/src-$be-$t"
		cp "$SRC/main.xo" "$SRC/xo.mod" "$OUT/src-$be-$t/"
		(cd "$OUT/src-$be-$t" && "$XO" build --backend="$be" -o "$OUT/c-$be-$t" . >/dev/null)
	done
	rm -rf "$OUT/srvsrc-$be"
	mkdir -p "$OUT/srvsrc-$be"
	cp "$SRC/server/main.xo" "$SRC/server/xo.mod" "$OUT/srvsrc-$be/"
	(cd "$OUT/srvsrc-$be" && "$XO" build --backend="$be" -o "$OUT/s-$be" . >/dev/null)
done
mkdir -p "$OUT/keys"
"$OUT/serve" -keys "$OUT/keys" -ca "$OUT/srvca.pem"
SPORT=$((PORT + 20))
# srv KEY BACKEND CONNECTIONS: one server run; sets c and ms. The
# server's stdin is a pipe from the subshell that waits for it and runs
# the client (a FIFO does not work: the native stdin never sees its EOF,
# gotchas/native-stdin-fifo-eof.md).
srv() {
	rm -f "$OUT/srvout.txt"
	(
		j=0
		until grep -q ready "$OUT/srvout.txt" 2>/dev/null; do
			j=$((j + 1))
			if [ $j -gt 1000 ]; then
				echo "xo server did not start" >&2
				exit 1
			fi
			sleep 0.02
		done
		if [ "$3" -gt 0 ]; then
			"$OUT/serve" -dial "127.0.0.1:$SPORT" -n "$3" -ca "$OUT/srvca.pem" >&2
		fi
	) | CERT="$OUT/keys/$1.crt" KEY="$OUT/keys/$1.key" PORT=$SPORT /usr/bin/time -l "$OUT/s-$2" >"$OUT/srvout.txt" 2>"$OUT/srvtime.txt"
	c=$(awk '/instructions retired/{print $1}' "$OUT/srvtime.txt")
	ms=$(awk '/real/{printf "%d", $1 * 1000}' "$OUT/srvtime.txt")
}
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
for w in ${WORKLOADS:-x25519 x25519only p256 p384 p521 rsa2048 ed25519 tls12 aes128 aes256 chacha aes128tls13 srv-rsa2048 srv-rsa4096 srv-p256}; do
	case $w in
	srv-*)
		for be in "$@"; do
			for n in $N 0; do
				best=""
				bt=""
				for k in 1 2 3 4 5; do
					srv "${w#srv-}" "$be" "$n"
					if [ -z "$best" ] || [ "$c" -lt "$best" ]; then best=$c; fi
					if [ -z "$bt" ] || [ "$ms" -lt "$bt" ]; then bt=$ms; fi
				done
				tag=$w
				[ "$n" = 0 ] && tag=$w-0
				echo "$tag $be xo $best $bt"
			done
		done
		continue
		;;
	esac
	n=$N
	# x25519only: the x25519 handshake with no ML-KEM share offered
	# (XO_TLS_MLKEM=0, std.md), as Mbed TLS offers none; x25519 is the
	# client's default, X25519MLKEM768 and X25519 as Go 1.26
	mlkem=""
	case $w in x25519only) mlkem=0 ;; esac
	case $w in aes128 | aes256 | chacha | aes128tls13) n=1 ;; esac
	for be in "$@"; do
		for t in ${TLSES:-xo}; do
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
