# A function value's thunk defined by both std's object and the program

Status: Observed (2026-11-16, task #146)

Symptom: with the std variant (separate compilation, the default), a
program's link fails with `duplicate symbol _xo_f_hpke_2ekey__err_2evalue
in prog-darwin.42.o and .../w-*/prog-darwin.0.o`. A fresh
`XO_BACKEND_CACHE_DIR` with `XO_SEPCOMP_BG=0` hides it (that build is
whole program), which makes it look like a stale cache entry; it is not
(std's entry is keyed by every std file's hash). `XO_BACKEND_CACHE=verify`
does not catch it either: verify compares run cache entries, and this
build fails at its link.

Cause: a std function generic only over effects (an implicit `Rand`
parameter, decision 0652) is built by the program, not by std's object
(no `[` in its name, but its effect variable makes it an instance). Its
body's function value (`map_err(key_err)`) makes the thunk `NAME.value`,
named by its callee alone, which std's object also made for another
body (decap). Generic instances were weak (arm64 gen.xo shared_name);
thunks were strong.

Fix: shared_name counts `NAME.value` as shared, so both copies are weak
definitions and the link keeps one. Test: golden sepcomp_thunk_shared
(arm64 TestGoldenRun, which links against the std variant).
