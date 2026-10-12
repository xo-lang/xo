# Self hosted SSA: a comparison by function name hides the module's order

Status: Observed (2026-10-11, decision 0166 stage 1)

Symptom: the self hosted LLVM emitter's module for selfhost/lexer
differed from internal/llvm's only by where `xo.debug[Str]` stood,
although TestSelfhostSSA reported every function identical at every
stage, and the emitter lowered every function identically.

Cause: TestSelfhostSSA compares functions by name (and the verifier's
lines as sorted lists), so the order of the module's functions is never
compared. The Xo SSA builder declared `xo.debug[T]` when asked for but
built its body after the current one, while the Go builder's genFunc
builds it at once; the functions the body declares (the Debug text of
the parts) came later in the module. The LLVM emitter writes functions
in the module's order and numbers string literals, defines types, and
writes helpers in the order of first use, so the order shows there.

Fix: selfhost/ssa builds `xo.debug[T]` when asked for, as decision
0150 already did for the generated functions whose bodies declare
functions; then (stage 2 of decision 0166) 17 modules still had
`zero.fn(...)` and the Debug text a frame's arguments ask for out of
order, from the frame argument formatters and zero functions still
queued, so every generated function is now built when asked for
(the arm64 emitter's port, decision 0158, found and fixed the same at
the same time: build.xo `build_gen`, the version kept at the merge). A comment saying a queued body "declares
nothing" was the trap: check what the body calls, not what it is. TestSelfhostLLVM counts the modules whose
definitions come in the same order and those byte identical; a drop in
either with every function identical points at the module's order.
When a later stage's output is written in module order (the arm64
emitter's assembly, the Go code generator), compare it whole too.
