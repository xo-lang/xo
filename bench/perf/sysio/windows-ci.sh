#!/bin/bash
# windows-ci.sh: bench/perf/sysio on the CI Windows runner
# (`gh workflow run ci.yml -f ref=BRANCH -f suite=script -f
# runner=xo-windows -f packages="bench/perf/sysio/windows-ci.sh [N]"`,
# knowledgebase/infrastructure/ci.md). It builds sysio with $XO_CLI and
# main's std ($XO_CI_BASE_STD, the C path or the base) and the branch's
# ($XO_CI_STD), LLVM, then runs measure-windows.ps1 on each for two
# rounds (base, branch, base, branch), N runs per workload (default 15).
# The rows go to $XO_CI_OUT/sysio-windows.txt (the script-out artifact):
# "round R base|branch", then "<workload> <min cycles> <min ms>
# <checksum>". WORKLOADS (space separated) overrides measure-windows.ps1's
# list.
set -euo pipefail
N=${1:-15}
: "${XO_CLI:?}" "${XO_CI_STD:?}" "${XO_CI_BASE_STD:?}" "${XO_CI_OUT:?}"
SRC=$(cd "$(dirname "$0")" && pwd)
W=$(mktemp -d)
mkdir -p "$XO_CI_OUT" "$W/src"
cp "$SRC/main.xo" "$SRC/xo.mod" "$W/src/"
for v in base branch; do
	std=$XO_CI_BASE_STD
	[ "$v" = branch ] && std=$XO_CI_STD
	(cd "$W/src" && "$XO_CLI" build --std="$(cygpath -m "$std")" --backend=llvm -o "$(cygpath -m "$W/t-$v.exe")" .)
done
ps=$(cygpath -w "$SRC/measure-windows.ps1")
args=()
if [ -n "${WORKLOADS:-}" ]; then
	args=(-Workloads "$(echo "$WORKLOADS" | tr ' ' ',')")
fi
out="$XO_CI_OUT/sysio-windows.txt"
: >"$out"
for r in 1 2; do
	for v in base branch; do
		echo "round $r $v" | tee -a "$out"
		powershell -NoProfile -ExecutionPolicy Bypass -File "$ps" -Exe "$(cygpath -w "$W/t-$v.exe")" -Out "$(cygpath -w "$W/out-$v")" -N "$N" "${args[@]}" | tr -d '\r' | tee -a "$out"
	done
done
rm -rf "$W"
