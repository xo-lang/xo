#!/bin/sh
# golden-run.sh: run a built golden program as internal/arm64's
# TestGoldenRun does (args.txt, env.txt, stdin.txt of its directory) and
# compare its exit status and standard output with the part of want.txt
# before `--- stderr` (standard error is printed on a mismatch, not
# compared: the tests normalize it). For a binary built elsewhere, run
# where the tests cannot (scripts/docker-linux.sh).
#
#   scripts/golden-run.sh BINARY GOLDEN_DIR
#
# Prints `ok NAME` or `FAIL NAME` and the difference; exits 1 on FAIL.
set -u
bin=$1
dir=$2
name=$(basename "$dir")
tmp=$(mktemp -d)
args=""
[ -f "$dir/args.txt" ] && args=$(cat "$dir/args.txt")
envs=""
[ -f "$dir/env.txt" ] && envs=$(cat "$dir/env.txt")
stdin=/dev/null
[ -f "$dir/stdin.txt" ] && stdin="$dir/stdin.txt"
# shellcheck disable=SC2086
# a writable copy under the golden's own name (the checkout may be
# mounted read only, some write files where they run, and fs_cwd_path
# checks its directory's name)
mkdir "$tmp/run"
cp -R "$dir" "$tmp/run/$name"
# a hung program (a deadlock, a server never stopped) ends after
# $XO_GOLDEN_TIMEOUT seconds, 600 by default, as exit 124, where coreutils'
# timeout exists (Linux; macOS has none)
limit=""
if command -v timeout >/dev/null 2>&1; then
  limit="timeout ${XO_GOLDEN_TIMEOUT:-600}"
fi
# shellcheck disable=SC2086
(cd "$tmp/run/$name" && env $envs $limit "$bin" $args <"$stdin" >"$tmp/out" 2>"$tmp/err")
code=$?
{
  printf 'exit %s\n--- stdout\n' "$code"
  cat "$tmp/out"
} >"$tmp/got"
want=$dir/want.txt
# on Windows (Git Bash), a golden whose output differs there has want_windows.txt
case $(uname -s) in MINGW* | MSYS* | CYGWIN*) [ -f "$dir/want_windows.txt" ] && want=$dir/want_windows.txt ;; esac
sed '/^--- stderr$/,$d' "$want" >"$tmp/want"
if cmp -s "$tmp/got" "$tmp/want"; then
  echo "ok $name"
  rm -rf "$tmp"
  exit 0
fi
echo "FAIL $name"
diff "$tmp/want" "$tmp/got" | head -20
echo "--- stderr"
head -20 "$tmp/err"
rm -rf "$tmp"
exit 1
