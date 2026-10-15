# LTO of single module LLVM builds (2026-10-15)

A single module LLVM build (no `XO_LLVM_SPLIT`) used to pass `-flto` at
every optimization level, so the link ran whole program optimization and
code generation on one core. Now `XO_LLVM_LTO` (full, thin, none) picks
the mode, and unset it is `none` at `-O0` and `full` otherwise
(selfhost/xo split.xo `single_lto`).

Machine: the development Mac (Apple silicon, arm64), one build at a time
through the heavy lock, base e7fba1f5 plus this change.

## selfhost/xo, `xo build --backend=llvm`

| Opt | LTO | Wall (s) | Peak RSS (GB) |
|---|---|---|---|
| O0 | none (new default) | 128.74 | 6.8 |
| O0 | full (old default) | 1871.27 | 10.8 |
| O2 | full (default) | 2143.01 | 9.9 |
| O2 | thin | 2018.41 | 10.9 |

Every built xo printed `xo 0.13.0 darwin/arm64`. The O2 none build was
stopped (its output loses, below). At O0 no LTO is 14.5x faster; at O2
thin saves 6 percent of the build and loses run time.

## bench/perf at O2: instructions retired, best of 5

Columns: full, thin, none, thin/full, none/full. Checksums matched.

```
alloc small 246407092 263287187 267014901 1.069 1.084
alloc mixed 462392865 465855659 465666166 1.007 1.007
alloc grow 64214617 64635545 64829405 1.007 1.010
alloc strings 246643214 251011238 252403070 1.018 1.023
alloc maps 266975387 270485225 273029975 1.013 1.023
alloc tasks 783081787 789280256 806508611 1.008 1.030
text parse_int 43458958 43043006 44046147 0.990 1.014
text quote 100927042 99726003 103129210 0.988 1.022
text split 82108487 80354284 88045535 0.979 1.072
text lines 139924208 132799171 147784297 0.949 1.056
text replace 57376828 56560746 59371700 0.986 1.035
text int_fmt 62967165 64152913 66010128 1.019 1.048
text base64 124737775 136292745 149388190 1.093 1.198
text hex 84657932 84288231 87124981 0.996 1.029
json small 298682168 300637525 310046359 1.007 1.038
json strings 270667879 271501916 275893450 1.003 1.019
json maps 299107396 286886443 303080562 0.959 1.013
json parse 252477040 252826184 255007405 1.001 1.010
json enc_strings 157602257 158755245 159738197 1.007 1.014
conc_spawn - 539289237 533754799 527216578 0.990 0.978
conc_fanout - 241396162 253305091 258081643 1.049 1.069
conc_pingpong - 324696421 336693716 344165377 1.037 1.060
conc_mutex - 118298112 122042549 134331682 1.032 1.136
coll putget 346781007 347223061 347494674 1.001 1.002
coll get 131880140 132212684 132457954 1.003 1.004
coll push 106585267 113006615 113124694 1.060 1.061
coll update 341689032 341651539 341737005 1.000 1.000
coll iter 75559476 84886799 85082695 1.123 1.126
coll copymut 79659743 78923449 79591147 0.991 0.999
coll branch 766393009 766646432 767142486 1.000 1.001
coll setat 100505699 100905759 101117855 1.004 1.006
coll stack 94986108 93260554 95528889 0.982 1.006
coll fields 108648903 108900463 109205583 1.002 1.005
coll grid 144622566 144992812 166257093 1.003 1.150
coll insrem 1933419280 1046938240 2034483096 0.541 1.052
coll copyset 46377700 46665413 46907281 1.006 1.011
coll sortint 591723845 583216385 592895423 0.986 1.002
coll sortpt 831476472 822288897 831105801 0.989 1.000
coll hof 309625373 314943809 315117392 1.017 1.018
coll maxby 1704733710 1708111941 1741567123 1.002 1.022
coll iface 280339024 280662325 280871188 1.001 1.002
```

Thin loses beyond noise on text base64 (1.093), coll iter (1.123), coll
push (1.060), conc_fanout (1.049), and wins only coll insrem (0.541);
none loses more (base64 1.198, grid 1.150, conc_mutex 1.136). So O2
keeps full LTO, and O0 (where speed is not the point) drops it.

Reproduce: `XO_LLVM_OPT=O2 XO_LLVM_LTO=<mode> xo build --backend=llvm`
per workload, `/usr/bin/time -l` for instructions retired.
