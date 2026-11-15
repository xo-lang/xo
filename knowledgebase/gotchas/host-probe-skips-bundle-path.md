# A probe that skips the bundle path does not prove a host

Status: Observed (2026-11-15)

**Symptom.** Host 4 (xo-host4) passed its probe through offload.sh (rc 0),
was put in the gate routing, and the first real gate sent there (Lang 3's
xtls6) failed at once with rc 2: `scp: ... No such file or directory`.
The lead paused the host (renamed its env to `offload-arm64d.env.paused`).

**Cause.** The probe ran a command over ssh in a lease, which needs only
sshd, the lock files and offload.q/offload.runs. A gate also copies its
bundle into `/work/in`, unpacks it against the gate repository
`/work/priv` (a clone the host fetches the sent REV into), writes results
under `/work/out`, and keeps reused trees in `/work/offgate-keep`. Host 4's
/work was built by hand from a list of what jobs use, and the list missed
those four, so the probe passed on a host no gate could run on.

**Fix.** Create `/work/in` (owned by root, as on host 3), `/work/out` and
`/work/offgate-keep` (xo, mode 775), and copy the gate repository from a
working host: `docker cp xo-host3:/work/priv - | docker cp - xo-host4:/work`,
then `chown -R xo:xo /work/priv` and compare `git -C /work/priv
for-each-ref | wc -l` on both. Prove a new or rebuilt host with a real
`offload-gate.sh` run pinned to it (a lane of its own, `OFFLOAD_ENV`,
`OFF_LANE`, `OFFGATE_REROUTE=0`) before it joins the routing; a probe
proves only that ssh and leases work.

Host 4 passed such a run after the fix (2026-11-15: the `repo` step of
d3c9515ba in a lane `pass4t` of its own, xobuild 210 s, rc 0) and went
back into the routing.
