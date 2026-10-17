# Native builds that name time zones read Go's zoneinfo.zip

Status: Observed (2026-10-11, decision 0157)

Symptom: building `internal/gen/testdata/golden/time_zones` with
`--backend=arm64` in a container with gcc and the prebuilt runtime but
no Go toolchain fails: `zone data: ... (set XO_ZONEINFO to a
zoneinfo.zip)`. A native build is otherwise free of Go.

Cause: `zoneinfoZip` (internal/build/tzdata.go) reads
`$GOROOT/lib/time/zoneinfo.zip` through `go env GOROOT` to embed the
zones a program names (decision 0094), on every backend.

Fix: set `XO_ZONEINFO` to a zoneinfo.zip (or build with
`--tzdata=system`). Next step (decision 0157): ship the zip in release
archives (`lib/xo/zoneinfo.zip`) and look there before `go env GOROOT`.

Update (2026-10-12, later): done for xo1, the released `xo`: release.sh
copies Go's zoneinfo.zip to each archive's `lib/xo/zoneinfo.zip`, and
compiler/xo tz.xo `shipped_zoneinfo` reads it (beside the running
binary's `lib/xo`, or beside `$XO_RTPACK`) before `go env GOROOT`. With
the zip and the pack, the time_zones and time_dynamic_zone goldens
build and pass on darwin/arm64 with `PATH=/nonexistent` (no Go, no cc).
The Go cmd/xo (frozen) still reads only `XO_ZONEINFO` or `go env
GOROOT`.

Update (2026-10-12): with no go command at all, the build used to print only "zone data: cannot find GOROOT: exec: \"go\": executable file not found"; it now says there is no go command and to set XO_ZONEINFO or build with --tzdata=system (internal/build loadZoneinfo, TestLoadZoneinfoNoGo; reported by the website session).
