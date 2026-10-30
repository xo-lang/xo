# std/udp changes

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
