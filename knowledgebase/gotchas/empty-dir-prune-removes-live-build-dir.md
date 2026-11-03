# An ageless empty-directory prune removes a live build's work directory

Status: Observed (2026-11-03, cmd/xo TestNoGCTestLeaks on the first
linux/arm64 offload host).

Symptom: a native build or `xo test` fails with
`xo: <user cache>/xo/build/HASH/arm64/w-RAND/xo_buildinfo.o: no such
file or directory` (userfs, same and notes-service in one gate run,
userfs alone in the next), only while another job runs on the same
host; the same xo passes alone and on the Mac.

Cause: each build makes its own work directory (`w-RAND`, main.xo's
native path; `-w-RAND` for LLVM) before it writes into it, and on
arm64 the first file it writes there is the build information object,
seconds later. offload.sh's disk pass, which every job runs first when
`/work` has under 30 GB free, deleted cache files idle over 6 hours and
then `find ... -type d -empty -delete` with no age: any build's still
empty work directory went with it. xo's own prune (compiler/prune) was
not involved: it keeps an entry used within 30 minutes. Repro
(`~/xo-lead/notes/lang1/emptydir-repro.sh`): that find in a loop
during `xo test --backend=arm64 --no-gc testdata/nogc/userfs` failed 6
of 6 builds with the same message; with `-mmin +60` added, 0 of 6.

Fix: offload.sh deletes an empty directory only when it is as idle as
the files (`-mmin +$age`): a work directory's modification time is its
creation or its last entry change. Any cleaner of a build cache must age
directories as well as files. Separately, the cmd/xo tests that build
through xotool.Build (TestNoGCTestLeaks, TestNoGCNotesServer,
heapstats, profurl) now pass their setenv'd XO_BUILD_DIR to xo
(buildOpts in env_test.go): setenv records a variable only for
`command`, so those builds used the shared `<user cache>/xo/build`,
which is how a host-wide prune could reach them.
