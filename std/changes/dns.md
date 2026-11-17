# std/dns changes

## 2026-11-16: feature: std/dns, Go's LookupCNAME, LookupNS and Resolver.LookupHost

- Kind: feature
- Commit: this commit
- Found by: task #184 (feature gaps, ~/xo-lead/notes/stdcov/502a0170c.json)
- Go reference: net (lookup.go LookupCNAME, LookupNS, Resolver.LookupHost; dnsclient_unix.go goLookupCNAME, goLookupHostOrder; hosts.go lookupStaticHost) of go1.26.1
- Test: std/dns_test.xo (lookup_cname through an alias's A answer, a plain name, a CNAME only name, invalid and missing names, /etc/hosts first; lookup_ns valid, partly and wholly invalid, missing; Resolver.lookup_host: an IP literal, localhost from /etc/hosts, A and AAAA, an alias, a bad address length, missing names)
- Shipped in: not yet released

`dns.lookup_cname` and `lookup_ns` over a Net, and as Resolver
methods, and `Resolver.lookup_host` for a host's addresses from one
chosen server. The CNAME lookup follows Go: /etc/hosts, then the owner
of the first address record of the A answer, else a CNAME query.

## 2026-11-16: fix: a name with a "." byte inside a label makes the answer malformed, as Go's dnsmessage says

- Kind: fix
- Commit: this commit
- Found by: azure-1 (TestDnsFuzz seed 11 case 1440 on linux/arm64 host 3, task #115)
- Go reference: net.Resolver over golang.org/x/net/dns/dnsmessage (Name.unpack refuses a label holding '.', golang/go#56246: "cannot unmarshal DNS message")
- Test: internal/stdtest TestDnsAnswerErrors (a PTR and an MX answer whose name has the label "bad_.ame"); TestDnsFuzz
- Shipped in: not yet released

The name reader took a label's bytes as they came, so an MX, SRV, PTR or TXT answer naming
`\x08bad_.ame\x07example\x00` was returned as the host "bad_.ame.example.", a text that reads as three
labels, not two. Go refuses such a name, and with it the message; std/dns now does the same
("cannot unmarshal DNS message").

## 2026-11-16: fix: the TCP retry of a truncated answer waits in the task, not the worker thread

- Kind: fix
- Commit: this commit
- Found by: azure-1 (the dns stress on linux/arm64, 64 tasks: "txt s705: ... server misbehaving", task #115)
- Go reference: net dnsclient_unix.go dnsStreamRoundTrip over a net.Conn (the goroutine parks on the netpoller)
- Test: internal/stdtest TestDnsTcpOneThread (the dns stress with XO_THREADS=1, 8 and 64 tasks)
- Shipped in: not yet released

A lookup whose UDP answer came back truncated asked again over TCP waiting in ppoll (poll on darwin) on
the worker thread for the connect and each read, where the UDP exchange already waited in the task on
the netpoller while other tasks run or wait. A program answering its own lookups (a test's DNS server,
the dns stress) could not run its server task on that thread, so with one worker thread, or every
worker in such a wait, the TCP exchange waited out its timeout twice (10 s) and the lookup failed with
"server misbehaving". The TCP exchange now waits on the netpoller too, under the same condition as UDP
(the ppoll wait remains for a lone task and on Windows).

## 2026-11-16: fix: an SRV answer whose owner name is not a domain name fails the lookup

- Kind: fix
- Commit: this commit
- Found by: azure-1 (TestDnsFuzz seeds 5 to 7, task #115)
- Go reference: net lookup.go Resolver.LookupSRV (the cname, the first SRV answer's name, must be isDomainName), dnsclient_unix.go goLookupSRV
- Test: internal/stdtest TestDnsAnswerErrors cases 10 and 14 (a control byte, and a byte that is not UTF-8, in the owner name)
- Shipped in: not yet released

`lookup_srv` returned the records of an answer whose first SRV record's owner name was not a domain name (a control byte,
or a byte that is not UTF-8, in a label). Go's LookupSRV checks that name (lookupSRV's cname) after the records parse and
before it filters the targets, and fails with "SRV header name is invalid"; std/dns now does.

## 2026-11-16: fix: a reply with an error RCODE cut inside its records is the RCODE's error

- Kind: fix
- Commit: this commit
- Found by: azure-1 (stdproven fuzz:dns, TestDnsFuzz seed 4 case 3977, task #115)
- Go reference: net dnsclient_unix.go checkHeader and extractExtendedRCode (go1.26: skipping the records minds no error; only the first answer's header must parse)
- Test: internal/stdtest TestDnsAnswerErrors case 6 (REFUSED, its one answer running past the end of the message)
- Shipped in: not yet released

A REFUSED (or other error RCODE) reply whose answer or authority records ran past the end of the message was "cannot
unmarshal DNS message": std/dns parsed every record before it read the RCODE. Go's checkHeader skips the records without
minding errors (the RCODE is then the header's, with no OPT record read) and needs only the first answer's header to
parse, so it is "server misbehaving" (a cut first answer header is still "cannot unmarshal DNS message").

## 2026-11-16: fix: RCODE and lame referral errors are Go's; a TXT string past its RDLENGTH fails the lookup

- Kind: fix
- Commit: this push
- Found by: internal/stdtest TestDnsFuzz (task #90): seed 2 cases 36 and 48 (REFUSED), seed 4 case 293 (a TXT RDLENGTH one short)
- Go reference: net/dnsclient_unix.go checkHeader (errServerMisbehaving, errLameReferral); golang.org/x/net/dns/dnsmessage unpackTXTResource (errCalcLen)
- Test: internal/stdtest TestDnsAnswerErrors (4 of its 6 cases fail on the old std, all pass now; Go's resolver checks each expectation); TestDnsFuzz seeds 2 to 4, 300 cases each, arm64 and llvm
- Timing: none (the kinds are classified where they were; one error check per TXT record)
- Shipped in: not yet released

## 2026-11-03: perf: a one-query lookup waits on the netpoller while other tasks run or wait

- Kind: perf
- Commit: this commit
- Found by: Lang 4, the twin row dns mx at 4.66x Go (bench/results/std-twin-rows2-2026-11-03.md)
- Go reference: net's dnsclient on the netpoller (Go waits there always)
- Test: goldens dns_records, dns_no_tasks; `xo bench --ab` of bench/perf/dns mx and ext and a lookup-only program
- Timing: host 1, 15 rounds, wall B/A: mx 0.175 (103.7 to 18.13 ms), ext 0.997, lone 0.999; against Go's twin mx 0.928 (was 4.657) (bench/results/dns-netpoll-2026-11-03.md)
- Shipped in: unreleased

A lookup with one query (lookup_mx, lookup_txt, lookup_srv, one address
family) waits for its reply in the task on the netpoller when another
task can run or waits for the network or a timer (pool.others_active,
the deadlock check's own test), and in ppoll on the thread otherwise.
The netpoller path was tried for every lookup on 2026-11-02 (31ba6938b)
and reverted (db6eefbfc) when a program with no concurrency waited for
the poller's tick; that tick is gone, but a lone task's lookup still
took twice the CPU there for the same wall time, so it keeps the thread.

## 2026-11-03: port: std/dns in a wasip2/wasm command

- Kind: port
- Commit: this commit
- Found by: Lang 4, decision 0447 step R2 (the networking std modules on wasip2)
- Go reference: none
- Test: goldens dns_records and dns_no_tasks as wasm commands (TestWasmCommandGoldens)
- Timing: none
- Shipped in: unreleased

lookup_mx, lookup_txt, lookup_srv and lookup_addr, with the default
resolver (resolv.conf read through the command's filesystem) or a
Resolver's server, run in a command over its own DNS client on
wasi:sockets UDP, with TCP after a truncated answer and the native
error texts (decision 0447 R2i).

## 2026-11-03: fix: a program whose only network is std/dns keeps the scheduler and the shutdown; a lookup unwinds on cancellation and shows prior output

- Kind: fix
- Commit: this commit
- Found by: Lang 1 (the netpoller tick of gotchas/netpoll-wait-idle-wake-latency.md: a lookup only program had no scheduler)
- Go reference: none
- Test: golden dns_no_tasks; internal/llvm TestNativeSignalNetNoTasks/dns
- Timing: none (the lookup's poll still waits on the thread)
- Shipped in: since std/dns (decision 0417) through v0.35.0

Three faults of a lookup:

- A program with no task in its code whose only network was std/dns was
  built without the scheduler (std/dns's lookups were not counted as
  concurrency), so the first SIGTERM killed it at once (status 143), with
  no defer run and its buffered output lost, where core 7.5 cancels
  nothing at the first signal. A call of a std function that declares a
  network effect now counts as concurrency (ssa/call.xo net_waits).
- A lookup waited in one poll until its timeout (10 s with the default
  resolver options), so the second signal, or a canceled task, did not
  unwind it until then. The poll now waits at most 100 ms at a time and
  looks at the task's cancellation between.
- What the program printed before a lookup stayed in the output buffer
  until the lookup ended: the lookup now flushes standard output before
  it waits, as a task's park does.

## 2026-11-02: fix: the resolver configuration and hosts file were copied from freed memory

- Kind: fix
- Commit: this commit
- Found by: Lang 1's race stress (net_lookup_dial_timeout, the tls and websocket goldens built with --race on linux/arm64: a heap-use-after-free in dns.config and dns.hosts_text)
- Go reference: none
- Test: internal/llvm TestRaceStdNoUseAfterFree/net_lookup_dial_timeout (a --race build reports the use after free before the fix)
- Timing: none
- Shipped in: v0.35.0 (26eaef119, dns.xo) through v0.35.0

Reading /etc/resolv.conf (or Windows' adapters), the host name and the
hosts file into the resolver's cached block copied each from a value
whose last use was in the arguments of the copy itself
(`rt.mem_copy(..., raw.data(host), host.len())`), so the value was
released before the copy (gotchas/raw-address-outlives-bytes.md). The
copy read the right bytes as long as nothing reused the block in between;
a --race build, whose heap is the C library's, reported it on the first
lookup. The values now stay alive past their copies.

## 2026-10-30: port: native Windows record lookups

- Kind: port
- Commit: this commit
- Found by: decision 0417's stage 2 (the lead's queue, after decision 0426)
- Go reference: net/dnsconfig_windows.go (the adapters' DNS servers from GetAdaptersAddresses, fec0::/10 skipped), net/hosts.go (the system directory's drivers\etc\hosts)
- Test: goldens dns_records and net_lookup_dial_timeout on the Windows box (windows/amd64 native) and on arm64, LLVM and Go parity
- Timing: bench/perf/dns on the owner's windows/amd64 box, 11 rounds, median wall: mx (2,000 lookups) Xo 0.217 s, Go net.Resolver (PreferGo) 0.189 s, 1.15x (1.22x after the server setup, none: 0.041 s and 0.045 s); asking GetAdaptersAddresses on every lookup took 11.8 s (60x), so the configuration is kept 5 seconds as Go keeps it

lookup_mx, lookup_txt, lookup_srv and lookup_addr work on native
Windows: they use Xo's own DNS client, as on Linux and macOS, with the
DNS servers of the adapters that are up and the Windows hosts file
(as Go's pure Go resolver, unlike Go's default there, which asks the
system resolver: Windows' NRPT, DNS over HTTPS and cache are not
consulted), instead of returning Io "dns lookups are not supported on windows yet"
(decision 0427).

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
