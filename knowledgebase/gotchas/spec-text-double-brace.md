# `{{` anywhere in the spec fails the benchmark harness

Status: Observed (2026-10-09, spec audit)

Symptom: `go test -short ./...` failed in `internal/benchharness`
(`TestHarness`, Python `test_every_prompt_renders`: `'{{' unexpectedly
found`) after a spec edit that only added prose to `spec/std.md`.

Cause: the agent benchmark prompts embed `spec/core.md` and
`spec/std.md` whole, and the self test treats any `{{` left in a rendered
prompt as an unfilled template placeholder. The draft std/time layout
syntax wrote a literal brace as `{{`.

Fix: never write `{{` in the spec (or anything a prompt embeds); spell the
idea another way (the std/time draft uses `{lbrace}`). Run
`go test ./internal/benchharness` after spec edits.
