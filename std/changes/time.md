# std/time changes

## 2026-10-21: fix: the sign of an offset under a minute

- Kind: fix
- Commit: dd2e401a
- Found by: differential test against Go
- Go reference: time.Time.Format's zone offset (format.go, go1.26.1)
- Test: std/native/time_test.xo (1,200 random instants in fixed zones, both layouts covering every field, against Go's Format); internal/llvm/testdata/golden/time_fixed_offsets (arm64, LLVM, and the Go backend agree)

`{offset}` and `{offset:compact}` of a zone whose offset is negative but
under a minute (-59s) showed `-00:00`; Go takes the sign of the offset in
whole minutes and shows `+00:00`. Native and the Go backend's formatter
both had it. Everything else matched Go on all 1,200 instants.
