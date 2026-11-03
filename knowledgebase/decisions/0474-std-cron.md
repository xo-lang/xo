# 0474: std/cron, schedule parsing and next fire time (proposal)

Status: Proposed

## Context

Survey rank 8 (std-coverage.md, "Cron schedules", robfig/cron): a scheduler service needs "every weekday at
09:30 in New York" and "every 15 minutes". The parts exist (std/time: Zone, civil, instant with an Ambiguity
policy, Ticker; spawn and sleep with the Clock capability). What is missing is the schedule itself: parsing
the 5 field spec and finding the next instant after a time, with the time zone and daylight saving rules that
robfig/cron's `Next` gets right and hand written loops get wrong.

## Options

1. `std/cron`, a pure schedule value; running the jobs stays with the caller's own loop (sleep until `next`):

   ```xo
   enum CronErr { Fields(n: Int), Value(field: Str, text: Str), Descriptor(text: Str) }   // display() texts of robfig's errors
   struct Schedule {}
   pub fn parse(spec: Str, zone: Zone = time.utc()) -> Schedule ! CronErr
   fn (s: Schedule) next(after: Time) -> Time?       // the first fire strictly after; None when none within 5 years (robfig's zero Time)
   fn (s: Schedule) zone() -> Zone
   ```

   The spec is robfig/cron's standard parser: 5 fields (minute 0-59, hour 0-23, day of month 1-31, month 1-12
   or JAN-DEC, day of week 0-6 or SUN-SAT, with 7 not accepted), each `*`, `?`, a number, a range `a-b`, a step
   `a-b/n` or `*/n`, comma lists; the descriptors `@yearly`, `@annually`, `@monthly`, `@weekly`, `@daily`,
   `@midnight`, `@hourly` and `@every DURATION` (a constant delay from `after`, robfig's ConstantDelaySchedule,
   rounded down to the second); a leading `TZ=Name` or `CRON_TZ=Name` field sets the zone (as the Go
   package). When both day of month and day of week are restricted, either matching fires (robfig's and Vixie
   cron's rule). `next` follows robfig's `SpecSchedule.Next`: a wall clock time that does not exist (the spring
   forward gap) fires at the first instant after the gap, one that happens twice (the fall back overlap)
   fires once, at the first occurrence.
   Cost: std/cron.xo, a pure native source (about 150 lines, std/go_compiled.txt), spec/std.md section,
   std/changes, corpus of seeded specs and start times in several zones (including DST gaps and overlaps)
   against robfig/cron v3 (an oracle module as bcryptoracle, GOPROXY permitting, else a faithful port of
   its `Next` in the oracle), a timing row against robfig's `Next`.
2. A scheduler type that owns jobs and runs them (robfig's `Cron.Start`): needs task spawning, panics and
   cancellation policy, and a Clock; larger and hard to test. Rejected here; a loop of `next` and `sleep` is
   about ten lines and the caller keeps control of overlap and failure policy.
3. Fixed spec strings only (no zone, no `@every`): smaller, but the zone rules are the point.

## Recommendation

Option 1. Names and grammar are robfig/cron's so specs port unchanged; `next` returns an Option instead of a
zero Time.

## Consequences

A new public module (hence this ADR), pure: no capability, time zone data through `time.zone`. 6 field specs
(seconds) and `Schedule` as a user type of `Eq` are not included; a seconds field can be added with a
`parse(spec, seconds: true)` option later.

## Alternatives

Listed above (2, 3). Also rejected: reading Quartz syntax (`L`, `W`, `#`): not in robfig/cron.
