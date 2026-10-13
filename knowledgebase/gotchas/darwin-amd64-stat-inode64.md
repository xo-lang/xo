# macOS x86_64's `stat` symbol is the old structure

Status: Anticipated (2026-10-13, decision 0213)

Symptom (expected): an Xo platform call of `stat`, `lstat`, or `fstat`
declared in std/native/sys_darwin.xo would read garbage sizes and times
on darwin/amd64: the fields of std/native/llrt/darwin.xo's layout (mode
at 4, mtime at 48, size at 96) are those of the 64 bit inode structure.

Cause: on x86_64 the C headers rename `stat` to `stat$INODE64` (and so
on) unless an old deployment target asks for the 32 bit inode
structure; the plain `_stat` symbol keeps the old layout for old
binaries. arm64 has only the 64 bit inode functions under the plain
names. An Xo function name cannot contain `$`.

Fix: sysio.xo uses libSystem only on darwin/arm64 (layer 2); darwin/amd64
stays on the C runtime. To extend it, give the platform declaration a
way to name the symbol (`stat$INODE64`) or declare an x86_64 variant,
then run TestSysioParity on an Intel Mac (scripts/intel-mac-test.sh).
