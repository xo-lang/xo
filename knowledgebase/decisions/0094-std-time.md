# 0094: std/time: per use zone data and named layout fields

Status: Accepted (2026-10-09, by the owner). Go backend and checker
rules implemented 2026-10-09 (see Implementation); native runtime and
cross backend diff tests implemented 2026-10-09 (see Native
implementation).

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
- Native backends: every function was `XO1201` (planned) at first;
  see Native implementation below.

## Note (2026-10-09): error display text

`ZoneErr` and `CivilErr` first shipped with `derive Display`, which
printed `Skipped(at: "2026-03-08T02:30:00 America/New_York")`. They now
declare `display()` with sentences in the style of `ParseErr` and
`LayoutErr` (std.md 13.5 lists all four); the variants and fields are
unchanged. Pinned by `std/rt/timedisplay_test.go` and the
`time_civil`, `time_zones`, and `time_dynamic_zone` goldens.

## Native implementation (2026-10-09)

All of std/time builds on the LLVM and arm64 backends (`--no-gc`) with
output byte identical to the Go backend's; nothing is left `XO1201`.

- `std/native/time.xo` implements the module in Xo (decision 0074):
  `Zone` is a struct (name, fixed flag, fixed offset in seconds, and the
  C runtime's zone id, 0 for UTC; a name always gets the same id, so
  `==` compares what std/rt compares), civil dates, `instant` with the
  same gap and overlap search, the layout parser, `format`, strict
  `parse`, `check_layout`, the four error texts, and `zone_debug`
  (`Zone(UTC)`). A bad layout faults through `rt.fault_at` at the
  caller's line with std/rt's text.
- Zone rules are C: `std/llrt/xo_time.c` ports Go's
  `LoadLocationFromTZData`, `Location.lookup` (the first zone rule for
  times before the first transition, binary search, the POSIX TZ footer
  through `tzset`, and the cache of the zone in effect when the zone is
  loaded, which Go consults first), and `LoadLocation`'s host
  directories, with a stored zip reader for the database. Intrinsics in
  `std/native/rttime.xo`: `tz_load`, `tz_load_host`, `tz_local_name`,
  `tz_offset`, `tz_abbr`. Writing the lookup in C kept one copy of Go's
  rules and a process wide zone table; everything else is Xo.
- Zone data per use, as on the Go backend: the SSA builder records
  `time.zone` calls and values (`internal/ssa/build_time.go`,
  `Module.TZ`); `internal/build/tzdata.go` (`tzSelect`, shared with the
  Go backend's `tzFiles`) writes `xo_tzdata.h` next to xo_rt.c: the TZif
  data of each constant name (mode 0), the whole zoneinfo.zip (mode 1,
  with the same build note), or nothing and the host database (mode 2,
  `--tzdata=system`). xo_std.c includes xo_time.c only under `XO_TIME`,
  which the build defines only for programs that load std/time, so other
  programs do not grow: hello world stays 310,392 bytes (arm64) and
  88,264 bytes (LLVM) on darwin/arm64, as before. A UTC only std/time
  program is 345,424 / 139,584 bytes, one constant zone 362,048 /
  156,928, a name known at run time 758,400 / 569,840.
- Prelude pieces the goldens need are native too (std/native/prelude.xo):
  `Duration.from_secs`, `scale`, `ceil_seconds`, `Time.parse_rfc3339`
  (the union of Go's fast path and its general parser for RFC3339Nano:
  one digit hours, a comma before the fraction, offsets up to +24:60,
  UnixNano wrapping for years outside Time), `Time.format_rfc3339`, and
  `Time.UNIX_EPOCH` read as a value. A std function with a native
  implementation and no `rt.Loc` parameter can be a function value
  (`let load = time.zone`).
- `time.local`: `TZ` (the program's zone data, then the host database),
  else the `/etc/localtime` link, else `/etc/localtime` itself as
  `Local`. A native test binary's Env is fake, so without `TZ` it gives
  `utc()` there (std/rt checks for the real Env instead).
- Tests: the goldens `time_civil`, `time_zones`, `time_format`,
  `time_dynamic_zone` (its `args.txt` is now passed natively too),
  `time_layout_fault`, and `duration_time` in
  `internal/llvm/testdata/gen_native.txt`; `TestTimeDiff`
  (`internal/llvm/time_diff_test.go`: about 70,000 lines of zone
  equality and Debug, transitions of 19 zones with odd rules, random
  instants over the whole range, gaps and overlaps, mutated parse texts,
  random layouts, Date arithmetic with negative years, compared Go
  against LLVM and arm64 with the leak check;
  `XO_TIMEDIFF_OUT=<prefix>` keeps the program and its output);
  `TestZoneDataPerUseNative`.

## Addendum: the zone database ships with std (owner, 2026-10-16)

Approved by the owner, relayed by the lead. The zone data was Go's
`lib/time/zoneinfo.zip` (`go env GOROOT`, or a release's copy of it), so
a native build that named a zone needed Go installed or XO_ZONEINFO.
Now std carries the IANA database itself:

- `std/tzdata/zoneinfo.zip`: the IANA time zone database (598 zones,
  about 400 KB), built by `scripts/tzdata-update.sh VERSION` the way Go's
  lib/time/update.bash builds its own: IANA's tzcode and tzdata of that
  release, IANA's zic (`make posix_only` with backzone and zone.tab's
  zones), every TZif file stored in a zip with sorted names and fixed
  times (the same VERSION gives the same bytes). `std/tzdata/VERSION`
  records the version, the SHA-256 of both archives and their source.
  First version 2026e (2026-10-16). Refresh per release.
- Every layout has it because it is a tracked file of std: a checkout, a
  seed (bin/xo beside std), a release (lib/xo, scripts/release.sh no
  longer copies Go's file).
- Lookup (selfhost/xo tz.xo zoneinfo_zip, gobackend.xo tz_files):
  `XO_ZONEINFO`, else std's file, else an older release's
  `lib/xo/zoneinfo.zip`; no `go env GOROOT`. Both backends use it.
- Unchanged: a constant name embeds only that zone, a name known only at
  run time the whole database, `--tzdata=system` reads the operating
  system's database at run time.
- Test: internal/llvm TestZonesWithoutGo (a constant and a run time
  name, built with no go on PATH and no XO_ZONEINFO, arm64).
