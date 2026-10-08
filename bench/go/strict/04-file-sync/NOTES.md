# Notes

## Setup

- go.mod was written by hand (`module benchtask`, `go 1.26`) instead of running
  `go mod init`, because the rules required the full solution to be written
  before running any command. The content is equivalent.

## Ambiguities resolved

- "Only able to read under src and write under dst": enforced with `os.Root`.
  All source reads go through a root opened on `src`, all destination writes
  (mkdir, temp file, chmod, chtimes, rename, remove) through a root opened on
  `dst`, so symlinks or `..` cannot escape. The only write outside the dst root
  is creating `dst` itself with `os.MkdirAll` when it does not exist.
- Overlapping trees (dst inside src or src inside dst, after resolving
  symlinks) are rejected up front, since the walk would read its own output or
  delete its own input.
- "Differs": copy when dst is missing, sizes differ, or src mtime is strictly
  newer. After copying, dst mtime and permission bits are set from src so an
  unchanged file is skipped on the next run. Contents are not hashed.
- Symlinks and other non regular files in src are counted as skipped and not
  followed. Their names still count as "present in src" for `--delete`.
- Skipped names (`.DS_Store` on macOS, `Thumbs.db` on Windows) are counted as
  skipped in src. In dst they are left alone, except inside a directory that
  `--delete` is removing (otherwise such a directory could never become empty).
- Case insensitivity uses `strings.ToLower` on the relative path, not full
  Unicode case folding or NFD normalization. Platform behavior comes from
  `DefaultOptions` (GOOS); tests set the fields explicitly so they behave the
  same on every OS.
- `--delete` removes extra files and then directories not in src, children
  before parents. If removing a child fails, its parent directories are not
  attempted (avoids a cascade of "directory not empty" failures).
- Type conflicts (src file vs dst directory, or src directory vs dst file) are
  counted as failures; nothing is replaced.
- Dry run prints `would copy`, `would mkdir`, `would delete`; counts in the
  summary are what would happen. A dry run into a missing dst does not create it.
- Exit status: 0 success, 1 if any item failed or the sync could not start,
  2 on a usage error.
- Temp file: `.filesync-<random>.tmp` created with O_EXCL in the target's
  directory, removed in a deferred cleanup on any failure (close, chmod,
  chtimes, or rename errors included). Cleanup errors are joined into the
  returned error rather than discarded.
- Copy failures in tests are injected through an unexported `copyFn` field.

## Bugs caught by tools

- None of the tools found a real bug. Tests passed on the first run.

## False positives

- nilaway (run 1): flagged `entries[i]` in `for i := len(entries) - 1; i >= 0; i--`
  as slicing a possibly nil slice. The index is bounded by `len`, so a nil
  slice never enters the loop. Rewrote with `slices.Backward`, which is also
  more idiomatic; no suppression comment used.

## Suppressions

- No `//nolint` comments and no `_ =` discards.
