# x86 release sizes and what a hollow port would remove (2026-10-25)

Measured for the owner's question whether tonight's size cuts reach x86.
xo 0.30.0 darwin/arm64 built from main 3e64d815d (xobuild key
19f8b9524cb2fc38), release builds with the default backend per target
(the arm64 backend for arm64, the amd64 backend for x86; the PE linker for
windows/amd64). Sizes are file bytes and the text section: sizebreak's
`__text` for Mach-O and ELF, and for PE the `.text` section header
(VirtualSize, raw size alongside) since sizebreak reads Mach-O and ELF
only. The programs are bench/perf/twins/hello/xo and bench/perf/http/xo.

## Sizes

hello:

| target | backend | file | text | raw .text | hollow |
|---|---|---|---|---|---|
| darwin/arm64 | arm64 | 299,935 | 170,832 | | on |
| darwin/arm64, XO_PASSES=-hollow | arm64 | 399,160 | 242,340 | | off |
| darwin/amd64 | amd64 | 415,663 | 259,728 | | not applied |
| linux/arm64 | arm64 | 286,192 | 184,448 | | on |
| linux/arm64, XO_PASSES=-hollow | arm64 | 388,320 | 254,544 | | off |
| linux/amd64 | amd64 | 411,040 | 280,696 | | not applied |
| windows/amd64 | amd64 | 473,327 | 338,024 | 338,432 | not applied |

CRUD server:

| target | backend | file | text | raw .text | hollow |
|---|---|---|---|---|---|
| darwin/arm64 | arm64 | 1,999,342 | 1,410,348 | | on |
| darwin/arm64, XO_PASSES=-hollow | arm64 | 2,082,007 | 1,456,556 | | off |
| darwin/amd64 | amd64 | 2,512,030 | 1,880,336 | | not applied |
| linux/arm64 | arm64 | 2,036,880 | 1,476,116 | | on |
| linux/arm64, XO_PASSES=-hollow | arm64 | 2,113,008 | 1,522,560 | | off |
| linux/amd64 | amd64 | 2,527,368 | 1,946,024 | | not applied |
| windows/amd64 | amd64 | 2,509,929 | 1,975,696 | 1,975,808 | not applied |

`XO_PASSES=-hollow` on darwin/amd64 builds the same __text (259,728 B):
the pass is arm64 only (compiler/arm64/hollow.xo, decision 0371), which is
the gap the owner asked about.

## What hollow removes where it runs

| program | target | text with | text without | removed | share of text | file removed |
|---|---|---|---|---|---|---|
| hello | darwin/arm64 | 170,832 | 242,340 | 71,508 | 29.5% | 99,225 |
| hello | linux/arm64 | 184,448 | 254,544 | 70,096 | 27.5% | 102,128 |
| CRUD | darwin/arm64 | 1,410,348 | 1,456,556 | 46,208 | 3.2% | 82,665 |
| CRUD | linux/arm64 | 1,476,116 | 1,522,560 | 46,444 | 3.0% | 76,128 |

The x86 builds today carry that code: darwin/amd64 hello is 259,728 B of
text against 242,340 B of arm64 text *with the dead code still in it*, and
linux/amd64 is 280,696 against 254,544.

## Unreachable bytes (sizebreak -reach, arm64 no-hollow builds)

| program | target | symbols | reachable | unreachable | bytes | share |
|---|---|---|---|---|---|---|
| hello | darwin/arm64 | 691 | 411 | 280 | 72,660 | 30.0% |
| CRUD | darwin/arm64 | 1,910 | 1,711 | 199 | 46,748 | 3.2% |

The hollow pass removed 71,508 B of hello's 72,660 B: the difference is the
one 4 byte `brk #0x484f` each emptied body keeps (280 x 4 = 1,120 B, plus
alignment). Reach is a lower bound: it follows direct branches and
materialized addresses, not indirect calls.

-reach does not measure ELF binaries: its roots are `_main`, `_xo_entry`,
`_xo_x_rt_program` and `_xo_f_main`, and an ELF build's symbols have no
leading underscore (the failure is "no entry symbol found"), so the
linux/arm64 no-hollow builds above have sizes but no reach numbers.

## Estimate for x86 once hollow is ported

The unreachable functions are a property of the program, not the target,
so scaling the arm64 removal by each target's text ratio estimates the
amd64 effect:

| program | target | basis | estimate | share of text |
|---|---|---|---|---|
| hello | darwin/amd64 | 72,660 x 259,728/242,340 (1.072) | 77,900 B | 30.0% |
| hello | linux/amd64 | 70,096 x 280,696/254,544 (1.103) | 77,300 B | 27.5% |
| CRUD | darwin/amd64 | 46,748 x 1,880,336/1,456,556 (1.291) | 60,350 B | 3.2% |
| CRUD | linux/amd64 | 46,444 x 1,946,024/1,522,560 (1.278) | 59,360 B | 3.0% |
| CRUD | windows/amd64 | the same function set | about 60,000 B | 3.0% |

So a port reaches most of the arm64 win on x86: about 76 KB of hello's
260 KB of darwin/amd64 text and about 59 KB of the CRUD server's 1.88 MB,
with windows/amd64 out of scope for the pass (PE).

## Tool gaps this measurement ran into

- sizebreak -reach decodes arm64 only, so x86 unreachable bytes cannot be
  measured directly today; the estimates above are scaled, not observed.
- sizebreak groups an ELF binary's symbols as 100% non-Xo: the decode
  looks for `_xo_f_<module>_2e<name>` and ELF symbols are
  `xo_f_<module>_2e<name>`. The linux group tables are therefore
  unavailable, although the totals (which the report only needs) are.
- sizebreak -reach's roots are named with Mach-O's leading underscore, so
  no ELF build is measured even on linux/arm64.
- PE is outside sizebreak: the .text numbers here come from a debug/pe
  section dump.
