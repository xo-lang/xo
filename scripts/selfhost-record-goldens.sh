#!/bin/sh
# selfhost-record-goldens.sh: record the Go side of every TestSelfhost*
# differential (decision 0178, stage 7 step 2) as hashes into
# $XO_SELFHOST_GOLDENS_DIR (default internal/selfhost/testdata/gogoldens,
# committed: one directory per kind, 16 sorted text files each, one line
# per piece, `<kind> <unit id> <piece> <hash>`, a piece being a whole
# dump or one function, helper, file, or declaration; the format is in
# internal/selfhost/gogoldens_test.go). The Go compiler packages compute
# every unit the full tier compares (every repository run, the spec
# blocks, the rune files, and every corpus and mutation at the full
# tier's sizes); no self hosted binary is built. Afterwards
#
#   XO_SELFHOST_GOLDENS=replay go test ./internal/selfhost
#
# compares the hashes of the Xo side's pieces with the recording instead
# of running the Go side (any tier, any -run). Usage:
#
#   scripts/selfhost-record-goldens.sh            # replace the recording
#   scripts/selfhost-record-goldens.sh --merge    # add to it (keep units
#                                                 # no test computes now)
#   RUN='^TestSelfhostLLVM$' scripts/selfhost-record-goldens.sh --merge
#
# Once the Go implementation is frozen (stage 7 step 1) and deleted, the
# recording is regenerated from the Xo implementation instead (see
# knowledgebase/decisions/0178-retire-go-implementation-plan.md).
set -eu
cd "$(dirname "$0")/.."
out=${XO_SELFHOST_GOLDENS_DIR:-internal/selfhost/testdata/gogoldens}
case "$out" in /*) ;; *) out=$(pwd)/$out ;; esac
if [ "${1:-}" != "--merge" ]; then
  rm -rf "$out"
fi
run=${RUN:-'^TestSelfhost(Differential|ParserDifferential|Format|Checker|CheckerMutations|CheckerModuleMutations|CheckerHatches|SSA|SSAMutations|Arm64|Arm64Mutations|LLVM|LLVMMutations|Gen|GenMutations|Defhash)$'}
start=$(date +%s)
XO_SELFHOST_GOLDENS=record XO_SELFHOST_GOLDENS_DIR="$out" XO_SELF_DRIVER=0 \
  go test -count=1 -timeout 3h -run "$run" ./internal/selfhost
echo "recorded in $(( $(date +%s) - start )) s into $out:"
for k in "$out"/*/; do
  units=$(cat "$k"*.txt | cut -d' ' -f2 | sort -u | wc -l | tr -d ' ')
  lines=$(cat "$k"*.txt | wc -l | tr -d ' ')
  echo "$(basename "$k") $units units, $lines lines, $(cat "$k"*.txt | wc -c | tr -d ' ') bytes"
done
echo "total $(cat "$out"/*/*.txt | wc -c | tr -d ' ') bytes ($(cat "$out"/*/*.txt | gzip -9 | wc -c | tr -d ' ') gzip -9)"
