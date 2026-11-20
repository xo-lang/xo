# A release step that only warned shipped archives without std/sqlite's Windows object

Status: Observed (2026-11-15, the owner's 0.51.1 Windows evaluation, bug 24; fixed 2026-11-20, task #244)

## Symptom

On windows/amd64, `xo build` of a program using std/sqlite failed with
`xo: clang: exec: clang: executable file not found`, while the 0.44.0
notes said releases carry a prebuilt SQLite object. The 0.51.1 archive
had `std/native/sqlite/sqlite3.c` and no `lib/xo/sqlite/windows-amd64`.

## Cause

scripts/release.sh builds the windows object only when XO_CLANG and
XO_WINSYSROOT are set, and printed one `==` line otherwise. The release
environment set neither, the line was lost in the log, and
release-verify.sh did not look for the object, so every release from
0.44.0 through 0.51.1 shipped without it.

## Fix

release.sh stops when the two variables are unset (opt out:
XO_RELEASE_NO_WIN_SQLITE=1), and release-verify.sh fails a windows
archive without sqlite.obj, its .key and .sha256 (opt out:
XO_VERIFY_NO_SQLITE=1; internal/repotest
TestReleaseVerifyWindowsSqlite). toolchain.md "Versions and releases"
step 4 names the two values on the Mac.

What a release promises is checked on the archive, and a step that
cannot do its part stops: a warning in a release log is not read.
