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

## A job routed to either arm64 host (2026-11-07, Xo Lang 4)

**Symptom.** A check sent with `OFF_LANE='arm64|pass2'` (offload.md: a
plain job that carries its own inputs) failed twice before running
anything. It cloned the public GitHub repository and fetched a bundle
made against origin/main: "Repository lacks these prerequisite commits"
(the public repository trails origin/main). Then the bundle, base64
encoded into the script's text, made offload.sh fail with
`/bin/bash: Argument list too long` and "no exit status from the host"
(20 commits past the hosts' base: a 126 KB bundle, 168 KB as base64).

**Cause.** A routed job does not know its host until heavyd starts it,
and each arm64 host has its own `/work/priv` and sent ref
(`refs/offload/sent`, `refs/offload-arm64b/sent`). offload.sh passes the
script as an argument, and Linux caps one argument at 128 KB
(MAX_ARG_STRLEN), so a script with a bundle inside fits only a few
commits.

**Fix.** Bundle from the commit both hosts have,
`git bundle create B BRANCH ^$(git merge-base refs/offload/sent refs/offload-arm64b/sent)`,
`--put` it to the same `/work/in/NAME.bundle` on each host (once with
`OFFLOAD_ENV=offload.env`, once with `offload-arm64b.env`), then route
the job; it clones `/work/priv` (`git clone --shared --no-checkout`) and
fetches the bundle. Delete the copy on the host the job did not run on.
