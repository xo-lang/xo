#!/bin/bash
# macho-link-compare.sh: every golden built for darwin/amd64 twice, linked
# by Xo's Mach-O linker and by Apple's ld (--as=cc), and the two compared
# layout independently with scripts/macho-symview.py (code: each
# instruction's operands named; data: fixups, unwind entries, FDEs,
# pointer free sections). The strings differ by design (ld merges equal
# literals, Xo's linker does not), and tz_table (cc builds its zone data
# from C). Decision 0254 stage 6a; the Intel Mac runs them
# (scripts/intel-goldens.sh). macOS only.
#
#   scripts/macho-link-compare.sh XO WORK
#
# Prints DIFF NAME CODE_LINES DATA_LINES per differing golden (its views
# kept in WORK) and "same N, differ M, failed K".
set -u
xo=$1
work=$2
root=$(cd "$(dirname "$0")/.." && pwd)
view=$root/scripts/macho-symview.py
rm -rf "$work"
mkdir -p "$work"
same=0
diff=0
fail=0
for d in $(find "$root/internal/llvm/testdata/golden" -mindepth 1 -maxdepth 1 -type d | sort); do
  n=$(basename "$d")
  if ! (cd "$d" && "$xo" build --backend=amd64 --target=darwin/amd64 -o "$work/$n.l" . > "$work/$n.lb" 2>&1 && "$xo" build --backend=amd64 --target=darwin/amd64 --as=cc -o "$work/$n.c" . > "$work/$n.cb" 2>&1); then
    fail=$((fail + 1))
    echo "FAIL $n"
    continue
  fi
  for k in l c; do
    # data in text (jump tables) decodes as instructions whose targets
    # move with the layout
    python3 "$view" code "$work/$n.$k" | grep -v "abs:\|<unknown>\|^loop\|^xbegin\|^jrcxz" > "$work/$n.$k.code"
    python3 "$view" data "$work/$n.$k" | grep -v "^bytes __TEXT,__cstring" | sort > "$work/$n.$k.data"
  done
  if cmp -s "$work/$n.l.code" "$work/$n.c.code" && cmp -s "$work/$n.l.data" "$work/$n.c.data"; then
    same=$((same + 1))
    rm -rf "$work/$n.l" "$work/$n.c" "$work/$n".*.dSYM "$work/$n".*.code "$work/$n".*.data
  else
    diff=$((diff + 1))
    echo "DIFF $n $(diff "$work/$n.l.code" "$work/$n.c.code" | grep -c '^<') $(diff "$work/$n.l.data" "$work/$n.c.data" | grep -c '^<')"
  fi
done
echo "same $same, differ $diff, failed $fail"
