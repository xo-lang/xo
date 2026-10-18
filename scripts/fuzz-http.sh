#!/bin/sh
# fuzz-http.sh OUTDIR [SECONDS]: a fuzzing session of the native HTTP
# stack, from the checkout to test: TestHTTPFuzz (the generator over
# HTTP/1.1, HTTP/2, and the client, seed from the clock) and the Go fuzzer
# on FuzzHTTPDiff (native against net/http), each for SECONDS (default
# 1800). New FuzzHTTPDiff failures land in internal/llvm/testdata/fuzz;
# review them before committing. Statuses go to OUTDIR/fuzz.log, the
# output to OUTDIR/httpfuzz.log and OUTDIR/diff.log. Uses $XO_CLI, else
# bin/xo (go run ./scripts/xobuild first). Run it in a heavy slot.
set -u
out=${1:?usage: fuzz-http.sh OUTDIR [SECONDS]}
secs=${2:-1800}
mkdir -p "$out" || exit 1
out=$(cd "$out" && pwd)
cd "$(dirname "$0")/.." || exit 1
: "${XO_CLI:=$PWD/bin/xo}"
export XO_CLI
log=$out/fuzz.log
seed=$(date +%s)
echo "$(git log --oneline -1 | cut -c1-12) seed=$seed seconds=$secs" >> "$log"
# each TestHTTPFuzz subtest runs for secs, so the timeout covers three
to=$((secs * 4 / 60 + 20))m
XO_FUZZ_SECONDS=$secs FUZZ_SEED=$seed go test -count=1 -timeout "$to" -run 'TestHTTPFuzz$' -v ./internal/llvm > "$out/httpfuzz.log" 2>&1
echo "httpfuzz=$?" >> "$log"
go test -count=1 -timeout "$to" ./internal/llvm -run '^$' -fuzz FuzzHTTPDiff -fuzztime "${secs}s" -parallel 6 > "$out/diff.log" 2>&1
echo "fuzzdiff=$?" >> "$log"
