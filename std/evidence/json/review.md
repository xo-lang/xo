kind: review
module: json
result: pass
commit: e36a1deed
source: 57a1aa4d4abc6193c4bc0845d6619bd8f3fee2e3c5b0a9911ee742d760a72820
date: 2026-11-13
host: darwin/arm64 (probes), linux/arm64 host 2 (fuzz, race, stress, perf)
command: read against knowledgebase/infrastructure/std-security-checklist.md; probes in this file's tests
measure: 0 Go vulnerability entries for encoding/json (8 sibling decoder classes checked, none shared); 3 limits tested at and past them, 5 inputs with no limit (4 linear, indent's output input x depth as Go); 1 superlinear cost found and fixed (the error path, 7cfd51187); constant time not applicable; 1 fault site found (F1, fixed); 4 defaults differ from Go's v1 and are recorded (HTML escaping R1 owner decided, case-sensitive names and missing fields as v2/by design, typed fields by Xo's grammars)

Security review of std/json (std/json.xo, std/native/json.xo,
std/native/llrt/jsontext.xo, the derived Json decoders of compiler/ssa
json.xo, and the Go backend's std/rt json*.go) against
knowledgebase/infrastructure/std-security-checklist.md (decision 0616), by
Xo Lang 3. std/json is optional for 0616's review kind; the lead asked for
it (2026-11-13) because it parses the body of every HTTP API. A first
pass on 2026-11-12 by a second agent found F1 to F10: F1 to F6 are named
below with their commits; F7 was a wording fix in that pass's notes (the
slow float path's 800 digit decimal is Go's), F8 the depth tests
(ab7910be3), F9 U+2028 and U+2029 in JsonWriter.raw with escape_html
(d523e77d2), F10 TokenReader skip and value errors (1daadd2b4).

## 1. Go's CVE history

The Go vulnerability database (vuln.go.dev, index downloaded 2026-11-13;
182 of the 187 stdlib entries fetched, the 5 others not served) has no
entry whose affected package is encoding/json. The entries for Go's other
decoders name the defect classes a JSON parser can share; each is checked
against std/json:

- Stack exhaustion from nesting: GO-2022-0521 (CVE-2022-28131) and
  GO-2022-0523 (CVE-2022-30633) encoding/xml, GO-2022-0526
  (CVE-2022-30635) and GO-2024-3106 (CVE-2024-34156) encoding/gob,
  GO-2026-5972 (CVE-2026-33818) encoding/asn1, GO-2026-6088
  (CVE-2026-56859) encoding/xml, GO-2021-0347 (CVE-2022-24921) regexp.
  Not shared: the scanner (jsontext.xo parse) stops at depth 10000
  (MAX_DEPTH, jsontext.xo:88, encoding/json's maxNestingDepth) before any
  recursive reader runs. The Go backend runs its own copy of Go's
  scanner with the same limit (std/rt/jsonscan.go:110, jsMaxNestingDepth),
  behind a fast path that hands deeper texts to it past depth 200
  (std/rt/jsonparsefast.go:31). A TokenReader fed by `write`
  (std/native/json.xo:1261 write, :1096 pull) keeps its open containers in a List, not on the call
  stack: depth there costs memory linear in the input and nothing else,
  as Go's Decoder.Token (whose token stack has no limit either).
  Test: golden json_depth_limits, 10000 accepted and 10001 refused through
  parse_value (arrays, objects), decode, decode_go and from_value into a
  recursive derived type, a TokenReader's skip and value, and the stream
  Decoder, on the main task and in a spawned task (a smaller stack), on
  arm64, LLVM and Go (ab7910be3, F8).
- Quadratic time on hostile input: GO-2025-4009 (CVE-2025-61723)
  encoding/pem. Shared, fixed: F2 duplicate keys hashed unseeded
  (40fa8e13a), F3 Go backend 2^depth on nested duplicate keys
  (610fa79b3), F4 O(depth x size) range checks (5721d1d7b), F5 the stream
  Decoder and TokenReader rescanning unfinished input per write (59d472e6a,
  b91d61086), F6 error paths built per level (3b43697f0, a42c8f224). Each
  has a test: goldens json_key_flood (F2), json_dup_nested (F4),
  json_stream_pieces (F5), std/rt/jsonvalue_dup_test.go (F3),
  std/native/json_test.xo (F6 native; the Go backend's F6 fix a42c8f224 is
  covered by the Go parity goldens).
- Memory exhaustion from a claimed length: GO-2025-4011 (CVE-2025-58185)
  encoding/asn1, GO-2021-0142 (CVE-2020-16845) encoding/binary. Not
  shared: JSON has no length prefixes; every allocation follows bytes
  already read. A 3.2 MB object of 200,000 keys parses in 33 ms with a
  55 MB peak footprint (darwin/arm64, this review's probe); the bound is
  the input's size, which the caller caps (section 2).
- Infinite loop on input: GO-2021-0234 (CVE-2021-27918) encoding/xml. Not
  shared: the scanner advances at least one byte per step; 2,000,000
  fuzz cases per run (fuzz.md) found no hang (a case over 10 s fails).
- Numeric parsing panics: GO-2021-0069 (CVE-2020-28362), GO-2021-0242
  (CVE-2021-33198), GO-2021-0317 (CVE-2022-23772), GO-2022-0537
  (CVE-2022-32189) math/big. Not shared: std/json has no big number type;
  numbers go through num.parse_float_at (strconv's Eisel-Lemire, then its
  800 digit decimal) or the Int scan. Probes: 1e999999999 and
  1e99999999999999999999 are refused ("cannot unmarshal number ... into
  Float"), 1e-999999999 is 0, a 1,000,001 digit integer is refused as an
  Int and as a Float, a 1,000,001 digit number with e-1000000 is 0 on both
  parse_value and decode, as Go's ParseFloat answers (its 800 digit
  truncation); 2^63 into Int is refused.

## 2. Limits on hostile input

| input a peer controls | limit | at the limit | past it |
| nesting depth of a text | 10000 (MAX_DEPTH, llrt/jsontext.xo:88, checked at :400) | json_depth_limits: ok | `exceeded max depth` |
| one stream Decoder value | max_value, 64 MiB by default (std/json.xo:94; checked std/native/json.xo:772-823) | json_depth_limits: 16 of 16 ok | `value exceeds max_value` |
| one TokenReader token | max_token, 64 MiB by default (std/json.xo:128; checked std/native/json.xo:1171, 1207) | json_depth_limits: 16 of 16 ok | `token exceeds max_token` |
| a whole text to parse_value, decode | none in std/json: the caller's; std/http caps a request body at 64 MiB (`Server.with_max_body`, XO_HTTP_MAX_BODY) | http goldens | 413 |
| object keys, duplicate keys | none: no count limit; cost linear (seeded hash, F2) | json_key_flood: 16384 keys colliding in FNV-1a's low 32 bits | no limit to pass |
| indent's output | none: each line carries depth copies of the prefix, so output is input x depth (a 1 MB text 9999 deep is about 10 GB in), as Go's Indent | the caller's: indent text it trusts or has capped | no limit to pass |
| error path of a number out of range | linear since 7cfd51187 (joined once; it copied the path below at every level, about 50 GB for a 10 MB text of 10000 levels of 1000 byte keys; found by the second reader) | golden json_float_range (that text: a 10010001 byte path) | no limit to pass |
| number length and exponent | none: linear scan; the exponent saturates at 10000 (llrt/num.xo:940) | golden json_float_range (this review's probes; `1` and 10^6 zeros then `e-1000000` is 0 through parse_value there, through decode by a probe only) | refused or 0, as Go |

Goldens named here are under internal/llvm/testdata/golden/ and run on
arm64, LLVM and the Go backend (parity). json_depth_limits also decodes
derived types whose Option shares a JSON node with its value (two decoder
frames a level: `struct L { n: L? }`, and `List[M?]`), compact and
indent, at 10000 and 10001 levels on the main task and in a spawned task
(1a5a2890c, the second reader's item).

The rows without a limit have no "past it" case: the cost is linear in
the input (indent: in its output), which the caller caps (std/http's 64 MiB body).

## 3. Constant time

Not applicable: std/json handles no secrets. A JSON text holding a secret
(a token in a body) is parsed in time that follows its bytes, as Go's.

## 4. Fault behaviour

- Found and fixed: F1, decode_go faulted on a type error under a non-ASCII
  key (go_final sliced the path one byte at a time, a cut inside a UTF-8
  sequence; 73587d7c4, 8416acd95). Test: std/native/json_test.xo (UTF-8 keys under a type error).
- Readers of a text index the tape only at offsets the scanner wrote for
  a text it accepted (value_at, the derived decoders, members, str_at); a
  refused text never reaches them, and their recursion is bounded by
  MAX_DEPTH (section 1).
- A Value built in code is not a peer's input, but from_value reads it
  as decode reads a text: it builds its own tape (value_tape, json.xo),
  which now refuses a Value past MAX_DEPTH with the scanner's error, as
  the Go backend already did through its text (found by the second
  reader; fixed in ce7445573, std/changes/json.md). encode, == and
  JsonWriter.value walk a code-built Value at any depth without a fault
  (probed to 1,000,000 levels on the main task and in a spawned task);
  golden json_depth_limits checks all four at 10001 on arm64, LLVM and Go.
- Strings a peer sends into typed fields reach other modules' parsers:
  a Time field std/time's RFC 3339 parse (llrt/jsontext.xo:623
  time_at), Bytes base64 (llrt/jsontext.xo:1787), Duration its text
  form. The corpus jsondecode pins them against encoding/json's twins;
  the fuzz harness's typed cases cover Int, Float and Str fields, and
  gain Time, Bytes and Duration fields with this review (rerun in
  fuzz.md). std/time's own parse is fuzzed by its own Proven pass.
- Other modules that turn a peer's text into JSON (std/toml's to_json,
  std/yaml) build a Value and hand it to std/json; the second reader
  asked whether a deep TOML document could fault there: a probe of
  toml.to_json on 1,000,000 nested arrays ran without a fault, so not
  a defect (the probe's observation; the mechanism is not traced here). A deeper Value reaching from_value is refused (above).
- Cosmetic: go_final (std/native/json.xo:44) drops a `[...]` inside a key from the
  Go field name in decode_go's error text; it does not fault.
- Evidence: fuzz.md, 1,000,000 cases per backend on arm64 and LLVM
  against encoding/json, 0 faults, 0 hangs, 0 mismatches at this source
  hash; an earlier run found, at 615,000 cases, a Duration text of 20 or
  more digits read as a small value (std/time's parse_duration wrap,
  fixed in 3dbf9d87b with Go's cases; a typed Duration field reached
  it). race.md and stress.md for shared use; perf.md: 25 workloads
  against Go's encoding/json, worst Xo / Go 0.980.

## 5. Defaults

- Unknown object members are ignored by decode, as encoding/json's
  Unmarshal (no DisallowUnknownFields default).
- A duplicate key keeps its last value, as Go.
- Member names match field names exactly (case-sensitive), as Go's
  encoding/json/v2; Go's v1 Unmarshal also matches a different case.
  `derive Json(match: fold)` (decision 0550) folds case; `decode_go`
  does not fold either. Safer than v1 (no two spellings of one field);
  recorded here as a difference from v1.
- `decode` refuses a missing field without a default ("missing field");
  Go's Unmarshal leaves it zero. `decode_go` follows Go. A difference
  by design (spec/std.md 5), recorded.
- Typed fields read other modules' grammars as Xo's spec gives them, not
  Go's leniencies: a Time must be RFC 3339 (Go's time.Parse also takes a
  one digit hour, `T6:16:55Z`), and a Duration text is trimmed of white
  space first (Duration.parse). The fuzz harness's Go side applies both
  (found by its fresh seeds, 2026-11-13).
- Invalid UTF-8 in a string decodes to U+FFFD per bad byte, as Go
  (probe: `"\xff\xfe\xc3"` reads as three U+FFFD); a lone surrogate
  escape decodes to U+FFFD, as Go.
- R1 (differs from Go's v1): `json.encode` does not escape `<`, `>` and
  `&`; Go's v1 Marshal and Encoder escape them by default. A JSON text
  written by hand into an HTML `<script>` element can close the element
  when a value holds `</script>`. Decided by the owner (2026-11-13): keep
  the minimal escaping as the default, as Go's encoding/json/v2 does
  (HTML escaping is opt-in there, jsontext.EscapeForHTML; v1 escapes only
  for compatibility), and document it: spec/std.md 5 now carries a
  security note next to encode naming the safe ways (`json_writer()`,
  which escapes by default, `html_escape`, std/template's HTML mode);
  std/changes/json.md records it. Not a defect.

## Sign-off

Second reader (a separate agent, read-only, 2026-11-13): first pass found
F1 to F10 (2026-11-12); a second pass at ce7445573 found the error path's
quadratic cost (fixed, 7cfd51187), the indent row, the Option depth cases
(1a5a2890c), three defaults and citation fixes; a third pass at 1a5a2890c
verified each and signed off once three moved line numbers were
corrected (done).
