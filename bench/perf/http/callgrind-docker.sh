#!/bin/sh
# callgrind-docker.sh: runs callgrind.sh in scripts/docker-linux.sh's
# linux/arm64 (or, ARCH=amd64, linux/amd64) Debian image from a Mac (decision 0180's numbers stay the
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
#
# MODE=short (2026-10-18, for gates): plain and tls only, one repeat,
# N=30, a couple of minutes; MODE=full (the default) is every scenario,
# REPS=2, N=100, for the nightly and per-batch runs and for changes that
# need it (knowledgebase/infrastructure/toolchain.md "HTTP callgrind:
# short or full"). N, REPS, SCENARIOS given explicitly win over MODE.
#
# ARCH=amd64 (2026-10-18): the container is linux/amd64 (Docker Desktop
# emulates it: valgrind's counts are the guest's instructions, the same
# run to run, only slower), the xo is a Go backend cross build (the
# native backends link only for the host), and the default backends are
# amd64 (Xo's own, decision 0254) and llvm.
#
# Guards (2026-10-18): a linux xo left in OUT is reused only when the
# commit it was built from (OUT/xo/commit.txt) is the checkout's and
# compiler/ and std/ have no local changes; every build runs with
# XO_INLINE_STRICT=1, so a module the inliner gives up on fails instead
# of being measured un-inlined (a branch's inliner bug once doubled
# tls-newconn's count with only a warning).
set -eu
xo=$1
out=$2
shift 2
arch=${ARCH:-arm64}
case "${MODE:-full}" in
short)
	N=${N:-30}
	REPS=${REPS:-1}
	SCENARIOS=${SCENARIOS:-"plain tls"}
	;;
full) ;;
*)
	echo "callgrind-docker.sh: MODE is short or full, not $MODE" >&2
	exit 2
	;;
esac
if [ "$arch" = amd64 ]; then
	backends=${*:-"amd64 llvm"}
	xobe=go
else
	backends=${*:-"arm64 llvm"}
	xobe=arm64
fi
root=$(cd "$(dirname "$0")/../../.." && pwd)
mkdir -p "$out/xo/bin" "$out/srv"
# decision 0264: refuse a darwin xo that is not this checkout's build;
# the stamp goes with the results
. "$root/bench/perf/xocheck.sh" "$xo" "$root"
echo "$XO_STAMP" > "$out/stamp.txt"
stamp="$(git -C "$root" rev-parse HEAD) $arch"
if ! git -C "$root" diff --quiet HEAD -- compiler std; then
	stamp="$stamp dirty"
fi
if [ ! -x "$out/xo/bin/xo" ] || [ "$(cat "$out/xo/commit.txt" 2>/dev/null)" != "$stamp" ] || [ "${stamp% dirty}" != "$stamp" ]; then
	rm -f "$out/xo/bin/xo" "$out/xo/commit.txt"
	(cd "$root/compiler/xo" && XO_INLINE_STRICT=1 XO_STD="$root/std" XO_BIN_CACHE=off "$xo" build --backend=$xobe --targets=linux/$arch -o "$out/xo/bin/xo" .)
	echo "$stamp" > "$out/xo/commit.txt"
fi
rm -rf "$out/xo/std"
cp -R "$root/std" "$out/xo/std"
cp "$root"/bench/perf/http/xo/*.xo "$root/bench/perf/http/xo/xo.mod" "$out/srv/"
(cd "$root/bench/perf/http/loadgen" && GOOS=linux GOARCH=$arch CGO_ENABLED=0 go build -o "$out/loadgen" .)
cp "$root/bench/perf/http/callgrind.sh" "$out/callgrind.sh"
cat > "$out/inside.sh" <<INSIDE
#!/bin/sh
set -eu
apt-get update > /dev/null 2>&1
apt-get install -y --no-install-recommends valgrind > /dev/null 2>&1
servers=""
for b in $backends; do
	if ! (cd /work/srv && XO_INLINE_STRICT=1 XO_STD=/work/xo/std XO_BIN_CACHE=off PWD=/work/srv /work/xo/bin/xo build --backend=\$b -o /work/srv-\$b . > /work/build-\$b.log 2>&1); then
		echo "callgrind-docker.sh: the \$b server did not build:" >&2
		tail -5 /work/build-\$b.log >&2
		exit 1
	fi
	servers="\$servers \$b=/work/srv-\$b"
done
valgrind --version
ANN=${ANN:-0} N=${N:-100} REPS=${REPS:-2} SCENARIOS="${SCENARIOS:-plain plain-newconn tls tls-newconn h2}" sh /work/callgrind.sh /work/run /work/loadgen \$servers
INSIDE
chmod +x "$out/inside.sh"
# not through a pipe: its status is the container's
st=0
XO_DOCKER_ROOT=1 "$root/scripts/docker-linux.sh" "$arch" "$out" -- ./inside.sh > "$out/results.txt" || st=$?
cat "$out/results.txt"
exit $st
