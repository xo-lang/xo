# A Go oracle's zone abbreviation offset depends on the machine

Status: Observed (2026-11-02, internal/stdtest/mailfuzz_test.go)

## Symptom

A differential test generated from Go (`mail.ParseDate`, or any
`time.Parse` with an `MST` layout element) expects
`Mon, 02 Jan 2006 15:04 EST` at 20:04 UTC, and std/mail gives 15:04 UTC
on every backend. On another machine the same generator writes 15:04.

## Cause

`time.Parse` gives a zone abbreviation the offset it has in `time.Local`
when that location knows the name (EST is -0500 on a New York Mac) and a
fabricated zone with offset zero otherwise (`time` package doc, "When
parsing a time with a zone abbreviation like MST"). std/mail has no local
zone, so it always takes the second branch. The generated answers then
depend on the TZ of the machine that wrote the corpus.

## Fix

Set `time.Local = time.UTC` while the generator asks Go, and restore it
after (mailfuzz_test.go's mlCases). Any other oracle that calls
`time.Parse`, `time.ParseInLocation(..., time.Local)` or formats in
local time needs the same pin.
