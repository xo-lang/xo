# A native program's stdin from a FIFO never ends

Status: Observed (2026-10-16, darwin/arm64, LLVM backend)

## Symptom

A native Xo program reading `os.stdio.read_line()` until it returns
None, with stdin redirected from a named pipe (`mkfifo f; prog < f &`,
then `exec 3> f`, a line written, `exec 3>&-`), never returns: no
writer holds the FIFO (lsof shows none), yet `x.main` stays parked in
`poll.pd_wait_ctx` and the program never exits. The same program with
stdin from an anonymous pipe (`producer | prog`) or `/dev/null` sees EOF
and exits at once.

Hit by bench/perf/tls's server workloads, whose first measure.sh held
the Xo server's stdin open through a FIFO; the run hung for 10 minutes.

## Cause

Not yet found. The native poller (std/native/llrt poll.xo, kqueue on
macOS) does not wake the stdin reader for a FIFO's end of file; the
anonymous pipe path works.

## Fix or workaround

Give a native child an anonymous pipe as stdin (a subshell piped into
it, or Go's cmd.StdinPipe), not a FIFO. Minimal repro: build
bench/perf/tls/server, then

    mkfifo f
    CERT=... KEY=... ./server < f > out.txt &
    exec 3> f; echo line >&3; exec 3>&-
    wait   # never returns

Run anything like this under a timeout.
