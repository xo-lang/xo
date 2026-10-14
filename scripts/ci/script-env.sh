#!/bin/bash
# scripts/ci/script-env.sh: smoke test of `suite=script` (scripts/ci/suite.sh):
# prints the environment the suite gives a script and writes one file to
# $XO_CI_OUT, which the workflow uploads as the artifact script-out.
#   gh workflow run ci.yml -f suite=script -f packages="scripts/ci/script-env.sh a b"
set -euo pipefail
echo "args: $*"
for v in XO_CLI XO_CI_STD XO_CI_BASE_STD XO_CI_OUT; do echo "$v=${!v}"; done
"$XO_CLI" version
test -d "$XO_CI_STD" && test -d "$XO_CI_BASE_STD"
echo "script-env ok $*" > "$XO_CI_OUT/script-env.txt"
