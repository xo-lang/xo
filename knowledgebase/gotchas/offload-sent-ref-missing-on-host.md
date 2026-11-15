# The sent ref names a commit the offload host no longer has

Observed 2026-11-15 (lead; fixed by Xo Lang 2). Status: Observed.

**Symptom.** aws-43's gate, rerouted to host 3 (offload-arm64c), exited
rc 2 with no step run: "Repository lacks these prerequisite commits:
3f597cb5e". The Mac's `refs/offload-arm64c/sent` named 3f597cb5e, so
offload-gate.sh had sent a thin bundle (`^refs/offload-arm64c/sent`).

**Cause.** The sent ref records what a host fetched once, not what it
still holds. The host's `/work/priv` kept fetched commits alive only
through `refs/keep/ab/*`, trimmed to the 16 newest by committer date, so
after 16 newer branch heads the sent commit had no ref and a git gc
dropped it. `origin/main` there is set to each gate's own main, which may
be older, so it did not keep the commit either. Every later thin bundle
to that host failed until the ref was cleared. (offload-prune.sh never
touches `/work/priv`; it was not the prune.)

**Fix.** Three parts (scripts/lead):
- offload-gate-remote.sh and stdproven-remote.sh pin the gate's main as
  `refs/keep/sent`, forward only; the Mac's sent refs are commits on its
  origin/main, so the newest main pinned keeps every older one.
- offload-gate.sh asks the host first (`offload.sh --has-commit REV`, a
  plain ssh: `git cat-file -e` in /work/priv); a missing commit drops the
  sent ref and sends the whole history, an ssh failure sends the whole
  history that once.
- A rc 2 gate whose log says the bundle did not fetch reruns once with
  `OFFGATE_FULL=1` (the whole history) and the sent ref dropped.

By hand: `git update-ref -d refs/offload-HOST/sent` in the main checkout
makes the next bundle a full one. Tests: scripts/lead/offload_gate_test.sh.
