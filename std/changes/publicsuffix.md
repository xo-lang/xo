# std/publicsuffix changes

## 2026-11-12: feature: std/publicsuffix, the public suffix list

- Kind: feature
- Commit: this push
- Found by: decision 0617 B (the owner's high group of std features, 2026-11-12)
- Go reference: golang.org/x/net/publicsuffix list.go and its generated tables (x/net v0.60.0, the list of 2026-02-06)
- Test: the publicsuffix corpus on go, arm64, llvm (3,058 hand picked and seeded domains, the suffix, ICANN flag and eTLD+1 or error text equal to x/net's); std/publicsuffix_test.xo ports Go's publicSuffixTestCases and eTLDPlusOneTestCases
- Timing: none yet (a binary search per label over a table)
- Shipped in: not yet released

A new merged, pure module: public_suffix, effective_tld_plus_one, VERSION; scripts/genpsl writes its tables from x/net's. spec/std.md 83. The cookie jar's use of it follows.
