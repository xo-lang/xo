# Test server children outlive a killed `go test`

Status: Observed (2026-10-16, Xo Lang 2; fixed by xo-4; again 2026-10-17, fixed by xo-4 with testtmp.Guard).

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

**Seen again (2026-10-17, found 2026-10-18).** Four golden programs
(tz_table, http_conn_fault, http_tls_keys, http_tls) from one `go test`
process's xotool build directory ran 27 hours with PPID 1 and needed
SIGKILL: the golden runners (internal/llvm runBinary and buildRun,
internal/arm64, internal/gen) started programs with plain exec.Command,
not Lifeline, and these ignore SIGTERM. Fix (xo-4): testtmp.Guard(cmd),
for runners with no testing.T: one lifeline per test process (its write
end held until the process ends), the program in a process group of its
own, and the watcher sends the group SIGTERM, then SIGKILL after 3 s;
the golden runners, internal/gen's run helpers, cmd/xo's env helper,
and xotool's runIn (every xo the tests run) use it. Test: internal/testtmp
TestGuardOutlivesNothing (a guarded shell and its grandchild, both
ignoring SIGTERM, gone 3.2 s after their parent exits with no cleanup;
it fails with Guard disabled).
