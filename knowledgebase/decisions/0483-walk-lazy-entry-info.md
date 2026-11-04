# 0483: directory walks with lazy entry info (DirEntry, Go's fs.DirEntry)

Status: Accepted (owner, 2026-11-03; proposed by Xo Lang 3). Implemented 2026-11-03 as free functions (read_dir_entries, walk_entries, walk_dir_entries) and the method `DirEntry.info(fs)`, not as Fs methods: an interface method would break every user Fs (decision 0307), and the ADR claimed no migration. Nothing existing changes, so the CHANGELOG entry is a feature.

## Context

`Fs.walk`, `walk_dir` and `read_dir` return `FileInfo` (name, path, is_dir, size, modified, is_symlink), so every
entry costs a stat (statx on Linux) as well as the directory read. Go's `filepath.WalkDir` and `os.ReadDir` return a
`fs.DirEntry` that has the name and the type from the directory entry itself (`d_type`), and stat only when the program
calls `Info()`.

The twin bench/perf/walkdir (1,620 entries, `walk_dir` against `filepath.WalkDir`, host 1,
bench/results/std-twin-rows4-2026-11-03.md:9-11):

| workload | what is read per entry | Xo / Go |
|---|---|---|
| walk | name and path (Go: no stat) | 1.54 |
| walk_info | Go also calls `d.Info()` on each entry (a stat), Xo's FileInfo has it already | 0.95 |

So Xo's walk is as fast as Go's when both stat, and 1.54 of Go's when Go does not. Most walks (find files by name or
extension, skip directories, count entries) never read the size or the time. Xo cannot skip the stat because `FileInfo`
is a plain struct with the stat result in its fields.

## Decision (proposed)

Add, without changing any existing function:

```
struct DirEntry {
  name: Str
  path: Str            // relative to the capability root, as FileInfo.path
  is_dir: Bool         // from the directory entry's type; a link is not followed
  is_symlink: Bool
}
fn (e: DirEntry) info(fs: Fs) -> FileInfo ! FsErr uses fs.read   // the stat, on demand (Go's DirEntry.Info)
fn (f: Fs) walk_entries(dir: Str) -> List[DirEntry] ! FsErr uses fs.read       // walk's order, no stat
fn walk_dir_entries[fx: Effects](fs: Fs, dir: Str, visit: fn(DirEntry) -> WalkStep uses fx)
    -> List[DirEntry] ! FsErr uses (fs.read, fx)                                // walk_dir's contract
fn (f: Fs) read_dir_entries(dir: Str) -> List[DirEntry] ! FsErr uses fs.read    // read_dir's order, no stat
```

`walk`, `walk_dir` and `read_dir` keep returning `FileInfo` and stay the default for programs that need sizes and times.
Where the file system reports no type in the directory entry (some network and FUSE file systems, `DT_UNKNOWN`), the
implementation stats that entry, as Go does. MemFs has no stat cost and fills the same fields.

## Consequences

- No migration: every existing caller compiles and behaves as before. New code that only filters by name or type moves
  to the `_entries` forms.
- A walk that never calls `info` does one `getdents` per directory and no statx; the expected gain is the 1.54 to about
  1.0 or better, bounded by the directory reads (to be measured, not promised).
- Six new public names and one struct; spec/std.md (the Fs listing and the `walk_dir` paragraph), xo-at-a-glance.md, a
  std/changes entry for prelude, and the Go backend (std/rt) and MemFs implementations, with Go-generated corpus rows for
  each (names, types, symlinks, DT_UNKNOWN fallback).
- `info` after the tree changed can fail with NotFound, as in Go; documented.

## Measurement plan

1. bench/perf/walkdir gets workloads `walk_entries` (Xo `walk_dir_entries`, name and path only) next to the existing
   `walk` (FileInfo), against the same Go `filepath.WalkDir` twin with no `Info()` call; `walk_info` stays unchanged and
   gains `walk_entries_info` (`info` on every directory entry) against Go's `d.Info()`.
2. Host 1, `xo bench --ab --early`, the rows in bench/results/std-twin-walkdir-DATE.md. Proven means at most about 1.05 of
   Go's wall (decision 0461) on `walk_entries`, and no regression on `walk_info` (0.95).
3. A syscall count (strace -c on the Linux host) of statx and getdents64 per workload, to show the saving is the stat.

## Alternatives

- **Make `FileInfo`'s size and modified lazy** (fields computed on read): not possible for a plain struct without a
  language feature, and it would hide a fallible stat behind a field read.
- **Change `walk` and `read_dir` to return `DirEntry`**: matches Go but breaks every caller that reads `size` or
  `modified` (all of `bench/paper`, the CLI tools) for a speed gain only some need.
- **Do nothing**: `walk_info` is already faster than Go's equivalent; the 1.54 is the cost of an answer Go does not compute.
  The row would be reported as "different work" in the coverage page, not as a gap.
