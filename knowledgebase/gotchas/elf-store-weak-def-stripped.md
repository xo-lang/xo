# A program's weak definition must root the store's copy

Status: Observed and fixed (2026-10-28)

**Symptom.** On linux/arm64, separately compiled builds of json_text and
conc_scopes died with SIGSEGV at pc 0, frames empty (json_text right after
its floats line, conc_scopes after "background stopped 1"); the same
programs built whole (XO_SEPCOMP=0) ran. gdb: `bl 0x0` in main (the call
of `xo.map.put[Map[Str, Int]]`), `blr x2` with x2 = 0 in cabi.task.free.

**Cause.** A program defines a shared helper or generic instance weak
(compiler/arm64 gen.xo shared_def, f35923ba3) and std's store may define
it too; the link keeps the store's copy, the first. elfstore.xo's
liveness walk was rooted at the program's undefined symbols only, so the
store's copy, which nothing in std reached, was stripped: the symbol
resolved into an empty dead section, address 0, and elflink resolves a
weak symbol nothing defines to 0 without a word. The Mach-O store link
roots the program's weak definitions too (store.xo); the ELF port missed it.

**Fix.** link_store_elf roots a program symbol that is undefined or weak;
a store link that leaves an Xo function (xo_f_*, a hashed xo_h*) weak and
undefined fails (elflink.xo ELinker.from_store). scripts/docker-sepcomp-sample.sh
runs 26 std-heavy goldens separately compiled and whole on linux in Docker
and compares them (main before the fix: 10 of 26 called 0; after: 26 of
26 the same); part of a linux store or default change's checks.
