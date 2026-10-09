# 0094: std/time: per use zone data and named layout fields

Status: Accepted (2026-10-09, by the owner; not yet implemented)

## Context

The spec audit (`bench/results/spec-audit-2026-10-09.md`, gap 2) found
`std/time` listed in core 14 but empty, and drafted an API in std.md 13
with two open points and letter based layout fields (`{yyyy}-{MM}-{dd}`).
Measured hello world sizes (2026-10-09, darwin/arm64): Go backend 7.7 MB,
native arm64 302 KB, LLVM 86 KB. Embedding the full IANA database (about
450 KB) in every binary would add 6% on the Go backend but 2.5x to 6x on
the native ones. Letter patterns where `MM` is month and `mm` is minute
are a well known bug source in Java, C#, and Swift, and exactly the kind
of habit agents carry over.

## Decision

- Keep the draft's types: `Time` is an instant; `Zone`, `Date`,
  `ClockTime`, `Civil` are separate; daylight saving gaps and overlaps
  are errors unless the caller picks `Ambiguity.Earlier` or `Later`;
  parsing is strict; names are English only.
- Zone data is embedded per use: `utc()` and `fixed` need none; a
  constant zone name embeds only that zone; a dynamic name embeds the
  full database with a build note naming the call; `--tzdata=system`
  reads the host database instead.
- Layout fields are named and lowercase with one spelling each
  (`{year}-{month}-{day} {hour}:{minute}:{second}`, `{month:short}`,
  `{hour12}`, `{frac:3}`, `{offset}`, `{zone}`; std.md 13.3). Constant
  layouts are checked at compile time (unknown field: error with a did
  you mean fix). `{yyyy}`, `{MM}`, and `%Y` forms are `XO0177` habits
  with a machine fix.
- A bad layout built at run time faults; `check_layout` returns a
  `Result`.
- Order: Go backend (over Go's `time`), checker rules, native C runtime
  with the same data, cross backend diff tests.

## Consequences

- Same answers on every host by default, which record and replay needs.
- Services that only use UTC pay nothing; binary growth is visible.
- The compiler must find constant zone names and constant layouts.
- Longer layout strings; common ones are constants.

## Alternatives

- Embed the full database always (draft): too large for native builds.
- System database by default (Go): host dependent results, needs `fs`
  authority.
- Letter patterns or `%` codes: case and memory traps.
- `format` returning a `Result` for every call: noise for constant
  layouts that cannot fail.
