#!/bin/sh
# pass-fuzz.sh: the miscompile class guard (task #358, owner 2026-11-18;
# 113 past fixes were a pass that changed what a program does): builds
# and runs goldens with a random subset of the optional passes off
# (XO_PASSES) and every frame slot poisoned (XO_POISON_FRAMES=1), each
# against its want.txt (the outputs Go's packages give, checked when each
# golden was recorded). A pass that is wrong only next to some other
# pass, or a read of a slot before its write, fails here by round.
#
#   scripts/pass-fuzz.sh XO BACKEND OUT SEED ROUNDS [NAME...]
#
# XO the xo binary (its std at XO/../../std; `-` is $XO_CLI), BACKEND amd64 or arm64
# (scripts/linux-goldens.sh's), OUT the work directory, SEED and ROUNDS
# pick the subsets: round r turns each pass off with probability 1/3,
# from a Park-Miller generator seeded with the POSIX cksum of "pass-fuzz
# SEED r" (awk's rand differs between macOS awk, gawk and mawk, and
# linearly spaced seeds gave correlated subsets: one pass off in 43 of 60
# seeds), so a seed and round name one subset on every host. NAMEs are goldens (default: scripts/docker-amd64-sample.txt
# plus the pass hunt's pass_*, slp_* and rc_empty_*). Round 0 is every
# pass on with poisoned frames. A failing golden is run once more with
# every pass on and no poison: `FAIL` lines name the round, the golden
# and its XO_PASSES; `BASE` lines a golden that fails without the round
# too (no pass's doing). OUT/summary.txt gets one line per round.
# Exits 1 when any round failed. Runs on a Linux host (linux-goldens.sh):
# `amd64-run.sh REV NAME script scripts/pass-fuzz.sh - amd64 OUT 1 4`.
set -u
xo=${1:?usage: pass-fuzz.sh XO BACKEND OUT SEED ROUNDS [NAME...]}
[ "$xo" = - ] && xo=${XO_CLI:?pass-fuzz: XO is - and XO_CLI is unset}
be=${2:?BACKEND}
out=${3:?OUT}
seed=${4:?SEED}
rounds=${5:?ROUNDS}
shift 5
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root" || exit 2
mkdir -p "$out"
if [ $# -gt 0 ]; then
	names="$*"
else
	names="$(sed 's/#.*//' scripts/docker-amd64-sample.txt) $(ls internal/llvm/testdata/golden | grep -E '^(pass_|slp_|rc_empty_)')"
fi
# the optional passes (compiler/arm64/passes.xo's list); x86_ ones only
# on amd64, the others reach both backends
passes=$(grep -E '^ +"[a-z_0-9]+",$' compiler/arm64/passes.xo | tr -d ' ",')
[ "$be" = amd64 ] || passes=$(echo "$passes" | grep -v '^x86_')
[ -n "$passes" ] || { echo "pass-fuzz: no passes in compiler/arm64/passes.xo"; exit 2; }
jobs=${XO_GOLDEN_JOBS:-$(( $(nproc 2>/dev/null || echo 4) / 2 ))}
: > "$out/summary.txt"
nf=0
r=0
while [ "$r" -le "$rounds" ]; do
	if [ "$r" = 0 ]; then
		v=""
	else
		v=$(echo "$passes" | awk -v s="$(echo "pass-fuzz $seed $r" | cksum | cut -d' ' -f1)" 'BEGIN { x = s % 2147483646 + 1 } { x = (x * 16807) % 2147483647; if (x < 715827882) printf "%s-%s", (n++ ? "," : ""), $0 }')
	fi
	d="$out/r$r"
	XO_PASSES=$v XO_POISON_FRAMES=1 XO_GOLDEN_JOBS=$jobs XO_CC=/bin/false XO_CLANG=/bin/false \
		sh scripts/linux-goldens.sh "$xo" "$be" "$d" $names > "$d.txt" 2>&1
	bad=$(awk '$1 != "ok" && $1 != "skip" { print $2 }' "$d/summary.txt" 2>/dev/null)
	n=0
	for g in $bad; do
		n=$((n + 1))
		if XO_GOLDEN_JOBS=1 XO_CC=/bin/false XO_CLANG=/bin/false sh scripts/linux-goldens.sh "$xo" "$be" "$d-base-$g" "$g" > "$d-base-$g.txt" 2>&1; then
			echo "FAIL seed $seed round $r $g with XO_PASSES=$v XO_POISON_FRAMES=1"
			nf=$((nf + 1))
		else
			echo "BASE seed $seed round $r $g fails with every pass on too"
			nf=$((nf + 1))
		fi
	done
	# with linux-goldens.sh's count line (ok, failed, did not build,
	# skipped): a round whose goldens never ran shows here
	echo "round $r: $n of $(echo $names | wc -w | tr -d ' ') failed ($(tail -1 "$d.txt")), XO_PASSES=$v" >> "$out/summary.txt"
	# a passing round's binaries go (a round is about 1 GB of them)
	[ "$n" = 0 ] && rm -rf "${d:?}"
	r=$((r + 1))
done
cat "$out/summary.txt"
echo "pass-fuzz: seed $seed, $rounds rounds of $(echo $names | wc -w | tr -d ' ') goldens on $be, $nf failed"
[ "$nf" = 0 ]
