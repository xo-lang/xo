#!/bin/bash
# cpuburst/run.sh OUT [ROUNDS] [LOOPS...]: connection bursts against a
# server whose handlers are CPU bound (README.md). Builds the Xo server
# with XO (default: xo on PATH) and the Go one, then for each round and
# each count of /cpu loops runs Xo with serve() in main (A), Xo with
# serve() in a spawned task (B), and Go, interleaved; each run is
# loadgen -burst 200 -reps 10 while LOOPS curl loops call /cpu.
set -u
out=${1:?usage: run.sh OUT [ROUNDS] [LOOPS...]}
rounds=${2:-2}
shift 2 2>/dev/null || shift $#
loops=${*:-0 8 16}
here=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$out"
xo=${XO:-xo}
(cd "$here/xo" && "$xo" build --backend=arm64 --release -o "$out/srv-xo" .) >"$out/build.txt" 2>&1 || { echo "xo build failed ($out/build.txt)"; exit 1; }
(cd "$here/go" && go build -o "$out/srv-go" .) || exit 1
(cd "$here/../loadgen" && go build -o "$out/loadgen" .) || exit 1
port=18095
run() { # WHO LOOPS ROUND
  case $1 in
  A) PORT=$port "$out/srv-xo" >"$out/srv.log" 2>&1 & ;;
  B) PORT=$port MODE=spawn "$out/srv-xo" >"$out/srv.log" 2>&1 & ;;
  Go) PORT=$port "$out/srv-go" >"$out/srv.log" 2>&1 & ;;
  esac
  sp=$!
  for _ in $(seq 1 100); do curl -s -o /dev/null "http://127.0.0.1:$port/users" && break; sleep 0.05; done
  lp=""
  for _ in $(seq 1 "$2"); do (while curl -s -o /dev/null "http://127.0.0.1:$port/cpu"; do :; done) & lp="$lp $!"; done
  [ "$2" -gt 0 ] && sleep 1
  echo "== $1 cpu$2 round $3 load $(uptime | sed 's/.*averages*: //' | cut -d' ' -f1 | tr -d ,)"
  "$out/loadgen" -url "http://127.0.0.1:$port" -burst 200 -reps 10 2>&1 | grep -E '^burst .*(failed|dial p50)' | sed 's/^burst 200 x 10: //'
  for p in $lp; do kill "$p" 2>/dev/null; done
  kill "$sp"; wait "$sp" 2>/dev/null
}
for r in $(seq 1 "$rounds"); do
  for l in $loops; do run A "$l" "$r"; run B "$l" "$r"; run Go "$l" "$r"; done
done | tee "$out/results.txt"
