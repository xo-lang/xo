# A later publish prepare moved release tags a built kit embeds

Status: Observed (2026-11-11, Xo Lang 2, 0.48.2 release kit)

## Symptom

The owner's publish tools (`publish.sh prepare`, outside this repository)
rerun after main moved gave the unpublished tags v0.48.1 and v0.48.2 new
public commits (d56cf2d46 became 4b9916993) with the same trees, dates and
subjects. The release archives were already built, signed and notarized
from d56cf2d46, and `xo version -v` names the commit it was built from, so
publishing that state would have released binaries naming a commit the
public repository does not have. prepare's checks (leaks, fast-forward,
purity) all passed: the tags were never published, so nothing compared them.

## Cause

prepare rewrites the private history in one git filter-repo pass, which
replaces a commit hash in a message only when the commit it names was
rewritten earlier in the pass. A release branch's commits (cherry-picks
whose messages cite main's commits: "cherry picked from commit ..", "fix on
main ..") are not descendants of the main commits they cite, so their order
in the pass depends on the refs; once main gained commits the release
commits came first, their citations stayed private hashes, and the message
change gave them new hashes. Annotated tag messages are not translated at
all.

## Fix

Explicit rules in the tools' replacements.txt for the cited hashes (the
earlier run's public hashes, checked against filter-repo's commit map):
v0.48.2 d56cf2d46, v0.48.1 02bdc9d43 again, and no private hash left in a
tag message.

## How to avoid

After any prepare that follows a release kit, compare each unpublished
tag's commit with the commit the kit was built from (the kit's
rel-final.log "building HASH") before handing the owner `publish.sh push`,
and look for private-only hashes in tag messages and in the tags' commit
messages (a hash `git cat-file -e` finds privately but not in the public
repository). A pruned commit (one that only touched a removed path, such as
scripts/lead/) or a branch that never reached main also leaves a private
hash; only those in a tag's chain or message matter for a kit.
