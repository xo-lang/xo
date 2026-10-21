# std/embed changes

## 2026-10-21: feature: std/embed, files embedded at build time

- Kind: feature
- Commit: (this change)
- Found by: owner request (Go's embed for Xo)
- Go reference: embed (go1.26): embed.FS keys, `.` and `_` names left out of a directory unless `all:`
- Test: internal/llvm/testdata/golden/embed_dir (arm64, LLVM, Go, amd64, wasm)

New module (decision 0304): `embed.Dir`, the files a constant embeds
with `embed("path or glob")`, and its lookups (read, read_text,
contains, paths, sub, len).
