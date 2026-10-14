#!/bin/bash
# windows-batch.sh: the Windows checks of a batch of merges (decision
# 0197, stage 6), run on a Windows host set up by scripts/remote-windows.sh
# (Git Bash, in the checkout): `scripts/remote-windows.sh --batch [COMMIT]`
# runs it there and fetches the log.
#
#   1. go run ./scripts/xobuild: an earlier xo, or the xo1 of the
#      go-compiler-final commit through the seed chain (opted in with
#      XO_SEED_FALLBACK=chain: there is no Windows seed yet, decision
#      0216), builds xo1, the shipped xo, from selfhost/xo with the Go
#      backend;
#   2. xo1 builds every LLVM golden for windows/amd64 and runs it
#      (TestGoldenWant with XO_GOLDEN_WANT_XO), then cross builds them all
#      for windows/arm64 (not run: no host);
#   3. the LLVM package's runtime tests that run on Windows: threads,
#      tasks, the event loop, the two phase shutdown (Ctrl-Break), and the
#      TLS and HTTP/2 differential tests;
#   4. `xo1 test` of the native test directories of cmd/xo's
#      TestNoGCTests, the LLVM backend's report against the Go backend's.
#
# Each step prints a "batch:" summary line; the exit status is the number
# of steps that failed. Set XO_BATCH_STEPS (e.g. "1 2") to run some only.
set -u
cd "$(dirname "$0")/.." || exit 1
root=$(pwd -W 2>/dev/null || pwd)
export XO_STD="$root/std"
xo1="$HOME/xo1.exe"
steps=${XO_BATCH_STEPS:-1 2 3 4}
failed=0
say() { echo "batch: $*"; }
want() { case " $steps " in *" $1 "*) return 0 ;; esac; return 1; }

if want 1; then
  # decision 0215: an earlier xo (else the go-compiler-final seed, built
  # with Go) builds bin/xo.exe with the Go backend (internal/xotool)
  if built=$(XO_SEED_FALLBACK=${XO_SEED_FALLBACK:-chain} go run ./scripts/xobuild) && cp "$built" "$xo1"; then
    say "1 xo1 built: $xo1"
  else
    say "1 FAIL: building xo1"; failed=$((failed + 1))
  fi
fi

if want 2; then
  for t in windows/amd64 windows/arm64; do
    tgt=""
    [ "$t" = windows/arm64 ] && tgt=$t
    line=$(XO_GOLDEN_WANT=1 XO_GOLDEN_WANT_TARGET=$tgt XO_GOLDEN_WANT_XO=$(cygpath -m "$xo1") \
      go test -count=1 -timeout 60m -run 'TestGoldenWant$' -v ./internal/llvm 2>&1 | tee "$HOME/batch-goldens-${t#*/}.log" | grep 'golden summary')
    say "2 $t: ${line#*: }"
    grep -E 'golden [^ ]+: fail' "$HOME/batch-goldens-${t#*/}.log" | sed 's/^ */batch:   /'
    case "$line" in *" 0 fail"*) ;; *) failed=$((failed + 1)) ;; esac
  done
fi

if want 3; then
  run='TestRuntimeThreads|TestRuntimeTasks|TestRuntimeEventLoop|TestGoldenThreads|TestGoldenTasks|TestDeadlock|TestNativeSignal|TestTLSDiff|TestTLSLeaks|TestH2Diff'
  if go test -count=1 -timeout 60m -run "$run" ./internal/llvm > "$HOME/batch-llvm.log" 2>&1; then
    say "3 LLVM runtime tests: ok"
  else
    say "3 FAIL: LLVM runtime tests (log $HOME/batch-llvm.log)"
    grep -E '^(--- FAIL|    --- FAIL)' "$HOME/batch-llvm.log" | sed 's/^/batch:   /'
    failed=$((failed + 1))
  fi
fi

if want 4; then
  g=internal/gen/testdata/golden
  same=0
  differ=0
  for d in cmd/xo/testdata/nogc/same cmd/xo/testdata/nogc/prop examples/notes-service $g/test_stdin_stop $g/test_signal_after $g/test_fakes $g/test_map_until $g/test_run_main_void $g/test_once $g/test_logs $g/test_shutdown $g/test_cli $g/test_term $g/test_proc_cmd $g/test_net_only $g/test_unix_kill; do
    "$xo1" test -v "$d" > "$HOME/batch-test-go.out" 2>&1
    gc=$?
    "$xo1" test --no-gc --backend=llvm -v "$d" > "$HOME/batch-test-llvm.out" 2>&1
    lc=$?
    if [ "$gc" = "$lc" ] && cmp -s "$HOME/batch-test-go.out" "$HOME/batch-test-llvm.out"; then
      same=$((same + 1))
    else
      differ=$((differ + 1))
      say "  differs: $d (go exit $gc, llvm exit $lc)"
    fi
  done
  say "4 xo test, LLVM against Go: $same same, $differ differ"
  [ "$differ" = 0 ] || failed=$((failed + 1))
fi

say "done: $failed step(s) failed"
exit "$failed"
