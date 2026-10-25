#!/usr/bin/env bash
# cache-churn.sh: the back-end cache's store churn by hour and by 24 hours,
# from DIR/store-log and DIR/prune-log (decision 0365's question: how much of
# what is stored is stored by ids that are gone). Read only.
#
#   scripts/cache-churn.sh [--dir DIR] [--since UNIX] [--live-min N]
#
# DIR is the back end cache (default: <user cache>/xo/backend, the Mac's
# ~/Library/Caches/xo/backend; XO_BACKEND_CACHE_DIR overrides). --since is
# the first second counted (default: 1792889100, 2026-10-24 about 20:45
# local, when the churn measurement for the Sunday review began). An id is
# LIVE when it stored something within the last N minutes (default 60) of
# the report time; an id that stored nothing since is retired, and its
# bytes are "retired" bytes: stored by a code generation key no xo is
# still using to build.
#
# Prints:
#   - one row an hour (UTC hour start): GB stored, GB pruned, distinct ids
#     that stored, GB stored by retired ids and their share of the hour;
#   - the same for the last 24 hours and for the whole period since --since.
# Bytes pruned are the prune-log's "bytes before" less "after" of each
# walk or slice line (a duplicate line, two prunes logging at one second
# with equal figures, counts once). Stored bytes are the store-log's BYTES.
set -euo pipefail
dir=${XO_BACKEND_CACHE_DIR:-}
since=1792889100
livemin=60
while [ $# -gt 0 ]; do
	case $1 in
	--dir) dir=$2; shift 2 ;;
	--since) since=$2; shift 2 ;;
	--live-min) livemin=$2; shift 2 ;;
	-h|--help) sed -n '2,24p' "$0"; exit 0 ;;
	*) echo "cache-churn.sh: unknown argument $1" >&2; exit 2 ;;
	esac
done
if [ -z "$dir" ]; then
	case $(uname) in
	Darwin) dir=$HOME/Library/Caches/xo/backend ;;
	*) dir=${XDG_CACHE_HOME:-$HOME/.cache}/xo/backend ;;
	esac
fi
[ -f "$dir/store-log" ] || { echo "cache-churn.sh: no $dir/store-log" >&2; exit 1; }
now=$(date +%s)
plog=$dir/prune-log
[ -f "$plog" ] || plog=/dev/null

# pass 1 over the store-log: each id's last store; pass 2: the buckets
awk -v since="$since" -v now="$now" -v live="$livemin" -v plog="$plog" '
function gb(x) { return sprintf("%.2f", x / 1073741824) }
function pct(a, b) { return b > 0 ? sprintf("%.0f%%", 100 * a / b) : "-" }
function row(label, st, pr, nid, ret) {
	printf "%-20s %9s %9s %6d %9s %6s\n", label, gb(st), gb(pr), nid, gb(ret), pct(ret, st)
}
FNR == NR {
	if ($2 == "store" && $1 >= since) {
		last[$6] = $1
	}
	next
}
$2 == "store" && $1 >= since {
	h = int($1 / 3600)
	st[h] += $4
	if (!(h SUBSEP $6 in seen)) { seen[h SUBSEP $6] = 1; nid[h]++ }
	retired = (last[$6] < now - live * 60)
	if (retired) ret[h] += $4
	if (!($6 in allid)) { allid[$6] = 1; nall++ }
	tot += $4
	if (retired) totret += $4
	if ($1 >= now - 86400) {
		t24 += $4
		if (retired) t24ret += $4
		if (!($6 in id24)) { id24[$6] = 1; n24++ }
	}
	if (minh == 0 || h < minh) minh = h
	if (h > maxh) maxh = h
}
END {
	# the prune-log: bytes before less after of each walk or slice line
	prev = ""
	while ((getline line < plog) > 0) {
		if (line == prev) continue
		prev = line
		n = split(line, f, " ")
		if (f[2] != "walk" && f[2] != "slice") continue
		if (f[1] < since) continue
		b = 0; a = 0
		if (match(line, /[0-9]+ bytes before/)) b = substr(line, RSTART, RLENGTH - 13) + 0
		if (match(line, /[0-9]+ after/)) a = substr(line, RSTART, RLENGTH - 6) + 0
		if (b > a) {
			p = b - a
			pr[int(f[1] / 3600)] += p
			ptot += p
			if (f[1] >= now - 86400) p24 += p
		}
	}
	printf "%-20s %9s %9s %6s %9s %6s\n", "hour (UTC)", "GB stored", "GB pruned", "ids", "GB retired", "share"
	for (h = minh; h <= maxh; h++) {
		cmd = "date -u -r " (h * 3600) " +\"%Y-%m-%d %H:00\""
		cmd | getline label
		close(cmd)
		row(label, st[h], pr[h], nid[h], ret[h])
	}
	print ""
	row("last 24 hours", t24, p24, n24, t24ret)
	cmd = "date -u -r " since " +\"%Y-%m-%d %H:%M\""
	cmd | getline sl
	close(cmd)
	row("since " sl " UTC", tot, ptot, nall, totret)
	printf "(an id is retired when it stored nothing in the last %d minutes before this report; share is retired bytes of stored bytes; the newest hours read low, an id still building is not retired yet)\n", live
}
' "$dir/store-log" "$dir/store-log"
