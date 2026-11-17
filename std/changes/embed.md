# std/embed changes

## 2026-11-17: fix: Dir.sub(".") is the whole Dir, and fs().glob(".") the root

- Kind: fix (behavior change)
- Commit: this push
- Found by: the new embed fuzz harness (embedfuzz, task #233), against Go's io/fs
- Go reference: io/fs Sub (Sub(fsys, ".") is fsys); filepath.Glob(".") (the real Fs's glob answers ["."])
- Test: std/embed_test.xo ("sub(\".\") is the whole Dir", "fs().glob(\".\") is the root"); corpus embedfuzz on arm64, llvm and go; TestEmbedFuzz (XO_EMBEDFUZZ_SEED) for fresh seeds
- Timing: none (one comparison per call)
- Shipped in: not yet released

`Dir.sub(".")` returned an empty Dir (it looked under the directory `./`); it is now the Dir itself, as Go's `fs.Sub(fsys, ".")`, natively and on the Go backend (std/rt/embed.go). `Dir.fs().glob(".")` returned no paths; it now returns `["."]`, as the real Fs's glob and Go's `filepath.Glob(".")`.

## 2026-11-16: perf: Dir.sub shares its files instead of copying them

- Kind: perf
- Commit: this push
- Found by: task #88 (performance Early to Proven), the module's first Go twin (bench/perf/embed: sub 2.085 of Go's fs.Sub on the Mac)
- Go reference: io/fs Sub (a wrapper that joins its directory to each name)
- Test: the embed_dir golden (a sub's len, paths, read, read_text, contains, a sub of a sub, its fs(), an empty sub); bench/perf/embed twincheck output matches Go
- Timing: bench/perf/embed on the Mac, Xo over Go: sub 2.085 to 0.268 (twincheck -time 3)
- Shipped in: not yet released

Dir.sub built a new map of every file under the directory. A Dir now keeps the prefix sub took off and shares its parent's map, as Go's fs.Sub wraps its FS: sub is constant time, and a read joins the prefix to the name. The Go backend (std/rt/embed.go) does the same. Two Dirs compare equal when they share the same files and prefix, so subs of different constants holding equal files no longer compare equal.

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
