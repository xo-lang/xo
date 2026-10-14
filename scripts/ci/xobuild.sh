#!/bin/bash
# scripts/ci/xobuild.sh: builds the checkout's xo on the runner with
# scripts/xobuild from the newest seed (not CI's cross built xo.exe) and
# prints its version: the check that a host can build xo itself.
#   gh workflow run ci.yml -f ref=BRANCH -f suite=script -f runner=xo-windows -f packages=scripts/ci/xobuild.sh
set -euo pipefail
unset XO_CLI
rm -f bin/xo bin/xo.exe bin/xo.key
t0=$(date +%s)
xo=$(go run ./scripts/xobuild)
echo "built $xo in $(( $(date +%s) - t0 )) s"
"$xo" version
