# The embedded xo1 sources missed the nested selfhost/cli packages

Status: Observed (2026-10-12, decision 0178 switch point B gaps; found
with `go list -f '{{.EmbedFiles}}' ./selfhost`).

## Symptom

`selfhost/embed.go` embedded `xo.mod */*.xo`. The CLI port put packages
one and two levels deeper (selfhost/cli/core, cli/core/format,
cli/modules, cli/semantic, cli/servers, cli/modules/xomod). The
repository builds selfhost/xo fine, but the Go `xo` writes the embedded
tree to `<user cache>/xo/xo1/<key>` and builds xo1 there (switch point
A, internal/selfdriver): selfhost/xo's `use .../selfhost/cli/core` has no
sources in that tree, so a native `xo build|run|test` with a fresh xo1
cache cannot build xo1. No test built xo1 from the embedded tree after
the CLI packages landed.

## Cause

`//go:embed` patterns are path.Match globs: `*` never crosses a `/`, and
a pattern naming a directory embeds it recursively but skips files whose
names start with `.` or `_`, so `*/*.xo` is exactly one level.

## Fix

The pattern lists each depth (`*/*.xo */*/*.xo */*/*/*.xo`), and
`TestEmbedComplete` (selfhost/embed_test.go) walks selfhost/ on disk and
fails for any .xo file the embedded FS lacks or holds differently. A
package nested deeper than three levels needs another pattern; the test
says which file.
