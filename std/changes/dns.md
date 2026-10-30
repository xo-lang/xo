# std/dns changes

## 2026-10-30: perf: resolv.conf and /etc/hosts read once, rechecked every 5 seconds

- Kind: perf
- Commit: this commit
- Found by: bench/perf/dns mx at 1.599x Go's instructions; XO_PROF showed open, read and sysctl (the hostname) on every lookup
- Go reference: net/dnsclient_unix.go resolverConfig (tryUpdate: stat at most every 5 seconds, reread on a changed mtime or size), net/hosts.go (the same for /etc/hosts)
- Test: goldens dns_records and net_lookup_dial_timeout (arm64, LLVM, Go parity); TestStaticWords (words 91 to 94)
- Timing: xo bench --ab bench/perf/dns mx, 15 rounds: against the parent, instructions 0.637 (0.634 to 0.644), wall 0.729 (0.645 to 0.865); against Go net.Resolver (PreferGo), instructions 1.004 (0.959 to 1.022, was 1.599), wall 1.12 (0.977 to 1.251, within noise); mx reads no /etc/hosts, so this is the resolv.conf half

Native lookups (lookup_mx, lookup_txt, lookup_srv, lookup_addr and the
address lookups behind net.dial) keep /etc/resolv.conf's text and the
hostname in a process cache, and /etc/hosts in another, and stat each
file at most every 5 seconds, reading it again only when its
modification time or size changed, as Go's resolver does. Each lookup
still parses the cached text.
## 2026-10-30: fix: lookup_addr no longer faults on native Windows

- Kind: fix
- Commit: this commit
- Found by: the native Windows golden run (dns_records exited 139 on windows/amd64)
- Go reference: none (Windows native lookups return Io "not supported" for now, decision 0417)
- Test: dns_records on the Windows box (want_windows.txt: every lookup "not supported", the unparsable address and Denied as elsewhere)
- Timing: none (an error path)

`lookup_addr` reads /etc/hosts before it asks, and on Windows the
native file read made Linux system calls and faulted; it now reads no
file there, so the lookup reports Io "dns lookups are not supported on
windows yet" as the others do.

## 2026-10-29: feature: std/dns, Go's MX, TXT, SRV and PTR lookups

- Kind: feature
- Commit: this commit
- Found by: the lead's queue (2026-10-29), decision 0417
- Go reference: net (lookup.go LookupMX, LookupTXT, LookupSRV, LookupAddr; dnsclient_unix.go goLookupMX, goLookupTXT, goLookupSRV, goLookupPTR, reverseaddr; dnsclient.go byPref and byPriorityWeight; hosts.go lookupStaticAddr) of go1.26.1
- Test: internal/llvm/testdata/golden/dns_records (a UDP and TCP DNS server in the program: MX order, TXT strings joined, SRV order, PTR for IPv4 and IPv6, some and all invalid records, truncation to TCP, NXDOMAIN, SERVFAIL, an address that does not parse, Net.only's Denied) on arm64, LLVM and the Go backend
- Timing: xo bench --ab bench/perf/dns mx (2,000 lookup_mx round trips to a local UDP server), A Go net.Resolver (PreferGo), B Xo native, 15 rounds: wall 1.27 (0.98 to 1.422, within noise; 19% spread), instructions 1.599 (1.523 to 1.617), the server in the program counted on both sides

`dns.lookup_mx`, `lookup_txt`, `lookup_srv` and `lookup_addr` over a
Net (`uses net.dial`), and `dns.Resolver{server}` to ask one server.
Native: Xo's DNS client on Linux and macOS (its file, clock, random,
socket and poll calls gained macOS paths; macOS's scoped resolvers are
not consulted, as with Go's own resolver); Windows returns `Io` ("dns
lookups are not supported on windows yet") for now. Unlike Go, a
response with some invalid records is `Ok` with the valid ones (Go
returns them with an error); `Err` when every record is invalid.
