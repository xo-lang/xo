kind: review
module: url
result: pass
commit: 649f5110d
source: 828c07e20050977f91ae4bedd1c8a18ed0bfc2a4e83f6bf20f743cf38fda169f
date: 2026-11-14
host: darwin/arm64 (probes and tests), linux/arm64 host 1 (perf), host 2 (fuzz, stress), host 3 (race)
command: none (section 3 does not apply: no secret); read against knowledgebase/infrastructure/std-security-checklist.md
measure: 1 Go vulnerability entry for std/url's surface (GO-2026-4341, already fixed) of the 6 net/url entries in the 182 stdlib entries read (2026-11-14; the other 5 are URL parsing, std/http's); 5 inputs a peer controls, each linear (texts tested at 4 MB, Values at 10,000 keys); 1 defect found and fixed (F1: Values lookups, adds and removes were quadratic in a query's 10,000 parameters, e15544c85); 1 fault site (go_q), not reachable, and every input index guarded; 0 insecure defaults

Security review of std/url (std/url.xo, std/native/url_native.xo, and on
the Go backend std/rt/url.go and the generated std/rt/std_gen_url.go)
against knowledgebase/infrastructure/std-security-checklist.md (decision
0616), by Xo Lang 3. std/url is Go's query and path escaping and
url.Values (spec/std.md 20). It reads text a peer sends: a request's
query string (std/http's `req.query()` and `req.form()` call
parse_query), escaped path segments, and form bodies. URL parsing
(scheme, host, IPv6 literals) is std/http's neturl, reviewed and fuzzed
with std/http (TestHttpUrlFuzz), not part of this module.

## 1. Go's CVE history

The Go vulnerability database (vuln.go.dev, the 182 stdlib entries
fetched on 2026-11-14) has six entries whose package is net/url:

- GO-2026-4341 (memory exhaustion in ParseQuery: a query of millions of
  `&` built a list of empty parts before any check; Go's fix counts the
  separators first and refuses more than 10,000 parameters, the
  `urlmaxqueryparams` GODEBUG). Applies: std/url's parse_query counts the
  `&` bytes before it splits and refuses more than 10,000 parameters with
  Go's text, the same count as go1.26.11 url.go:960
  (`strings.Count(query, "&") + 1`). Test: std/url_test.xo "parse_query
  counts the separators before it splits (GO-2026-4341)" (10,000 `&`
  refused, 9,999 accepted, 3,000,000 pairs refused).
- GO-2025-4010 and GO-2026-4601 (bracketed IPv6 hosts), GO-2022-0211
  (Hostname and Port validation), GO-2022-0988 (JoinPath `..`),
  GO-2026-6218 (quadratic resolvePath): URL parsing and resolution, which
  std/url does not have; std/http's neturl carries them (its review).

Sibling classes, checked:

- Semicolons as separators (Go 1.17's change to ParseQuery, a cache
  poisoning and parameter smuggling class with no vulndb entry, behind
  net/http's AllowQuerySemicolons): parse_query
  refuses any `;` with Go's text, which wins over an earlier escape error
  as in Go's parseQuery. Test: corpus urlfuzz (Go's answers on texts with
  `;` anywhere) and the module tests.
- Quadratic parsing (GO-2025-4009 encoding/pem, GO-2026-4977 net/mail):
  F1 below, in Values rather than parsing. Escape, unescape and
  parse_query read their text once (4 MB inputs: 2 to 11 ms on the Mac,
  section 2).
- Invalid UTF-8 from a decode: spec/std.md 20 and decision 0259 keep each
  byte that is not UTF-8 as its `%XX` escape, so Str stays valid UTF-8
  (Go returns the raw bytes). The fuzz compares under that rule.
- Hash flooding: since F1's fix a query past 16 keys keeps them in a
  Str keyed Map, so a peer chooses the keys of a hash table. Xo's Str
  hash is seeded once per process from the operating system's generator
  (decision 0545, Accepted 2026-11-07), as Go seeds its maps; 0545 notes
  the seed resists flooding by a peer who cannot observe the table's
  order, and parse_query's 10,000 parameter limit bounds the keys of one
  query either way.
- Allocation from a claimed size: none; outputs are sized from the input
  (escape: the input's length plus two bytes per escaped byte, counted
  first).

## 2. Limits on hostile input

