#!/bin/bash
# Tests scripts/bisect.sh on a throwaway repository, with xobuild replaced
# (XO_BISECT_XOBUILD=true) and a shell command as the test: the first bad
# commit is found; a GOOD that fails or a BAD that passes stops the run
# (exit 3) before bisecting; --skip-verify bisects anyway; adjacent GOOD
# and BAD. Run it after
# changing bisect.sh (no xo is built; a few seconds).
set -u -o pipefail
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/bisect-test.XXXXXX") || exit 1
trap 'rm -rf "${tmp:?}"' EXIT
repo=$tmp/repo
mkdir -p "$repo/scripts"
cp "$here/bisect.sh" "$repo/scripts/bisect.sh"
cd "$repo" || exit 1
git init -q .
git config user.email t@example.com
git config user.name t
git add scripts
git commit -q -m c0
# c1..c6; the file `broken` appears at c4 and stays
for i in 1 2 3 4 5 6; do
	echo "$i" >n
	if [ "$i" = 4 ]; then echo x >broken; fi
	git add -A
	git commit -q -m "c$i"
done
c() { git log --format=%H --grep="^c$1\$"; }
fails=0
check() { # name want_exit want_text got_exit got_out
	if [ "$4" != "$2" ] || ! grep -qF -- "$3" <<<"$5"; then
		echo "FAIL $1: exit $4 (want $2), want text '$3' in:"
		sed 's/^/  /' <<<"$5"
		fails=$((fails + 1))
	else
		echo "ok   $1"
	fi
}
export XO_BISECT_XOBUILD=true
t='test ! -e broken'
out=$(scripts/bisect.sh "$t" "$(c 1)" "$(c 6)" 2>&1); rc=$?
check finds-first-bad 0 "first bad commit: $(git rev-parse --short "$(c 4)")" $rc "$out"
out=$(scripts/bisect.sh "$t" "$(c 5)" "$(c 6)" 2>&1); rc=$?
check good-fails 3 "GOOD $(c 5 | cut -c1-9) fails (exit 1), so it is not a good commit" $rc "$out"
out=$(scripts/bisect.sh "$t" "$(c 1)" "$(c 3)" 2>&1); rc=$?
check bad-passes 3 "BAD $(c 3 | cut -c1-9) passes, so it is not a bad commit" $rc "$out"
out=$(scripts/bisect.sh 'exit 125' "$(c 1)" "$(c 6)" 2>&1); rc=$?
check good-skipped 3 "GOOD $(c 1 | cut -c1-9) was skipped" $rc "$out"
# the invalid bisect the check prevents: every step bad, the commit
# right after GOOD named
out=$(scripts/bisect.sh --skip-verify "$t" "$(c 4)" "$(c 6)" 2>&1); rc=$?
check skip-verify 0 "first bad commit: $(git rev-parse --short "$(c 5)")" $rc "$out"
# adjacent GOOD and BAD: the step runs in BAD's tree, not an empty one
out=$(scripts/bisect.sh "$t" "$(c 3)" "$(c 4)" 2>&1); rc=$?
check adjacent 0 "first bad commit: $(git rev-parse --short "$(c 4)")" $rc "$out"
if [ "$fails" != 0 ]; then
	echo "bisect_test.sh: $fails failed"
	exit 1
fi
echo "bisect_test.sh: all passed"
