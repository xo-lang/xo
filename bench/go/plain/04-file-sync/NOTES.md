# Notes

## Ambiguities resolved

- "Only read under src and write under dst": enforced with `os.Root` (`os.OpenRoot` for both trees). All file operations go through the roots, so `..` paths and symlinks escaping either tree are rejected by the standard library. Additionally, src and dst that are equal or nested in each other are rejected up front (symlinks resolved), since otherwise the walk would see its own output.
- "Differs": a file is copied when it is missing in dst, sizes differ, or the src mtime is strictly newer. Same size with an older src is skipped. After a copy the dst mtime and permission bits are set from src so that the next run skips it.
- Symlinks and other non regular files in src are skipped (counted in `skipped`) and are not followed.
- Ignored names (`.DS_Store` on macOS, `Thumbs.db` on Windows) in src are counted as skipped. In dst they are left alone by `--delete`, except when their parent directory is itself being deleted (otherwise they would keep that directory from becoming empty).
- `--delete` removes extra files and then extra directories, deepest first; directories count toward `deleted`. If any part of the src walk failed, deletion is disabled for that run (an unreadable src directory would otherwise look like "missing" and its dst copy would be wiped).
- Case insensitivity is applied via lowercase path keys when matching dst entries against src for `--delete`, and for skip names and the overlap check. Unicode normalization (NFC/NFD) is not handled.
- `--dry-run` performs no writes, including not creating a missing dst (treated as empty). It prints `mkdir`, `copy`, `delete` lines and a summary prefixed with `dry run:` whose counts are what would happen.
- Exit codes: 0 success, 1 if any item failed or the sync could not start, 2 for usage errors. Flags may appear before or after the positional arguments.
- Temp files are named `.filesync-<random>.tmp`, created with `O_EXCL` in the destination directory, synced, closed, chmod/chtimes applied, then renamed. Any error after creation removes the temp file.
- Type conflicts (src file vs dst directory, or src directory vs dst file) are reported as failures, not resolved by deleting.
- Per item failures go to stderr; action lines and the summary go to stdout.

## Bugs caught by the loop

None. The first run of the build, vet, and test loop passed, so the loop caught no bugs in this implementation. Tests cover: new file copied and unchanged skipped (plus a second idempotent run), size and mtime based changes, dry run leaving dst byte and mtime identical (and not creating a missing dst), `--delete` removing extra files and nested directories, injected copy failure leaving no temp file and preserving the old dst file, an unreadable src file giving exit status 1, case insensitive matching, skip names, overlap rejection, and usage errors.
