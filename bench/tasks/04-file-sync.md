# Task 04: File sync

Write a command line tool that makes directory B mirror directory A (one way).

## Requirements

- Usage: `filesync [--dry-run] [--delete] <src> <dst>`.
- Walk `src` recursively. For each file, copy it to the same relative path in
  `dst` if it is missing or differs (size differs, or modified time is newer in
  `src`). Create directories as needed.
- With `--delete`, remove files and empty directories in `dst` that do not
  exist in `src`.
- With `--dry-run`, perform no writes; print what would happen.
- Copy via a temporary file in the destination directory, then rename, so a
  partially written file is never visible. Always clean up the temp file on
  failure.
- The tool must only be able to read under `src` and write under `dst`.
- Path comparison is case insensitive on Windows and macOS, case sensitive on
  Linux. Skip `.DS_Store` on macOS and `Thumbs.db` on Windows.
- Print a summary: copied, deleted, skipped, failed counts. Exit status 1 if
  anything failed.

## Tests to write

- New file is copied; unchanged file is skipped.
- `--dry-run` writes nothing.
- `--delete` removes extra files.
- A failure while copying leaves no temp file behind.
