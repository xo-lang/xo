kind: review
module: idna
result: pass
commit: 091fae48f
source: 43b4baadf7fdc861b507d754c17e8964dbf7cf90ebef483c2d75810202155e30
date: 2026-11-14
host: darwin/arm64 (probes and tests), linux/arm64 host 2 (fuzz, stress, race), host 1 (perf)
command: none (section 3 does not apply: no secret); read against knowledgebase/infrastructure/std-security-checklist.md
measure: 1 Go vulnerability entry for golang.org/x/net/idna (GO-2026-5026, matched: Xo refuses the same labels as v0.60.0); 3 inputs a peer may control, each bounded: decode at x/net's 1024 rune output cap, encode n log n since G1's fix (it was quadratic in a label's distinct non-ASCII runes, as x/net's still is: 64,000 runes 1.27 s, now 6 ms, x/net 20.4 s; 6e5bce7d2); 0 defects found; 0 fault sites (no or_fault, no unwrap; every index reads valid UTF-8 or a checked offset); 0 insecure defaults

Security review of std/idna (std/idna.xo, which the Go backend also
compiles, decision 0312; it calls std/norm, reviewed with norm) against
knowledgebase/infrastructure/std-security-checklist.md (decision 0616),
by Xo Lang 3. std/idna is a port of golang.org/x/net v0.60.0's idna:
UTS 46 mapping and validation, RFC 3492 punycode, the hyphen, joiner and
Bidi rules (spec/std.md 61, decision 0476). It reads names a peer can
send: a host in a URL or header that a program converts, and std's
httpproxy converts a non-ASCII request host before matching NO_PROXY
(httpproxy.xo:175-181, on the outgoing request's URL, which a program
may build from a peer's input).

## 1. Go's CVE history

The Go vulnerability database (vuln.go.dev, searched 2026-11-14 for
golang.org/x/net/idna; 182 stdlib entries read for siblings) has one
entry for the package:

- GO-2026-5026 (x/net/idna accepted punycode labels that decode to
  ASCII only, so two spellings of one name compared unequal; fixed in
  x/net v0.60.0, which std/idna ports). Applies, matched: to_ascii and
  to_unicode refuse such labels with A3 on every profile, as v0.60.0
  does. Probe (2026-11-14, native): three labels of that shape give A3
  on every profile, as Go's "invalid label"; an upper case `XN--` form
  gives A3, P1, A3 and no error on the four profiles where Go gives
  invalid label, disallowed rune U+0058, invalid label and no error; the fuzz harness
  (below) generates random `xn--` labels and compares every answer with
  v0.60.0.

Sibling classes, checked:

- Quadratic decoding (GO-2025-4009 encoding/pem, GO-2026-4977
  net/mail): G1 below.
- Integer overflow in punycode: decode and encode keep x/net's int32
  limits; madd (idna.xo:442-448) refuses any step past 2^31 - 1, and its
  product (a digit up to 35 or a code point gap up to 0x10FFFF times a
  weight or count at most 2^31) fits an Int. A label past them is A3,
  as in Go.
- Normalization cost: std/idna calls std/norm, whose stream-safe fix
  (3a99cbf9e, norm's review) bounds a run of combining marks.

## 2. Limits on hostile input

| Input a peer may control | Limit | Where | Test |
|---|---|---|---|
| A name to to_ascii or to_unicode | Registration (dns_len): each label 1 to 63 bytes, the name at most 253, after mapping; Lookup: no length limit, as x/net | idna.xo process (1018-1128; labels :1110-1112, name :1118-1124) | fuzz TestIdnaFuzz (names past 253 bytes and labels past 63, on the four profiles, against x/net v0.60.0); corpus testdata/corpus/idna |
| A punycode label to decode (`xn--`) | at most 1024 output runes (x/net's cap, idna.xo:530-532, A3 past it), int32 overflow refused (A3); one pass of digits and one insertion into at most 1024 runes per decoded rune | idna.xo decode (476-551), madd (442) | std/idna_test.xo: 1024 runes round trip, 1025 are x/net's invalid label; longer labels are refused at the cap or the overflow check in about 1 ms (probe, 193 KB and 832 KB labels) |
| A label to encode | none on Lookup, as x/net; time n log n in the label's runes since G1's fix (labels over 64 runes take encode_long) | idna.xo encode, encode_long | std/idna_test.xo: 6 labels of 65 to 464 runes against x/net, and 64,000 distinct runes (x/net's output length) |

## 3. Constant time

Not applicable: std/idna handles no secret.

## 4. Fault behaviour

No defects found. The fuzz harness (TestIdnaFuzz, 8 checks per name:
to_ascii and to_unicode on the four profiles) and the stress program
(std/evidence/idna/stress, 64 names against v0.60.0) found no
difference from x/net.

### G1 (fixed beyond Go, 6e5bce7d2): punycode encode was quadratic in distinct runes

encode scanned the whole label once for each distinct non-ASCII code
point (RFC 3492 section 6.3, x/net's punycode.go encode), so a label of
n distinct non-ASCII runes cost n squared steps. Lookup, the profile
to_ascii uses by default and httpproxy uses, has no length limit, so a
caller who passes a peer's long name paid it. Mac, native, one label of
distinct CJK runes: 16,000 runes 260 ms, 64,000 runes 1.27 s; x/net
v0.60.0 on the same input 2.2 s and 20.4 s (Registration refuses the
name only after encoding it: Xo 1.29 s, x/net 13.1 s). Shared with Go,
with no vulndb entry.

Fix: a label over 64 runes encodes through encode_long, which takes the
runes in order of code point then position and counts the runes below
the current code point between two emissions with a Fenwick tree over
positions, so it emits the same deltas, with the same int32 overflow
checks, in n log n: 64,000 runes 6 ms (x/net 20.4 s). Shorter labels
keep RFC 3492's loop (faster at that size). Tests: std/idna_test.xo
(x/net's answers for 6 long labels, and the 64,000 rune label's
length); TestIdnaFuzz on arm64, llvm and go; TestStdCorpora idna.

### Fault sites

- None. idna.xo has no or_fault, unwrap or fault call. rune_at
  (914-926) reads up to 3 bytes past p only for a lead byte of that
  width in a Str, which is valid UTF-8. Table lookups (lookup, 230-255)
  index generated tables by a code point at most 0x10FFFF; BLOCKS (to
  U+10000 since 5a23d3a84) has 1,025 entries and a rune below 0x10000
  reads entries r >> 6 and (r >> 6) + 1, at most 1,024.

## 5. Defaults

- to_ascii and to_unicode default to Lookup, x/net's default for names
  from a user or peer (non-transitional, STD3 rules, hyphen, joiner and
  Bidi checks, no DNS length check), as Go.
- No option weakens a check beyond x/net's profiles.

## Sign-off

Second reader: a subagent of Xo Lang 3 (2026-11-14), reading the code
separately: needs changes (decode's 1024 rune cap was the real bound and
the long-label probe measured a refusal; the httpproxy claim; no test at
G1's bound; a line number), all made above; G1's fix came after its
read; it confirmed encode's quadratic shape, madd's products, and
decode's linear cost under the cap.
