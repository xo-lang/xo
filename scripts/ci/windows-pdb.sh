#!/bin/bash
# scripts/ci/windows-pdb.sh: decision 0344's PDB (stages 1 to 3: functions,
# lines, inlined calls) read by Windows' own PDB reader, dbghelp.dll, on
# the xo-windows runner (one owner-approved run, 2026-10-22), with
# decision 0343's crash checks in the same job:
#   gh workflow run ci.yml -f suite=script -f runner=xo-windows \
#     -f ref=BRANCH -f packages="scripts/ci/windows-pdb.sh"
# It runs scripts/ci/windows-crash.sh (its checks and windows-crash.txt),
# keeping its program, PDB and reports, then names the segv and task
# reports' frames from the PDB alone (scripts/pdbframes: SymFromAddr,
# SymGetLineFromAddr, and the inline frames of SymQueryInlineTrace) and
# compares that with each report's own traceback, which the runtime names
# from the DWARF: all of it for segv, its first five lines (the program's
# frames, before the runtime's thunks the PDB names by their symbols) for
# task. cdb, when the runner has the Windows SDK's, also lists poke and
# the first frame's function (logged, not checked). $XO_CI_OUT/windows-pdb.txt;
# exit 1 when either part fails. Dry run 2026-10-22: pdbframes under Wine
# (its dbghelp reads PDBs too) printed the report's traceback exactly.
set -uo pipefail
out=${XO_CI_OUT:?XO_CI_OUT}
np() { if command -v cygpath >/dev/null; then cygpath -w "$1"; else echo "$1"; fi; }
up() { if command -v cygpath >/dev/null; then cygpath -u "$1"; else echo "$1"; fi; }
o=$(up "$out")
. scripts/ci/ci-out.sh
w=$(ci_work windows-pdb) || exit 2
trap 'rm -rf "${w:?}"' EXIT
sum="$o/windows-pdb.txt"
: > "$sum"
note() { echo "$1" | tee -a "$sum"; }
fail=0
WINDOWS_CRASH_KEEP="$w/keep" bash scripts/ci/windows-crash.sh || { note "FAIL windows-crash.sh (windows-crash.txt)"; fail=1; }
k="$w/keep"
[ -f "$k/crash.exe" ] && [ -f "$k/crash.pdb" ] || { note "FAIL no crash.exe and crash.pdb kept"; exit 1; }
note "crash.pdb: $(wc -c < "$k/crash.pdb") bytes"
if ! go build -o "$(np "$w/pdbframes.exe")" ./scripts/pdbframes > "$o/pdbframes-build.txt" 2>&1; then
  note "FAIL build pdbframes: $(tail -5 "$o/pdbframes-build.txt")"
  exit 1
fi
# NAME LINES: the report's traceback (its first LINES lines, all when 0)
# against pdbframes'
cmp_trace() {
  local name=$1 lines=$2
  sed -n '/^traceback:/,$p' "$k/$name.err" > "$w/$name.want"
  "$w/pdbframes.exe" "$(np "$k/crash.exe")" < "$k/$name.err" > "$w/$name.got" 2> "$w/$name.gerr"
  if [ "$lines" != 0 ]; then
    head -n "$lines" "$w/$name.want" > "$w/$name.w2"; mv "$w/$name.w2" "$w/$name.want"
    head -n "$lines" "$w/$name.got" > "$w/$name.g2"; mv "$w/$name.g2" "$w/$name.got"
  fi
  if cmp -s "$w/$name.want" "$w/$name.got"; then note "ok   $name: the PDB's traceback is the report's"; else
    note "FAIL $name: the PDB's traceback differs"; fail=1
  fi
  { echo "---- $name report:"; cat "$w/$name.want"; echo "---- $name from the PDB:"; cat "$w/$name.got" "$w/$name.gerr"; } >> "$sum"
}
cmp_trace segv 0
cmp_trace task 6
cdb=""
for c in "/c/Program Files (x86)/Windows Kits/10/Debuggers/x64/cdb.exe" "/c/Program Files/Windows Kits/10/Debuggers/x64/cdb.exe"; do
  [ -x "$c" ] && cdb=$c && break
done
if [ -n "$cdb" ]; then
  pc=$(sed -n 's/^frames: \(0x[0-9a-f]*\).*/\1/p' "$k/segv.err")
  ib=$(sed -n 's/^pc .*(image base \(0x[0-9a-f]*\)).*/\1/p' "$k/segv.err")
  a=$(printf '0x%x' $((pc - ib + 0x140000000)))
  timeout 120 "$cdb" -z "$(np "$k/crash.exe")" -y "$(np "$k")" -c ".reload /f; lm; x crash!poke; ln $a; .lines -e; q" > "$o/cdb.txt" 2>&1
  note "cdb (logged in cdb.txt): $(grep -c 'poke' "$o/cdb.txt") lines name poke"
else
  note "no cdb on the runner: dbghelp only"
fi
ci_offload "$o" "$sum"
echo "windows-pdb: $(grep -c '^ok' "$sum") ok, $(grep -c '^FAIL' "$sum") failed"
exit $fail
