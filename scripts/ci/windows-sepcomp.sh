#!/bin/bash
# scripts/ci/windows-sepcomp.sh: separate compilation on windows/amd64
# (decision 0388) on the xo-windows runner, before it is the default there
# (owner-approved runs, 2026-10-28):
#   gh workflow run ci.yml -f suite=script -f runner=xo-windows \
#     -f ref=BRANCH -f packages="scripts/ci/windows-sepcomp.sh"
# 1. The default: after `xo dev stdcache` (both concurrency settings), a
#    plain `xo build` is the same bytes as one with XO_SEPCOMP=1 and the
#    variants named, so the default took the separate path.
# 2. The per-object PDB in a real debugger: cdb on a separately compiled
#    program stops at a program function and at a std function (from
#    std's store and std.store.pdb), and `kn` shows source lines and `dv`
#    the locals.
# 3. A stripped (-s) separate build: the async preemption goldens,
#    fault_unwind, fault_defer_chain, fault_tasks, json_text and
#    conc_scopes against their want.txt (and unstripped); and cdb's `kn`
#    from ntdll's process exit walks the program's frames by its unwind
#    tables alone (no PDB) to the thread's start.
# $XO_CI_OUT/windows-sepcomp.txt, every cdb transcript beside it; exit 1
# when a check fails.
set -uo pipefail
out=${XO_CI_OUT:?XO_CI_OUT}
xo=${XO_CLI:?XO_CLI}
np() { if command -v cygpath >/dev/null; then cygpath -w "$1"; else echo "$1"; fi; }
up() { if command -v cygpath >/dev/null; then cygpath -u "$1"; else echo "$1"; fi; }
o=$(up "$out")
xo=$(up "$xo")
. scripts/ci/ci-out.sh
w=$(ci_work windows-sepcomp) || exit 2
trap 'rm -rf "${w:?}"' EXIT
sum="$o/windows-sepcomp.txt"
: > "$sum"
note() { echo "$1" | tee -a "$sum"; }
fail=0
check() { # NAME CONDITION-RESULT(0 ok) DETAIL
  if [ "$2" = 0 ]; then note "ok   $1"; else note "FAIL $1: $3"; fail=1; fi
}
export XO_STD=${XO_CI_STD:-$(dirname "$(dirname "$xo")")/std}
note "xo $("$xo" version 2>&1 | head -1); std $XO_STD"
# a cache of the run's own
export XO_BACKEND_CACHE_DIR
XO_BACKEND_CACHE_DIR=$(np "$w/cache")

# a PATH with no env program (Git Bash's usr/bin has one; PowerShell's
# and cmd's PATH do not): go's directory and System32 only, as a user's
# shell (CI 40810568719: xo spawned env(1) and failed in 30 ms)
gobin=$(dirname "$(command -v go)")
CLEAN="$gobin:/c/Windows/System32:/c/Windows"
note "the clean PATH: $CLEAN (env there: $(PATH=$CLEAN command -v env || echo none))"
# std's variants, as the default finds them
PATH=$CLEAN "$xo" dev stdcache --conc=0 > "$w/sc0.txt" 2> "$o/stdcache-conc0.err" || { note "FAIL xo dev stdcache --conc=0: $(tail -3 "$o/stdcache-conc0.err")"; exit 1; }
PATH=$CLEAN "$xo" dev stdcache --conc=1 > "$w/sc1.txt" 2> "$o/stdcache-conc1.err" || { note "FAIL xo dev stdcache --conc=1: $(tail -3 "$o/stdcache-conc1.err")"; exit 1; }
note "ok   xo dev stdcache with no env program on PATH"
# the Go backend, the same way (0.33.1's go_build spawned env(1) too)
gp="$w/goprog"; mkdir -p "$gp"
printf 'module goprog\nedition preview\n' > "$gp/xo.mod"
printf 'fn main(os: Os) {\n  os.stdio.println("go backend ${[1, 2, 3].len()}")\n}\n' > "$gp/main.xo"
got=$(cd "$gp" && PATH=$CLEAN "$xo" run --backend=go . 2> "$o/goback.err" | tr -d '\r')
[ "$got" = "go backend 3" ]; check "xo run --backend=go with no env program on PATH" $? "got '$got': $(tail -3 "$o/goback.err")"
ST0=$(sed -n 1p "$w/sc0.txt" | tr -d '\r'); FA0=$(sed -n 2p "$w/sc0.txt" | tr -d '\r')
ST1=$(sed -n 1p "$w/sc1.txt" | tr -d '\r'); FA1=$(sed -n 2p "$w/sc1.txt" | tr -d '\r')
note "std store conc0 $ST0 ($(wc -c < "$(up "$ST0")") bytes), conc1 $ST1"
SEP=(XO_SEPCOMP=1 "XO_SEPCOMP_FACTS=$FA0" "XO_SEPCOMP_FACTS_CONC=$FA1" "XO_SEPCOMP_STORE=$ST0" "XO_SEPCOMP_STORE_CONC=$ST1")

# a program with a recursive function (not inlined away), std calls and
# locals
p="$w/prog"
mkdir -p "$p"
printf 'module prog\nedition preview\n' > "$p/xo.mod"
cat > "$p/main.xo" <<'EOF'
fn sum_to(n: Int) -> Int {
  if n <= 0 {
    return 0
  }
  let rest = sum_to(n - 1)
  n + rest
}

