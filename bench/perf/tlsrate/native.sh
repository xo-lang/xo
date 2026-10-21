#!/bin/bash
# bench/perf/tlsrate/native.sh XO OUT [BACKEND]: the HTTP/2 and TLS runs of
# docker.sh natively on this machine (no taskset, no server CPU column:
# /proc is Linux's), the Go server against the Xo server of BACKEND
# (default arm64), loopback, same client. REPS (2) interleaves them.
set -eu
xo=$1
out=$2
be=${3:-arm64}
here=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$out/srv" "$out/keys-ec"
cp "$here"/xosrv/main.xo "$here/xosrv/xo.mod" "$out/srv/"
(cd "$here/client" && go build -o "$out/client" .)
(cd "$here/gosrv" && go build -o "$out/gosrv" .)
(cd "$out/srv" && "$xo" build --backend="$be" -o "$out/xosrv" .)
"$out/client" -gencert "$out/keys-ec" -key ec
port=${PORT:-21500}
run() { # server label client args...
	srv=$1; label=$2; shift 2
	port=$((port + 1))
	if [ "$srv" = go ]; then
		CERT=$out/keys-ec/cert.pem KEY=$out/keys-ec/key.pem PORT=$port "$out/gosrv" > "$out/srv.log" 2>&1 &
	else
		CERT=$out/keys-ec/cert.pem KEY=$out/keys-ec/key.pem PORT=$port "$out/xosrv" > "$out/srv.log" 2>&1 < <(sleep 600) &
	fi
	p=$!
	sleep 2
	res=$("$out/client" -url 127.0.0.1:$port "$@" 2>&1 | tail -1)
	kill $p 2>/dev/null || true
	wait $p 2>/dev/null || true
	echo "$srv $label $res"
}
for r in $(seq 1 "${REPS:-2}"); do
	for s in go xo; do
		run $s "h2 1 stream" -mode h2 -c 1 -d 6s
		run $s "h2 100 streams" -mode h2 -c 100 -d 6s
		run $s "h2 download" -mode h2big -size 134217728 -reps 3
		run $s "hs x25519" -mode hs -curve x25519 -c 16 -d 6s
		run $s "hs p256" -mode hs -curve p256 -c 16 -d 6s
		run $s "hs resumed" -mode hs -resume -c 16 -d 6s
		run $s "bulk tls13" -mode bulk -tls 13 -size 134217728 -reps 3
		run $s "bulk tls12 chacha" -mode bulk -tls 12 -cipher chacha -size 134217728 -reps 3
	done
done
