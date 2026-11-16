# A gate refused before its host fetched the bundle left the bundle behind

Status: Observed (Xo Lang 2, 2026-11-16, task #125)

## Symptom

Host 2's /work filled to 6.5 GB free of 141 GB. /work/in held 52 GB of
124 MB gate bundles, the same gates' names (gate-ev115ws-..., gate-ev90nip-...,
gate-l3datejson-...) again every one to two minutes with a new run id.
Then every gate there failed gate.sh's hostcheck ("HOSTCHECK-FAIL: 7 GB
free on the file system of /tmp, under 10").

## Cause

offload-gate.sh uploads the bundle (offload.sh --put) before it asks
heavyd for a lease, and only the host's offload-gate-remote.sh removes
it, right after fetching it. A try that ended before the fetch kept its
bundle:

- a lease refused at the agent's cap (rc 5, "this session already has 2
  env:offload-arm64b.env leases queued or running"): an evidence loop
  retried every two minutes, each try leaving 124 MB;
- a cancelled lease, one that never started, a gate stopped by a signal.

Only the reroutes (rc 77, rc 78) and the thin-bundle retry removed it.
Once the disk was low, every gate on the host failed its hostcheck, and
callers that retried on failure kept adding bundles.

## Fix

scripts/lead/offload-gate.sh (task #125):

- its exit trap removes the bundle (offload.sh --rm) when the run ended
  other than 0 or 1, and TERM and INT now reach that trap;
- before uploading it asks the host for OFFGATE_MIN_FREE_GB (default 12)
  free on /work (offload.sh --has-free GB, a plain ssh): a host short of
  disk gets nothing, and the gate moves to the next host of
  OFFGATE_HOSTS, or ends rc 6 when none is left;
- a HOSTCHECK-FAIL on disk moves the gate to the next host instead of
  ending it rc 1, so it is not retried on the same host.

scripts/lead/stdproven-offload.sh had the same leak (its
/work/in/sp-NAME.bundle, put before the lease; azure-1 found it,
2026-11-16, a loop retrying every 60 s on hosts 2, 3 and 4): its exit
trap removes the bundle on any rc but 0 and 1, and it asks --has-free
SP_MIN_FREE_GB (12) before the upload, ending rc 6 on a short host
(stdproven_offload_test.sh, 4 checks that failed before).

scripts/lead/offload_gate_test.sh covers the four cases (8 checks that
failed before the fix). A loop that retries offload-gate.sh on rc 5
should wait for a lease to end instead (heavy.sh --status), as the
agents' gate chains do.
