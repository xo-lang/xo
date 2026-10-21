# A module's bodies cached as the build's root replayed where it is a library

Status: Observed (2026-10-21).

Symptom: after `xo test compiler/ssa`, building compiler/arm64's tests
(which import ssa) said `the inlined arm64 module does not verify
(... ssa.body_facts: call value_self_faults: call of an unknown
function)` and failed with `arm64 backend: runtime call
value_self_faults`; with a fresh cache, or XO_FE_CACHE=off, it built.

Cause: a module that is the build's root names its functions unprefixed,
a library's are named with its module. The front end cache keyed a
program module's body by its interface closure and the build's root
directory, the same for both builds, so the entry the test build
recorded (calling `value_self_faults`) replayed in the library build,
where the function is `ssa.value_self_faults`. std bodies had the same
issue handled (std_roots).

Fix: ssa fe_keys keys a program module's body also by whether its module
is the root (`rootmod=`). Test: internal/arm64
TestFrontCacheRootThenLibrary.
