kind: review
module: time
result: pass
commit: cde48f697
source: b179c89e3ae58e0d68d5883cdb26c3b8a7f2a8b8c5ac3ea5e505915154dfd398
date: 2026-11-14
host: darwin/arm64 (probes and tests), linux/arm64 host 1 (perf), host 2 (fuzz, stress), host 3 (race)
command: none (section 3 does not apply: no secret); read against knowledgebase/infrastructure/std-security-checklist.md
measure: 0 Go vulnerability entries for time (all 187 stdlib entries read, 2026-11-14); 6 sibling classes checked; 8 inputs a peer controls, each bounded or linear and tested; 1 defect found and fixed (F1: Date and Civil methods faulted on fields far outside the calendar, a 500 per request on a server; f0454f0a2, and the second reader's three edge cases in cde48f697); 1 known issue on the Go backend (Civil.format of a year beyond about 2.9e11); 4 fault sites, none reachable from peer input; 0 insecure defaults

Security review of std/time (std/time.xo, std/native/time.xo,
std/native/llrt/tz.xo, tzrules.xo, tzhost.xo, tztable.xo, and on the Go
backend std/rt/time*.go over Go's time) against
knowledgebase/infrastructure/std-security-checklist.md (decision 0616),
by Xo Lang 3. std/time reads text a peer sends (dates in JSON bodies and
headers, durations in configuration and query strings, zone names in
requests) and values a peer builds (a Date or Civil decoded from JSON
through derive Json), so it opts in. std/timego (Go layouts) is its own
module and not part of this review.

## 1. Go's CVE history

The Go vulnerability database (vuln.go.dev, every one of the 187 stdlib
entries fetched on 2026-11-14) has no entry whose package is `time`.

Sibling classes in other Go packages, checked against Xo's time:

- Path traversal through a name that becomes a file path (the class of
  GO-2023-2185 and GO-2023-1568 in path/filepath; Go's LoadLocation
  refuses `..` and a leading `/` or `\`): tzrules.xo valid_name refuses
  an empty name, one over 255 bytes, `Local`, a leading `/`, and `\`,
  NUL and `..` anywhere, a superset of Go's checks. With --tzdata=system the name is then read
  only under the zoneinfo directories (tzhost.xo:104). Test:
  std/native/time_test.xo "hostile texts give errors, not faults"
  (`../../../../etc/passwd`, `/etc/passwd`, `Europe/../../x`, `a\b`, 256
  bytes, `Local`, empty).
- Quadratic parsing (GO-2025-4009 encoding/pem, GO-2025-4015
  net/textproto, GO-2026-4977 net/mail): parse reads its text once, field by field; a fraction
  of any length is read in one pass (1 MB of digits: an answer in under
  a millisecond; 4 MB in 0.6 ms on the second reader's probe). Test: the same test, 1 MB digit runs in the year, the
  fraction and a Duration.
- Integer overflow on a peer's number (no Go entry): Duration.parse refuses a
  value past Int64 (`2562048h`, `-9223372036854775809ns`) and accepts the
  ends. Test: the same test.
- Allocation from a claimed size: none; time allocates only its output
  text and the layout's items.
- Malformed zone data (TZif): tzrules.parse reads data the program
  embeds or the host's zoneinfo files, never a peer's; not peer input.
- A value decoded from a peer and used in arithmetic (no Go entry; Go's
  time.Date wraps): F1 below.

## 2. Limits on hostile input

| Input a peer controls | Limit | Where | Test at and past it |
|---|---|---|---|
| Text to parse and parse_civil | fixed width fields ({year} 4 digits, {month} 2, ...); a fraction of any length, read once; the text's length is the caller's | native/time.xo parse_items | time_test.xo hostile texts: 19 digit and 1 MB years refused, 1 MB fraction refused (as on the Go backend), 100,000 byte text against 50,000 fields refused; TestStdCorpora time and timefuzz (Go's answers) |
| Offsets in parsed text | +hh:mm with hh up to 99 refused past Go's range | parse_items | `+99:99` refused (same test); corpora |
| Duration.parse | Int64 nanoseconds; overflow is an error | prelude Duration.parse | 2562047h passes, 2562048h refused; Int64.min passes, one less refused; 500,000 units linear |
| Zone names | 255 bytes, no `..`, `\`, NUL, leading `/` (valid_name); `Local` refused by tz.load (tz.xo:22, :34), since valid_name's own callers accept it | tzrules.xo:174 valid_name | seven names refused by time.zone (section 1) |
| Date, ClockTime, Civil fields (derive Json) | any Int; methods normalize; weekday, display, to_time, format never fault; add_days, add_months, days_until, unix fault only when the answer does not fit Int | native/time.xo split_days, civil_split, era_year | time_test.xo: 200,000 random fields against the direct formulas, the Int range's ends in every field; golden time_hostile_date |
| Layouts | the program's (a bad layout faults; check_layout checks one first) | layout_or_fault | spec 13.3; a server taking a layout from a peer calls check_layout |
| Ticker periods | the program's (a period <= 0 faults, as Go's NewTicker panics) | native/time.xo:577 | not peer input |
| Fixed zone offsets | the program's (past 24h faults) | time.fixed | not peer input |

## 3. Constant time

Not applicable: std/time handles no secret.

## 4. Fault behaviour

### F1 (fixed in f0454f0a2 and cde48f697): Date and Civil methods on wide fields

A Date, ClockTime or Civil may hold any fields; spec 13.2 and decision
0614 say the methods normalize them, for any year. derive Json reads any
Int, so a server that decodes a Date from a request body holds whatever
the peer sent. The native methods multiplied fields before normalizing
them (norm_month's `y * 12`, days_from_civil's `era * 146097`,
civil_local's `days * 86400`, date_display's `-d.year`), so a year near
Int's ends, a month near Int.min or a day near Int's ends made
weekday, add_days, add_months, days_until, display, Civil.unix,
Civil.to_time and Civil.format fault on overflow. In a std/http handler a
fault is isolated (spec/std.md, http: a 500, a log record and a crash
bundle; the server keeps serving), so each such request cost a 500 and
a crash bundle, not the process; a program outside a handler stopped. (Crash
bundles are bounded: native builds write none, core 11.2; the Go
backend writes at most 100 for isolated faults per process, std/rt
main.go maxIsolatedBundles, and the default directory keeps the newest
100. Go's net/http logs one line per recovered panic.)
0.51.0 and earlier have it.

Fix: the day number is split into whole 400 year cycles (146097 days, a
whole number of weeks) and a rest near 1970 before anything is
multiplied (split_days, month_carry, split_floor, civil_split), and a
year is rebuilt from its cycles in an order that overflows only when it
does not fit (era_year). weekday, display, to_time and format answer
for every field; add_days, add_months, days_until and unix fault only
when their answer does not fit Int, which spec 13.2 now says, with the
advice to check a peer's year before arithmetic. The second reader
found three faults f0454f0a2 left: days_until and unix faulted when
their answer was within one 400 year cycle of Int's ends (a negative
rest under a product past Int; for unix, a positive offset bringing the
seconds back), and format faulted on a year that normalizes past Int
(month 13 of year Int.max). cde48f697 joins cycles and rest in mul_add,
which overflows only when the answer does not fit, and prints such a
year from its cycles. The Go backend, whose int64 arithmetic wrapped to
wrong answers instead (no fault), uses the same split, so it gives
native's answers wherever native answers; where native faults (an
answer past Int) it still wraps to a wrong one. Tests: std/native/time_test.xo (agreement with the direct
formulas on 200,000 random dates; the Int range's ends in every field);
golden time_hostile_date (a router answering a JSON Date at the Int
range's ends: 200 on arm64, llvm and go; 500 on 0.51.0's native
backends).

First line of defense: decoding could refuse out of range years, as
Go's time.Time.UnmarshalJSON does (RFC 3339, years 0 to 9999). Date is a
plain derive Json struct whose fields are any Int by design (decision
0614 keeps Civil for any year), so a decode-time range would change its
JSON contract; the fix makes every method safe instead, and the spec
tells servers to check a decoded year before arithmetic whose answer
may not fit.

Known issue (Go backend): Civil.format goes through Go's time.Time,
whose years end near plus or minus 2.9e11, so a Civil beyond that
formats a wrong date and weekday there (native formats it right). Not a
fault.

### Fault sites

- layout_or_fault: a bad layout faults (format, parse, layout); layouts
  are the program's, and check_layout checks a runtime one first.
- ticker and Ticker.reset: a period <= 0 faults (Go's NewTicker panics).
- time.fixed: an offset past 24h faults.
- add_days, add_months, days_until, unix: only when the answer does not
  fit Int (above).

No `!` unwraps on input in these files. TestStdCorpora timefuzz and
TestTimeFuzz (fuzz.md) compare parse and format with Go's on random and
mutated texts and layouts on every backend.

## 5. Defaults

- local(env) reads TZ, then the system's zone; utc() is explicit.
- Zone data is embedded by default (only the zones a program names);
  --tzdata=system reads the host's files, under valid_name.
- parse applies the zone given only when the text has no offset.
- No option weakens a check.

## Sign-off

Second reader: Xo Lang 3 second reader, 2026-11-14, read at cde48f697;
checked the split arithmetic (split_days, month_carry, split_floor,
era_year, mul_add, civil_split, wide_year_text) by bounds and by probes
at Int's ends in every field on arm64, llvm and go (identical outputs,
23,947 lines), the parse, parse_civil, Duration.parse and zone name
paths for faults and linear cost (4 MB inputs in under 9 ms), and the Go
source claims (zoneinfo.go:670 LoadLocation, time.go:1614
UnmarshalJSON); findings: D1 to D3 (days_until and unix faulted on
answers that fit, format on a year past Int), fixed in cde48f697;
wording W1 to W4 applied (the Go backend's parity stated as "where
native answers", the 1 MB fraction refused, `Local` refused by tz.load,
the Go known issue's wrong date and weekday).
