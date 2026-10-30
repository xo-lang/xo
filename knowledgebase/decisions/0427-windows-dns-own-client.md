# 0427: native Windows DNS lookups use Xo's own client

Status: Accepted (Xo lang 5, 2026-10-30; the lead's queue item 2 after decision 0426). Partly supersedes decision 0417's stage 2 plan (binding DnsQuery_W).

## Context

Decision 0417 gave native Windows `Err(Io("dns lookups are not supported on windows yet"))` for lookup_mx, lookup_txt, lookup_srv and lookup_addr, with a stage 2 that binds DnsQuery_W. Std/native/llrt/dns.xo already speaks DNS over UDP and TCP on Linux and macOS, and its sockets layer (llrt sock.xo) already runs on Winsock.

## Decision

Native Windows record lookups (lookup_mx, lookup_txt, lookup_srv, lookup_addr) run dns.xo's own client, as on Linux and macOS, configured the way Go's pure Go resolver (GODEBUG=netdns=go) is on Windows (dnsconfig_windows.go), unlike Go's default on Windows, which asks the system resolver:

- servers: the DNS servers of the adapters that are up, from GetAdaptersAddresses (IPHLPAPI; std/native/llrt/winadapters.xo), IPv4 and IPv6, skipping the fec0::/10 site local servers Windows lists when none is set; 127.0.0.1 and ::1 when no adapter names one;
- the rest of resolv.conf's defaults: the search domain from the host's name, ndots 1, timeout 5 seconds, two attempts;
- the hosts file: the system directory's drivers\etc\hosts (GetSystemDirectoryW), cached and rechecked every 5 seconds like /etc/hosts; no nsswitch.conf (files, then DNS);
- the transport: Winsock sockets made non-blocking with ioctlsocket, waited on with WSAPoll, UDP with TCP for a truncated answer.

## Consequences

- One DNS client on every native OS: the same errors ("no such host", "server misbehaving", Timeout), the same invalid record handling and the same order, so the dns_records golden's want_windows.txt is the same as its want.txt.
- Windows' own resolver features are not consulted: the Name Resolution Policy Table (DirectAccess, split DNS of some VPNs and group policies), DNS over HTTPS set in Windows, and the system cache. This is unlike Go's default on Windows, which uses the system resolver; Go's pure Go resolver has the same gap. The spec says so.
- The configuration is kept 5 seconds, as Go keeps it, in resolv.conf's cache (static words 91 and 92): GetAdaptersAddresses takes milliseconds, and asking it on every lookup made bench/perf/dns mx 60x Go's time on Windows (1.15x with the cache).
- lookup_host on native Windows is unchanged.

## Alternatives

- DnsQuery_W (decision 0417's stage 2 plan; Go's default for LookupMX, LookupTXT and LookupSRV on Windows): uses the system's resolver and its policies, but needs a binding of DNS_RECORD lists per type, gives Windows' error codes instead of Go's DNSError texts, and reaches a Resolver's own server (the goldens' and every test fixture's) only through DnsQueryEx's server list, without the port the fixtures listen on, so the Resolver{server} path would need dns.xo anyway.
- Keep `Err(Io)` on Windows: leaves the exporter and any MX or SRV user broken there.
