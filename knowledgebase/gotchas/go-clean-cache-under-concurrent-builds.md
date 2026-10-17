# `go clean -cache` under running builds: "package X is not in std"

Status: Observed (2026-10-16)

## Symptom

Go backend tests fail at random under `-short` (TestSubdirModules,
TestLogAttribution, TestGoldenRelease, internal/gen) with errors like

```
/usr/local/go/src/net/http/client.go:20:2: package net/http/internal/ascii is not in std (/usr/local/go/src/net/http/internal/ascii)
open <GOCACHE>/2b/2b26...-a: no such file or directory
```

though the package's files exist, and pass when run again.

## Cause

Something removed parts of the shared GOCACHE (`<user cache>/xo/gocache`)
while `go build`s were running: a disk check's `go clean -cache` (it
removes the two hex digit subdirectories, where Go also keeps its index
of GOROOT's packages), or an `rm -rf` of those subdirectories picked by
the directory's modification time (reads do not change it). A go command
then finds its std index or an action entry gone. Repro, 2026-10-16: a
loop of `go clean -cache` beside 30 builds of a net/http program on one
GOCACHE failed 12 of them with exactly these errors. Removing every file
(subdirectories kept) beside the builds failed 1 of 30. xo's own trims
lock the cache (gotchas/gocache-trim-races-concurrent-build.md,
go-cache-trim-races-builds.md); a cleanup outside xo takes no lock.

## Fix

Never run `go clean -cache` or remove GOCACHE subdirectories while
sessions build. To free space, remove files idle for over two hours and
keep the directories: `find "$GOCACHE" -mindepth 2 -type f -mmin +120
-delete`. Go refreshes an entry's time on use at most hourly, so such a
file is not in use; this is compiler/prune's rule for the cache
(decision 0223, KEEP).
