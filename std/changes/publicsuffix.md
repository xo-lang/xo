# std/publicsuffix changes

## 2026-11-15: perf: no address parse for a host name

- Kind: perf
- Commit: this push
- Found by: the module's first Go twin (bench/perf/publicsuffix, perf.md: suffix 1.566 of x/net's wall)
- Go reference: golang.org/x/net/publicsuffix PublicSuffix, which has no address check
- Test: the publicsuffix corpus on go, arm64, llvm (its IPv4 and IPv6 cases among them); std/publicsuffix_test.xo
- Timing: bench/perf/publicsuffix on the Mac, Xo over Go: suffix 1.490 to 0.781, etld1 1.276 to 0.770 (twincheck -time 3)
- Shipped in: not yet released

public_suffix parsed every domain as an IP address before the table search. It now parses only a text that can be one (a colon, or a last byte that is a digit): a host name's last label has neither.

## 2026-11-12: feature: std/publicsuffix, the public suffix list

- Kind: feature
- Commit: this push
- Found by: decision 0617 B (the owner's high group of std features, 2026-11-12)
- Go reference: golang.org/x/net/publicsuffix list.go and its generated tables (x/net v0.60.0, the list of 2026-02-06)
- Test: the publicsuffix corpus on go, arm64, llvm (3,058 hand picked and seeded domains, the suffix, ICANN flag and eTLD+1 or error text equal to x/net's); std/publicsuffix_test.xo ports Go's publicSuffixTestCases and eTLDPlusOneTestCases
- Timing: none yet (a binary search per label over a table)
- Shipped in: not yet released

A new merged, pure module: public_suffix, effective_tld_plus_one, VERSION; scripts/genpsl writes its tables from x/net's. spec/std.md 83. The cookie jar's use of it follows.
