# std/udp changes

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
