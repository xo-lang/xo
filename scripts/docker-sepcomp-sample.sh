#!/bin/sh
# docker-sepcomp-sample.sh: separate compilation's check on linux (decision
# 0388): every golden of scripts/docker-sepcomp-sample.txt built by default
# (separately compiled: the program against std's store) and with
# XO_SEPCOMP=0 (the whole program), in scripts/docker-linux.sh's container,
# and the two runs compared (exit status and output); a default build that
# did not take std's variants fails too (it would compare a whole program
# with itself).
#
#   scripts/docker-sepcomp-sample.sh LINUXXO OUT [ARCH]
#
# LINUXXO is a directory holding a linux xo as bin/xo, cross built on the
# Mac (in compiler/xo: `xo build --target=linux/ARCH -o LINUXXO/bin/xo .`,
# through heavy.sh); the checkout's std is copied beside it. OUT keeps the
# goldens, the binaries, their outputs and results.txt. ARCH is arm64 (the
# default) or amd64 (emulated). Run outside heavy.sh, as every Docker run
# (`~/xo-lead/agents/docker-up.sh && scripts/docker-sepcomp-sample.sh ...;
# ~/xo-lead/agents/docker-up.sh stop`).
set -eu
lx=$1
out=$2
arch=${3:-arm64}
root=$(cd "$(dirname "$0")/.." && pwd)
[ -x "$lx/bin/xo" ] || { echo "no $lx/bin/xo"; exit 2; }
rm -rf "$out"
mkdir -p "$out/xo/bin" "$out/g"
cp "$lx/bin/xo" "$out/xo/bin/xo"
cp -R "$root/std" "$out/xo/std"
for g in $(grep -v '^#' "$root/scripts/docker-sepcomp-sample.txt"); do
	cp -R "$root/internal/llvm/testdata/golden/$g" "$out/g/$g"
done
cat > "$out/run.sh" <<'R'
export HOME=/work/home XO_BACKEND_CACHE=on XO_BACKEND_CACHE_DIR=/work/cache XO_SEPCOMP_BG=0
mkdir -p /work/home
X=/work/xo/bin/xo
for c in 0 1; do $X dev stdcache --conc=$c > /dev/null || { echo "stdcache --conc=$c failed"; exit 1; }; done
bad=0
for d in /work/g/*/; do
	g=$(basename "$d")
	cd "$d"
	if ! XO_SEPCOMP=0 $X build -o /work/$g.whole . > /work/$g.whole.log 2>&1; then echo "$g: whole build failed"; bad=1; continue; fi
	if ! $X build --phase-times -o /work/$g.sep . > /work/$g.sep.log 2>&1; then echo "$g: separate build failed: $(tail -2 /work/$g.sep.log)"; bad=1; continue; fi
	if ! grep -q "cache std" /work/$g.sep.log; then echo "$g: the default did not take std's variants"; bad=1; continue; fi
	/work/$g.whole > /work/$g.whole.out 2>&1 < /dev/null; a=$?
	/work/$g.sep > /work/$g.sep.out 2>&1 < /dev/null; b=$?
	if [ $a -ne $b ] || ! cmp -s /work/$g.whole.out /work/$g.sep.out; then
		echo "$g: DIFFERS (exit $a whole, $b separate): $(tail -1 /work/$g.sep.out)"; bad=1
	else
		echo "$g: same (exit $a)"
	fi
done
exit $bad
R
set +e
nice -n 10 "$root/scripts/docker-linux.sh" "$arch" "$out" -- sh /work/run.sh > "$out/results.txt" 2>&1
rc=$?
cat "$out/results.txt"
exit $rc
