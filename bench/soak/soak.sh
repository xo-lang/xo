#!/bin/sh
# bench/soak/soak.sh XO OUT: a steady-load soak of examples/notes-service
# (README.md). MODE=native (default) builds and runs the server and the
# load client on this machine; MODE=docker builds a linux/arm64 xo (like
# bench/perf/http/callgrind-docker.sh) and runs both in the Docker
# container (servers on cores 0-3, the client on 4-7).
#   DURATION (1h) RATE (500/s) EVERY (10s) BACKEND (arm64)
#   WARMUP (0.1: the fraction of the run before drift is judged; the
#   connection pool and the heap grow in steps for the first 40 minutes)
# The server runs with XO_LEAKCHECK=1 and gets SIGTERM at the end, so its
# leak report is in OUT/server.log. Exit status: the drift verdict.
set -eu
xo=$1
out=$2
mode=${MODE:-native}
dur=${DURATION:-1h}
rate=${RATE:-500}
every=${EVERY:-10s}
be=${BACKEND:-arm64}
warm=${WARMUP:-0.1}
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
mkdir -p "$out/srv"
cp "$root"/examples/notes-service/main.xo "$out/srv/"
printf 'module soak/notes\nedition preview\n' > "$out/srv/xo.mod"
if [ "$mode" = docker ]; then
	mkdir -p "$out/xo/bin"
	. "$root/bench/perf/xocheck.sh" "$xo" "$root"
	if [ ! -x "$out/xo/bin/xo" ]; then
		(cd "$root/compiler/xo" && XO_INLINE_STRICT=1 XO_STD="$root/std" XO_BIN_CACHE=off "$xo" build --backend=arm64 --targets=linux/arm64 -o "$out/xo/bin/xo" .)
	fi
	rm -rf "$out/xo/std"
	cp -R "$root/std" "$out/xo/std"
	(cd "$here/client" && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$out/soak" .)
	cat > "$out/inside.sh" <<INSIDE
#!/bin/bash
set -u
cd /work/srv && XO_STD=/work/xo/std XO_BIN_CACHE=off PWD=/work/srv /work/xo/bin/xo build --backend=$be -o /work/notes .
cd /work
NOTES_TOKEN=soak-token NOTES_ADDR=127.0.0.1:18700 XO_LEAKCHECK=1 taskset -c 0-3 ./notes > server.log 2>&1 &
p=\$!
sleep 2
taskset -c 4-7 ./soak -url http://127.0.0.1:18700 -token soak-token -pid \$p -rate $rate -d $dur -every $every -warmup $warm -out /work/result
st=\$?
kill \$p; wait \$p 2>/dev/null
exit \$st
INSIDE
	chmod +x "$out/inside.sh"
	XO_DOCKER_ROOT=1 "$root/scripts/docker-linux.sh" arm64 "$out" -- ./inside.sh
	exit $?
fi
(cd "$here/client" && go build -o "$out/soak" .)
(cd "$out/srv" && "$xo" build --backend="$be" -o "$out/notes" .)
NOTES_TOKEN=soak-token NOTES_ADDR=127.0.0.1:18700 XO_LEAKCHECK=1 "$out/notes" > "$out/server.log" 2>&1 &
p=$!
sleep 2
st=0
"$out/soak" -url http://127.0.0.1:18700 -token soak-token -pid "$p" -rate "$rate" -d "$dur" -every "$every" -warmup "$warm" -out "$out/result" || st=$?
kill "$p" 2>/dev/null || true
wait "$p" 2>/dev/null || true
cp "$out/server.log" "$out/result/" 2>/dev/null || true
exit $st