fn main(os: Os) {
  let words = "alpha,beta,gamma".split(",")
  var total = 0
  for w in words {
    total += w.to_upper().len()
  }
  os.stdio.println("sum ${sum_to(5)} words ${words.len()} total ${total}")
}
EOF
build() { # DIR OUT [ARGS...]: with $BENV
  local d=$1 x=$2
  shift 2
  (cd "$d" && env "${BENV[@]}" "$xo" build --backend=amd64 "$@" -o "$(np "$x")" .) > "$x.log" 2>&1
}
BENV=(XO_SEPCOMP_DEFAULT=1)
build "$p" "$w/default" ; rd=$?
BENV=("${SEP[@]}")
build "$p" "$w/explicit"; re=$?
BENV=(XO_SEPCOMP=0)
build "$p" "$w/whole"; rw=$?
check "builds (default $rd, explicit sep $re, whole $rw)" $((rd + re + rw)) "$(tail -3 "$w/default.log" "$w/explicit.log" "$w/whole.log")"
cmp -s "$w/default.exe" "$w/explicit.exe"; check "the default build is the separate build's bytes" $? "$(cmp "$w/default.exe" "$w/explicit.exe" 2>&1 | head -1)"
cmp -s "$w/default.exe" "$w/whole.exe"; [ $? != 0 ]; check "and not the whole program's" $? "default.exe equals whole.exe: the default did not take the separate path"
for v in explicit whole; do "$w/$v.exe" > "$w/$v.out" 2>&1; done
cmp -s "$w/explicit.out" "$w/whole.out"; check "separate and whole print the same ($(head -1 "$w/whole.out"))" $? "$(cat "$w/explicit.out")"
note "explicit.pdb $(wc -c < "$w/explicit.pdb" 2>/dev/null) bytes, whole.pdb $(wc -c < "$w/whole.pdb" 2>/dev/null)"

cdb=""
for c in "/c/Program Files (x86)/Windows Kits/10/Debuggers/x64/cdb.exe" "/c/Program Files/Windows Kits/10/Debuggers/x64/cdb.exe"; do
  [ -x "$c" ] && cdb=$c && break
done
dbg() { # NAME EXE COMMANDS: cdb's transcript in $o/cdb-NAME.txt
  "$cdb" -lines -y "$(np "$(dirname "$2")")" -c "$3" "$(np "$2")" > "$o/cdb-$1.txt" 2>&1
}
if [ -z "$cdb" ]; then
  note "FAIL no cdb on the runner (the Windows SDK's debugger)"
  fail=1
else
  note "cdb $cdb"
  # the program's function, from the program's own PDB part
  dbg prog "$w/explicit.exe" '.lines -e; bm explicit!*sum_to*; g; kn 6; dv /t; q'
  grep -q "sum_to" "$o/cdb-prog.txt"; check "cdb stops in sum_to" $? "no sum_to frame (cdb-prog.txt)"
  grep -Eq "main\.xo @ [0-9]+" "$o/cdb-prog.txt"; check "cdb's kn names main.xo lines" $? "no main.xo @ LINE (cdb-prog.txt)"
  grep -Eq "[[:space:]]n = " "$o/cdb-prog.txt"; check "cdb's dv shows n" $? "dv shows no n (cdb-prog.txt)"
  # std's functions, from std.store.pdb's parts
  dbg std "$w/explicit.exe" '.lines -e; x explicit!*split*; x explicit!*to_upper*; bm explicit!*split*; g; kn 6; dv /t; q'
  grep -Eq "std[\\/][^ ]*\.xo @ [0-9]+" "$o/cdb-std.txt"; check "cdb's kn names a std source line in a std function" $? "no std/...xo @ LINE (cdb-std.txt)"
  grep -Eq "^[[:space:]]*[A-Za-z_].* = " "$o/cdb-std.txt"; check "cdb's dv lists a std function's locals" $? "dv listed nothing (cdb-std.txt)"
fi

# stripped separate builds of goldens, and unstripped
G=internal/llvm/testdata/golden
for g in async_preempt_alloc async_preempt_regs fault_unwind fault_defer_chain fault_tasks json_text conc_scopes; do
  d="$w/g-$g"
  cp -R "$G/$g" "$d"
  [ -f "$d/xo.mod" ] || printf 'module %s\nedition preview\n' "$g" > "$d/xo.mod"
  for s in strip plain; do
    BENV=("${SEP[@]}")
    if [ $s = strip ]; then build "$d" "$w/$g-$s" -s; else build "$d" "$w/$g-$s"; fi
    rb=$?
    if [ $rb != 0 ]; then check "$g $s builds" 1 "$(tail -3 "$w/$g-$s.log")"; continue; fi
    if [ $s = strip ] && [ -f "$w/$g-$s.pdb" ]; then check "$g -s writes no PDB" 1 "$g-$s.pdb exists"; fi
    sh scripts/golden-run.sh "$w/$g-$s.exe" "$d" > "$o/golden-$g-$s.txt" 2>&1
    check "$g ($s, separate) against want.txt" $? "$(tail -6 "$o/golden-$g-$s.txt")"
  done
done

# the unwind tables alone: cdb's stack at the process's exit, stripped
if [ -n "$cdb" ]; then
  for s in strip plain; do
    dbg "exit-$s" "$w/fault_unwind-$s.exe" 'bp ntdll!RtlExitUserProcess; g; kn 40; q'
    grep -q "RtlUserThreadStart" "$o/cdb-exit-$s.txt"; check "cdb walks fault_unwind ($s) from its exit to the thread's start" $? "no RtlUserThreadStart frame (cdb-exit-$s.txt)"
    if grep -q "unwind information not available\|Frame IP not in any known module" "$o/cdb-exit-$s.txt"; then check "cdb found unwind information ($s)" 1 "cdb-exit-$s.txt warns"; else check "cdb found unwind information ($s)" 0 ""; fi
    note "  $s: $(grep -c '^[0-9a-f][0-9a-f] ' "$o/cdb-exit-$s.txt") frames"
  done
fi
note "result: $([ $fail = 0 ] && echo PASS || echo FAIL)"
exit $fail
