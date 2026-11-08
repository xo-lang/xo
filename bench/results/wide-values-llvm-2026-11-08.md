# Wide values: equality by address, and a literal experiment dropped

Date: 2026-11-08. darwin/arm64 (the development Mac); identity on the
linux/arm64 offload host 2.

## Equality by address (0a1b1e803)

A 4,096 word struct compared once with `==` (scratch program wbC):

| Measure | Before | After | No `==` |
|---|---|---|---|
| native __text (bytes) | 801,600 | 424,576 | 423,752 |
| native emit | 3.77 s | 0.28 s | |
| LLVM cc | 188.8 s | 15.1 s | 12.1 s |

Identity against 4444be505: arm64, 638 programs, 52 differ (the TLS and
crypto goldens, tuple_wider, wide_box, xo; same xo_x_obj_str_eq call
count in tls12_cbc and tls_x509, about 1.6 KB less __text); LLVM, the 50
goldens with eq, tuple, box, tls, crypto or wide in their names, 2 differ
(eq_large_struct, tuple_wider); none failed.

## Constant aggregate literals: no gain, not pushed

The 12.1 s "no `==`" figure above predates the equality change. The
remaining wide-literal cost looked like the insertvalue chains: wbN's
main (a 4,096 word struct of nested 8 field defaults) held 18,739
insertvalues of constants. Emitting an all-constant struct or tuple
literal as one LLVM constant aggregate cut the module's insertvalues from
20,884 to 1,790 and its IR from 4.8 MB to 4.0 MB, but clang's time did
not change (base and new alternating, same IR otherwise):

| clang on wbN's IR | base | new |
|---|---|---|
| -O1, round 1 / 2 | 3.92 / 3.89 s | 3.96 / 3.89 s |
| -O2, round 1 | 4.42 s | 4.40 s |
| xo build --backend=llvm cc phase, wbN | 5.35 s | 5.27 s |
| the same, wbC | 10.78 s | 10.69 s |
| the same, hello world | 3.67 s | 3.68 s |

Constant insertvalue chains are cheap for clang, and wbN costs about
1.6 s more than a hello world (most of a build's clang time is std's
code). An earlier -O1 reading of 8.4 s against 5.5 s was load on the Mac,
not the change. The patch is kept out of the repository (it would also
change what debug info sees of those values).
