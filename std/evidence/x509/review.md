kind: review
module: x509
part: certificate and PEM parsing, chain building, name constraints, policies, host names (std/x509.xo, std/native/x509.xo, std/native/llrt/x509cert.xo, x509verify.xo, tlsname.xo's PEM and host name code, cbyte.xo), std/asn1.xo, PKCS #12 (pkcs12.xo)
result: pass
commit: df9112a76
source: bfc4fc502052681b685fe9eb23baef48faa325e32adec1cf843881b3dcf4ce0c
date: 2026-11-13
host: darwin/arm64 (probes against go1.27.4), linux/arm64 host 1 (gates)
command: read against knowledgebase/infrastructure/std-security-checklist.md; Go vulnerability database sweep (crypto/x509, encoding/pem, encoding/asn1 and crypto/tls name handling entries), each compared with go1.27.4's source and checked by a probe; probes under the session scratchpad (x509nc, x509lim, x509nq, x509pool, pemq, x509host, x509ip, x509ext, x509pg, x509em, asnw, x509dd)
measure: 25 Go advisories checked: 10 affected the native backends, 9 fixed with go1.27.4's behaviour and a test that fails without the fix; 8 more defects found by the readers and fixed (2 constraint bypasses: an IPv4-mapped SAN and constraints with host bits; 5 quadratic costs; 1 timing leak); 1 open (GO-2022-0171, darwin trust settings, owner go needed to test); 0 fault sites reachable from input in 1000 PEM and 1000 certificate fuzz corpus cases

Security review of std/x509 for decision 0616, by Xo Lang 3. First
reader: a read-only subagent's advisory sweep; second reader: another
read-only subagent, whose findings are section 6. Every claim was checked
by a probe against Go 1.27.4 (toolchain go1.27.4 darwin-arm64) before a
fix, and every fix has a test that fails without it.

The Go backend uses its Go toolchain's crypto/x509. On go1.26.1 it lacks
the fixes of GO-2026-4866, 4946, 4947, 5037, 5972 and 6809, and accepts
the IPv4-mapped SAN of section 6; moving the toolchain to the newest
release is with the owner (before 0.51).

## 1. Go's CVE history

| Advisory | Defect | Xo before | Fix and test |
|---|---|---|---|
| GO-2026-4866 (CVE-2026-33810) | an excluded DNS constraint missed a wildcard SAN in another case | affected | f95f1fad2; std/native/x509_test.xo A1 to A6 |
| GO-2025-4009 (CVE-2025-61723) | PEM quadratic in BEGIN lines | 1 MB: 45 s; differed from pem.Decode on 156 of 1000 texts | 416680e84 (pem_next is Go's pem.Decode); pemfuzz corpus, 1 MB linear test |
| GO-2026-5972 (CVE-2026-33818) | ASN.1 nesting unbounded | stack exhaustion | 35f40bbf8 (10000 levels); asn1_test.xo 9999, 10000, 200000 |
| GO-2026-4947 (CVE-2026-32280) | chain building past the signature check limit | worked past it | afd688ebf (every candidate counted, the limit ends the search); x509_test.xo D3, D4, TestPathologicalChains port (6adefd27a) |
| GO-2026-4946 (CVE-2026-32281) | policy graph pruned per mapping | per mapping | afd688ebf (once per certificate) |
| GO-2025-4155 (CVE-2025-61729) | host name error text quadratic | every name listed | 5a3af268f (100 names); HOST_CASES |
| GO-2026-5037 | host split per SAN | per SAN | 5a3af268f (once) |
| GO-2025-4007 (CVE-2025-58187) | name constraints quadratic | 4000 by 4000: 62 ms (Go 1.8 ms) | 598637444 (sorted sets, maps): 1 ms |
| GO-2026-6809 | distrusted intermediates accepted | no list | b9ff07cfc (x509cert.spki_distrusted, x509verify, and the macOS framework's chain) |
| GO-2026-4599 (CVE-2026-27137) | email constraints of one kind | not affected | 615 cases equal to go1.27.4 (78 in x509_test.xo); the error text for an empty host URI fixed in d9bc11905 |
| GO-2026-4600 (CVE-2026-27138) | fault on malformed names in constraint checks | not affected | the same 615 cases: no fault, Go's errors |
| GO-2022-0171 | darwin roots without their trust settings | system_roots lists the default anchors | OPEN: documented in spec/std.md system_roots; the fix (anchors with a Deny setting dropped) needs a throwaway keychain test, which waits for the owner's go (ADR 0584) |

Not affected (first reader's evidence, spot checked by probe): GO-2022-0223,
0191, 0229, 0433, 0434, 0535, 2598, 3373, 3421, 3749, 4011, 4013, 4175.
The first reader's claim that mailbox domains with '@' were mishandled
was disproved by probe. Its claim about IP constraints with host bits was
also marked disproved at first, wrongly: the probe used Go-made
certificates, which Go's encoder always masks; the second reader caught
it (section 6, item 4).

## 2. Limits on hostile input

A TLS peer sends certificates to every Xo server asking for client
certificates and to every Xo client; files and HTTP bodies reach
parse_pem and asn1.parse.

- PEM: linear (Go's Decode: the first END line, the last BEGIN before it).
- ASN.1 (std/asn1): 10000 levels (Go's maxDecodeDepth); read by offset, so
  linear on every backend, WebAssembly and Go included (3f456d5c7: a
  slice per level copied there, 800 KB of nesting ran wasm out of memory).
  9999 levels fit wasm's stack.
- Certificate parser (x509cert.xo, cryptobyte style): no recursion;
  duplicate extensions and policies found with maps (97ce69bc8; 30000
  extensions: 453 ms before, 4 ms after, Go 5 ms).
- Chain building: 100 signature checks counting every candidate (Go's
  maxChainSignatureChecks); candidates deduplicated among each
  certificate's parents (284287b6c; 8000 identical intermediates: 7 ms).
- Name constraints: sorted sets and maps (n log n); IP subtrees masked first (df9112a76).
- Policy graph: insertion ordered maps (4f7eed8bb; 3000 policies and
  3000 mappings: 29 ms before, 1 ms after, Go 1.9 ms).
- Host name errors: 100 names at most.
- RSA keys over 8192 bits refused (x509cert.rsa_too_large, Go's).

## 3. Constant time

Certificates, chains and names are public. The one secret comparison is
the PKCS #12 MAC, now ct.bytes_equal as Go's hmac.Equal (97ce69bc8; it
was `!=`). Private key parsing goes to std/crypto's key types, reviewed
with std/crypto.

## 4. Fault behaviour

x509fuzz (Go-made certificates from random templates and PKITS, half
mutated): no fault; parse returns Err with Go's text, on arm64, llvm and
go. pemfuzz: 1000 texts, no fault. The constraint cases of section 1: no
fault. Follow-up runs, recorded as their own evidence files when they
land: fuzz 10^6, race, stress (std/evidence/x509/stress/main.xo), perf.

## 5. Defaults

- verify with empty key_usages checks ServerAuth, as Go's nil.
- dns_name "" checks no host, as Go's empty DNSName; std/tls sets it from
  the server name.
- SHA-1 and MD5 signatures refused with Go's text; an unhandled critical
  extension refused; ML-DSA keys are not implemented and fail closed.
- The distrust list is always on, as Go's (no switch).
- system_roots: GO-2022-0171 above.

## 6. Second reader's findings

1. An IPv4-mapped IPv6 SAN (::ffff:10.0.0.5) matched an IPv4 host but was
   checked against the IPv6 constraints, so an excluded 10.0.0.0/8 missed
   it. Confirmed by probe (go1.26.1 shares it); fixed in 97ce69bc8 as
   go1.27.4: such an address in a SAN or an IP constraint is a parse
   error. x509_test.xo E3, E4.
2. Quadratic scans before any signature check (extension and policy
   duplicates) and after one (the policy graph): fixed, section 2.
3. PKCS #12 MAC comparison not constant time: fixed, section 3.
4. IP constraints with host bits were not masked before the sets sorted
   and searched them: an excluded 10.0.0.255/24 next to 10.0.0.1/32 did
   not exclude 10.0.0.5. The first probes missed it because Go's
   CreateCertificate masks what it encodes. Confirmed with hand-made
   constraint DER (go1.26.1 shares it); fixed in df9112a76 as go1.27.4's
   newIPNetConstraints. x509_test.xo H1 to H8.
5. Notes kept: in_chain is stricter than Go's alreadyInChain (it refuses
   more, never less); parse_ip refuses zones (fails closed); the macOS
   distrust check's text when the framework's chain does not parse
   differs from Go's (both refuse).
6. Not security: the public x509.verify parses every root and
   intermediate again per call (half its time with the 150 macOS roots);
   std/tls passes parsed roots and does not. Keeping the parsed form needs
   an API decision (a pool type).

## Sign-off

Signed by a second agent (Xo Lang 3 subagent), 2026-11-13: read at 79278fb32 (a lang3-x509review branch commit before its rebase onto main), with the fixes 97ce69bc8, 3f456d5c7, 4f7eed8bb, 284287b6c, d9bc11905 and df9112a76 (pushed as df9112a76 after a rebase; only a test label changed). I checked these against go1.27.4's crypto/x509 (parser.go, verify.go, constraints.go, x509.go, root_darwin.go), encoding/pem, encoding/asn1, cryptobyte and x/crypto pkcs12: x509cert.xo (the parser, extensions, SANs, name constraints, policies, the signature algorithm checks), x509verify.xo (chain building and its signature check limit, alreadyInChain, the constraint sets, the policy graph, the distrust list), tlsname.xo (pem_next, the host name match and its error text), tlscert.xo (apple_chain_distrusted), std/asn1.xo and pkcs12.xo. My findings, all fixed: an IPv4-mapped IPv6 SAN escaped IPv4 excluded ranges, now a parse error with Go's text in a SAN and in an IP constraint (97ce69bc8; E3, E4); IP constraints with host bits were sorted and searched unmasked, which the first probe missed because Go's CreateCertificate masks what it encodes (x509.go:1401-1402), now masked as Go's newIPNetConstraints (df9112a76; I decoded H1 to H8, confirmed their DER carries host bits, and their verdicts match my trace of the masked sets); duplicate extensions and policies were found by scans quadratic in their number before any signature check, now maps; the policy graph's list scans are insertion ordered maps (4f7eed8bb), and since valid policies are unique per stratum the lookups and results match the old code and verify.go:1065-1192; the PKCS #12 MAC compared with `!=`, now ct.bytes_equal as Go's hmac.Equal; asn1 sliced each nesting level, which copies on the WebAssembly and Go backends, now reads by offset (3f456d5c7) with every byte read bounded by lim and the depth cap unchanged at 10000; three wrong hashes in section 1's table were corrected and an OID figure without a committed source dropped. I agree with the not-affected verdicts I spot-checked (GO-2023-2598, GO-2024-3373, GO-2025-3749), and the review's figures match the commit messages and std/changes/x509.md and asn1.md. Notes kept, none a bypass: in_chain is stricter than Go's alreadyInChain; parse_ip refuses IPv6 zones (fails closed); the macOS framework chain's parse failure text differs from Go's (both refuse); MD5 and SHA-1 signatures are refused with Go's text; ML-DSA certificates fail closed. The result is the review's; the fuzz 10^6, race, stress and perf runs of section 4 are follow-ups with their own evidence files.
