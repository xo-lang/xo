# 0417: DNS record lookups in std/dns

Status: Accepted (lead, 2026-10-29, answering the draft's four questions; drafted by Lang 5). Not built yet.

## Context

`Net.lookup_host` (decision 0406) answers A and AAAA through the resolver
`dial` uses. Services also need the other records Go's net package looks
up: MX to deliver mail, TXT for SPF, DKIM and domain verification, SRV
for service discovery (the 2026-10-29 std survey's exporter and gateway),
and PTR for logging a peer's name. std-coverage listed them as missing.

The native resolver (std/native/llrt/dns.xo, decisions 0203 and 0206)
is a DNS client in Xo following Go's pure Go resolver: resolv.conf's
servers, search list, ndots, timeout, attempts, rotate, EDNS0, TCP on a
truncated answer. It asks only for A and AAAA.

## Decision

1. A module, `std/dns`, of free functions that take the Net, as std/udp
   and `netip.lookup` do:

   ```xo
   struct Mx { host: Str, pref: Int }                                   // Go's net.MX
   struct Srv { target: Str, port: Int, priority: Int, weight: Int }    // Go's net.SRV
   struct Resolver { server: Str = "" }        // "" is the system's (resolv.conf)

   pub fn lookup_mx(n: Net, name: Str) -> List[Mx] ! NetErr uses net.dial
   pub fn lookup_txt(n: Net, name: Str) -> List[Str] ! NetErr uses net.dial
   pub fn lookup_srv(n: Net, service: Str, proto: Str, name: Str) -> List[Srv] ! NetErr uses net.dial
   pub fn lookup_addr(n: Net, ip: Str) -> List[Str] ! NetErr uses net.dial
   ```

   and the same four as methods of `Resolver`, which send every query to
   `server` (`"127.0.0.1:5353"`), Go's `Resolver{PreferGo: true, Dial: ...}`.
   The free functions are `Resolver{}`'s.

   Not methods of the `Net` interface: adding methods to Net breaks every
   user implementation of it (the user_caps NoNet golden, any test double),
   as `dial_timeout` and `lookup_host` did.

2. The effect is `net.dial`, as `lookup_host`'s: a lookup sends packets to
   a server. No new `net.lookup` effect: every program that looks names up
   also dials, so it would add a word to signatures and separate nothing.
   An attenuated Net (`only(hosts)`) refuses a name it does not admit with
   `Denied` (for lookup_srv the `name`, for lookup_addr the IP), as for
   `lookup_host`. FakeNet answers no record lookup (`Io`, "no such host");
   a test points a `Resolver` at a server it runs.

3. Backends: native on Linux and macOS through dns.xo, generalized from A
   and AAAA to any record type (macOS reads the /etc/resolv.conf configd
   writes; its scoped resolvers, a VPN's split DNS, are not consulted, as
   in Go's pure Go resolver). The Go backend uses Go's net.Resolver, on
   every platform from the start. Native Windows, stage 1: `Err(Io("dns
   lookups are not supported on windows yet"))`, said in the spec and
   std/changes; stage 2 binds DnsQuery_W (superseded by decision 0427:
   stage 2 runs dns.xo's own client with the adapters' DNS servers).

4. Invalid records. Go returns the valid records and an error together
   when some records are invalid (a host that is not a domain name). A
   Result cannot carry both, so: Ok(the valid records) when at least one
   is valid, Err when none is. The doc comments say so.

5. Go's order: MX by preference with equal preferences shuffled; SRV by
   priority with a weighted shuffle within one (RFC 2782, Go's
   byPriorityWeight); TXT a record's strings joined; names rooted
   ("mail.example.com."); lookup_addr reads /etc/hosts first. Errors are
   `NetErr.Io` with Go's DNSError text ("lookup NAME on SERVER: no such
   host", "server misbehaving"), `Timeout` when the timeout and attempts
   run out.

## Consequences

- Tests are deterministic through a fixture: a golden runs a UDP (and,
  for truncation, TCP) DNS server task on 127.0.0.1:0 with canned
  answers, queries it with `Resolver{server}`, and compares with Go's
  resolver dialing the same fixture. Distinct preferences and priorities
  keep the order fixed; both invalid record cases (some, all) are rows.
- dns.xo's query and answer code serves any record type; lookup_host is
  unchanged.
- Windows native users get an error until stage 2 (decision 0427, 2026-10-30).

Stage 2 superseded by 0427 (2026-10-30).

## Alternatives

- Methods on `Net`: breaking (above).
- A new `net.lookup` effect: finer than any program needs (above).
- Returning the valid records and an error together (Go's two results):
  would need a new result shape for four functions; Ok-when-any-valid
  keeps Go's intent.
- The system resolver on macOS (res_query in libresolv, Go's cgo path):
  honours scoped resolvers, but needs a C call per platform; Xo's own
  client works the same on Linux and macOS and is tested once.
