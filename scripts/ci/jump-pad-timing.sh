#!/bin/bash
# jump-pad-timing.sh: decision 0398's jump padding on a CPU without the
# JCC erratum (the xo-amd64-big runner, a Sapphire Rapids Xeon): sort1M
# (the Intel tester's micro benchmark, List[Int].sort of 1M LCG Ints) and
# bench/perf/llvm/xo/interp built for linux/amd64 by main's xo (no
# padding) and by the branch's ($XO_CLI), each at 8 placements
# (XO_ARM64_PAD=0,4,..,28, a one byte nop each on x86), timed in
# interleaved rounds; the summary gives each variant's spread over
# placements, so the padding is judged across layouts, never at one.
# `perf stat` cycles and instructions for the pad 0 pair when perf is
# there.
#   gh workflow run ci.yml -f ref=BRANCH -f suite=script -f runner=xo-amd64-big \
#     -f packages="scripts/ci/jump-pad-timing.sh [BASE_COMMIT]"
set -uo pipefail
cd "$(dirname "$0")/../.."
root=$PWD
. scripts/ci/ci-out.sh
out=${XO_CI_OUT:-$(mktemp -d)}
mkdir -p "$out"
w=$(ci_work jpad)
trap 'rm -rf "${w:?}"; git worktree prune' EXIT
rounds=${JPAD_ROUNDS:-7}
pads="0 4 8 12 16 20 24 28"
{ lscpu 2>/dev/null | grep -E 'Model name|^CPU\(s\)' ; uptime; } | tee "$out/machine.txt"

# the unpadded xo: the first argument (a commit before decision 0398, for a
# run from main), else the merge base
base=${1:-${XO_CI_BASE:-origin/main}}
git rev-parse --verify -q "$base^{commit}" >/dev/null || git fetch -q origin "$base"
git worktree add -q --detach "$w/base" "$base" || exit 1
(cd "$w/base" && go run ./scripts/xobuild) || exit 1
basexo=$w/base/bin/xo
echo "base: $(git rev-parse --short "$base") $($basexo version | head -1)"
echo "branch: $(git rev-parse --short HEAD) $($XO_CLI version | head -1)"

mkdir -p "$w/sort1m" "$w/bin"
printf 'module example.com/ci/sort1m\nedition preview\n' > "$w/sort1m/xo.mod"
cat > "$w/sort1m/main.xo" <<'XO'
// sort1M: 1M LCG Ints from a frozen Buf, List[Int].sort (pdqsort); the
// best of 5 sorts in this process as "sort1M BEST_MS CHECKSUM".
fn main(os: Os) {
  var best = 1 << 60
  var sum = 0
  for _ in 0..5 {
    var xs = Buf[Int].with_capacity(1_000_000)
    var x = 12345
    for _ in 0..1_000_000 {
      x = (x * 1103515245 + 12345) % 2147483648
      xs.push(x)
    }
    let t0 = os.clock.now()
    let sorted = xs.freeze().sort()
    let ms = (os.clock.now() - t0).millis()
    if ms < best { best = ms }
    sum = sorted[500_000]
  }
  os.stdio.println("sort1M ${best} ${sum}")
}
XO

build() { # VARIANT XO STD
  local v=$1 xo=$2 std=$3 k
  for k in $pads; do
    XO_STD=$std XO_ARM64_PAD=$k "$xo" build --backend=amd64 -o "$w/bin/sort-$v-p$k" "$w/sort1m" || return 1
    XO_STD=$std XO_ARM64_PAD=$k "$xo" build --backend=amd64 -o "$w/bin/interp-$v-p$k" "$root/bench/perf/llvm/xo/interp" || return 1
  done
}
build base "$basexo" "${XO_CI_BASE_STD:-$w/base/std}" || { echo "base build failed"; exit 1; }
build branch "$XO_CLI" "${XO_CI_STD:-$root/std}" || { echo "branch build failed"; exit 1; }

ms() { perl -MTime::HiRes=time -e 'printf "%.0f", time*1000'; }
: > "$out/times.txt"
for r in $(seq 1 "$rounds"); do
  for v in base branch; do
    for k in $pads; do
      s=$("$w/bin/sort-$v-p$k")
      echo "sort $v p$k $(echo "$s" | awk '{print $2}') $(echo "$s" | awk '{print $3}')" >> "$out/times.txt"
      t0=$(ms); "$w/bin/interp-$v-p$k" > /dev/null; t1=$(ms)
      echo "interp $v p$k $((t1 - t0))" >> "$out/times.txt"
    done
  done
done
if command -v perf >/dev/null; then
  for v in base branch; do
    for p in sort interp; do
      perf stat -e cycles,instructions -x, "$w/bin/$p-$v-p0" 2>&1 >/dev/null | awk -F, -v n="$p $v" '{print n, $3, $1}' >> "$out/perf.txt"
    done
  done
  cat "$out/perf.txt"
fi
python3 - "$out/times.txt" <<'PY' | tee "$out/summary.txt"
import sys, statistics, collections
d = collections.defaultdict(list)
for line in open(sys.argv[1]):
    p = line.split()
    if len(p) >= 4 and p[3].isdigit():
        d[(p[0], p[1], p[2])].append(int(p[3]))
for prog in ("sort", "interp"):
    for v in ("base", "branch"):
        meds = {k[2]: statistics.median(xs) for k, xs in d.items() if k[0] == prog and k[1] == v}
        if not meds:
            continue
        vals = sorted(meds.values())
        print("%-6s %-6s ms over %d pads: min %d median %d max %d (%s)" % (prog, v, len(vals), vals[0], statistics.median(vals), vals[-1], " ".join("%s=%d" % (k, meds[k]) for k in sorted(meds, key=lambda s: int(s[1:])))))
PY
echo "jump-pad-timing done"
