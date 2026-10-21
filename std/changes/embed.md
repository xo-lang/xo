# std/embed changes

## 2026-10-21: feature: embed.Dir.fs, a read-only Fs over the files

- Kind: feature
- Commit: a0d510fd
- Found by: decision 0304's plan (Go's embed.FS is an fs.FS)
- Go reference: embed.FS (go1.26): Open, ReadDir, ReadFile, fs.Glob over the embedded tree
- Test: internal/llvm/testdata/golden/embed_dir_fs (arm64, LLVM, Go, amd64, wasm)

`d.fs()` returns an `Fs` over the Dir's files for code written against
`Fs`: reads, stat, read_dir, walk, glob and sub work; every write, open,
and lock is `FsErr.Denied`. On the native backends it is a std struct
converted to `Fs` (decision 0307).

## 2026-10-21: feature: std/embed, files embedded at build time

- Kind: feature
- Commit: 28b3fe3b
- Found by: owner request (Go's embed for Xo)
- Go reference: embed (go1.26): embed.FS keys, `.` and `_` names left out of a directory unless `all:`
- Test: internal/llvm/testdata/golden/embed_dir (arm64, LLVM, Go, amd64, wasm)

New module (decision 0304): `embed.Dir`, the files a constant embeds
with `embed("path or glob")`, and its lookups (read, read_text,
contains, paths, sub, len).
