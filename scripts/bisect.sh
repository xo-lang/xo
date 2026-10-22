#!/bin/bash
# Bisects a failure over commits: `git bisect run` from GOOD (passes) to
# BAD (fails), each step's xo built by this checkout's xobuild (so with
# its seed choice: the newest seed or cached build of an ancestor of the
# step, internal/xotool FetchSeedFor, whatever the step's own xobuild
# did), the test then run in the step's tree with that xo
# (XO_CLI_STALE_OK=1). Prints the first bad commit and its author.
#
# usage: scripts/bisect.sh [--pkg=PKG] [--timeout=D] [--keep] TEST GOOD BAD
#
#   TEST  a go test -run pattern (TestName, or TestA|TestB): run as
#         `go test -count=1 -run '^(TEST)$' PKG` (a pattern with a / for
#         subtests, TestGolden/name, as given), PKG the package of the
#         first name at BAD unless --pkg is given; a step where the test
#         does not exist yet ("no tests to run") is skipped. Anything else
#         (it has a space) is a shell command run in the step's tree with
#         XO_CLI set: exit 0 good, 125 skip, any other bad.
#   GOOD, BAD  commits: the test passes at GOOD and fails at BAD.
#
# Every cache xo writes is private (back-end cache, build directory, Go
# cache, test binaries) and deleted at the end, so an old xo's prune never
# touches the shared caches (CLAUDE.md, owner 2026-10-21; gotchas/
# xobuild-seed-newer-than-commit.md); the
# step builds of xo go to the shared checkout cache (by source key,
# reused by later runs). A step whose xo does not build is skipped.
# Heavy: run it as HEAVY_CLASS=bulk ~/xo-lead/agents/heavy.sh NAME
# scripts/bisect.sh ... (each step builds xo and runs the test).
# Work in ${XO_BISECT_WORK:-$TMPDIR/xo-bisect.PID}; --keep keeps it.
set -u -o pipefail
pkg=""
timeout=30m
keep=0
while [ $# -gt 0 ]; do
	case "$1" in
	--pkg=*) pkg=${1#--pkg=} ;;
	--timeout=*) timeout=${1#--timeout=} ;;
	--keep) keep=1 ;;
	--) shift; break ;;
	-*) echo "bisect.sh: unknown flag $1" >&2; exit 2 ;;
	*) break ;;
	esac
	shift
