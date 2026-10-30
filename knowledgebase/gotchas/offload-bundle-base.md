# An offload bundle made against origin/main does not fetch on the host

Observed 2026-10-30 (Xo Lang 2). Status: Observed.

**Symptom.** A branch sent to the linux/arm64 offload host as `git bundle
create B BRANCH ^origin/main` failed there with "Repository lacks these
prerequisite commits" (a main commit pushed an hour before), and the job
ran nothing.

**Cause.** A bundle that excludes commits needs the receiving repository
to have them. The host's `/work/priv` has only what tier bundles brought
it, up to `refs/offload/sent` (the last tier the host fetched), which is
usually behind origin/main.

**Fix.** Bundle from the host's known base, as scripts/lead/tier.sh does:
`git bundle create B refs/offload/sent..BRANCH` (or the whole branch
when that ref is missing), then `offload.sh --put B /work/NAME.bundle`
and fetch it in the job. The Windows and amd64 hosts take commits
through `amd64-run.sh`, which handles this itself.
