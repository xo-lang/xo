# std/httpproxy changes

## 2026-11-17: perf: a request's host and port are no longer joined and split again

- Kind: perf
- Commit: this push
- Found by: task #103 (compiler/llrt perf levers from #88: httpproxy route, 1.665 of Go on host 1 at a2dcc9bc7)
- Go reference: x/net/http/httpproxy proxyForURL, which joins canonicalAddr and splits it in useProxy
- Test: std/httpproxy_test.xo "proxy_for's one pass over the host agrees with use_proxy(canonical_addr(u))"
- Shipped in: not yet released

proxy_for reads the request URL's host once (a byte above 0x7f, a bracket, a colon, or anything but digits and dots) and asks the NO_PROXY rules with the host and port it has, where it built `host:port` and split it again with up to four searches; a host with a bracket or a non-ASCII byte still takes the join and split, for SplitHostPort's errors and the IDNA form. bench/perf/httpproxy_rules route on the Mac: 1.149 of Go's wall (twincheck -time 9), from 455 ms to 380 ms a run.

## 2026-11-17: fix: a NO_PROXY CIDR mask may have leading zeros

- Kind: fix
- Commit: this push
- Found by: task #115, TestHttpproxyFuzz seed 4 (NO_PROXY "0.0.0.0/00" bypassed the proxy in Go, not in Xo)
- Go reference: x/net/http/httpproxy config.init, whose net.ParseCIDR reads the mask with dtoi (leading zeros allowed)
- Test: std/httpproxy_test.xo "a NO_PROXY CIDR mask may have leading zeros, as net.ParseCIDR reads it" (fails before)
- Shipped in: not yet released

NO_PROXY entries such as "10.0.0.0/08" were not read as CIDR blocks, since netip.parse_prefix refuses a mask with leading zeros; they now match as in Go.

## 2026-11-17: test: a differential fuzz against x/net/http/httpproxy

- Kind: test
- Commit: this push
- Found by: task #115 (fuzz evidence for Proven)
- Go reference: golang.org/x/net v0.62.0 http/httpproxy Config.ProxyFunc
- Test: TestHttpproxyFuzz (internal/stdtest/httpproxyfuzz_test.go, oracle scripts/stdcorpus/httpproxyoracle, Xo side scripts/httpproxydiff/xo): seeds 2 to 5, 20,000 generated settings and request URLs each (15,265 to 15,361 compared after the documented departures), equal on arm64 and llvm
- Shipped in: not yet released

Generated HTTP_PROXY, HTTPS_PROXY, NO_PROXY and CGI settings with mutated names, ports, IPv6 literals, CIDR blocks and IDNA hosts; the cases Go answers by its documented departures (an error, a scheme other than http and https, the http:// retry) are skipped. It found the leading zero mask above.

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
