#!/bin/bash
# scripts/ci/windows-crash.sh: decision 0343 stage 2, the crash report of
# a windows/amd64 program (std/native/llrt/crash.xo on_crash_win, the
# unhandled exception filter), the run Wine cannot stand in for. For
# suite=script on the xo-windows runner (one owner-approved run,
# 2026-10-22):
#   gh workflow run ci.yml -f suite=script -f runner=xo-windows \
#     -f ref=BRANCH -f packages="scripts/ci/windows-crash.sh"
# Builds internal/arm64/testdata/crash (TestCrashReport's program: a C
# function behind `use c` writes through address 16, reaches ud2, or
# recurses without end, two Xo frames down) with --backend=amd64 and
# --backend=llvm, runs each kind and checks what TestCrashReport checks
# on Unix: the first line, the registers, the frames' pcs, the traceback
# with the C function, poke's frames and main; that the exit code stays
# the exception's NTSTATUS (read with PowerShell, which keeps the 32
# bits); that a stack overflow is still entry.xo on_overflow's ("fatal
# error: stack overflow", exit 2); that XO_CRASH_REPORT=0 prints nothing.
# $XO_CI_OUT/windows-crash.txt has a line per check and every report;
# exit 1 on any mismatch.
set -uo pipefail
RUN_TIMEOUT=${RUN_TIMEOUT:-60}
if command -v timeout >/dev/null; then tmo() { timeout -k 10 "$RUN_TIMEOUT" "$@"; }; else tmo() { "$@"; }; fi
out=${XO_CI_OUT:?XO_CI_OUT}
np() { if command -v cygpath >/dev/null; then cygpath -w "$1"; else echo "$1"; fi; }
up() { if command -v cygpath >/dev/null; then cygpath -u "$1"; else echo "$1"; fi; }
o=$(up "$out")
. scripts/ci/ci-out.sh
w=$(ci_work windows-crash) || exit 2
trap 'rm -rf "${w:?}"' EXIT
sum="$o/windows-crash.txt"
: > "$sum"
"$XO_CLI" version --json | tee -a "$sum"
fail=0
note() { echo "$1" | tee -a "$sum"; }
# one run: NAME EXE KIND WANT_CODE EXTRA_ENV (VAR=V, or - for none), then
# the regexes stderr must match (EMPTY: stderr must be empty). The exit
# code is read in a second run under PowerShell, which keeps its 32 bits
# (bash's status keeps 8).
check() {
  local name=$1 exe=$2 kind=$3 want=$4 extra=$5
  shift 5
  local ev=(XO_CRASH_KIND="$kind")
  [ "$extra" = - ] || ev+=("$extra")
  env "${ev[@]}" "$w/$exe" > "$o/run.out" 2> "$o/run.err" < /dev/null
  local rc=$?
  local got
  got=$(env "${ev[@]}" powershell.exe -NoProfile -NonInteractive -Command "& '$(np "$w/$exe")' 2>&1 | Out-Null; [Console]::Out.Write(\$LASTEXITCODE)")
  local ok=1 why=""
  [ "$got" = "$want" ] || { ok=0; why="exit code $got, want $want"; }
  for re in "$@"; do
    if [ "$re" = EMPTY ]; then
      [ -s "$o/run.err" ] && { ok=0; why="$why; stderr not empty"; }
    elif ! grep -Pzq -- "$re" "$o/run.err"; then
      ok=0; why="$why; no match for $re"
    fi
  done
  if [ $ok = 1 ]; then note "ok   $name (code $got, bash status $rc)"; else note "FAIL $name: $why (bash status $rc)"; fail=1; fi
  { echo "---- $name stdout:"; cat "$o/run.out"; echo "---- $name stderr:"; cat "$o/run.err"; } >> "$sum"
}
AV=-1073741819
ILL=-1073741795
for be in amd64 llvm; do
  exe=crash-$be.exe
  if ! "$XO_CLI" build --backend=$be -o "$(np "$w/$exe")" internal/arm64/testdata/crash > "$o/build-$be.txt" 2>&1; then
    note "FAIL build $be: $(tail -5 "$o/build-$be.txt")"
    fail=1
    continue
  fi
  frames='(?m)^  \S*crashc_write\n  poke \(main\.xo:1[4-7]\)\n  poke \(main\.xo:10\)\n  poke \(main\.xo:10\)\n  main \(main\.xo:32\)$'
  check "$be segv" $exe segv $AV - \
    '^fatal error: EXCEPTION_ACCESS_VIOLATION: access violation \(code 0xc0000005, write\) at address 0x10\n' \
    '\nregisters:\n  rax ' '\nframes: 0x' '\ntraceback:\n' "$frames"
  grep -q "crash report faulted" "$o/run.err" && { note "FAIL $be segv: the handler ran twice"; fail=1; }
  check "$be trap" $exe trap $ILL - \
    '^fatal error: EXCEPTION_ILLEGAL_INSTRUCTION: illegal instruction \(code 0xc000001d\)\n' \
    'crashc_trap\n  poke \(main\.xo:1[45]\)\n'
  check "$be overflow" $exe overflow 2 - '^fatal error: stack overflow\n'
  grep -q "EXCEPTION_" "$o/run.err" && { note "FAIL $be overflow: reported as an exception"; fail=1; }
  check "$be task" $exe task $AV - \
    '^fatal error: EXCEPTION_ACCESS_VIOLATION: access violation \(code 0xc0000005, write\) at address 0x10\n' \
    'crashc_write\n  poke \(main\.xo:1[4-7]\)\n  poke \(main\.xo:10\)\n  poke \(main\.xo:10\)\n'
  check "$be off" $exe segv $AV XO_CRASH_REPORT=0 EMPTY
done
ci_offload "$o" "$sum"
echo "windows-crash: $(grep -c '^ok' "$sum") ok, $(grep -c '^FAIL' "$sum") failed"
exit $fail
