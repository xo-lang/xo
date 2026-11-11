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

publish.sh (2026-11-11) checks it: the first prepare that makes a tag pins
its public commit (tag-pins.txt in the tools' directory), and prepare and
push fail when a tag's commit differs from its pin (XO_PUBLISH_REPIN=TAG
accepts a deliberate move of an unpublished tag), or when an unpublished
tag's message, or a message in its chain not yet published, names a commit
the private repository has and the public one does not. Without the rules
above, prepare fails naming both moved tags and all seven hashes. A failure
names the hash and its commit: add a rule mapping it to the cited commit's
public hash (filter-repo's .git/filter-repo/commit-map in the public
checkout), after checking the hash appears in no file content the rule
would also change. Hashes of a pruned commit (one that only touched a
removed path, such as scripts/lead/) or a branch that never reached main
stay private in main's messages; the check looks only at unpublished tags.
