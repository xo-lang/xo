#!/bin/bash
# shipped-in.sh REV... : for each revision, the first release tag (v*, by
# version) that contains it, or "unreleased", with the tag's date. Every
# "Shipped in", "since vX" or "first in vX" claim in std/changes,
# CHANGELOG.md, a report or a message comes from this output
# (knowledgebase/infrastructure/agent-tools.md "Origin claims").
#
# A tag containing the commit that wrote some code is not proof that a
# release program reached it: when the claim is about a behavior (a bug a
# user could hit), also run it for the commit that made the path
# reachable (the feature that first called it) and use the later tag.
set -euo pipefail
if [ $# -eq 0 ]; then
  echo "usage: scripts/shipped-in.sh REV..." >&2
  exit 2
fi
git fetch -q --tags origin 2>/dev/null || true
for r in "$@"; do
  c=$(git rev-parse --verify -q "$r^{commit}") || { echo "$r: not a commit" >&2; exit 2; }
  t=$(git tag --contains "$c" --list 'v*' --sort=v:refname | head -1)
  s=$(git log -1 --format='%h %ad' --date=short "$c")
  if [ -z "$t" ]; then
    echo "$r ($s): unreleased"
  else
    echo "$r ($s): first in $t ($(git log -1 --format=%ad --date=short "$t"))"
  fi
done
