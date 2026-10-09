# 0094: std/time: per use zone data and named layout fields

Status: Accepted (2026-10-09, by the owner). Go backend and checker
rules implemented 2026-10-09 (see Implementation); native runtime and
cross backend diff tests not yet.

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

## Implementation (2026-10-09)

- Go backend: `std/time.xo`, `std/rt/timezone.go`, `timecivil.go`,
  `timefmt.go`. Zone data comes from the building Go toolchain's
  `lib/time/zoneinfo.zip`: `internal/gen/timezone.go` records constant
  names and run time calls of `time.zone`, `internal/build/tzdata.go`
  writes `xo_tzdata.go` (TZif per constant zone, or the whole zip with
  go:embed plus a note per call), `--tzdata=system` on build, run, and
  test. Measured on darwin/arm64: UTC only +0, one zone +848 bytes, run
  time name +414 KB (std.md 13.5).
- Checker: `internal/check/timelayout.go`, new code `XO0420` for a bad
  constant layout, `XO0177` for `{yyyy}`, `yyyy-MM-dd`, `%Y`, and
  `2006-01-02` forms.
- Points the API left open were settled without changing it (std.md
  13.5): `{frac}` includes the dot, `Civil.zone` is the zone name, gap
  `Earlier` uses the offset before the change, `{hour12}` needs
  `{ampm}`, parsing a gap or overlap without an offset is an error.
- Native backends: every function is `XO1201` (planned); types and
  constants build.
