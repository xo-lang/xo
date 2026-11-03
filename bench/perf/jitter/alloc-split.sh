#!/bin/bash
# alloc-split.sh MODE SPECS NAME ROUNDS: the split form of alloc-points.sh (MODE last), alloc-own.sh (own) and
# alloc-env.sh (own, SPECS REV[+VAR=VALUE...]) (Lead, Lang 3, 2026-11-02): the builds run OFF the first host and
# only alloc-measure-remote.sh holds its exclusive lease. Prints the host 1 lease seconds. The revisions come from
# the checkout this script lives in. ALLOC_BUILD=mac (default, 2026-11-02: host 2's pass2 queue held a build 36 min
# while host 1 sat idle): alloc-build-mac.sh cross-builds linux/arm64 on the Mac as a HEAVY_CLASS=gate job, with the
# darwin v0.35.0 xo XO_MAC_SEED (default ~/xo-seed-0.35.0-51882bb7/xo/bin/xo; it must be the release of host 2's
# OFF_SEED). ALLOC_BUILD=host2: alloc-build-remote.sh on the SECOND host (lane pass2, not exclusive), the tar comes by
# offload.sh --get (43ca0e3d7, installed). Either way the tar goes through the Mac to the first host.
# ALLOC_PADS (default "0 1 2 3"; "" for one build per spec): each spec, and the base release (the spec
# "base", built by the seed), is built at each XO_ARM64_PAD, and pads.py judges each against the base by its
# median over pads, slower or faster only when the pad ranges do not overlap: a revision's /alloc moves about
# 4 percent with where its code lands, so one build per side can show a regression that is placement
# (Lead, 2026-11-03; gotchas/arm64-loop-alignment-stable-not-fast.md). The rounds per pad stay ROUNDS.
set -u
S=$(cd "$(dirname "$0")" && pwd)
cd "$S" && cd "$(git rev-parse --show-toplevel)"
mode=$1 rev=$2 name=$3 rounds=$4
case $mode in last|own) ;; *) echo "alloc-split: MODE last|own"; exit 2;; esac
A=${OFFLOAD_SH:-$HOME/xo-lead/agents/offload.sh}
seed2=$(grep -h '^\(export \)\?OFF_SEED=' $HOME/xo-lead/offload-arm64b.env | tail -1 | sed 's/.*=//')
bm=${ALLOC_BUILD:-mac}
case $bm in mac|host2) ;; *) echo "alloc-split: ALLOC_BUILD mac|host2"; exit 2;; esac
pads=${ALLOC_PADS-0 1 2 3}
if [ -n "$pads" ]; then
	ex="" ; for x in base ${rev//,/ }; do for k in $pads; do ex="$ex${ex:+,}$x+XO_ARM64_PAD=$k"; done; done; rev=$ex
fi
a=""; brs=()
for x in ${rev//,/ }; do [ "${x%%+*}" = base ] && { a="$a${a:+,}$x"; continue; }; c=$(git rev-parse --verify "${x%%+*}^{commit}") || exit 2; e=""; [ "$x" != "${x#*+}" ] && e="+${x#*+}"; a="$a${a:+,}$c$e"; [ $bm = host2 ] && { git branch -q -f pt-$$-${#brs[@]} $c; brs+=(pt-$$-${#brs[@]}); }; done
bundle=$(mktemp /tmp/jithost.XXXXXX); tar=$(mktemp /tmp/jittar.XXXXXX)
trap 'rm -f "$bundle" "$tar"' EXIT
t0=$(date +%s)
if [ $bm = mac ]; then
	seedm=${XO_MAC_SEED:-$HOME/xo-seed-0.35.0-51882bb7/xo/bin/xo}
	[ -x "$seedm" ] || { echo "alloc-split: no darwin seed xo at $seedm (XO_MAC_SEED)"; exit 3; }
	HEAVY_CLASS=gate $HOME/xo-lead/agents/heavy.sh alloc-mac-$name $S/alloc-build-mac.sh $mode $a $name "$seedm" $tar || { echo "alloc-split: the Mac build failed"; exit 5; }
	t1=$(date +%s)
else
	git bundle create -q $bundle "${brs[@]}"; git branch -q -D "${brs[@]}"
	OFFLOAD_ENV=offload-arm64b.env $A --put $bundle /work/in/$name.bundle || exit 4
	OFFLOAD_ENV=offload-arm64b.env $A $S/alloc-build-remote.sh $mode $a $name "$seed2" /work/priv /work /work/in/$name.bundle || { echo "alloc-split: the build failed"; exit 5; }
	t1=$(date +%s)
	OFFLOAD_ENV=offload-arm64b.env $A --get /work/out/$name.tar $tar || exit 6
fi
$A --put $tar /work/in/$name.tar || exit 7
t2=$(date +%s)
echo "alloc-split: build ($bm) $((t1 - t0)) s, tar to host 1 $((t2 - t1)) s"
log=$(mktemp /tmp/jitlog.XXXXXX)
OFF_EXCLUSIVE=1 $A $S/alloc-measure-remote.sh $name $rounds /work/in/$name.tar /work | tee "$log"
rc=${PIPESTATUS[0]}
[ -n "$pads" ] && python3 -I $S/pads.py "$log"
rm -f "$log"; echo "alloc-split: host 1 lease (measure only) $(( $(date +%s) - t2 )) s"; exit $rc
