#!/bin/bash
# usego-workflows.sh XO OUTDIR: builds and runs each `use go` workflow in
# bench/usego-workflows (one program per library: zap, chi, yaml, toml,
# pgx, aws) and prints one line per workflow, PASS, FAIL (with the first
# failing step or the build's first error) or SKIP, with the Go modules and
# versions the build used and the Go version. Each workflow builds in its
# own copy under OUTDIR (nothing is written into the checkout). PGX_URL, if
# set, is passed to pgx (the Docker tier starts a postgres container).
# Exit 1 when a workflow fails that bench/usego-workflows/EXPECTED does not
# list (those print XFAIL, or XPASS once they pass).
set -u
XO=${1:?usage: usego-workflows.sh XO OUTDIR}
OUT=${2:?usage: usego-workflows.sh XO OUTDIR}
here=$(cd "$(dirname "$0")/.." && pwd)
src=$here/bench/usego-workflows
mkdir -p "$OUT"
echo "go: $(go version)"
echo "xo: $("$XO" version)"
rc=0
# NAME -> reason, from EXPECTED: a failure there is XFAIL, a pass XPASS
expected() { awk -v n="$1" '$1 == n { $1 = ""; sub(/^ /, ""); print }' "$src/EXPECTED" 2>/dev/null; }
report() { # STATUS NAME TEXT
	local why
	why=$(expected "$2")
	if [ -n "$why" ]; then
		[ "$1" = FAIL ] && { echo "XFAIL $2 ($why): $3"; return 0; }
		[ "$1" = PASS ] && { echo "XPASS $2 (remove from EXPECTED): $3"; return 0; }
	fi
	echo "$1 $2 $3"
	[ "$1" = FAIL ] && rc=1
	return 0
}
for d in "$src"/*/; do
	name=$(basename "$d")
	w=$OUT/$name
	rm -rf "${w:?}"
	cp -R "$d" "$w"
	# pins the modules a workflow's xo.mod leaves out (XO1319's fix) in the
	# copy; a pinned module's hash needs one binding first (XO1320,
	# decision 0356): tidy, check, tidy
	(cd "$w" && { "$XO" mod tidy; "$XO" check .; "$XO" mod tidy; } >"$w/tidy.log" 2>&1)
	mods=$(awk '$1 == "require" && $2 == "go" { printf "%s@%s ", $3, $4 }' "$w/xo.mod")
	if ! (cd "$w" && "$XO" build --backend=go -o "$w/prog" . >"$w/build.log" 2>&1); then
		first=$(grep -m1 -E 'XO[0-9]{4}|error|failed' "$w/build.log" | cut -c1-200)
		report FAIL "$name" "(build) $mods: $first"
		continue
	fi
	out=$(cd "$w" && OUT_DIR="$w" "$w/prog" 2>"$w/run.err")
	code=$?
	echo "$out" >"$w/run.log"
	last=$(echo "$out" | tail -1)
	case "$last" in
	"PASS $name") report PASS "$name" "$mods" ;;
	"SKIP $name"*) echo "$last $mods" ;;
	*)
		step=$(echo "$out" | grep -m1 '^FAIL ' | cut -c1-200)
		[ -z "$step" ] && step="exit $code: $(head -c 200 "$w/run.err")"
		report FAIL "$name" "(run) $mods: $step"
		;;
	esac
done
exit $rc
