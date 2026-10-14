# macOS x86_64's `stat` symbol is the old structure

Status: Observed (2026-10-13, decision 0213; the fix verified on an Intel Mac 2026-10-14)

Symptom (expected): an Xo platform call of `stat`, `lstat`, or `fstat`
declared in std/native/sys_darwin.xo would read garbage sizes and times
on darwin/amd64: the fields of std/native/llrt/darwin.xo's layout (mode
at 4, mtime at 48, size at 96) are those of the 64 bit inode structure.

Cause: on x86_64 the C headers rename `stat` to `stat$INODE64` (and so
on) unless an old deployment target asks for the 32 bit inode
structure; the plain `_stat` symbol keeps the old layout for old
binaries. arm64 has only the 64 bit inode functions under the plain
names. An Xo function name cannot contain `$`.

Fix (2026-10-14, B16 option D): std/native/sys_darwin.xo declares
`stat64`, `lstat64`, and `fstat64` (the 64 bit inode structure under
plain names, exported on x86_64 and arm64 alike) and
`__getdirentries64` (what readdir calls; its records are the 64 bit
inode struct dirent, d_reclen at 16, d_namlen at 18, d_name at 21) in
place of stat, lstat, fstat, opendir, and readdir, on both
architectures, so darwin/arm64 runs the same code. sysio.layer() is 2
on darwin/amd64 too, and xo_fs.c's operations are gone. Of the other
declared symbols only open, read, write, and close have variants on
x86_64 (`$NOCANCEL`), whose plain names are the right ones (the
Xcode SDK's .tbd files, 2026-10-14). On an Intel Mac (scripts/intel-mac-test.sh,
2026-10-14, 88341497): TestSysioParity, TestErrnoTables, and the fs
goldens pass (bench/results/fs-ops-2026-10-14.md, "darwin/amd64").
