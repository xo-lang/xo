# std/udp changes

## 2026-11-03: port: std/udp in a wasip2/wasm command

- Kind: port
- Commit: this commit
- Found by: Lang 4, decision 0447 step R2 (the networking std modules on wasip2)
- Go reference: none
- Test: goldens udp_echo and udp_no_tasks as wasm commands (TestWasmCommandGoldens)
- Timing: none
- Shipped in: unreleased

udp.listen, udp.dial, read_from, write_to, write, local_addr and close
run over wasi:sockets/udp in a command, with the native error texts and
deadlines (decision 0447 R2h).

## 2026-11-03: fix: a program whose only network is std/udp builds, and keeps the shutdown

- Kind: fix
- Commit: this commit
- Found by: Lang 1, checking which std modules the compiler's concurrency count missed (after the netpoller tick of gotchas/netpoll-wait-idle-wake-latency.md)
- Go reference: none
- Test: golden udp_no_tasks; internal/llvm TestNativeSignalNetNoTasks/udp
- Timing: none
- Shipped in: since std/udp (decision 0406) through v0.35.0

A program with no task in its code whose only network was std/udp did
not build (`_xo_x_nc_conn_free` is named in `netconn.conn_new` but no
function of the program defines it): the compiler counted neither its
network nor its concurrency, so it built neither the network runtime's
exports nor the scheduler. A call of a std function that declares a
network effect now counts as both (ssa/call.xo net_waits), so such a
program builds, and the two phase shutdown of core 7.5 applies to it: a
blocked read unwinds at the second signal and main's defers run.

## 2026-10-30: fix: a cut datagram on Windows

- Kind: fix
- Commit: this commit
- Found by: the lead's native Windows golden run (2026-10-30: udp_echo
  timed out after "dial echo")
- Go reference: UDPConn.ReadFrom (Go's Windows netFD treats
  WSAEMSGSIZE as a read that filled the buffer)
- Test: internal/llvm/testdata/golden/udp_echo, natively on the Windows
  offload host

On windows/amd64, `read_from(max)` of a datagram longer than max returned
an error instead of its first max bytes: sock.xo's Winsock error mapping
turned WSAEMSGSIZE into EIO, so netconn.xo's check for the truncation
never matched. A server reading with a small max stopped at the first long
datagram and its peer waited forever. WSAEMSGSIZE now maps to EMSGSIZE,
which the check tests.

## 2026-10-29: feature: std/udp

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey (std-coverage.md S7): an exporter
  sending statsd and syslog
- Go reference: net.ListenPacket, net.DialUDP, UDPConn.ReadFrom/WriteTo
  (a datagram cut to the buffer); decision 0406
- Test: internal/llvm/testdata/golden/udp_echo (native and Go)

`udp.listen(net, addr)` and `udp.dial(net, addr)` return a UdpConn with
`read_from`, `write_to`, `read`, `write`, `local_addr` and `close`, on the
native backends over llrt/netconn.xo and on the Go backend over Go's net.
