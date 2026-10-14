#!/bin/sh
# selfhost-record-goldens.sh: record the output of the self hosted
# components as the reference of the TestSelfhost* tests (decision 0178
# stage 7 step 2, decision 0215) into $XO_SELFHOST_GOLDENS_DIR (default
# internal/selfhost/testdata/gogoldens, committed: one directory per
# kind, 16 sorted text files each, one line per piece, `<kind> <unit id>
# <piece> <hash>`, a piece being a whole dump or one function, helper,
# file, or declaration; the format is in
# internal/selfhost/gogoldens_test.go). Until decision 0215 the recording
# was the Go implementation's output (the last one at the tree of the
# go-compiler-final tag and then at the merged main of 2026-10-13); since
# then a recording run takes the Xo side's output as the reference: run
# it after a change that alters the output on purpose, and read the diff
# of the recording. The run takes the full tier's inputs with the go
# backend build of each component. Afterwards
#
#   go test ./internal/selfhost
#
# compares the Xo side with the recording (replay, the default). Usage:
#
#   scripts/selfhost-record-goldens.sh            # add to the recording (keep
#                                                 # the units no test computes now)
#   RUN='^TestSelfhostLLVM$' scripts/selfhost-record-goldens.sh
#   scripts/selfhost-record-goldens.sh --replace  # start from nothing: this
#                                                 # drops the Go recording
#
# A unit recorded again for an input replaces the unit recorded before
# for it (its slot). The components are built by the xo internal/xotool
# names ($XO_CLI, else the checkout's bin/xo).
set -eu
cd "$(dirname "$0")/.."
out=${XO_SELFHOST_GOLDENS_DIR:-internal/selfhost/testdata/gogoldens}
case "$out" in /*) ;; *) out=$(pwd)/$out ;; esac
if [ "${1:-}" = "--replace" ]; then
  rm -rf "$out"
fi
run=${RUN:-'^TestSelfhost(Differential|ParserDifferential|Format|Checker|CheckerMutations|CheckerModuleMutations|CheckerHatches|SSA|SSAMutations|Arm64|Arm64Mutations|LLVM|LLVMMutations|Gen|GenMutations|Defhash)$'}
start=$(date +%s)
XO_SELFHOST_GOLDENS=record XO_SELFHOST_GOLDENS_DIR="$out" \
  go test -count=1 -timeout 3h -run "$run" ./internal/selfhost
echo "recorded in $(( $(date +%s) - start )) s into $out:"
for k in "$out"/*/; do
  units=$(cat "$k"[0-9a-f].txt | cut -d' ' -f2 | sort -u | wc -l | tr -d ' ')
  lines=$(cat "$k"[0-9a-f].txt | wc -l | tr -d ' ')
  pool=$(cat "$k"pool-*.txt 2>/dev/null | wc -l | tr -d ' ')
  echo "$(basename "$k") $units units, $lines lines, $pool pool lines, $(cat "$k"*.txt | wc -c | tr -d ' ') bytes"
done
echo "total $(cat "$out"/*/*.txt | wc -c | tr -d ' ') bytes ($(cat "$out"/*/*.txt | gzip -9 | wc -c | tr -d ' ') gzip -9)"
