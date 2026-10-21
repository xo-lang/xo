#!/bin/sh
# bench/perf/tlsrate/docker.sh XO OUT [BACKEND...]: the Xo TLS and HTTP/2
# server against Go's net/http, in Docker (linux/arm64, quiet: servers on
# cores 0-5, the Go client on 6-11). XO is a darwin xo built from this
# checkout; it cross builds a linux xo (the Go backend), which builds
# xosrv/ with each BACKEND (default arm64 llvm) inside the container. The
# Go server and the client are cross built by Go. Runs, per server:
#   hs     TLS 1.3 handshakes per second: x25519, p256, x25519 resumed
#          (ECDSA P-256 certificate), plus x25519 with an RSA 2048 one
#   bulk   256 MiB over one TLS connection: TLS 1.3 (AES-128-GCM),
#          TLS 1.2 AES-128-GCM and TLS 1.2 ChaCha20-Poly1305
#   h2     HTTP/2 requests per second with 1 and 100 streams on one
#          connection, and a 256 MiB download
# Each result is a JSON line with the server's CPU time per unit of work.
# OUT/results.txt has them all; REPS (2) interleaves the servers.
set -eu
xo=$1
out=$2
shift 2
backends=${*:-"arm64 llvm"}
root=$(cd "$(dirname "$0")/../../.." && pwd)
here=$root/bench/perf/tlsrate
mkdir -p "$out/xo/bin" "$out/srv" "$out/home"
. "$root/bench/perf/xocheck.sh" "$xo" "$root"
echo "$XO_STAMP" > "$out/stamp.txt"
stamp="$(git -C "$root" rev-parse HEAD) arm64"
if ! git -C "$root" diff --quiet HEAD -- compiler std; then
	stamp="$stamp dirty"
fi
if [ ! -x "$out/xo/bin/xo" ] || [ "$(cat "$out/xo/commit.txt" 2>/dev/null)" != "$stamp" ] || [ "${stamp% dirty}" != "$stamp" ]; then
	rm -f "$out/xo/bin/xo" "$out/xo/commit.txt"
	(cd "$root/compiler/xo" && XO_INLINE_STRICT=1 XO_STD="$root/std" XO_BIN_CACHE=off "$xo" build --backend=arm64 --targets=linux/arm64 -o "$out/xo/bin/xo" .)
	echo "$stamp" > "$out/xo/commit.txt"
fi
rm -rf "$out/xo/std"
cp -R "$root/std" "$out/xo/std"
cp "$here"/xosrv/main.xo "$here/xosrv/xo.mod" "$out/srv/"
(cd "$here/client" && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$out/client" .)
(cd "$here/gosrv" && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$out/gosrv" .)
cat > "$out/inside.sh" <<INSIDE
#!/bin/bash
set -u
ulimit -n 65536 2>/dev/null || true
servers="go"
for b in $backends; do
	if ! (cd /work/srv && XO_INLINE_STRICT=1 XO_STD=/work/xo/std XO_BIN_CACHE=off PWD=/work/srv /work/xo/bin/xo build --backend=\$b -o /work/xosrv-\$b . > /work/build-\$b.log 2>&1); then
		echo "tlsrate/docker.sh: the \$b server did not build:" >&2
		tail -5 /work/build-\$b.log >&2
		exit 1
	fi
	servers="\$servers xo-\$b"
done
mkdir -p /work/keys-ec /work/keys-rsa
/work/client -gencert /work/keys-ec -key ec
/work/client -gencert /work/keys-rsa -key rsa
port=21000
# run SERVER KEYS LABEL CLIENTARGS...: starts the server, runs the client, prints a line
run() {
	srv=\$1; keys=\$2; label=\$3; shift 3
	port=\$((port + 1))
	case \$srv in
	go) CERT=/work/keys-\$keys/cert.pem KEY=/work/keys-\$keys/key.pem PORT=\$port taskset -c 0-5 /work/gosrv > /work/srv.log 2>&1 & ;;
	xo-*) CERT=/work/keys-\$keys/cert.pem KEY=/work/keys-\$keys/key.pem PORT=\$port taskset -c 0-5 /work/xosrv-\${srv#xo-} > /work/srv.log 2>&1 < <(sleep 3600) & ;;
	esac
	p=\$!
	sleep 2
	res=\$(taskset -c 6-11 timeout 120 /work/client -url 127.0.0.1:\$port -pid \$p "\$@" 2>&1 | tail -1)
	kill \$p 2>/dev/null; wait \$p 2>/dev/null
	echo "\$srv \$label \$res"
}
for r in \$(seq 1 ${REPS:-2}); do
	for s in \$servers; do
		run \$s ec "hs x25519 ec" -mode hs -curve x25519 -c 32
		run \$s ec "hs p256 ec" -mode hs -curve p256 -c 32
		run \$s ec "hs x25519 resumed" -mode hs -curve x25519 -resume -c 32
		run \$s rsa "hs x25519 rsa2048" -mode hs -curve x25519 -c 32
		run \$s ec "bulk tls13" -mode bulk -tls 13 -reps 3
		run \$s ec "bulk tls12 aes" -mode bulk -tls 12 -cipher aes -reps 3
		run \$s ec "bulk tls12 chacha" -mode bulk -tls 12 -cipher chacha -reps 3
		run \$s ec "h2 1 stream" -mode h2 -c 1
		run \$s ec "h2 100 streams" -mode h2 -c 100
		run \$s ec "h2 download" -mode h2big -reps 3
	done
done
INSIDE
chmod +x "$out/inside.sh"
st=0
XO_DOCKER_ROOT=1 "$root/scripts/docker-linux.sh" arm64 "$out" -- ./inside.sh > "$out/results.txt" || st=$?
cat "$out/results.txt"
exit $st
