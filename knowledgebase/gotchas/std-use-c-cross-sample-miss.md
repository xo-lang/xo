# A std module's `use c` broke cross builds only the sample could see

Status: Observed 2026-11-07

**Symptom.** hourly.sh's cross-target step (`xo dev goldens --build-only
--flags="--backend=amd64 --target=windows/amd64" --sample 4`) failed on
internal/llvm/testdata/golden/native_sqlite with clang's "fatal error:
'stdlib.h' file not found". It had been broken since std/sqlite landed
(f04a8b4ca, 2026-11-06); the every 4th sample reached native_sqlite only
after new goldens shifted the list.

**Cause.** A windows/amd64 build with a `use c` source compiles it with
clang against the MSVC C library and the Windows SDK, which a Mac or Linux
host has only as the XO_WINSYSROOT sysroot (xwin's), and hourly.sh sets
none. dev goldens counts that failure as a documented limit when the
golden's own directory has a `use c` line (use_c_add and the others), but
native_sqlite's `use c` comes from std/sqlite, so it counted as a
failure. Decision 0534 stage 2 (a prebuilt SQLite object per target, no
C compiler) was never built.

**Fix.** xo build stops such a build with its own message naming
XO_WINSYSROOT and 0534 stage 2 (compiler/xo main.xo win_sysroot_missing),
which dev goldens' classify counts as unsupported; cmd/xo
TestStdSQLiteCrossBuilds builds native_sqlite for windows/amd64,
linux/amd64 and darwin/amd64 every time, so a sample cannot skip it. A
golden that reaches C through std is a cross build case of its own: give
it a test outside the sample.

**Also (2026-11-07, the lead's tier 58 on linux/arm64).** The test passed
on the Mac and failed on Linux: darwin/amd64 skipped the "does not cross
link" stop on every host (main.xo's mac_x86, meant for Xo's Mach-O linker,
which needs no cc), so a `use c` program reached the Linux host's clang
with Apple's `-arch x86_64` ("unsupported option '-arch' for target
'aarch64-unknown-linux-gnu'"). The skip now holds only on a Mac or with no
cc (mac_x86_links); from Linux the build stops with the documented
message. A cross test that accepts "a documented limit" runs on each host
kind it can run on before it lands.
