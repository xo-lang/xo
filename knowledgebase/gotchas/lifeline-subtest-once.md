# A Lifeline on a subtest's t kills a shared server with that subtest

Status: Observed (2026-10-16, internal/llvm FuzzHTTPDiff on main
1068a8ca); fixed 2026-10-16 (xo-4).

## Symptom

`go test ./internal/llvm` (the full run) sat idle for 20 minutes and
more in FuzzHTTPDiff's seed corpus: goroutine 1 in
testing.runFuzzTests, a seed input in http1Send's dial retry loop, the
test binary's only child (the native HTTP server) a zombie.

## Cause

FuzzHTTPDiff started its native server once, in a sync.Once inside the
fuzz target, with the first input's t ("the server runs until the test
binary exits"). Since 72d61e52, http1Server runs the child behind
testtmp.Lifeline(t, ...), whose t.Cleanup cuts the lifeline and kills
the child: the server died when that first input's subtest ended, and
nothing waited for it. Every later input dialed the dead address 301
times, 200 ms apart, about a minute each. Any helper that registers a
cleanup (Lifeline, t.TempDir, t.Cleanup itself) on the t of a subtest
or a fuzz input ends with that subtest, not with the value cached in
the Once.

## Fix

A resource shared across subtests or fuzz inputs is made with the
parent's t, or the F's before f.Fuzz (an F's Cleanup cannot be called
inside the fuzz target): FuzzHTTPDiff builds and starts both servers on
f. http1Send stops dialing an address whose server has exited (the
servers map of startHTTP1Server and startFuzzServer) and reports its
exit status and standard error; TestHTTP1SendServerExited. Lifeline's
kill deadline extends to the test's own deadline (t.Deadline) when
that is later.