done
if [ $# -ne 3 ]; then
	echo "usage: scripts/bisect.sh [--pkg=PKG] [--timeout=D] [--keep] TEST GOOD BAD" >&2
	exit 2
fi
test_arg=$1
good=$2
bad=$3
root=$(cd "$(dirname "$0")/.." && pwd)
for c in "$good" "$bad"; do
	if ! git -C "$root" rev-parse -q --verify "$c^{commit}" >/dev/null; then
		echo "bisect.sh: unknown commit $c (git fetch origin first?)" >&2
		exit 2
	fi
done
good=$(git -C "$root" rev-parse "$good")
bad=$(git -C "$root" rev-parse "$bad")
if ! git -C "$root" merge-base --is-ancestor "$good" "$bad"; then
	echo "bisect.sh: $good is not an ancestor of $bad" >&2
	exit 2
fi
tmp=${TMPDIR:-/tmp}
work=${XO_BISECT_WORK:-${tmp%/}/xo-bisect.$$}
work=${work%/}
mkdir -p "$work" || exit 1
cleanup() {
	if [ "$keep" = 1 ]; then
		echo "bisect.sh: kept $work" >&2
	else
		rm -rf "${work:?}"
	fi
}
trap cleanup EXIT
tree=$work/tree
# a shared clone: the step checkouts never touch this checkout or its
# worktrees
git clone -q --shared --no-checkout "$root" "$tree" || exit 1
# this checkout's xobuild, built once
(cd "$root" && go build -o "$work/xobuild" ./scripts/xobuild) || exit 1
mode=cmd
run_pat="^($test_arg)\$"
if [[ "$test_arg" != *" "* ]] && [[ "$test_arg" =~ ^Test[^[:space:]]*$ ]]; then
	mode=gotest
	# a pattern with subtests (TestGolden/name) is go test's own
	case "$test_arg" in */*) run_pat=$test_arg ;; esac
	if [ -z "$pkg" ]; then
		first=${test_arg%%|*}
		first=${first%%/*}
		f=$(git -C "$root" grep -l -E "^func ${first}\(" "$bad" -- '*_test.go' | head -1)
		if [ -z "$f" ]; then
			echo "bisect.sh: no func ${first} in a _test.go file at $bad (use --pkg=)" >&2
			exit 2
		fi
		f=${f#"$bad":}
		pkg=./$(dirname "$f")
	fi
fi
cat >"$work/step.sh" <<EOF
#!/bin/bash
set -u -o pipefail
work='$work'
cd "\$work/tree" || exit 128
c=\$(git rev-parse --short=9 HEAD)
for d in backend build gocache; do rm -rf "\$work/cache/\$d"; mkdir -p "\$work/cache/\$d"; done
export XO_BACKEND_CACHE_DIR="\$work/cache/backend" XO_BUILD_DIR="\$work/cache/build" XO_GOCACHE="\$work/cache/gocache" XO_BIN_CACHE=off
unset XO_SEED XO_CLI
if ! "\$work/xobuild" >"\$work/build-\$c.log" 2>&1; then
	echo "bisect.sh: \$c: xo does not build (skipped; \$work/build-\$c.log)"
	tail -3 "\$work/build-\$c.log"
	exit 125
fi
export XO_CLI="\$work/tree/bin/xo" XO_CLI_STALE_OK=1
EOF
if [ "$mode" = gotest ]; then
	cat >>"$work/step.sh" <<EOF
go test -count=1 -timeout '$timeout' -run '$run_pat' '$pkg' >"\$work/test-\$c.log" 2>&1
rc=\$?
if grep -q 'no tests to run' "\$work/test-\$c.log" && ! grep -q -- '--- FAIL' "\$work/test-\$c.log"; then
	echo "bisect.sh: \$c: no test matches (skipped)"
	exit 125
fi
EOF
else
	printf '%s\n' "bash -c $(printf '%q' "$test_arg") >\"\$work/test-\$c.log\" 2>&1" 'rc=$?' >>"$work/step.sh"
fi
cat >>"$work/step.sh" <<'EOF'
if [ $rc = 0 ]; then
	echo "bisect.sh: $c: good"
	exit 0
fi
if [ $rc = 125 ]; then
	echo "bisect.sh: $c: skipped by the command"
	exit 125
fi
echo "bisect.sh: $c: bad (exit $rc)"
tail -5 "$work/test-$c.log" | sed 's/^/  /'
exit 1
EOF
chmod +x "$work/step.sh"
echo "bisect.sh: $test_arg ($mode${pkg:+, $pkg}) from good ${good:0:9} to bad ${bad:0:9}, $(git -C "$root" rev-list --count "$good..$bad") commits; work $work"
git -C "$tree" bisect start "$bad" "$good" >/dev/null || exit 1
git -C "$tree" bisect run "$work/step.sh" >"$work/bisect.log" 2>&1
rc=$?
grep -E '^(bisect.sh: |  )' "$work/bisect.log"
first=$(sed -n 's/^\([0-9a-f]\{40\}\) is the first bad commit$/\1/p' "$work/bisect.log" | head -1)
if [ -z "$first" ]; then
	echo "bisect.sh: no single first bad commit (git bisect run exit $rc):" >&2
	tail -20 "$work/bisect.log" >&2
	exit 1
fi
echo "first bad commit: $(git -C "$root" log -1 --format='%h %s' "$first" | cut -c1-160)"
echo "author: $(git -C "$root" log -1 --format='%an <%ae>, %ad' --date=iso "$first")"
