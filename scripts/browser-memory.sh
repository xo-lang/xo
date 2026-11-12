#!/bin/bash
# browser-memory.sh: the playground pages' memory budgets for an xo tree
# (decision 0615; owner, 2026-11-12: every release checks /repl/ and
# /play/ memory so they cannot quietly grow).
#
#   scripts/browser-memory.sh [--record LABEL] SITE_REPO
#
# Builds the playground assets from this tree (the website's
# scripts/build-playground.py, with this tree's bin/xo) in a scratch
# worktree of SITE_REPO (the xolang.dev repository, at its origin/main, or
# $BROWSER_MEMORY_SITE_REV: a release's site branch before it is pushed),
# builds the site with hugo ($HUGO, else hugo on PATH), and runs the
# site's scripts/test-memory.mjs in headless Chrome: the REPL compiler's
# peak over the 30-entry session on /repl/ and /play/'s renderer RSS peak
# over 30 runs, against the budgets in the site's data/memory.json.
#
# Each run appends one row to $BROWSER_MEMORY_LOG (default
# bench/results/browser-memory.md in this tree): the label (--record's,
# else the tree's commit), the date, the commit, both peaks and budgets,
# and ok or FAIL. A release records its row with --record vX.Y.Z in the
# version commit; scripts/release-publish.sh refuses a tag whose tree has
# no ok row for it. The lead's 8-hourly tier runs it with
# BROWSER_MEMORY_LOG in its notes, so a growth shows within hours.
#
# Exit 0 within budget, 1 over a budget or a wrong REPL value, 2 when it
# could not run. Run it under heavy.sh (HEAVY_CLASS=bulk HEAVY_DARWIN=1:
# about 6 minutes with the asset build). Raising a budget is a commit to
# the site's data/memory.json that says why, never automatic.
set -u -o pipefail
die() { echo "browser-memory.sh: $*" >&2; exit 2; }
label=""
if [ "${1:-}" = "--record" ]; then label=${2:?--record needs a label}; shift 2; fi
[ $# -eq 1 ] || die "usage: scripts/browser-memory.sh [--record LABEL] SITE_REPO"
site=$1
tree=$(git rev-parse --show-toplevel) || die "not in an xo tree"
rev=$(git -C "$tree" rev-parse --short=9 HEAD)
[ -n "$label" ] || label=$rev
log=${BROWSER_MEMORY_LOG:-$tree/bench/results/browser-memory.md}
hugo=${HUGO:-$(command -v hugo || true)}
[ -x "$hugo" ] || die "no hugo (set HUGO)"
[ -x "$tree/bin/xo" ] || die "no $tree/bin/xo (go run ./scripts/xobuild first)"
git -C "$site" fetch -q origin || die "cannot fetch $site"
work=$(mktemp -d "${TMPDIR:-/tmp}/xo-browser-memory.XXXXXX") || die "mktemp"
cleanup() { git -C "$site" worktree remove --force "$work/site" > /dev/null 2>&1; rm -rf "${work:?}"; }
trap cleanup EXIT
git -C "$site" worktree add -q --detach "$work/site" "${BROWSER_MEMORY_SITE_REV:-origin/main}" || die "cannot make a worktree of $site"
(cd "$work/site" && XO_PLAY_COMMIT=$rev python3 scripts/build-playground.py "$tree" > "$work/assets.log" 2>&1) || { tail -5 "$work/assets.log" >&2; die "the asset build failed"; }
(cd "$work/site" && "$hugo" --gc --minify --quiet --destination "$work/public") || die "hugo failed"
MEMORY_OUT=$work/memory.json node "$work/site/scripts/test-memory.mjs" "$work/public" | tee "$work/out.txt"
s=${PIPESTATUS[0]}
[ $s -le 1 ] || die "test-memory.mjs could not drive the pages (exit $s)"
[ -f "$log" ] || printf '# Browser memory of the playground pages (decision 0615)\n\n| label | date | xo commit | /repl/ compiler peak (budget) | /play/ renderer RSS peak (budget) | result |\n|---|---|---|---|---|---|\n' > "$log"
row=$(python3 - "$work/memory.json" "$label" "$rev" "$([ $s = 0 ] && echo ok || echo FAIL)" <<'EOF'
import json, sys
m = json.load(open(sys.argv[1]))
r, p = m.get("repl", {}), m.get("play", {})
print(f"| {sys.argv[2]} | {m['date']} | {sys.argv[3]} | {r.get('peak', '-')} MB ({r.get('budget', '-')}) | {p.get('peak', '-')} MB ({p.get('budget', '-')}) | {sys.argv[4]} |")
EOF
) || die "no numbers"
echo "$row" >> "$log"
echo "browser-memory.sh: $row (appended to $log)"
exit $s
