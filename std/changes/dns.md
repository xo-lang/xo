# std/dns changes

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
