#!/bin/bash
# intel-mac-test.sh: run a test command for one commit on the owner's
# Intel Mac (darwin/amd64), fetch the log, and clean up. Decision 0196:
# the LLVM backend must pass the golden suite on darwin/amd64 before the
# Go compiler is deleted, and this Mac (Apple silicon, no Rosetta) cannot
# run x86_64 binaries.
#
# Usage: scripts/intel-mac-test.sh [--clean] [--purge] [COMMIT] [-- CMD...]
#   COMMIT defaults to HEAD. CMD (default: the LLVM golden suite,
#   go test -count=1 -run 'TestGolden$' ./internal/llvm) runs in the
#   checkout with go on PATH, under caffeinate -i. The log lands in
#   $XO_INTEL_OUT (default bench/results/remote/intel-<commit>.log, not
#   committed by this script); the script exits with CMD's status.
#   --clean  afterwards removes the checkout and every cache this script
#            made (keeps only the Go toolchain in ~/xo-ci/go)
#   --purge  afterwards removes ~/xo-ci entirely
#
# Host: $XO_INTEL_HOST (default user@intel-mac.local), key auth.
# The laptop is the owner's: everything lives under ~/xo-ci (the Go
# toolchain from go.dev in ~/xo-ci/go, the checkout in ~/xo-ci/xo, and
# HOME, GOCACHE, GOPATH, and TMPDIR pointed inside ~/xo-ci/state, so
# os.UserCacheDir and every xo cache stay there); no sudo, no installs
# elsewhere. GOMAXPROCS is 6 (of 12 threads) so the laptop stays usable.
# The script refuses to start on battery below $XO_INTEL_MIN_BATT
# percent (default 50). The command runs in the foreground of the ssh
# session: when this script or the connection dies, it dies too, so
# nothing is left running.
set -euo pipefail

HOST=${XO_INTEL_HOST:-user@intel-mac.local}
GOVER=${XO_INTEL_GO:-1.26.1}
MINBATT=${XO_INTEL_MIN_BATT:-50}
CLEAN=""
while [ $# -gt 0 ]; do
  case "$1" in
    --clean) CLEAN=clean; shift;;
    --purge) CLEAN=purge; shift;;
    --) break;;
    *) break;;
  esac
done
rev=HEAD
if [ $# -gt 0 ] && [ "$1" != "--" ]; then rev=$1; shift; fi
[ "${1:-}" = "--" ] && shift
if [ $# -gt 0 ]; then CMD="$*"; else CMD="go test -count=1 -run 'TestGolden\$' ./internal/llvm"; fi

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
commit=$(git rev-parse --short "$rev")
full=$(git rev-parse "$rev")
out=${XO_INTEL_OUT:-$root/bench/results/remote/intel-$commit.log}
mkdir -p "$(dirname "$out")"
log() { echo "intel-mac-test: $*" >&2; }
ssh_() { ssh -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=60 -o ServerAliveCountMax=5 "$HOST" "$@"; }

batt=$(ssh_ pmset -g batt)
pct=$(echo "$batt" | grep -o '[0-9]*%' | head -1 | tr -d %)
if echo "$batt" | grep -q "Battery Power" && [ "${pct:-0}" -lt "$MINBATT" ]; then
  log "on battery at ${pct}% (below ${MINBATT}%): plug the laptop in first"
  exit 2
fi
log "battery: ${pct}%, $(echo "$batt" | grep -o "'.*'")"

ssh_ "test -x ~/xo-ci/go/bin/go || { mkdir -p ~/xo-ci && curl -fsSL https://go.dev/dl/go$GOVER.darwin-amd64.tar.gz | tar -C ~/xo-ci -xz; }"

bundle=$(mktemp -t xo-intel).bundle
# a bundle needs a ref, not a bare commit: a temporary one, removed after
git update-ref "refs/xo-intel/$commit" "$full"
git bundle create "$bundle" "refs/xo-intel/$commit" >/dev/null 2>&1
git update-ref -d "refs/xo-intel/$commit"
scp -o BatchMode=yes -q "$bundle" "$HOST:xo-ci/xo.bundle"
rm -f "$bundle"

remote_log="xo-ci/logs/$commit.log"
cleanup() {
  case "$CLEAN" in
    clean) ssh_ "chmod -R u+w ~/xo-ci/state 2>/dev/null; rm -rf ~/xo-ci/xo ~/xo-ci/state ~/xo-ci/logs ~/xo-ci/xo.bundle" || true;;
    purge) ssh_ "chmod -R u+w ~/xo-ci 2>/dev/null; rm -rf ~/xo-ci" || true;;
    *) ssh_ "rm -f ~/xo-ci/xo.bundle" || true;;
  esac
  [ -n "$CLEAN" ] && log "cleaned ($CLEAN)"
  ssh_ "du -sh ~/xo-ci 2>/dev/null; df -h ~ | tail -1" >&2 || true
}
trap cleanup EXIT

# the command is passed base64 encoded so its quoting survives ssh
cmd64=$(printf '%s' "$CMD" | base64 | tr -d '\n')
log "running on $commit: $CMD"
set +e
ssh_ "bash -s" <<EOF
set -e
X=\$HOME/xo-ci
mkdir -p \$X/logs \$X/state/home \$X/state/tmp \$X/state/gocache \$X/state/gopath
[ -d \$X/xo/.git ] || git init -q \$X/xo
git -C \$X/xo fetch -q \$X/xo.bundle '+refs/xo-intel/*:refs/xo-intel/*'
git -C \$X/xo checkout -q -f $full
cd \$X/xo
export HOME=\$X/state/home TMPDIR=\$X/state/tmp/ GOCACHE=\$X/state/gocache GOPATH=\$X/state/gopath
export PATH=\$X/go/bin:\$PATH GOMAXPROCS=6 GOTOOLCHAIN=local GOFLAGS=
cmd=\$(echo $cmd64 | base64 -D)
{ echo "# intel-mac-test $commit \$(date -u +%FT%TZ) \$(sw_vers -productVersion) \$(uname -m) \$(clang --version | head -1)"; echo "# \$cmd"; } > \$X/logs/$commit.log
set +e
caffeinate -i bash -c "\$cmd" >> \$X/logs/$commit.log 2>&1
rc=\$?
echo "# exit \$rc \$(date -u +%FT%TZ)" >> \$X/logs/$commit.log
exit \$rc
EOF
rc=$?
set -e
scp -o BatchMode=yes -q "$HOST:$remote_log" "$out" || log "could not fetch the log"
log "exit $rc; log in $out"
exit $rc
