#!/bin/bash
# twin-r13.sh BUNDLE SEED REV: bench/perf/bytestransform (index, lossy, valid) against its Go twin,
# xo bench --ab with Go as A and Xo as B, 15 rounds. Never pattern-kills.
set -u -o pipefail
bundle=$1 seed=$2 r=$3
cd /work/priv || exit 2
git fetch -q "$bundle" "+refs/heads/*:refs/l2/*" || exit 2
W=/work/l2twin13
rm -rf "${W:?}"; git worktree prune; mkdir -p "$W/go"
trap 'rm -rf "${W:?}"; git -C /work/priv worktree prune' EXIT
git worktree add -q --detach "$W/src" "$r" || exit 2
cd "$W/src" || exit 2
XO_SEED=$seed go run ./scripts/xobuild > "$W/xb.txt" 2>&1 || { echo "xobuild failed"; tail -5 "$W/xb.txt"; exit 3; }
XO_STD=$W/src/std bin/xo build -o "$W/xobt" ./bench/perf/bytestransform || exit 4
cp bench/perf/bytestransform/twin.go.txt "$W/go/main.go" && (cd "$W/go" && go mod init twin > /dev/null 2>&1 && go build -o "$W/gobt" .) || exit 4
echo "go $(go version | cut -d' ' -f3); host $(uname -srm)"
for w in index lossy valid; do
  echo "== $w: $("$W/gobt" $w) | $("$W/xobt" $w)"
  XO_STD=$W/src/std bin/xo bench --ab --rounds=15 --no-guard "$W/gobt" "$W/xobt" -- $w 2>&1 | grep -E 'wall_ms|max_rss|cpu_ms' 
done
echo "twin-r13: ok"
