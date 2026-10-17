# lld-link stamps the link time into every Windows binary

Status: Observed (2026-10-12), fixed 2026-10-13 with `/Brepro`.

Symptom: xo1 built natively on Windows by xo1 (the fixed point of
decision 0197 stage 7) was byte identical to its parent except two
fields: the COFF file header's TimeDateStamp and the optional header's
CheckSum (bench/results/windows-2026-10-12.md, "Native build on
Windows, and the fixed point"; compared with both zeroed). Any two
links of the same program a second apart differ the same way, so a
rebuild never compares equal with `cmp` or a hash.

Cause: lld-link, like MSVC's link.exe, writes the link's wall clock time
into the COFF header (and the debug directory) unless told otherwise;
the checksum covers the header, so it changes with it. clang's driver
passes no determinism flag for the msvc triples by default.

Fix: both drivers link Windows with `-fuse-ld=lld -Wl,/Brepro`
(compiler/xo/link.xo sys_link_args, internal/build/llvm.go
sysLinkArgs). With /Brepro, lld-link writes a hash of the output in the
time stamp field instead of the time. Checked 2026-10-13 on a Windows
Server 2025 EC2 instance (scripts/remote-windows.sh --cmd, on demand,
i-EXAMPLE48, commit 86407341): the arith golden built twice, 3 s
apart, by the Go driver and by xo1 (built there by the Go seed): each
driver's two binaries are byte identical (`cmp`), and all four are
203,264 bytes with TimeDateStamp 0x8775d4e6 (a hash, not a 2026 time). The 25 minute xo1 self build was
not rerun for this: the link flag is the same for every program.

Watch for: a tool that reads the PE time stamp as a date (it is now a
hash and can be any value, including one in the future).
