# std/httpproxy changes

## 2026-11-16: perf: a request sent direct no longer copies the proxy URL

- Kind: perf
- Commit: this push
- Found by: task #103 (compiler/llrt perf levers from #88: httpproxy route 1.73 of Go)
- Go reference: x/net/http/httpproxy proxyForURL, which returns its *url.URL pointer only when a proxy applies
- Test: the std/httpproxy tests (NO_PROXY by name, suffix, CIDR, IP and port; https and http proxies; CGI); bench/perf/httpproxy_rules twincheck output matches Go
- Timing: bench/perf/httpproxy_rules route on the Mac, 10x loop: 5.86 s to 4.82 s (checksum unchanged); host 1 --ab (n=3) route: 552.3 to 482.4 ms, 0.873 (0.862 to 0.879), max_rss the same
- Shipped in: not yet released

Rules.proxy_for copied the proxy URL for every request and dropped it again when NO_PROXY sent the request direct. It now checks the scheme and NO_PROXY first and copies the URL only when it is the answer.

## 2026-11-16: perf: a host name skips the address parse

- Kind: perf
- Commit: this push
- Found by: task #88 (performance Early to Proven), the module's first Go twin (bench/perf/httpproxy_rules against x/net/http/httpproxy v0.60.0: route 2.053 on the Mac)
- Go reference: x/net/http/httpproxy useProxy, whose net.ParseIP returns nil for a name without building an error
- Test: the std/httpproxy tests (NO_PROXY by name, suffix, CIDR, IP and port); bench/perf/httpproxy_rules twincheck output matches Go
- Timing: bench/perf/httpproxy_rules on the Mac, Xo over Go: route 2.053 to 1.731 (twincheck -time 3)
- Shipped in: not yet released

use_proxy parsed every host as an IP address, and netip.parse_addr formats its error text for each host name. It now parses only a host that can be an address (a colon, or only digits and dots).

## 2026-11-12: feature: std/httpproxy, Go's ProxyFromEnvironment rules

- Kind: feature
- Commit: this push
- Found by: decision 0617 B (the owner's high group of std features, 2026-11-12)
- Go reference: golang.org/x/net/http/httpproxy proxy.go (v0.47.0, the copy go1.26.1 vendors for net/http)
- Test: the httpproxy corpus on go, arm64, llvm (48 seeded HTTP_PROXY, HTTPS_PROXY and NO_PROXY settings and 1,920 request URLs, every answer equal to net/http's ProxyFromEnvironment run with that environment); std/httpproxy_test.xo ports Go's proxyForURLTests, UseProxyTests, TestInvalidNoProxy and TestAllNoProxy
- Timing: none (string matching once a request, beside a connection)
- Shipped in: not yet released

A new merged module: Config, from_env, proxy_func, proxy_from_env and parse_proxy. A proxy of another scheme than http or https is ignored, and under CGI an http request connects directly where Go returns an error. spec/std.md 82. The Client's with_proxy and with_proxy_for follow.
