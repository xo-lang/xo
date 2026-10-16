# 0247: A native build checks the runtime's bodies as it builds them

Status: Proposed (stub, xo-4b, 2026-10-16; approved by the lead for
the playground, decision 0242)

The function bodies of the native implementation and runtime modules
(std/native/*.xo, std/native/llrt/*.xo) are checked when the SSA builder
first builds them, not all before the build; `xo check --no-gc` and a
-short test (TestStdChecks) keep checking every body. To be filled in
with the measurements.