| Input a peer controls | Limit | Where | Test at and past it |
|---|---|---|---|
| A query to parse_query | 10,000 parameters (Go's), counted before splitting; a `;` or a bad escape refused | url.xo:78-97 (the count at :88, MAX_PARAMS :258) | module test above; 4 MB of `a=` in 5.6 ms, two 4 MB pairs in 10.8 ms (Mac probe) |
| Text to query_unescape and path_unescape | linear, one pass to validate and one to decode; parse_query calls it only on a part with a `%` or `+` | url.xo:320-355 | 4 MB of `%41` 2.4 ms, of `%ff` 8.2 ms (kept as escapes), of `+` 4.6 ms; a bad escape after 4 MB 2.2 ms |
| Text to query_escape and path_escape | linear; the output's size counted before it is written | url_native.xo:20-38 (the buffer sized at :38) | 4 MB of `é ` 5.7 ms; every ASCII byte against Go on arm64, llvm and go |
| Keys and values in Values (a peer's query, then get, all, has, add, set, remove, encode) | up to 16 keys a scan of at most 16, past 16 a Map, so constant work per key since F1's fix; at most 10,000 from parse_query | url.xo:57-74, add :210, remove :244 | std/url_test.xo: 10,000 keys, 9,999 values of one key; probes in F1 |
| A decoded byte that is not UTF-8 | kept as `%XX` | url.xo:359 text_of | corpus urlfuzz; module tests |

## 3. Constant time

Not applicable: std/url handles no secret.

## 4. Fault behaviour

### F1 (fixed in e15544c85): Values was quadratic in a query's parameters

url.Values kept its keys in a list and each key's values in a parallel
list. get, all, has, add, set and remove found a key by scanning the
list, and add and parse_query added a value by copying the key's value
list, changing the copy and setting it back (a copy-on-write copy per
value, spec core 5.1). parse_query accepts up to 10,000 parameters, so
a handler that read every key of a peer's query, copied the keys into
other Values, or received one key 10,000 times did quadratic work per
request. Mac native, 9,999 parameters: get of every key 65 ms (Go
0.17 ms), add of every key 78 ms (Go 4.5 ms), 9,999 values of one key
parsed and added 189 ms. Bounded by the parameter limit, so a CPU cost
per request (about 400 times Go's), not unbounded. 0.51.0 and earlier
have it, on every backend (the Go backend runs the same generated code).

Fix: Values keeps up to 16 keys (SMALL, url.xo:57) in two lists,
found by a scan of at most 16, and moves them into an insertion ordered
Map (decision 0091 keeps first-appearance order, which keys() promises)
when a seventeenth key arrives (grown, url.xo:66-74); remove moves them
back to the lists when the Map falls to 16 (url.xo:244-256), a copy of at
most 17 entries, so add and remove stay constant work per key at any
size. A value is pushed in place, in the list or through Map.update, not
copied per value. The small form keeps a few-key query free of a Map
(bench/perf/url encode: a Map for a few keys was 1.083 of the lists). A
first version kept a key index beside the lists and rebuilt it on
remove, which made removing every key 1.2 s; a Map only version removed
every key in 0.15 ms (the list version 15 ms), and the hybrid keeps
that. After: get of every key 0.15 ms, add of every key 0.56 ms, one
key's values 0.54 ms. Tests: std/url_test.xo "Values finds keys through
add, set, remove and parse_query, in first-appearance order" and "many
values of one key, parsed and added"; goldens http_form, http_tls,
parse_edge_formats, url_basic, views_parsers on arm64, llvm and go.

### Fault sites

- go_q (an escape error's text, url.xo:281): `or_fault("rune")` on a slice
  decode_rune reported as one valid rune, so it is valid UTF-8; not
  reachable.
- unescape (url.xo:328-347): the first pass reads b[i + 1] and b[i + 2]
  only after `i + 2 >= b.len()` short-circuits, and the decode pass runs
  only on text the first pass accepted, so its b[i + 1] and b[i + 2] are
  in range; `UInt8(unhex(..) * 16 + unhex(..))` is at most 255 because
  both digits passed is_hex. text_of's UPPER_HEX index is a nibble.
- No `!` unwraps on input; escape writes through its address into a
  buffer sized by a first pass over the same table (url_native.xo), the
  tables are 256 bytes, and every ASCII byte was compared with Go's on
  arm64, llvm and go.

## 5. Defaults

- parse_query refuses semicolons and more than 10,000 parameters, Go's
  defaults; no option raises them (Go's GODEBUG has no Xo counterpart).
- No option weakens a check.

## Sign-off

Second reader: a subagent of Xo Lang 3 (2026-11-14), reading the code
separately: needs changes (F1's fix described as Map only, hash flooding
under the Map, file:line in section 2, unescape's guarded indexes, the
measure line), all made above; it confirmed the separator count, the
semicolon order, escape's sized output and go_q's unreachable fault.
