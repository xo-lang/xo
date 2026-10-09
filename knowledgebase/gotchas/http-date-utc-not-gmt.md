# time.RFC1123 in UTC ends in "UTC", but HTTP dates need "GMT"

Status: Anticipated (2026-10-09, std/time)

Symptom: a `Date`, `Last-Modified`, or `Expires` header written with
`time.format(t, time.utc(), time.RFC1123)` reads
`Fri, 09 Oct 2026 08:05:03 UTC`; strict HTTP clients and caches expect
`GMT` (RFC 9110 IMF-fixdate) and may ignore it.

Cause: `{zone}` prints the zone's abbreviation, and the abbreviation of
`utc()` is `UTC`. Go has the same split (`time.RFC1123` versus
`http.TimeFormat`).

Fix: write the zone literally and format in UTC:
`time.format(t, time.utc(), "{weekday:short}, {day} {month:short} {year} {hour}:{minute}:{second} GMT")`.
Parsing with `time.RFC1123` accepts `GMT` and `UTC` (std.md 13.5).
