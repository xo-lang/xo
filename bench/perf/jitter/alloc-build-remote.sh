#!/bin/bash
# alloc-build-remote.sh MODE SPECS NAME SEED PRIV WORK BUNDLE (the SECOND host, lane pass2, NOT exclusive):
# the build half of alloc-points-remote.sh, alloc-own-remote.sh and alloc-env-remote.sh (Lead, Lang 3,
# 2026-11-02: their builds ran inside host 1's exclusive lease). SPECS is a comma list of REV[+VAR=VALUE...].
# MODE last: every server is built from the LAST rev's bench/perf/jitter/xo (alloc-points); own: from its
# own tree (alloc-own, alloc-env). The VARs apply to that server's xo build (and are carried for its run).
# Writes $WORK/out/NAME.tar: bin/{loadgen,srv-base,srv-pK,env-pK} and revs.txt (the "p$k hash date built" lines
# and "base VERSION"), for alloc-measure-remote.sh on host 1. Static linux/arm64 binaries run on either host.
set -u
mode=$1 a=$2 name=$3 seed=$4 P=$5 work=$6 bundle=$7
W=$work/rvb$name; OUT=$work/out; mkdir -p "$OUT"
t0=$(date +%s)
find "$OUT" -name "*.tar" -mmin +720 -delete 2>/dev/null
L=$P.lockdir boot=$(cat /proc/sys/kernel/random/boot_id 2>/dev/null)
hp= hb=
until mkdir "$L" 2>/dev/null; do
	read -r hp hb < "$L/holder" 2>/dev/null
	if [ -n "$hp" ] && { [ "$hb" != "$boot" ] || ! kill -0 "$hp" 2>/dev/null; }; then
		rm -f "$L/holder"; rmdir "$L" 2>/dev/null
	fi
	hp= hb=; sleep 1
done
echo "$$ $boot" > "$L/holder"
trap 'rm -f "$L/holder"; rmdir "$L" 2>/dev/null' EXIT
cd "$P" || exit 2
git for-each-ref --format='%(refname)' refs/ab 2>/dev/null | while read -r r; do git update-ref -d "$r"; done
rm -rf .git/refs/ab .git/objects/pack/tmp_*
git fetch -q "$bundle" "+refs/heads/*:refs/ab/*" || { echo "alloc-build: the bundle did not fetch"; exit 2; }
rm -f "$bundle" 2>/dev/null
rm -rf "$W"; git worktree prune
specs=(${a//,/ }) revs=() envs=()
for sp in "${specs[@]}"; do revs+=("${sp%%+*}"); e=""; [ "$sp" != "${sp#*+}" ] && e="${sp#*+}"; envs+=("${e//+/ }"); done
for k in "${!revs[@]}"; do git worktree add -q --detach "$W/w$k" "${revs[$k]}" || exit 2; done
rm -f "$L/holder"; rmdir "$L"; trap - EXIT
cleanup() { cd "$work"; for k in "${!revs[@]}"; do git -C "$P" worktree remove --force "$W/w$k" 2>/dev/null; done; rm -rf "$W"; }
trap cleanup EXIT
B=$W/bin; mkdir -p "$B"
echo "host load $(cat /proc/loadavg)"
J=$W/w$((${#revs[@]} - 1))/bench/perf/jitter
( cd "$J/loadgen" && go build -o "$B/loadgen" . ) || exit 1
"$seed" build -o "$B/srv-base" "$J/xo" > "$W/xobuild-base.txt" 2>&1 || { tail -5 "$W/xobuild-base.txt"; exit 1; }
: > "$B/revs.txt"
echo "base $("$seed" version)" >> "$B/revs.txt"
for k in "${!revs[@]}"; do
	T=$W/w$k
	( cd "$T" && XO_SEED=$seed go run ./scripts/xobuild ) > "$W/xobuild$k.txt" 2>&1 || { echo "p$k ${revs[$k]}: xobuild failed: $(tail -3 "$W/xobuild$k.txt" | tr '\n' ' ')"; continue; }
	src=$J/xo; [ "$mode" = own ] && src=$T/bench/perf/jitter/xo
	env ${envs[$k]} "$T/bin/xo" build -o "$B/srv-p$k" "$src" > "$W/srv$k.txt" 2>&1 || { echo "p$k: server build failed: $(tail -3 "$W/srv$k.txt" | tr '\n' ' ')"; continue; }
	line="p$k $(git -C "$T" rev-parse --short HEAD) $(git -C "$T" log -1 --format=%cs) built"
	[ -n "${envs[$k]}" ] && line="$line env [${envs[$k]}]"
	echo "$line"; echo "$line" >> "$B/revs.txt"
	echo "${envs[$k]}" > "$B/env-p$k"
	rm -rf "$T/bin"
done
rm -f "$OUT/$name.tar"
tar -C "$B" -cf "$OUT/$name.tar" . && echo "BUILD-DONE $name $(( $(date +%s) - t0 )) s, $(du -h "$OUT/$name.tar" | cut -f1)"
