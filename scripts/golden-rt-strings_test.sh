#!/bin/bash
# golden-rt-strings_test.sh: scripts/golden-rt-strings.py on a small tree
# (task #552: #540's std/rt fault text change selected no golden)
set -u
here=$(cd "$(dirname "$0")" && pwd)
T=$(mktemp -d)
trap 'rm -rf "${T:?}"' EXIT
mk() { mkdir -p "$(dirname "$T/$1")"; printf '%b' "$2" >"$T/$1"; }
mk put_fault/want.txt "exit 2\\nstdout:\\nstderr:\\nfault: BytesBuilder.put: 3..5 outside the builder's 4 bytes\\n"
mk put_go/want.txt "exit 0\\n"
mk put_go/want_go.txt "fault: BytesBuilder.put: 9..12 outside the builder's 1 bytes\\n"
mk hello/want.txt "exit 0\\nstdout:\\nhello, world\\n"
mk other/want.txt "fault: List.get: index 4 outside the list\\n"
fails=0
t() {
  local want=$1 why=$2 diff=$3
  got=$(printf '%b' "$diff" | python3 "$here/golden-rt-strings.py" "$T")
  if [ "$got" = "$want" ]; then echo "PASS $why"; else echo "FAIL $why: got '$got', want '$want'"; fails=$((fails + 1)); fi
}
# #540's diff (671edf90b), the added line's format split at its verbs
t put_fault,put_go "a Sprintf format's pieces, want.txt and want_go.txt" \
  '--- a/std/rt/list.go\n+++ b/std/rt/list.go\n-\t\tFaultHere("BytesBuilder.put: the range is outside the builder")\n+\t\tFaultHere(fmt.Sprintf("BytesBuilder.put: %d..%d outside the builder'"'"'s %d bytes", at, n, len(b.b)))\n'
t other "a removed literal a golden still holds" \
  '-\tFaultHere("List.get: index %d outside the list")\n'
t "" "pieces under 8 characters select nothing" \
  '+\tx := fmt.Sprintf("%d: %s, %v", a, b, c)\n'
t "" "a change with no string" \
  '+\tn := len(b.b) + 1\n'
t "" "context lines are not read (diff -U0 has none; a space line is skipped)" \
  ' \tFaultHere("hello, world and more")\n'
t hello "a raw string literal" \
  '+\tprint(`hello, world`)\n'
# scripts/gate.sh's rt_string_goldens: a std/rt change in a branch (and
# in its uncommitted files) selects the goldens holding its strings
fn=$(sed -n '/^rt_string_goldens() {$/,/^}$/p' "$here/gate.sh")
if [ -z "$fn" ]; then echo "FAIL scripts/gate.sh has no rt_string_goldens"; fails=$((fails + 1)); else
  eval "$fn"
  R=$T/repo
  mkdir -p "$R/scripts" "$R/std/rt" "$R/internal/llvm/testdata"
  cp "$here/golden-rt-strings.py" "$R/scripts/"
  cp -R "$T/put_fault" "$T/other" "$R/internal/llvm/testdata/" 2>/dev/null
  mkdir -p "$R/internal/llvm/testdata/golden" && mv "$R/internal/llvm/testdata/put_fault" "$R/internal/llvm/testdata/other" "$R/internal/llvm/testdata/golden/"
  printf 'package rt\n\nfunc f() { FaultHere("BytesBuilder.put: the range is outside the builder") }\n' >"$R/std/rt/list.go"
  ( cd "$R" && git init -q && git add -A && git -c user.name=t -c user.email=t@t commit -qm base )
  printf 'package rt\n\nfunc f() { FaultHere(fmt.Sprintf("BytesBuilder.put: %%d..%%d outside the builder%ss %%d bytes", 1, 2, 3)) }\n' "'" >"$R/std/rt/list.go"
  got=$(cd "$R" && XO_GATE_BASE=$(git rev-parse HEAD) rt_string_goldens)
  if [ "$got" = put_fault ]; then echo "PASS gate.sh rt_string_goldens on an uncommitted std/rt change"; else echo "FAIL gate.sh rt_string_goldens: got '$got', want 'put_fault'"; fails=$((fails + 1)); fi
  ( cd "$R" && git -c user.name=t -c user.email=t@t commit -qam change )
  got=$(cd "$R" && XO_GATE_BASE=$(git rev-parse HEAD^) rt_string_goldens)
  if [ "$got" = put_fault ]; then echo "PASS gate.sh rt_string_goldens on a committed std/rt change"; else echo "FAIL gate.sh rt_string_goldens committed: got '$got', want 'put_fault'"; fails=$((fails + 1)); fi
fi
if [ "$fails" -gt 0 ]; then echo "golden-rt-strings_test: $fails failed"; exit 1; fi
echo "golden-rt-strings_test: ok"
