#!/bin/bash
# scripts/ci/windows-crash.sh: decision 0343 stage 2, the crash report of
# a windows/amd64 program (std/native/llrt/crash.xo: the last vectored
# handler on_crash_veh and the unhandled exception filter on_crash_win),
# the run Wine cannot stand in for. For suite=script on the xo-windows
# runner (one owner-approved run, 2026-10-22):
#   gh workflow run ci.yml -f suite=script -f runner=xo-windows \
#     -f ref=BRANCH -f packages="scripts/ci/windows-crash.sh"
# The runner has no clang (CI 39827577406: `use c` cannot bind a header),
# and safe Xo raises no hardware fault, so the faults come from a scratch
# std module written here, crashpoke, in a copy of the checkout's std
# (std/crashpoke.xo for the Go backend's checker, std/native/crashpoke.xo
# with the runtime's intrinsics), built with --std and --backend=amd64:
#   segv      a write through address 16 two Xo frames down (the vectored
#             handler: every frame the program's)
#   task      the same in a task (a worker's fiber)
#   thread    an OS thread started at a ud2 in VirtualAlloc'd code (the
#             unhandled filter: the fault outside the image, on a thread's
#             own stack, under RtlUserThreadStart)
#   overflow  Xo recursion without end: still entry.xo on_overflow's
#             "fatal error: stack overflow", exit 2
#   off       segv with XO_CRASH_REPORT=0: no report
# Each run checks the report's lines and the exit code, the NTSTATUS
# (read with PowerShell, which keeps its 32 bits). $XO_CI_OUT/windows-crash.txt
# has a line per check and every report; exit 1 on any mismatch.
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
# the scratch std and the program
cp -R std "$w/std"
cat > "$w/std/crashpoke.xo" <<'EOF'
// scripts/ci/windows-crash.sh's scratch module (never in the checkout's std)
pub fn write(a: Int) -> Int { a }

pub fn deep(n: Int) -> Int { n }

pub fn trap_thread() -> Int { 0 }
EOF
cat > "$w/std/native/crashpoke.xo" <<'EOF'
// scripts/ci/windows-crash.sh's scratch module: hardware faults
use native/rt
use native/sys_windows

pub fn write(a: Int) -> Int {
  rt.mem_store(a, 0, 42, 8)
  rt.mem_load(a, 0, 8)
}

pub fn deep(n: Int) -> Int {
  deep(n + 1) + 1
}

/// A thread started at a ud2 in code of its own; this one waits for the
/// fault to end the process.
pub fn trap_thread() -> Int {
  // MEM_COMMIT | MEM_RESERVE, PAGE_EXECUTE_READWRITE; ud2 is 0f 0b
  let p = sys_windows.VirtualAlloc(0, 4096, 12288, 64)
  rt.mem_store(p, 0, 2831, 2)
  let _t = sys_windows.CreateThread(0, 65536, p, 0, 0, 0)
  while true {
    sys_windows.Sleep(1000)
  }
  0
}
EOF
mkdir -p "$w/prog"
cat > "$w/prog/main.xo" <<'EOF'
use std/crashpoke

fn poke(kind: Str, n: Int) -> Int {
  if n > 0 {
    return poke(kind, n - 1) + 1
  }
  match kind {
    "overflow" => crashpoke.deep(n) + 1
    "thread" => crashpoke.trap_thread() + 1
    _ => crashpoke.write(16) + 1
  }
}

fn main(os: Os) {
  let kind = os.env.get("XO_CRASH_KIND") ?? "segv"
  os.stdio.println("before")
  if kind == "task" {
    scope s {
      s.spawn(fn() {
        let _n = poke("segv", 2)
      })
    }
    return
  }
  os.stdio.println("${poke(kind, 2)}")
}
EOF
# one run: NAME KIND WANT_CODE EXTRA_ENV (VAR=V, or - for none), then the
# regexes stderr must match (EMPTY: stderr must be empty). The exit code
# is read in a second run under PowerShell.
check() {
  local name=$1 kind=$2 want=$3 extra=$4
  shift 4
  local ev=(XO_CRASH_KIND="$kind")
  [ "$extra" = - ] || ev+=("$extra")
  (export "${ev[@]}"; tmo "$w/crash.exe" > "$o/run.out" 2> "$o/run.err" < /dev/null)
  local rc=$?
  local got
  got=$(export "${ev[@]}"; tmo powershell.exe -NoProfile -NonInteractive -Command "& '$(np "$w/crash.exe")' 2>&1 | Out-Null; [Console]::Out.Write(\$LASTEXITCODE)")
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
if ! "$XO_CLI" build --std="$(np "$w/std")" --backend=amd64 -o "$(np "$w/crash.exe")" "$(np "$w/prog")" > "$o/build.txt" 2>&1; then
  note "FAIL build: $(tail -5 "$o/build.txt")"
  exit 1
fi
first='^fatal error: EXCEPTION_ACCESS_VIOLATION: access violation \(code 0xc0000005, write\) at address 0x10\n'
frames='(?m)^  crashpoke\.write \(crashpoke\.xo:6\) \[inlined\]\n  poke \(main\.xo:7\)\n  poke \(main\.xo:5\)\n  poke \(main\.xo:5\)\n'
check segv segv $AV - "$first" '\nregisters:\n  rax ' '\nframes: 0x' '\ntraceback:\n' "$frames" '(?m)^  main \(main\.xo:\d+\)$'
grep -q "crash report faulted" "$o/run.err" && { note "FAIL segv: the handler ran twice"; fail=1; }
check task task $AV - "$first" "$frames" '(?m)^  main\.fn1 \(main\.xo:\d+\)$'
check thread thread $ILL - '^fatal error: EXCEPTION_ILLEGAL_INSTRUCTION: illegal instruction \(code 0xc000001d\)\n' '\nframes: 0x'
check overflow overflow 2 - '^fatal error: stack overflow\n'
grep -q "EXCEPTION_" "$o/run.err" && { note "FAIL overflow: reported as an exception"; fail=1; }
check off segv $AV XO_CRASH_REPORT=0 EMPTY
ci_offload "$o" "$sum"
echo "windows-crash: $(grep -c '^ok' "$sum") ok, $(grep -c '^FAIL' "$sum") failed"
exit $fail
