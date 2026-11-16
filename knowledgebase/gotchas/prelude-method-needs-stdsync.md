# A new prelude method needs stdsync, not a hand edit of the spec index

Status: Observed (2026-11-15, decision 0667)

## Symptom

A push adding `BytesBuilder.write_list` and `Bytes.from_list` to
std/prelude.xo, with the index lines of spec/std.md written by hand, failed
its offload gate in internal/repotest: `TestStdIndexCurrent`, "spec/std.md's
prelude index is not std/prelude.xo's: run `go run ./scripts/stdsync`". The
local goldens, the wasm run and the Go parity run had all passed.

## Cause

The "Every function and method of std/prelude.xo" section of spec/std.md is
generated, with a declaration count ("585 declarations.") the hand edit did
not change, and cmd/xo/testdata/habits/prelude.sha256 hashes the prelude.

## Fix

After any change to std/prelude.xo's declarations run `go run
./scripts/stdsync` (and `go run ./scripts/stdgen` for the Go backend's
signatures), commit what they write, and run `go test ./internal/repotest
-run '^(TestStdIndexCurrent|TestStaticWords)$'` before the gate. The prose
parts of spec/std.md (the BytesBuilder section's paragraphs, the short
index near the top) are still written by hand.
