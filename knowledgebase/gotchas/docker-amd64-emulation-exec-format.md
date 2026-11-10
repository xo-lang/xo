# Docker Desktop's amd64 emulation: "exec format error"

Status: Observed 2026-11-10.

**Symptom.** The 0.46.0 seed cut failed at step 6, `race-runtime.sh
linux-amd64 failed`, its log one line: `exec /usr/bin/sh: exec format
error`. `docker run --rm --platform linux/amd64 alpine:3.20 uname -m`
gave the same. The 0.44.0 cut (2026-11-08) built the same runtime in the
same Docker Desktop.

**Cause.** Docker Desktop on the arm64 Mac ran linux/amd64 containers
through its x86_64 emulation (Rosetta or QEMU binfmt), which was no longer
registered: an arm64 kernel cannot exec an x86-64 binary without it. The
linux/arm64 containers (linux-arm64's runtime, windows-amd64's MinGW
build) still ran, so only the amd64 step failed, after about five minutes
of the cut.

**Fix.** Build linux-amd64's runtime natively on the owner's linux/amd64
offload host: `XO_RACE_REMOTE_LINUX_AMD64=$HOME/xo-lead/agents/race-remote.sh`
(scripts/lead/race-remote.sh, through offload.sh with offload-amd64.env;
race-runtime.sh calls it, about 40 s). The host has no root, no pip and
no python3-venv, so race-runtime.sh takes Kitware's cmake and ninja's
release binaries at pinned SHA-256s there. Unset, seed-cut.sh now checks
that Docker runs a linux/amd64 container before it builds anything.

A patch seed (XO_SEED_TAG) builds in the tag's tree, so seed-cut.sh must run its own
scripts/race-runtime.sh, not the tree's: the 0.46.0 cut's second try ran v0.46.0's older copy,
which ignored the variable and failed the same way (fixed 2026-11-10).
