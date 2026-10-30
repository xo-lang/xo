# A large constant's initializer costs clang's greedy register allocator most of a build

Status: Observed (2026-10-30)

Symptom: the template goldens were the slowest LLVM tests of the full tier (TestGoldenSplit
template_* 140 s each, TestGolden 83 s, TestDebugInfoSameOutputLLVM 63 s); a template_basic build
at -O1 spent 52.6 of 54 s in clang (`phase cc`), the same on an identical rebuild.

Cause: std/html's ENTITY table (2,231 entries) is built by one generated function,
`xo.const[html.ENTITY]`, about 45 thousand lines of straight line IR (template.ATTR_TYPES 2.4
thousand). clang -ftime-report: Greedy Register Allocator 39.7 of 46 s, superlinear in the
function's size. Every program that imports std/html (template, html) paid it.

Fix: compiler/llvm func.xo big_const_init marks a constant's initializer over BIG_CONST_INSTRS
(2000) SSA instructions `noinline optnone` (#20, #21 under --race's runtime): it runs once, and
optnone takes LLVM's fast register allocator. clang -O1 on template_basic's IR (xo bench --ab, n=5, Mac): 41.8 to 5.2 s (0.125), clang's RSS 0.881, the object
3.1 to 2.4 MB (its __text 448,528 to 448,716 bytes). To find another: `xo asm --backend=llvm` and count each define's lines; then
`clang -O1 -c -ftime-report` on the IR.
