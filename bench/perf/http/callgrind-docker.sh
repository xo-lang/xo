#!/bin/sh
# callgrind-docker.sh: runs callgrind.sh in scripts/docker-linux.sh's
# linux/arm64 Debian image from a Mac (decision 0180's numbers stay the
# Mac's; these are the quiet ones: Docker Desktop's VM has no hardware
# counters for perf stat, and valgrind's count is the same run to run).
#
#   bench/perf/http/callgrind-docker.sh XO OUT [BACKEND...]
#
# XO is a darwin xo whose std is the checkout's; it cross builds a
# linux/arm64 xo (compiler/xo, with the checkout's std beside it), and Go
# cross builds loadgen. In the container (as root, to install valgrind)
# that xo builds bench/perf/http/xo with each BACKEND (default arm64 llvm;
# LLVM builds only for the host, so the server is built there for both),
# and callgrind.sh measures each. OUT keeps the binaries, logs, and
# results.txt. N, REPS, SCENARIOS pass through.
set -eu
xo=$1
out=$2
shift 2
backends=${*:-"arm64 llvm"}
root=$(cd "$(dirname "$0")/../../.." && pwd)
mkdir -p "$out/xo/bin" "$out/srv"
if [ ! -x "$out/xo/bin/xo" ]; then
	(cd "$root/compiler/xo" && XO_STD="$root/std" XO_BIN_CACHE=off "$xo" build --backend=arm64 --targets=linux/arm64 -o "$out/xo/bin/xo" .)
fi
rm -rf "$out/xo/std"
cp -R "$root/std" "$out/xo/std"
cp "$root"/bench/perf/http/xo/*.xo "$root/bench/perf/http/xo/xo.mod" "$out/srv/"
(cd "$root/bench/perf/http/loadgen" && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$out/loadgen" .)
cp "$root/bench/perf/http/callgrind.sh" "$out/callgrind.sh"
cat > "$out/inside.sh" <<INSIDE
#!/bin/sh
set -eu
apt-get update > /dev/null 2>&1
apt-get install -y --no-install-recommends valgrind > /dev/null 2>&1
servers=""
for b in $backends; do
	if ! (cd /work/srv && XO_STD=/work/xo/std XO_BIN_CACHE=off PWD=/work/srv /work/xo/bin/xo build --backend=\$b -o /work/srv-\$b . > /work/build-\$b.log 2>&1); then
		echo "callgrind-docker.sh: the \$b server did not build:" >&2
		tail -5 /work/build-\$b.log >&2
		exit 1
	fi
	servers="\$servers \$b=/work/srv-\$b"
done
valgrind --version
N=${N:-100} REPS=${REPS:-2} SCENARIOS="${SCENARIOS:-plain plain-newconn tls tls-newconn h2}" sh /work/callgrind.sh /work/run /work/loadgen \$servers
INSIDE
chmod +x "$out/inside.sh"
# not through a pipe: its status is the container's
st=0
XO_DOCKER_ROOT=1 "$root/scripts/docker-linux.sh" arm64 "$out" -- ./inside.sh > "$out/results.txt" || st=$?
cat "$out/results.txt"
exit $st
