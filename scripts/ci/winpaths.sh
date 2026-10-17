#!/bin/bash
# scripts/ci/winpaths.sh: xo given native Windows paths (C:\x\y), as Go
# tools and PowerShell pass them: check, build with -o and --std, and run
# the result. Run on the Windows runner with the branch's own xo:
#   gh workflow run ci.yml -f ref=BRANCH -f suite=script -f runner=xo-windows -f packages=scripts/ci/winpaths.sh
set -euo pipefail
command -v cygpath >/dev/null || { echo "winpaths.sh: for Windows (Git Bash)"; exit 2; }
w=$(cygpath -w "$PWD")
out=$(cygpath -w "$XO_CI_OUT")
set -x
"$XO_CLI" check "$w\\compiler\\xo"
"$XO_CLI" build --backend=llvm --std="$w\\std" -o "$out\\recursion.exe" "$w\\internal\\llvm\\testdata\\golden\\recursion"
"$XO_CLI" build --backend=llvm --std "$w\\std" -o "$out\\recursion2.exe" "$w\\internal\\llvm\\testdata\\golden\\recursion"
"$out\\recursion.exe" | grep -q 'fib 75025'
"$out\\recursion2.exe" | grep -q 'fib 75025'
echo "winpaths ok"
