# Apple's ar has no deterministic mode and records the user's ids

Status: Observed (2026-10-11, decision 0157)

Symptom: the TLS archive of a prebuilt runtime pack (`libxotls.a`)
differed between two builds of the same objects. `ar rcsD` (GNU and
LLVM's deterministic flag) fails on macOS: `ar: illegal option -- D`.

Cause: Apple's cctools `ar` writes each member's modification time, uid,
gid, and mode into its header. `ZERO_AR_DATE=1` zeroes the times only:
the uid and gid of whoever built it remain, so archives built on two
machines (or by two users) still differ.

Fix: `internal/build` `normalizeAr` rewrites every member header after
`ar` runs (time 0, uid 0, gid 0, mode 644). The headers have fixed
widths, so the symbol table's offsets stay valid and ld64 and GNU ld
link the result. Members are archived in name order. Checked by
`TestRTPackDeterministic` and two runs of `scripts/release.sh` giving the
same SHA256SUMS.
