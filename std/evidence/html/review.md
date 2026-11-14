kind: review
module: html
result: pass
commit: 67ba54a24
source: 5747b639da584a7ec48020992f3b0645191623cb176c4d06ef908f4d699be444
date: 2026-11-14
host: darwin/arm64 (probes and tests), linux/arm64 host 1 (perf), host 2 (fuzz, stress), host 3 (race)
command: none (section 3 does not apply: no secret); read against knowledgebase/infrastructure/std-security-checklist.md
measure: 0 Go vulnerability entries for html (182 stdlib entries read, 2026-11-14); 2 inputs a peer controls, each linear and tested at 4 MB; 1 defect found and fixed (F1: unescape_string faulted on Int overflow for a long numeric reference, 4ad3745a2, in 0.51.1) and 1 divergence from Go fixed with it (the three byte rule); 1 fault site, not reachable; 0 insecure defaults

Security review of std/html (std/html.xo, std/htmlent.xo,
std/native/html_native.xo, and on the Go backend std/rt/html.go, Go's
EscapeString, with unescape_string compiled from std/html.xo) against
knowledgebase/infrastructure/std-security-checklist.md (decision 0616),
by Xo Lang 3. std/html is Go's html package: escape_string, used to put
a peer's text into a page, and unescape_string, used on text from pages
and forms a peer wrote (spec/std.md 39). Tokenizing and parsing are
std/htmltok and std/htmlparse, their own modules.

## 1. Go's CVE history

The Go vulnerability database (vuln.go.dev, the 182 stdlib entries read on
2026-11-14) has no entry whose package is html. Sibling classes:

- Escaping that misses a character (the class of html/template's
  GO-2026-4865 and GO-2026-6091, contexts rather than characters):
  escape_string replaces exactly Go's five (`<`, `>`, `&`, `'`, `"`);
  every byte of a fuzz case and the five in every position are compared
  with EscapeString (corpus htmlfuzz). It is not a context aware escaper;
  std/template's html mode is that (spec/std.md 30, 39).
- Integer overflow on a peer's number (no Go entry; Go's rune wraps):
  F1.
- Quadratic decoding (GO-2025-4009 encoding/pem, GO-2026-4977 net/mail):
  none; section 2.

## 2. Limits on hostile input

| Input a peer controls | Limit | Where | Test at and past it |
|---|---|---|---|
| Text to escape_string | linear; output sized by a first pass (n plus the extra bytes counted), a word at a time to the next special byte | html_native.xo:41-85 (size :70, skip :85) | 4 MB of `<` 10.8 ms (Go backend 9.0) |
| Text to unescape_string | linear: each `&` reads its name or digits once; the legacy prefix search tries at most 5 lengths (from min(name length - 1, LONGEST_ENTITY_WITHOUT_SEMICOLON = 6) down to 2) | html.xo:37-221 (digits :32-35 and :148, prefix search :196-213) | 4 MB of `&amp;` 2.3 ms, of `&` 22 ms, one 4 MB name 2.5 ms, one 4 MB number 6.3 ms, `&#65;` 7.1 ms, `&ampx` (legacy prefixes) 308 ms (the Go backend's Go code 85 ms: linear, 3.6 times Go; a perf item, fixed in 67ba54a24: entity2_of no longer scans the two character table for a name without `;`, and a name without `;` searches the 106 legacy names, not all 2,138; 2.4 MB of `&ampx` three times 930 to 165 ms native on the Mac, Go 48 ms; per reference still a constant: two binary searches and up to 5 prefix searches); corpus htmlfuzz |

## 3. Constant time

Not applicable: std/html handles no secret.

## 4. Fault behaviour

### F1 (fixed in 4ad3745a2, 0.51.1): a long numeric reference faulted

unescape_string accumulated a numeric reference's digits in an Int with
`10 * x + d` (16 for hexadecimal), which faults on overflow (spec core:
"Overflow faults"), so a reference with enough digits stopped the
program, or answered 500 with a crash bundle in an http handler (spec
std http: an isolated fault). Any program unescaping a peer's text was
reachable. 0.35.0 (std/html's first release) to 0.51.0 on the native
backends; the Go backend's Go code (0.35 to 0.48) and its compiled
std/html.xo (0.49 on, int64 arithmetic that wraps) gave answers and no
fault. Found by the new fuzz harness on its first seed.

Fix: the digits accumulate as Go's rune (int32) does in unescapeEntity,
wrapped after every digit (rune32), so 16 * x + 15 never overflows and
the answer is Go's (a number past 2^31 is the code point it wraps to, a
negative one U+FFFD). With it, Go's check that nothing matched when the
reference ends within three bytes of its `&` (`&#x;` is U+FFFD, `&#9`
before another byte stays text), where Xo had differed on 215 of the
2,000 seed 1 cases. Tests: std/html_test.xo "a numeric reference past
Int answers as Go's rune does, not a fault" and "Go's three byte rule
for numeric references" (Go's answers); corpus htmlfuzz on go, arm64
and llvm.

std/htmltok's own unescape (x/net/html's) stops accumulating past
U+10FFFF and does not fault (read; its fuzz is htmltok's).

### Fault sites

- unescape_string: `Rune.try(0xFFFD).or_fault("rune")`, a constant that
  is a valid rune; not reachable.
- unescape_string's indexes: `ent.REPLACEMENT[r - 0x80]` (html.xo:155)
  runs only for 0x80 <= r <= 0x9F (:154), and the table has 32 entries;
  the byte reads of a reference (:70-95, :108, :117) are guarded by the
  `rest` bound and the checks at :102 and :110. rune32 (:32-35) wraps
  the accumulator to 32 bits, so `16 * x + 15` cannot overflow.
- Silent fallbacks: `?? ""` on a name's bytes (:178) and `?? s` on the
  output (:220). The input is valid UTF-8 and the code cuts it only at
  ASCII bytes (`&`, `;`, `#`, digits) and writes only valid runes, so
  neither fallback is taken; neither hides an error a peer can cause.
- escape_string (native) writes through the output's address into a
  buffer sized by the same table (ESC_EXTRA, 256 bytes) on a first pass;
  every byte value in the fuzz alphabet and the five specials are
  compared with Go's.

## 5. Defaults

- No options.

## Sign-off

Second reader: a subagent of Xo Lang 3 (2026-11-14), reading the code
separately: needs changes (guarded indexes and silent fallbacks not
listed, the prefix search's count, file:line, the cause of the legacy
prefix cost), all made above; it confirmed rune32's wrap, the three byte
rule, escape_string's sized output and htmltok's 0x10FFFF cap.
