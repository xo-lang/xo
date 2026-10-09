# Go's time.LoadLocation prefers the host's zone database

Status: Observed (2026-10-09, std/time, decision 0094)

Symptom: a program that imports Go's `time/tzdata` still gives different
zone answers on different hosts (an old `/usr/share/zoneinfo` on one
machine), so record and replay and goldens disagree across machines.

Cause: `time.LoadLocation` looks in `$ZONEINFO`, then the system
directories, and uses the embedded `time/tzdata` only when both miss.
`time/tzdata` is also all or nothing (about 450 KB) and has no API to
read one zone.

Fix: std/time never calls `time.LoadLocation` for embedded data. The
build extracts TZif files from the Go toolchain's
`lib/time/zoneinfo.zip` (`internal/build/tzdata.go`) and the runtime
loads them with `time.LoadLocationFromTZData` (`std/rt/timezone.go`,
with a small reader for the stored zip when the whole database is
embedded). Only `--tzdata=system` and `local` (host dependent by
design) call `time.LoadLocation`. Zone answers still follow the Go
toolchain's database version: two machines with different Go releases
can differ for recent rule changes.
