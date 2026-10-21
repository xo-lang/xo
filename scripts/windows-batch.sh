#!/bin/bash
# windows-batch.sh: the Windows checks of a batch of merges (decision
# 0197, stage 6), run on a Windows host set up by scripts/remote-windows.sh
# (Git Bash, in the checkout): `scripts/remote-windows.sh --batch [COMMIT]`
# runs it there and fetches the log.
#
#   1. go run ./scripts/xobuild: an earlier xo, normally the prebuilt
#      windows-amd64 seed (decision 0216; the go-compiler-final chain only
#      when XO_SEED_FALLBACK=chain is set), builds xo1, the shipped xo,
#      from compiler/xo with the Go backend;
#   2. xo1 builds every LLVM golden for windows/amd64 and runs it
#      (TestGoldenWant with XO_GOLDEN_WANT_XO), then cross builds them all
#      for windows/arm64 (not run: no host);
#   3. the LLVM package's runtime tests that run on Windows: threads,
#      tasks, the event loop, the two phase shutdown (Ctrl-Break), and the
#      TLS and HTTP/2 differential tests;
#   4. `xo1 test` of the native test directories of cmd/xo's
#      TestNoGCTests, the LLVM backend's report against the Go backend's;
#   5. asynchronous preemption on the native amd64 backend (decision 0318
#      stage 3): the goldens async_preempt_regs and async_preempt_alloc
#      and the preempt program, built with `xo1 build --backend=amd64`, run with
#      XO_THREADS=1, their output against want, and a nonzero preempt
#      count (XO_RTSTATS).
#
# Each step prints a "batch:" summary line; the exit status is the number
# of steps that failed. Set XO_BATCH_STEPS (e.g. "1 2") to run some only.
set -u
cd "$(dirname "$0")/.." || exit 1
root=$(pwd -W 2>/dev/null || pwd)
export XO_STD="$root/std"
xo1="$HOME/xo1.exe"
steps=${XO_BATCH_STEPS:-1 2 3 4 5}
failed=0
say() { echo "batch: $*"; }
want() { case " $steps " in *" $1 "*) return 0 ;; esac; return 1; }

if want 1; then
  # an xo1 left by an earlier run must not stand in for this one
  rm -f "$xo1"
  # decision 0215: an earlier xo (else the go-compiler-final seed, built
  # with Go) builds bin/xo.exe with the Go backend (internal/xotool)
  if built=$(go run ./scripts/xobuild) && cp "$built" "$xo1"; then
    say "1 xo1 built: $xo1"
  else
    say "1 FAIL: building xo1"; failed=$((failed + 1))
  fi
fi

if want 2; then
  # preflight: one golden (arith) through the same test first, so a
  # broken toolchain is one error instead of a failure per golden
  # (2026-10-21: the test's clang cache wrapper was not found, and all
  # 271 goldens failed with "needs clang"); the same go test path, since
  # the test binary sets $XO_CLANG itself
  pline=$(XO_GOLDEN_WANT=1 XO_GOLDEN_WANT_XO=$(cygpath -m "$xo1") \
    go test -count=1 -timeout 10m -run 'TestGoldenWant$/^arith$' -v ./internal/llvm 2>&1 | tee "$HOME/batch-preflight.log" | grep -E 'golden (summary|arith)')
  pre_ok=0
  case "$pline" in
  *" 1 pass,"*) pre_ok=1 ;;
  *)
    say "2 FAIL: preflight (golden arith through TestGoldenWant): ${pline:-no summary; see $HOME/batch-preflight.log}"
    grep -m3 -E 'needs clang|FAIL|error' "$HOME/batch-preflight.log" | sed 's/^ */batch:   /'
    failed=$((failed + 1))
    ;;
  esac
  if [ "$pre_ok" = 1 ]; then
  for t in windows/amd64 windows/arm64; do
    tgt=""
    [ "$t" = windows/arm64 ] && tgt=$t
    line=$(XO_GOLDEN_WANT=1 XO_GOLDEN_WANT_TARGET=$tgt XO_GOLDEN_WANT_XO=$(cygpath -m "$xo1") \
      go test -count=1 -timeout 60m -run 'TestGoldenWant$' -v ./internal/llvm 2>&1 | tee "$HOME/batch-goldens-${t#*/}.log" | grep 'golden summary')
    say "2 $t: ${line#*: }"
    grep -E 'golden [^ ]+: fail' "$HOME/batch-goldens-${t#*/}.log" | sed 's/^ */batch:   /'
    # TestGoldenWant logs want and got of each differing program: print them
    awk '/output differs from want.txt/ {p=1} /^ *--- (PASS|FAIL|SKIP)|^=== / {p=0} p' "$HOME/batch-goldens-${t#*/}.log"
    case "$line" in *" 0 fail"*) ;; *) failed=$((failed + 1)) ;; esac
  done
  fi
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

if want 5; then
  bad=0
  for d in internal/llvm/testdata/golden/async_preempt_regs internal/llvm/testdata/golden/async_preempt_alloc internal/llvm/testdata/sched/preempt; do
    if [ -f "$d/want.txt" ]; then
      exp=$(sed -n '/^--- stdout$/,/^--- stderr$/p' "$d/want.txt" | sed '1d;$d')
    else
      exp="preempt 5"
    fi
    exe="$HOME/batch-async.exe"
    rm -f "$exe"
    if ! (cd "$d" && "$xo1" build --backend=amd64 -o "$exe" .) > "$HOME/batch-async-build.log" 2>&1; then
      bad=$((bad + 1))
      say "  5 ${d##*/}: build failed"
      tail -5 "$HOME/batch-async-build.log" | sed 's/^/batch:     /'
      continue
    fi
    for i in 1 2 3; do
      out=$(set -o pipefail; XO_THREADS=1 XO_LEAKCHECK=1 XO_RTSTATS=1 timeout 300 "$exe" 2> "$HOME/batch-async.err" | tr -d '\r')
      rc=$?
      n=$(grep -o 'preempt=[0-9]*' "$HOME/batch-async.err" | head -1 | cut -d= -f2)
      if [ "$rc" != 0 ] || [ "$out" != "$exp" ] || [ "${n:-0}" = 0 ]; then
        bad=$((bad + 1))
        say "  5 ${d##*/} run $i: exit $rc preempt=${n:-0}"
        echo "$out" | head -5 | sed 's/^/batch:     /'
        tail -3 "$HOME/batch-async.err" | sed 's/^/batch:     /'
      else
        say "  5 ${d##*/} run $i: ok preempt=$n"
      fi
    done
  done
  say "5 asynchronous preemption (amd64): $bad bad run(s)"
  [ "$bad" = 0 ] || failed=$((failed + 1))
fi

say "done: $failed step(s) failed"
exit "$failed"
