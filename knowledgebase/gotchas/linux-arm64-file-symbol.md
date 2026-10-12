# arm64 backend binaries differed between two builds on Linux

Status: Observed (2026-10-12, EC2 Amazon Linux 2023, gcc 11 as `cc`),
fixed in internal/arm64/arm64.go and selfhost/arm64/gen.xo.

Symptom: internal/build TestRuntimeObject/arm64 failed on linux/arm64 with
`binaries differ: 363976, 363976, 363976 bytes`: three builds of one
program, same size, 6 bytes apart.

Cause: the backend's assembly had no `.file` directive. CC() is `cc`,
gcc on that image, which assembles the .s to a temporary object
(/tmp/ccXXXXXX.o), and GNU ld gives an object with local symbols and no
STT_FILE a file symbol named after the object: `ccufyG0A.o` in one
build, `ccLWaDNr.o` in the next. Mach-O has no such symbol, and the
LLVM backend's objects carry their source name, so only arm64 on Linux
was hit.

Fix: on Linux the assembly starts with `.file "prog.s"`; the symbol is
then the same in every build (checked with `readelf -s`: `FILE LOCAL
ABS prog.s`). The golden assembly testdata/*.linux.s gained the line.
TestRuntimeObject is the regression test (on a Linux host with gcc as cc).

Look first at the STT_FILE symbols (`readelf -s -W bin | grep FILE`) and
at `strings -t d` around the bytes `cmp -l` reports when two Linux
binaries differ by a few bytes.
