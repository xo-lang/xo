# std/debug changes

## 2026-10-21: feature: std/debug, live profiles and scheduler counters

- Kind: feature
- Commit: 0c17dba55
- Found by: decision 0325 (the owner's language poll)
- Go reference: net/http/pprof (`/debug/pprof/profile?seconds=`, `/heap`)
- Test: internal/llvm/testdata/golden/debug_routes (auth refused, counters, heap, a one second profile, a second request during it refused, a profile again; arm64, LLVM, Go parity)

A router a server mounts to profile itself while it runs: `GET
/profile?seconds=N&hz=H` (CPU, native: the XORAW container `xo prof`
converts; Go: pprof), `GET /heap`, `GET /sched`, each behind an `auth`
function, one profile at a time. The native profiler can now be started
and stopped at run time (llrt/prof.xo profile_begin, profile_stop,
profile_collect: the ring stays mapped, signal handlers in flight are
counted so a stopped profile has no late sample).
