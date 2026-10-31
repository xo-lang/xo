# scripts/identity: Mach-O targets need the Mac's objdump

Status: Observed (2026-10-30, Xo Lang 2)

Symptom: `go run ./scripts/identity A B` on the linux/arm64 offload host
fails in seconds with `identity: objdump .../a/prog: exit status 1`
(default targets darwin/arm64,linux/arm64; also darwin/amd64).

Cause: identity compares Mach-O binaries by `objdump -d` (their UUID and
signature differ run to run); GNU objdump on Linux does not read Mach-O.

Fix: on a Linux host pass `-targets linux/arm64` (or linux/amd64); run
the darwin targets on the Mac (gate or bulk with HEAVY_DARWIN=1).
