#!/bin/bash
# twin.sh BUNDLE SEED REV BENCHDIR WORKLOAD...: BENCHDIR against its Go twin (twin.go.txt, twin.txt's
# require lines), xo bench --ab with Go as A and Xo as B, 15 rounds, per workload. Never pattern-kills.
set -u -o pipefail
case ${1:-} in env:*) TWIN_BUILD_ENV=${1#env:}; shift ;; esac
bundle=$1 seed=$2 r=$3 dir=$4; shift 4
cd /work/priv || exit 2
if [ "$bundle" != - ]; then git fetch -q "$bundle" "+refs/heads/*:refs/l2/*" || exit 2; fi
W=/work/l2twin
rm -rf "${W:?}"; git worktree prune; mkdir -p "$W/go"
trap 'rm -rf "${W:?}"; git -C /work/priv worktree prune' EXIT
git worktree add -q --detach "$W/src" "$r" || exit 2
cd "$W/src" || exit 2
XO_SEED=$seed go run ./scripts/xobuild > "$W/xb.txt" 2>&1 || { echo "xobuild failed"; tail -5 "$W/xb.txt"; exit 3; }
env XO_STD=$W/src/std ${TWIN_BUILD_ENV:-} bin/xo build -o "$W/xoprog" "./$dir" || exit 4
echo "build env: ${TWIN_BUILD_ENV:-none}"
cp "$dir/twin.go.txt" "$W/go/main.go"
( cd "$W/go" && go mod init twin > /dev/null 2>&1 && \
  { grep '^require ' "$W/src/$dir/twin.txt" || true; } | while read -r _ m v; do go mod edit -require="$m@$v"; done && \
  GOFLAGS=-mod=mod go build -o "$W/goprog" . ) || { echo "go twin build failed"; exit 4; }
echo "go $(go version | cut -d' ' -f3); host $(uname -srm); rev $r"
for w in "$@"; do
  echo "== $w: $("$W/goprog" $w) | $("$W/xoprog" $w)"
  XO_STD=$W/src/std bin/xo bench --ab --rounds=15 --no-guard "$W/goprog" "$W/xoprog" -- $w 2>&1 | grep -E 'wall_ms|max_rss|cpu_ms'
done
echo "twin: ok"
