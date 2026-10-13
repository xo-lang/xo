# A development xo binary embeds std/native: rebuild it after a std edit

Status: Observed (2026-10-13, decision 0201).

**Symptom.** After editing std/native/llrt/scan.xo, `bench/perf/text/
measure.sh .tools/bin/xo ...` printed instruction counts identical to the
previous run to the last few thousand (split, lines, hex unchanged), as
if the edit had not happened; `go test` of the same code saw the edit.

**Cause.** internal/check reads the native runtime modules from
`std.NativeFiles`, an embedded file system: `go build -o .tools/bin/xo
./cmd/xo` copies std/native into the binary. `XO_STD` does not change
that. `go test` rebuilds the package, so tests see the edit; a binary
built earlier does not.

**Fix.** Rebuild the binary before every measurement or scratch run
after a std edit (`go build -o .tools/bin/xo ./cmd/xo`); the scan work's
measurement script did it first thing.
