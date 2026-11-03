# 0476: std/norm and std/idna, Unicode normalization and internationalised domain names (proposal)

Status: Partly accepted (owner, 2026-11-03): std/norm now, with its real cost (`__text` and binary size of a program importing it) reported; std/idna waits for the owner after that number

## Context

Survey rank 12 (std-coverage.md): unicode normalization (NFC) and IDNA, from golang.org/x/text/unicode/norm and
golang.org/x/net/idna (the dataport case: comparing names from files in different encodings, and domain names
with non ASCII labels). Xo has no normalization (a string `"é"` and `"é"` compare unequal) and
`std/url` and the HTTP client cannot reach a host written in Unicode. Both need Unicode data tables that Xo's
std does not carry (std/unicode has categories and case folding, not the composition data).

## Options

1. Two modules, both generated from x/text and x/net's tables (a generator in scripts/, output committed as
   Xo constants; Unicode version pinned and recorded in the module's header):

   ```xo
   // std/norm
   enum Form { Nfc, Nfd, Nfkc, Nfkd }
   fn normalize(s: Str, f: Form) -> Str
   fn is_normalized(s: Str, f: Form) -> Bool     // Go's Form.IsNormalString (quick check, then compare)

   // std/idna
   enum Profile { Lookup, Registration, Display }  // x/net/idna's profiles
   fn to_ascii(domain: Str, p: Profile = Profile.Lookup) -> Str ! IdnaErr     // punycode, UTS 46 mapping
   fn to_unicode(domain: Str, p: Profile = Profile.Lookup) -> Str ! IdnaErr
   ```

   `normalize` is x/text's algorithm (decomposition, canonical ordering, composition with the exclusions);
   the tables are the Unicode decomposition mappings, combining classes and the composition exclusions (about
   2,000 mappings, 100 KB as packed Xo constants), included in a program only when std/norm is imported.
   `to_ascii` and `to_unicode` follow x/net/idna's Lookup profile (UTS 46 mapping and validation, Bidi and
   contextual rules, punycode of RFC 3492), needing the IDNA mapping table (about 8,000 ranges, 40 KB) plus std/norm.
   Cost: two generators and tables, the algorithms (about 300 lines for norm, 400 for idna incl. punycode),
   spec sections, change logs, corpora (x/text's norm test vectors including NormalizationTest.txt, and
   x/net/idna's tests, run against the real packages for seeded strings), timing rows against both.
   Normalization first (it has no other dependency), IDNA after it.
2. Only normalization: covers the dataport case (comparing names); leaves IDNA. A reasonable first step if
   the tables for IDNA seem too large; the survey names both.
3. Normalize only to NFC with a small table of Latin and common scripts: a partial table silently wrong
   outside it; rejected.

## Recommendation

Option 1, implemented as norm first, then idna, so the owner can stop after norm if the table size is a
concern. Table size is the cost to watch: report the `__text` and binary size of a program importing each.

## Consequences

Two new public modules (hence this ADR) and about 150 KB of generated Unicode tables, pulled in only by
programs that import them. The Unicode version is pinned (the x/text release's), and a version bump is a
std change log entry with a regenerated table. `std/url` and `http` are not changed: `idna.to_ascii` is called
by the caller on a host before building a `Url`.

## Alternatives

Listed above (2, 3). Also rejected: normalizing inside `Str` operations or comparison (changes the meaning of
`==` for every program).

## Owner decision (2026-11-03)

std/norm is accepted now: implement it and report its real cost (`__text` and binary size of a program that imports
it against one that does not). std/idna waits for the owner's word after that number.
