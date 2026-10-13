# The EC2 scripts sent HEAD instead of the requested commit

Status: Observed (2026-10-13), fixed.

Symptom: an agent's EC2 queue job for its branch commit ended in a minute
with an empty log ("packages ok: 0"): the instance never checked out the
commit.

Cause: scripts/remote-full.sh and remote-windows.sh ran `git bundle
create b.bundle <sha>`, which git refuses for a bare sha, then fell back
to `git bundle create b.bundle HEAD` silently, so the instance got the
main checkout's HEAD and `git checkout <sha>` failed. The lead's batches
never noticed because they always ran main's HEAD.

Fix: both scripts bundle through a temporary branch ref
(refs/heads/xo-remote-<sha>, deleted right after; a refs/heads ref
because `git clone` of a bundle fetches only branches and tags) and stop
if bundling fails.
