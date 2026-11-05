# A zip header's Modified is written in its own time zone

Status: Observed (2026-11-05)

## Symptom

internal/stdtest TestArchiveFuzzCorpusCurrent passed on the Mac and
failed on both linux/arm64 offload hosts with the same Go version: the
corpus its generator wrote was not the committed one.

## Cause

Go's archive/zip writes a FileHeader's Modified as an MS-DOS date and time
in Modified's location. The generator built its times with time.Unix,
which is local time, so the bytes depended on the machine's time zone
(the Mac's EST against the hosts' UTC). tar stores Unix seconds and was
not affected.

## Fix

Build every time a generator or golden writes into an archive with
`.UTC()` (archivefuzz_test.go afTime). Check a generated corpus with
TZ=UTC and another zone before committing it.
