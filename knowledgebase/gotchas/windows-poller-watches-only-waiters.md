# The Windows poller watches only what someone waits on

Status: Observed 2026-11-01.

## Symptom

On windows/amd64 only, every second request of a keep-alive client timed out
(goldens http_server, `client error request timed out`, and http_tls,
`Timeout`), and the request after it was fine. Linux and
darwin passed the same goldens.

## Cause

Linux and darwin register every descriptor with the poller when it opens, so
readiness reaches `pd_ready` whether or not anyone waits. On Windows the AFD
poll is armed per wait (`pd_arm`, from the wait functions) for the directions
in `D_RWAIT` and `D_WWAIT`, and the WSAPoll fallback builds its set from the
same two words. A connection parked by decision 0449 records `D_CONT` and no
waiter, and `pd_park` did not arm, so nothing watched its socket.

## Fix

A new reason to be woken by a descriptor counts in `pd_arm`'s `want` and in
the WSAPoll set, and whoever records it arms under the record's lock
(`pd_arm`) or pokes the poller after it (WSAPoll), as `pd_wait_ctx` does.
Run a change to the park or wait paths on Windows too:
`OFFLOAD_ENV=offload-win.env amd64-run.sh REV NAME http_server http_tls`.
