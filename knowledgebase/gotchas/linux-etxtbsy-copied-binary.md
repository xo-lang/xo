# "text file busy" running a binary the bin cache copied (Linux)

Status: Observed (2026-10-12, EC2 Amazon Linux 2023), fixed in
internal/build/bincache.go (copyFile).

Symptom: internal/gen TestGoldenNoMono failed on Linux with
`fork/exec .../bin/test_fs_files.test: text file busy` for a few test
programs, a different few each run. Never on the Mac.

Cause: the bin cache places a binary with a hard link, and falls back to
copying when the link fails. On Amazon Linux 2023 /tmp (the test build
directories) is a tmpfs and the user cache is on the root disk, so every
fetch and store copied, from the test process itself. While copyFile held
the destination open for writing, another test's goroutine forked; the
child inherits every descriptor until its exec closes the close-on-exec
ones. When copyFile returned and the test exec'd the copy inside that
window, the kernel refused it (ETXTBSY: a writer still has it open). This
is Go issue 22315; Go's own tests retry on ETXTBSY.

Fix: copyFile writes while holding syscall.ForkLock for reading, so no
fork can start (and so no child can inherit the write descriptor) until
it is closed. The os/exec fork holds ForkLock for writing. Regression
test: internal/build TestCopyFileNoTextBusy (copies `true` and runs each
copy while four goroutines fork and exec); without the lock it fails on
the first copy on Linux.

Avoid: any code of ours that writes an executable and then runs it in a
process that also forks from other goroutines must close the file under
syscall.ForkLock.RLock (or write it from a subprocess).
