#!/bin/bash
# alloc-split.sh MODE SPECS NAME ROUNDS: the split form of alloc-points.sh (MODE last), alloc-own.sh (own) and
# alloc-env.sh (own, SPECS REV[+VAR=VALUE...]) (Lead, Lang 3, 2026-11-02): the builds run on the SECOND host
# (alloc-build-remote.sh, lane pass2, not exclusive), the tar of binaries goes by the Mac to the first host,
# and only alloc-measure-remote.sh holds its exclusive lease. Prints the host 1 lease seconds. The revisions
# come from the checkout this script lives in (git bundle of REVS); needs offload.sh --get (43ca0e3d7, installed).
set -u
S=$(cd "$(dirname "$0")" && pwd)
cd "$S" && cd "$(git rev-parse --show-toplevel)"
mode=$1 rev=$2 name=$3 rounds=$4
case $mode in last|own) ;; *) echo "alloc-split: MODE last|own"; exit 2;; esac
A=${OFFLOAD_SH:-$HOME/xo-lead/agents/offload.sh}
seed2=$(grep -h '^\(export \)\?OFF_SEED=' $HOME/xo-lead/offload-arm64b.env | tail -1 | sed 's/.*=//')
a=""; brs=()
for x in ${rev//,/ }; do c=$(git rev-parse --verify "${x%%+*}^{commit}") || exit 2; e=""; [ "$x" != "${x#*+}" ] && e="+${x#*+}"; a="$a${a:+,}$c$e"; git branch -q -f pt-$$-${#brs[@]} $c; brs+=(pt-$$-${#brs[@]}); done
bundle=$(mktemp /tmp/jithost.XXXXXX); tar=$(mktemp /tmp/jittar.XXXXXX)
trap 'rm -f "$bundle" "$tar"' EXIT
git bundle create -q $bundle "${brs[@]}"; git branch -q -D "${brs[@]}"
t0=$(date +%s)
OFFLOAD_ENV=offload-arm64b.env $A --put $bundle /work/in/$name.bundle || exit 4
OFFLOAD_ENV=offload-arm64b.env $A $S/alloc-build-remote.sh $mode $a $name "$seed2" /work/priv /work /work/in/$name.bundle || { echo "alloc-split: the build failed"; exit 5; }
t1=$(date +%s)
OFFLOAD_ENV=offload-arm64b.env $A --get /work/out/$name.tar $tar || exit 6
$A --put $tar /work/in/$name.tar || exit 7
t2=$(date +%s)
echo "alloc-split: build on host 2 $((t1 - t0)) s, tar to host 1 $((t2 - t1)) s"
OFF_EXCLUSIVE=1 $A $S/alloc-measure-remote.sh $name $rounds /work/in/$name.tar /work
rc=$?; echo "alloc-split: host 1 lease (measure only) $(( $(date +%s) - t2 )) s"; exit $rc
