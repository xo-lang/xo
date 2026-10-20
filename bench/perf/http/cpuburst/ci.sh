#!/bin/bash
# bench/perf/http/cpuburst/ci.sh [ROUNDS] [LOOPS...]: run.sh on a CI
# runner (suite=script) with the checkout's xo, into $XO_CI_OUT.
set -u
here=$(cd "$(dirname "$0")" && pwd)
rounds=${1:-3}
shift || true
XO=$XO_CLI exec "$here/run.sh" "${XO_CI_OUT:-$PWD/cpuburst-out}" "$rounds" "${@:-0 8 16}"
