#!/bin/sh
# Nightly full test run with high random seeds. Writes one log per run to
# bench/results/nightly/YYYY-MM-DD.log and appends a one line summary to
# bench/results/nightly/summary.txt. Exit status is the test status.
#
# Usage: bench/nightly.sh            (from anywhere in the repo)
# Scheduling (not installed by default), for example with cron:
#   0 3 * * * cd /path/to/xo && bench/nightly.sh >/dev/null 2>&1
set -u
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root" || exit 1
out="$root/bench/results/nightly"
mkdir -p "$out"
day=$(date +%Y-%m-%d)
log="$out/$day.log"
start=$(date +%s)
{
	echo "xo nightly $day, commit $(git rev-parse --short HEAD)"
	echo "load: $(uptime)"
	# what the default full suite leaves out (decision 0111): more random
	# programs, every build through the toolchain (no binary cache), and
	# the sanitizers' 1 s sleep at exit
	XO_RANGE_SEEDS=${XO_RANGE_SEEDS:-25} XO_LIST_SEEDS=${XO_LIST_SEEDS:-40} \
		XO_COMPILE_PROPS=${XO_COMPILE_PROPS:-500} XO_HYBRID_SEEDS=${XO_HYBRID_SEEDS:-24} \
		XO_BIN_CACHE=${XO_BIN_CACHE:-off} \
		GORACE=${GORACE:-atexit_sleep_ms=1000} XO_TSAN_ATEXIT_SLEEP=${XO_TSAN_ATEXIT_SLEEP:-1000} \
		go test -count=1 -timeout 60m ./...
	s1=$?
	go test -count=1 -race -timeout 30m ./std/rt
	s2=$?
	go vet ./...
	s3=$?
	echo "status: test=$s1 race=$s2 vet=$s3"
} >"$log" 2>&1
status=$(tail -1 "$log")
secs=$(($(date +%s) - start))
echo "$day $(git rev-parse --short HEAD) ${secs}s $status" >>"$out/summary.txt"
case "$status" in
*"test=0 race=0 vet=0"*) exit 0 ;;
*) exit 1 ;;
esac
