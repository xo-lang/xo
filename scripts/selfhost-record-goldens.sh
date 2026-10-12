#!/bin/sh
# selfhost-record-goldens.sh: record the Go side of every TestSelfhost*
# differential (decision 0178, stage 7 step 2) into
# $XO_SELFHOST_GOLDENS_DIR (default internal/selfhost/testdata/gogoldens,
# git ignored: about 1.4 GB, one gzip file per unit). The Go compiler
# packages compute every unit the full tier compares (every repository
# run, the spec blocks, the rune files, and every corpus and mutation at
# the full tier's sizes); no self hosted binary is built. Afterwards
#
#   XO_SELFHOST_GOLDENS=replay go test ./internal/selfhost
#
# compares the Xo side against the recording instead of running the Go
# side (any tier, any -run). Usage:
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
  echo "$(basename "$k") $(find "$k" -name '*.gz' | wc -l | tr -d ' ') units $(du -sk "$k" | cut -f1) KiB"
done
du -sh "$out"
