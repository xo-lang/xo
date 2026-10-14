# The ELF goldens need a linux/arm64 runtime pack a checkout does not have

Status: Observed (2026-10-14), fixed (2026-10-14).

Symptom: TestDevGoldensELF (internal/devgoldens, linux/arm64 hosts only)
failed every program in the Linux arm64 milestone run at main f8ebc9ec:
`--ld=macho needs a prebuilt linux/arm64 runtime pack for the flags -O2
(decision 0157): prebuilt runtime: not used (no prebuilt runtime for
linux/arm64 in /home/ec2-user/xo/lib/xo/rtpack)`.

Cause: `--ld=macho` links no C, so it takes the runtime from a pack
(XO_RTPACK, else lib/xo/rtpack beside bin/). A release ships one; a
checkout's bin/xo (internal/xotool) has none. The Go test that replaced
scripts/elf-goldens.py (decision 0209) ran the builds without building
a pack first, and the test only runs on linux/arm64, so the Mac never
saw it.

Fix: the test builds the pack of the checkout's runtime sources with
`xo dev rtpack --target=linux/arm64` into `<user cache>/xo/devgoldens/
rtpack/<pack key>` (reused while `xo dev rtpack --key` is the same) and
sets XO_RTPACK to it. It skips, naming the reason, only when the host
has no C compiler or archiver to build the pack; any other failure of
the pack build fails the test.
