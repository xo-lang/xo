#!/bin/bash
# checkall.sh XO OUT: every error `xo check` reports over the repository's
# Xo programs, test files included, without building or running any:
# test/..., cmd/xo/testdata/..., every internal/*/testdata/... (the
# goldens, the stdtest corpora, the check tests' programs), and each std
# module with tests of its own by `xo vet --json --cost --std-module=M`
# (std is checked only as a program's dependency or that way; its
# check_errors and error diagnostics count). OUT gets one line an error,
# sorted: FILE<TAB>CODE<TAB>MESSAGE (no line, so an edit above it does
# not make it new). Some testdata programs hold errors on purpose, so the
# gate compares two OUTs (the merge base's xo and the change's: gate.sh
# prove_checkall) and fails on errors only the change's xo reports
# (lead, 2026-11-15, #54: decision 0662's checker change passed its gate
# and broke std/sql's corpus tests, cmd/xo TestCheckFlag's program and
# the native_sqlite golden, none of which its gate checked). An error
# its line expects (`// ERROR XOnnnn ...`, the errorcheck tests'
# annotation, which their own test checks) is not listed, so a new
# errorcheck test is not a new error (Xo Lang 3, 2026-11-16).
# Run from the repository root.
set -u
xo=$1 out=$2
tmp=$(mktemp -d "${TMPDIR:-/tmp}/xo-checkall.XXXXXX")
trap 'rm -rf "${tmp:?}"' EXIT
roots=$(
  for d in test cmd/xo/testdata internal/*/testdata; do
    [ -d "$d" ] && [ -n "$(find "$d" -name '*.xo' -print -quit)" ] && echo "$d"
  done
)
i=0
for d in $roots; do
  i=$((i + 1))
  "$xo" check --json "$d/..." >"$tmp/c$i.json" 2>"$tmp/c$i.err"
  # exit 2 is a usage error (no JSON): an error line of its own
  [ $? = 2 ] && printf '%s\t%s\t%s\n' "$d" usage "$(head -1 "$tmp/c$i.err")" >>"$tmp/usage.txt"
done
mods=$(git ls-files 'std/*_test.xo' 'std/*/*_test.xo' | grep -v '^std/native/' |
  sed -E 's|^std/||; s|/.*$||; s|_test\.xo$||' | sort -u)
for m in $mods; do
  "$xo" vet --json --cost --std-module="$m" >"$tmp/v-$m.json" 2>"$tmp/v-$m.err"
  [ $? = 2 ] && printf 'std/%s\t%s\t%s\n' "$m" usage "$(head -1 "$tmp/v-$m.err")" >>"$tmp/usage.txt"
done
python3 - "$tmp" >"$out.tmp" <<'EOF'
import glob, json, os, re, sys
tmp = sys.argv[1]
rows = []
lines = {}
def expected(sp, code):
    # the diagnostic's line carries `// ERROR` naming its code
    f, n = sp.get("file", ""), sp.get("line", 0)
    if not f or not n or not code:
        return False
    if f not in lines:
        try:
            with open(f, encoding="utf-8", errors="replace") as fh:
                lines[f] = fh.read().split("\n")
        except OSError:
            lines[f] = []
    ls = lines[f]
    if n > len(ls):
        return False
    m = re.search(r"//\s*ERROR\b(.*)$", ls[n - 1])
    return bool(m) and code in m.group(1).split()
for p in sorted(glob.glob(os.path.join(tmp, "*.json"))):
    try:
        with open(p) as f:
            res = json.load(f)
    except (OSError, ValueError):
        continue  # no JSON: a usage error, recorded by the shell
    name = os.path.basename(p)
    if name.startswith("v-") and res.get("check_errors") and not any(
            (d.get("severity") or "error") == "error" for d in res.get("diagnostics") or []):
        rows.append("std/%s\tcheck_errors\t%d" % (name[2:-5], res["check_errors"]))
    for d in res.get("diagnostics") or []:
        if (d.get("severity") or "error") != "error":
            continue
        sp = d.get("span") or {}
        if expected(sp, d.get("code", "")):
            continue
        msg = " ".join((d.get("message") or "").split())
        rows.append("%s\t%s\t%s" % (sp.get("file", ""), d.get("code", ""), msg))
try:
    with open(os.path.join(tmp, "usage.txt")) as f:
        rows += [l.rstrip("\n") for l in f if l.strip()]
except OSError:
    pass
for r in sorted(rows):
    print(r)
EOF
mv "$out.tmp" "$out"
echo "checkall: $(echo "$roots" | wc -w | tr -d ' ') roots, $(echo "$mods" | wc -w | tr -d ' ') std modules, $(wc -l <"$out" | tr -d ' ') errors"
