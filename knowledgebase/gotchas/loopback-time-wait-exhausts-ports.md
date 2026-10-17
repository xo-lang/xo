# The HTTP diff tests filled every ephemeral port with TIME_WAIT

Status: Observed 2026-10-16, fixed the same day (xo-4).

## Symptom

While two full internal/llvm runs overlapped on the Mac, unrelated
tests failed with `connect: can't assign requested address`
(TestModProxy, TestSignalHTTPInFlight, the http_tls_keys golden), and
so did `git fetch`. netstat showed about 16,800 sockets in TIME_WAIT,
all 127.0.0.1 to 127.0.0.1: every one of macOS's 16,384 ephemeral ports
(49152-65535).

## Cause

The HTTP diff tests (TestHTTPParseDiff, TestHTTP1ParseDiff,
TestHTTPFuzz, FuzzHTTPDiff) send each input on a connection of its own
through internal/llvm http1Send: about 3,080 connections a backend for
the parse diff. One run of them alone peaked at 16,563 TIME_WAIT
sockets, each on the client's own local port (a socket in TIME_WAIT
holds its local port for 2 MSL: 30 s on macOS, net.inet.tcp.msl 15000;
60 s on Linux).

http1Send set SO_LINGER 0 before closing, meant to end with a reset and
no TIME_WAIT. It never worked: the client half closes first (CloseWrite,
so the server sees the end of the input), so once the server's FIN
arrives the client socket is already in TIME_WAIT, disconnected, and
close() sends no reset. Linger 0 only helps a socket closed while still
connected.

## Fix

internal/xotool ports.go:

- DialLoopback dials from a private local port, 20000-32767, below both
  the macOS and the Linux (32768-60999) ephemeral ranges, with
  SO_REUSEADDR: TIME_WAIT there takes no port from anything else, and a
  port in TIME_WAIT toward one server is reused toward another (a
  connect to the same server fails with EADDRINUSE or EADDRNOTAVAIL and
  the next port is tried). The range serves 12,768 connections to one
  server per 2 MSL. http1Send uses it.
- NeedPorts fails a test at once with "ephemeral ports exhausted: N of
  M in TIME_WAIT" when more than three quarters are; the HTTP and TLS
  diff tests, TestModProxy and TestSignalHTTPInFlight call it first.
  DialErr adds the same count to a dial error that ran out of ports.

Measured on the Mac (the HTTP, TLS and h2 diff tests of internal/llvm,
one run, 2026-10-16): before, a peak of 16,563 sockets in TIME_WAIT,
all on ephemeral ports; after, 1,325 on ephemeral ports (975 of them
one listener's port, so about 350 ports held) and 25,315 on the private
ports.

To check a machine: `netstat -an -p tcp | grep -c TIME_WAIT` (macOS),
`ss -tan state time-wait | wc -l` (Linux).
