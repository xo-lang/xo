# 0406: S7: UDP sockets, a DNS lookup API and a dial timeout in std's Net

Status: Accepted (lead, 2026-10-29: 0.35 std item S7; Xo Lang 2). Not yet
implemented.

## Context

std's Net (std.md 3.2) dials and listens on TCP only, resolves host names
only inside `dial`, and has no dial timeout besides cancellation. An
exporter (the 0.35 survey, std-coverage.md S7) needs UDP (statsd, syslog,
DNS itself), a name lookup on its own, and a bounded connect.

The native resolver exists: llrt/netconn.xo `resolve` calls the system's
getaddrinfo on macOS and Windows (decision 0195) and llrt/dns.xo's Xo
resolver on Linux (decisions 0203, 0206: /etc/hosts, resolv.conf, A and
AAAA). The Go backend has Go's net.

## Decision

```xo
fn (n: Net) dial_timeout(addr: Str, d: Duration) -> Conn ! NetErr uses net.dial
fn (n: Net) lookup_host(host: Str) -> List[Str] ! NetErr uses net.dial
fn (n: Net) lookup_ip(host: Str) -> List[netip.Addr] ! NetErr uses net.dial
fn (n: Net) listen_udp(addr: Str) -> UdpConn ! NetErr uses net.listen
fn (n: Net) dial_udp(addr: Str) -> UdpConn ! NetErr uses net.dial
struct UdpConn {}
fn (c: UdpConn) read_from(max: Int = 65535) -> (Bytes, Str) ! NetErr uses net   // a datagram and its sender "host:port"
fn (c: UdpConn) write_to(data: Bytes, addr: Str) -> () ! NetErr uses net
fn (c: UdpConn) read(max: Int = 65535) -> Bytes ! NetErr uses net                // dial_udp's peer only
fn (c: UdpConn) write(data: Bytes) -> () ! NetErr uses net
fn (c: UdpConn) local_addr() -> Str
fn (c: UdpConn) close()
```

- `dial_timeout` is Go's `net.DialTimeout`: the limit covers the lookup and
  the connect (each address tried in turn shares it); past it the result is
  `NetErr.Timeout`. Cancellation still applies.
- `lookup_host` is Go's `net.LookupHost` (addresses as text, IPv4 and IPv6
  in the resolver's order) and `lookup_ip` its typed form (`LookupIP`).
  Both use the resolver `dial` uses, so a lookup and a dial agree on every
  platform. An IP literal returns itself. Errors are Go's texts
  (`lookup NAME: no such host`) in `NetErr.Io`. TXT, MX and SRV records
  are left for a later decision (their resolvers differ per platform).
- UDP is Go's `net.ListenPacket("udp", addr)` and `net.DialUDP`: a
  datagram per call; `read_from`'s max truncates a longer datagram as Go
  does (the rest discarded); `read` and `write` need a `dial_udp` socket.
  A UdpConn's calls perform the effect of how it was made (decision 0280):
  net.listen from listen_udp, net.dial from dial_udp (`uses net` above
  stands for that).
- Attenuation (Net.only, decision 0175): `dial_udp` and each `write_to`
  destination are checked as `dial` checks its address (`Denied`), and a
  lookup's host as a dial's host before any query.
- FakeNet (decision 0083): lookups answer from the fake's hosts (a new
  `FakeNet.host(name, addrs)`, localhost by default), and UDP sockets on the
  fake network pass datagrams in memory.
- Natively UDP is netconn's socket code with SOCK_DGRAM (recvfrom and
  sendto, nonblocking under the poller); on Windows the same through
  Winsock (WSARecvFrom/WSASendTo on the IOCP path).

## Consequences

- An exporter sends statsd and syslog over UDP, resolves names to check
  them, and bounds a connect, on every backend.
- Tests: Go's net tests where they fit (UDP echo on 127.0.0.1:0 and [::1]:0
  with read_from's sender, write_to after close, a truncated datagram,
  lookup of localhost and of an IP literal, an unknown host's text, a dial
  timeout against an unrouted address), arm64, LLVM and Go; Linux in
  Docker; Windows in the approved CI run. Timing rows: UDP echo round trips
  per second and lookup_host("localhost") against Go twins.

## Alternatives

- A `dial(addr, timeout: Duration = 0s)` parameter in place of
  `dial_timeout`: changes every dial's signature for an option most calls
  do not pass; Go has both forms, and `dial_timeout` reads plainly.
- A `Resolver` value (Go's net.Resolver): a later step if programs need a
  custom server or record types.
