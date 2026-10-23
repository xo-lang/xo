# darwin cc links embed object modification times

Status: Observed 2026-10-23 (internal/arm64 TestModuleCheckAuto, golden
use_c_add).

Symptom: two builds of a program linked through cc on darwin (a `use c`
source, `--ld=cc`) differ in 179 bytes with identical code: the LC_UUID
and the code signature's hashes. TestModuleCheckAuto reported "the warm
build with XO_CHK_CACHE=auto differs from the cache-off build" once three
new goldens moved use_c_add into its every-8th sample; two cache-off
builds differed the same way, so no cache was at fault.

Cause: ld64 writes each object's modification time into the debug map's
OSO stab entries (`nm -ap`), and the UUID and the signature hash them;
xo writes the objects anew on each build.

Fix: compiler/xo link.xo try_link runs the darwin cc link with
ZERO_AR_DATE=1, which makes ld64 write 0 (as reproducible build tools
do); dsymutil skips its time check for 0 (`--dsym` still writes the
DWARF). Xo's own Mach-O linker writes no debug map and was already
deterministic.

Related, same day (internal/arm64 TestDebugInfoSameCode/use_c_add): a
darwin cc link with debug information got `-headerpad 0x1000` (room for
the __DWARF load command embed_dsym adds, decision 0346 stage 3) and an
`-s` link did not, so __text started 0xfe0 later and every address and
adrp offset differed between the two. Every darwin cc link (and the
LLVM backend's) now gets the pad, `-s` included (4 KB of header); the
binary cache key still tells the two apart (`dwarf-embed`).
