# A Windows path reaches xo with backslashes; it is normalized at entry, nowhere else

Status: Observed (CI 40815894916, 2026-10-29; the readers' survey the same day)

## Symptom

`XO_BACKEND_CACHE_DIR=C:\x` (or any path flag, variable or package
argument given with `\`) was taken as a relative path: check.path_volume
knew only the `C:/` form, so `C:\x/std` was joined onto the working
directory. A UNC path `\\server\share\x` had no volume at all, and
check's clean collapsed `//server/share` to `/server/share`.

## Cause

Paths flow through xo in `/` form (std.md 3.1); a `\` is a character of
a name on every other host, so the readers cannot normalize blindly, and
each reader that did it on its own (bcache.xo for one variable) left the
others out.

## Fix

One place, on a Windows host only: compiler/xo opts.xo's `path_env` at
main's start rewrites every path variable in PATH_ENV (`\` to `/`), and
`os_path` does the same to a path flag's value (`-o`, `--std`, `--dir`,
`--wd`, ...) and to the package arguments at parse; nothing downstream
normalizes again. check.path_volume and clean know UNC volumes
(`//server/share`, hostpath's volume_w rule). A new XO_ variable that
names a file or directory goes into PATH_ENV (compiler/xo opts_test.xo
lists the classified ones) as well as internal/xotool's classification.
The windows arm of check_test.xo's volume table runs in CI's xo-windows
job (the Mac cannot execute a windows test binary), and
scripts/ci/windows-sepcomp.sh covers the backslashed cache directory.
