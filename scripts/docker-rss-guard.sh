#!/bin/bash
# docker-rss-guard.sh: guards the fiber stack THP fix (84e5ccc93,
# gotchas/fiber-stacks-huge-pages.md) in Docker (linux/arm64): the CRUD
# server of bench/perf/http runs with transparent huge pages set to
# "always" in the container's kernel, 1024 keep-alive connections each
# send requests, and the run fails when the server's peak RSS (VmHWM)
# exceeds a bound. With the fix it is about 50 MB; without it every
# connection's fiber stack faulted a 2 MiB huge page (about 2.2 GB).
#
#   scripts/docker-rss-guard.sh XO_CLI [BOUND_MB]
#
# BOUND_MB defaults to 150. Exit 0 when the peak is within the bound, 1
# when it is over, 2 on a usage, build or setup error; 0 with a note when
# Docker is not running (hourly.sh runs it on the Mac). The darwin xo
# cross builds the server (the Linux LLVM backend is not needed) and the
# Go load generator is cross built; about a minute, outside heavy.sh,
# niced as every Docker run.
set -uo pipefail

xo=${1:?usage: docker-rss-guard.sh XO_CLI [BOUND_MB]}
bound=${2:-150}
[ -x "$xo" ] || { echo "docker-rss-guard: $xo is not an executable xo" >&2; exit 2; }
if ! docker info >/dev/null 2>&1; then
  echo "docker-rss-guard: skipped, Docker is not running"
  exit 0
fi
root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/xo-rss-guard.XXXXXX")
trap 'rm -rf "${work:?}"' EXIT

(cd "$root/bench/perf/http/xo" && XO_STD="$root/std" XO_BIN_CACHE=off "$xo" build --backend=arm64 --target=linux/arm64 -o "$work/srv" .) > "$work/build.log" 2>&1 ||
  { tail -5 "$work/build.log" >&2; echo "docker-rss-guard: the server did not build" >&2; exit 2; }
(cd "$root/bench/perf/http/loadgen" && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o "$work/loadgen" .) > "$work/go.log" 2>&1 ||
  { tail -5 "$work/go.log" >&2; echo "docker-rss-guard: loadgen did not build" >&2; exit 2; }

cat > "$work/inside.sh" <<'IN'
#!/bin/bash
# as root, so the huge page mode can be set when the VM allows it
ulimit -n 65536
thp=/sys/kernel/mm/transparent_hugepage/enabled
(echo always > $thp) 2>/dev/null
mode=$(sed -E 's/.*\[([a-z]+)\].*/\1/' $thp 2>/dev/null)
PORT=25600 /work/srv > /work/srv.log 2>&1 &
p=$!
sleep 2
/work/loadgen -url http://127.0.0.1:25600 -c 1024 -d 6s -warmup 2s -pid $p > /work/lg.json 2>/work/lg.err
hwm=$(awk '/VmHWM/{print $2}' /proc/$p/status)
kill $p; wait $p 2>/dev/null
echo "thp=$mode hwm_kb=$hwm $(tail -1 /work/lg.json | sed -E 's/.*"rps":([0-9.]+).*"errors":([0-9]+).*/rps=\1 errors=\2/')"
[ -n "$hwm" ] || { cat /work/srv.log /work/lg.err; exit 2; }
echo "$hwm" > /work/hwm
IN
chmod +x "$work/inside.sh"
out=$(XO_DOCKER_ROOT=1 nice -n 10 "$root/scripts/docker-linux.sh" arm64 "$work" -- ./inside.sh 2>&1) || { echo "$out" >&2; echo "docker-rss-guard: the container failed" >&2; exit 2; }
echo "$out"
hwm=$(cat "$work/hwm" 2>/dev/null) || { echo "docker-rss-guard: no RSS read" >&2; exit 2; }
mb=$((hwm / 1024))
case $out in *thp=always*) ;; *) echo "docker-rss-guard: note: huge pages are not 'always' in this kernel, the check is weaker" ;; esac
if [ "$mb" -gt "$bound" ]; then
  echo "docker-rss-guard: FAILED, peak RSS $mb MB over $bound MB (fiber stacks faulting huge pages? gotchas/fiber-stacks-huge-pages.md)" >&2
  exit 1
fi
echo "docker-rss-guard: ok, peak RSS $mb MB within $bound MB"
