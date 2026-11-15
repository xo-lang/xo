kind: review
module: url
result: pass
commit: 5ed0cb811
source: 22ca5e53994e9c578a7e72da23bf5b0aa2a2c98a70e183a197f5528e72e0bee7
date: 2026-11-14
host: darwin/arm64 (probes and tests), linux/arm64 host 1 (perf), host 2 (fuzz, stress), host 3 (race)
command: none (section 3 does not apply: no secret); read against knowledgebase/infrastructure/std-security-checklist.md
measure: 6 Go vulnerability entries for std/url's surface (GO-2026-4341 for parse_query; GO-2025-4010, GO-2026-4601, GO-2022-0211, GO-2022-0988 and GO-2026-6218 since the URL type, decision 0642), all six net/url entries of the 182 stdlib entries read (2026-11-14), each handled and tested; 9 inputs a peer controls, each linear (texts at 4 to 16 MB, Values at 10,000 keys); 3 defects found and fixed (F1: Values quadratic in a query's parameters, e15544c85; F2: a bracketed host byte from 0x80 read as a zone, so a host Go refuses could parse, 5ed0cb811; F3: resolve_path's memory at about 12 bytes an input byte, 5ed0cb811); 0 reachable fault sites, every input index guarded; 0 insecure defaults

Security review of std/url (std/url.xo, std/native/url_native.xo, and on
the Go backend std/rt/url.go and the generated std/rt/std_gen_url.go)
against knowledgebase/infrastructure/std-security-checklist.md (decision
0616), by Xo Lang 3. std/url is Go's query and path escaping and
url.Values (spec/std.md 20). It reads text a peer sends: a request's
query string (std/http's `req.query()` and `req.form()` call
parse_query), escaped path segments, and form bodies. Since decision
0642 (e2feedd59) it also parses URLs (Go's url.URL: `parse`,
`parse_request_uri`, `resolve_reference`, `join_path`, `display`), the
same parser std/http's neturl carries for request targets; neturl stays
reviewed and fuzzed with std/http (TestHttpUrlFuzz), and this review now
covers std/url's own (std/url.xo:400-1424, fuzzed by TestUrlTypeFuzz
against Go's url.Parse, ResolveReference, JoinPath and String).

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
- Since the URL type (decision 0642) std/url parses URLs as well, so the
  five URL parsing and resolution entries apply to it (std/http's neturl
  still carries them for requests, its review). Each has a module test in
  std/url_test.xo with Go's answers (go1.26.11), and TestUrlTypeFuzz
  (internal/stdtest/urltypefuzz_test.go, corpus urltypefuzz) compares
  parse, display, resolve_reference and join_path with Go's on generated
  hosts, zones, ports, userinfo and dot segments:
  - GO-2025-4010 (a bracketed host that is not IPv6): parse_host
    (url.xo:1065-1103) parses the bracket's text with netip and refuses
    any error (`invalid host: ParseAddr(...)`) and an IPv4 address
    (`invalid IP-literal`), keeping IPv4-mapped IPv6, as Go's parseHost.
    Test: "parse refuses the hosts Go refuses, with Go's texts"
    (`[abc]`, `[1.2.3.4]`, `[]`); the generated `[::ffff:` cases keep
    the mapped form.
  - GO-2026-4601 (a `[` not at the host's start): refused with
    `invalid IP-literal` before any other check (url.xo:1067). Test: the
    generated case for `com[::1]`.
  - GO-2022-0211 (a host with a suffix in neither hostname nor port):
    the text after `]`, and after the port colon of a name, must be a
    valid optional port (valid_port, url.xo:883). An http or https host
    splits at its first colon (urlstrictcolons, Go 1.26's default), so a
    second colon is an invalid port; other schemes split at the last
    colon, as Go (`postgres://a:80:90/` has hostname `a:80`, port `90`).
    Test: the same module test (`[::1]junk`, `[::1]%3A80`,
    `example.com:80junk`, `example.com:80:90`, the postgres case).
  - GO-2022-0988 (JoinPath kept `../`): join_path (url.xo:481) and
    Url.join_path (:682) join through joined, go_join and clean_path
    (:1306-1424), which clean as Go's fixed JoinPath does. Test: the
    generated `../go` case, and "resolve_reference and join_path stay
    linear on long dot segments" (`rel` + `../../x` is `x`).
  - GO-2026-6218 (quadratic resolvePath): resolve_path (url.xo:1185-1276)
    finds each element's fate in one backward pass (a byte an element:
    each `..` cancels the nearest name before it, as Go's truncation
    pops it) and writes the kept ones once; no rewrite per step. Test:
    the linear module test (200,000 `../`, 400,000 slashes); Mac probe
    at 16 MB: 39 ms for dot segments, 76 ms and 70 MB maximum resident
    for a path of slashes (F3).

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
- Invalid UTF-8 from a decode: spec/std.md 20 and 20.1 and decision 0259
  keep each byte that is not UTF-8 as its `%XX` escape, so Str stays
  valid UTF-8 (Go returns the raw bytes). The fuzz compares under that
  rule. For the URL type this covers a host (not bracketed), user and
  password too, with one consequence Go does not have: `http://%FF/`
  and `http://%25FF/` both give the host text `%FF` (Go: the byte 0xFF,
  and the text `%FF`), so hostname() is the same for both and display
  writes `%25FF` for each (Go writes `%FF` for the first). A caller that
  allow-lists hosts compares Xo's text, which is never a valid DNS name
  for either input. A bracketed host's address part is not text: a byte
  from 0x80 there is refused as Go refuses it (F2).
- A path that displays as an authority (Go parity): a Url with no
  scheme, host or user whose path starts with `//` displays as `//x/...`,
  which parse reads back with host x, as Go's String does. Two ways to
  get one: parse_request_uri of a `//x/y` target, and resolve_reference
  of a reference with repeated `/` against a base with no host. A caller
  building a Location header from request_uri or display of a peer's
  target sees what Go's caller sees.
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
| A query to parse_query | 10,000 parameters (Go's), counted before splitting; a `;` or a bad escape refused | url.xo:78-97 (the count at :88, MAX_PARAMS :259) | module test above; 4 MB of `a=` in 5.6 ms, two 4 MB pairs in 10.8 ms (Mac probe) |
| Text to query_unescape and path_unescape | linear, one pass to validate and one to decode; parse_query calls it only on a part with a `%` or `+` | url.xo:320-355 | 4 MB of `%41` 2.4 ms, of `%ff` 8.2 ms (kept as escapes), of `+` 4.6 ms; a bad escape after 4 MB 2.2 ms |
| Text to query_escape and path_escape | linear; the output's size counted before it is written | url_native.xo:20-38 (the buffer sized at :38) | 4 MB of `é ` 5.7 ms; every ASCII byte against Go on arm64, llvm and go |
| Keys and values in Values (a peer's query, then get, all, has, add, set, remove, encode) | up to 16 keys a scan of at most 16, past 16 a Map, so constant work per key since F1's fix; at most 10,000 from parse_query | url.xo:57-74, add :210, remove :244 | std/url_test.xo: 10,000 keys, 9,999 values of one key; probes in F1 |
| A decoded byte that is not UTF-8 | kept as `%XX` | url.xo:359 text_of | corpus urlfuzz; module tests |
| A URL to parse or parse_request_uri | no length limit (Go has none; std/http bounds a request target by its head limit); linear: an ASCII control byte refused first (Go's `invalid control character`), host, port, userinfo and zone checked a byte at a time against Go's tables (MODES, url.xo:703, identical to encoding_table.go) | parse_in url.xo:963, parse_authority :1044, parse_host :1065, valid_userinfo :1159 | module test "parse refuses the hosts Go refuses"; TestUrlTypeFuzz; Mac probe: a 16 MB path 8 ms, 16 MB after a control byte refused in 3 ms, 16 MB of `%41` 23 ms |
| A reference to resolve_reference and parse_ref | no length limit; linear time, a byte an element of memory (F3) | resolve_reference :603, resolve_path :1185 | module test "resolve_reference and join_path stay linear on long dot segments" (200,000 `../`, 400,000 slashes); Mac probe at 16 MB: dot segments 39 ms and 46 MB maximum resident, slashes 76 ms and 70 MB |
| Elements to join_path | no length limit; linear: one join, then clean_path's backward pass and one write (a byte an element) | joined :1306, go_join :1329, clean_path :1347 | the same module test (100,000 `a/../`); Mac probe: 16 MB of `a/../` 27 ms, 16 MB of slashes 43 ms |
| A Url to display, redacted, request_uri | linear: each part escaped once (escape_in sizes nothing up front; a part with nothing to escape is returned as it is), then joined by one interpolation, which the compiler sizes once | display :494, escape_in :710 | Mac probe: a 7 MB result in 2 ms |

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

### F2 (fixed in this push): a bracketed host byte from 0x80 read as a zone

parse_host decoded a bracketed host's address part (before `%25`) to
text under section 1's rule, so a byte from 0x80 became the text `%XX`,
and netip then read that `%` as the start of a zone: `http://[::1%80]/`
parsed as `::1` in zone `80`, where Go refuses `ParseAddr("::1\x80")`.
So a host check weaker than Go's for a peer's URL, in the URL type only
(e2feedd59, unreleased). Found by TestUrlTypeFuzz seed 11 (an error text
that differed, `[f%80::1%25eth0]`), then by reading parse_host. Fix:
the address part is decoded as bytes, and bytes that are not UTF-8 are
refused with Go's text (bracket_addr_err, url.xo:1124: netip parses a
stand in with each such byte as `g`, which reaches the same error at the
same offset, and the quoted input and rest are rewritten from the bytes).
Test: "parse refuses the hosts Go refuses, with Go's texts" (`[::1%80]`,
`[f%80::1%25eth0]`, `[%80]`, `[1.2.3.%80]`, `[fe80::1%25%80]`);
TestUrlTypeFuzz seeds 11, 12 and 13 at 20,000 cases.

### F3 (fixed in this push): resolve_path's memory

resolve_path kept three Ints an element in a list sized from the whole
input (about 12 bytes an input byte, and 24 for a path of slashes, so a
16 MB reference cost about 190 to 400 MB; Go's buffer is the input's
length). Found by the second reader. Fix: the backward pass keeps one
byte an element, and clean_path (join_path's), which made a Str an
element, does the same. A 16 MB path of slashes now resolves at 70 MB
maximum resident, of which the input is 16 MB and the full path and the
result 32 MB.

### Fault sites

- go_q (an escape error's text, url.xo:281): `or_fault("rune")` on a slice
  decode_rune reported as one valid rune, so it is valid UTF-8; not
  reachable.
- unescape (url.xo:328-347): the first pass reads b[i + 1] and b[i + 2]
  only after `i + 2 >= b.len()` short-circuits, and the decode pass runs
  only on text the first pass accepted, so its b[i + 1] and b[i + 2] are
  in range; `UInt8(unhex(..) * 16 + unhex(..))` is at most 255 because
  both digits passed is_hex. text_of's UPPER_HEX index is a nibble.
- The URL type (url.xo:400-1424): no `or_fault` and no `!` unwrap.
  resolve_path and clean_path read b[at] and b[at + 1] only when the
  element's length is 1 or 2 (elem_kind, :1278), read fate[cnt - 1 - e]
  for e below the element count both passes share, and read the result's
  second byte only when it has two; parse_host's `rfind(":") ?? first`
  (:1106) follows a find of the same byte. The `to_str() ?? ""`s take
  bytes that are valid UTF-8 by construction (escape_bytes :751 and
  text_of :378 write ASCII; resolve_path :1268 and clean_path :1423 write
  slices of a valid Str cut at `/`; parse_host :1093 and
  bracket_addr_err :1129 after a to_str test or on ASCII), so the `""`
  is never taken.
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

Second reader of the URL type sections: a subagent of Xo Lang 3
(2026-11-14), reading std/url.xo:400-1305 against Go's url.go before the
review's text: needs changes. resolve_path's memory (F3, fixed), the
stale measure line, intro and hash, tests named for each entry, the
host and userinfo collapse under section 1's rule, the `//` path that
displays as an authority, line references, and the last-colon split of
other schemes, all made above. It checked MODES byte for byte against
encoding_table.go and found no fault reachable from input and no check
weaker than Go's. F2 was found after its reading, by the fuzz.
