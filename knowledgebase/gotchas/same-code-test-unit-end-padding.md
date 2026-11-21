# TestDebugInfoSameCode failed on one function: otool gave it the bytes after its end

Status: Observed (2026-11-21, task #601, tier 922 on 0.55.0 candidate 8)

Symptom: `TestDebugInfoSameCode/http_max_idle` and `/grpc_balance`
(darwin/arm64) failed with "1 functions differ", again on the test's
rebuild: `_xo_f_report_23spec3c4cd265d449` had one more `udf #0x0` at
its end in the build without debug information, and
`_xo_f_grpc_2eClient_2ethrottle__fail_2efn65_23once` a `brk #0x484f`
and a `udf #0x0` more in the build with it.

Cause: no code differed. `otool -tV` gives a function every word up to
the next symbol it prints. The last function of a unit (one object
file of the build) so also gets the unit's end marker
(`_xo_utend_*: brk #18511`, which otool prints no symbol line for) and
the zero words the linker puts before the next object's `__text` when
that object is 8 byte aligned: a unit with a Float constant has
`.p2align 3` in its text since 553a8b9bd (2026-11-19, task #87). Which
function ends a unit, and whether 4 bytes of padding follow, depends on
the sizes before it, and the build with debug information has a
function more in some units. The assembly kept with `XO_KEEP_ASM=1
XO_BUILD_DIR=DIR` showed it: `report#spece24cd2630584` was the first
function of prog-darwin.43.s, and `throttle_fail.fn65#once` the last of
its unit only in the build with debug information. Latent since
553a8b9bd; std/http's change e16a72414 (task #577) moved the layout
onto it for these two goldens.

Fix: the test's reader (internal/arm64 debugvars_test.go sameCodeFuncs)
drops trailing zero words and the unit end marker from a function's
end; TestSameCodeFuncsUnitEnd holds the cases, and that real code after
a function still counts.

Avoid: when a same code test reports a difference at the very end of a
function, look at the kept assembly for what follows the function
before suspecting the code generator: unit ends, markers and linker
padding belong to no function.
