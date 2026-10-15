#!/bin/bash
# nolibc-goldens.sh: the LLVM golden programs built with XO_NOLIBC=1 (no C
# library, decision 0206) by $XO_CLI on both native backends, with the
# branch's std and with main's ($XO_CI_BASE_STD), on linux/arm64
# (`gh workflow run ci.yml -f ref=BRANCH -f suite=script -f
# packages=scripts/ci/nolibc-goldens.sh`): each run's summary, and the
# programs whose result differs between the two stds. Programs nolibc
# refuses (`use c`, TLS roots) fail with both. Exit 1 when the branch
# fails a program main passes.
set -uo pipefail
cd "$(dirname "$0")/../.."
root=$PWD
out=${XO_CI_OUT:-$(mktemp -d)}
mkdir -p "$out"
rc=0
for be in arm64 llvm; do
  for v in base branch; do
    std=$root/std
    [ "$v" = base ] && std=$XO_CI_BASE_STD
    XO_NOLIBC=1 "$XO_CLI" dev goldens --backend="$be" --force --no-cache --json --flags="--std=$std" internal/llvm/testdata/golden > "$out/$be-$v.json" 2> "$out/$be-$v.err"
    echo "$be $v: exit $? $(tail -1 "$out/$be-$v.err")"
  done
  python3 - "$out/$be-base.json" "$out/$be-branch.json" "$be" <<'PY' || rc=1
import json, sys
def load(p):
    r = {}
    for line in open(p):
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            o = json.loads(line)
        except Exception:
            continue
        n = o.get("name") or o.get("dir") or o.get("program")
        if n:
            r[n] = o.get("status") or o.get("result") or ("pass" if o.get("ok") else "fail")
    return r
a, b = load(sys.argv[1]), load(sys.argv[2])
worse = [n for n in sorted(a) if a[n] == "pass" and b.get(n) != "pass"]
better = [n for n in sorted(b) if b[n] == "pass" and a.get(n) != "pass"]
print(f"{sys.argv[3]}: base {sum(v == 'pass' for v in a.values())} pass of {len(a)}, branch {sum(v == 'pass' for v in b.values())} pass of {len(b)}")
for n in worse:
    print(f"  worse: {n} ({b.get(n)})")
for n in better:
    print(f"  better: {n}")
sys.exit(1 if worse else 0)
PY
done
exit $rc
