# Test server children outlive a killed `go test`

Status: Observed (2026-10-16, Xo Lang 2; fixed by xo-4).

**Symptom.** Native network servers that Go tests start (TestHTTPParseDiff's
http-parse-* binaries, TestHTTPFuzz's fuzz server, the TLS and H2 diff
peers, TestGoldenASan's net_echo and http_* goldens) pile up with PPID 1
at 0% CPU, blocked in accept.

**Cause.** Their stop (a returned stop function, a t.Cleanup kill, a
deferred kill) runs only when the Go test returns. A `go test -timeout`
panic, an os.Exit, or a SIGKILL of the test binary runs none of it, and
a server that never reads stdin has no way to notice its parent is gone
(macOS has no PR_SET_PDEATHSIG).

**Fix.** internal/testtmp Lifeline(t, cmd, d) before Start: the child
runs behind internal/testtmp/lifeline, which starts a watcher holding a
pipe whose write end only the test process holds (fd 3) and then execs
the program, so cmd.Process stays the program (its signals, exit status,
and Kill). The kernel closes the write end however the test process
ends; the watcher then SIGKILLs the program. Lifeline also adds a
t.Cleanup kill and a deadline. Call it for every long-running native
child a Go test starts; tests: internal/testtmp TestLifelineParentDies
(the holder of the write end killed, the child gone within 10 s),
TestLifelineCut, TestLifelineExitStatus. $XO_LIFELINE names a prebuilt
wrapper where there is no Go toolchain (Docker runs of cross built test
binaries).
