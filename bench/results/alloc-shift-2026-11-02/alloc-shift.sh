#!/bin/bash
# alloc-shift.sh REVS(comma) SHIFTS(comma) NAME ROUNDS: /alloc c256 and c32 of the jitter server of each
# revision linked at each XO_DATA_SHIFT (decision 0459), first linux/arm64 host, exclusive.
set -u
cd $HOME/godev/claude-space/xolang/xo
shifts=$2 name=$3 rounds=$4
L=$HOME/xo-lead; . $L/offload.env
priv=/work/priv; work=/work; put=/work/in/$name.bundle
a=""; brs=()
for x in ${1//,/ }; do c=$(git rev-parse --verify "$x^{commit}") || exit 2; a="$a${a:+,}$c"; git branch -q -f as-$$-${#brs[@]} $c; brs+=(as-$$-${#brs[@]}); done
bundle=$(mktemp /tmp/allocshift.XXXXXX)
git bundle create -q $bundle "${brs[@]}"; git branch -q -D "${brs[@]}"
$HOME/xo-lead/agents/offload.sh --put $bundle $put || { rm -f $bundle; exit 4; }
rm -f $bundle
OFF_EXCLUSIVE=1 $HOME/xo-lead/agents/offload.sh $HOME/xo-lead/notes/xo-lang-2/bin/alloc-shift-remote.sh $a $name "$OFF_SEED" $priv $work $put $rounds $shifts
