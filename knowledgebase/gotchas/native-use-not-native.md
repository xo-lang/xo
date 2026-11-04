# A std/native module reached by a plain `use` is not marked native

Status: Observed (2026-11-03, `go run ./scripts/stdgen -check` failed on main)

Symptom: after 8c8c46e67, stdgen -check said std/rt/std_sigs_prelude.go
was stale, and regenerating it added native/rt's bodyless functions
(Rt_str_find, Rt_rs_append, ...) that std/rt does not define, so
`go vet ./std/rt` failed. pushq's stdgen check fires only for
`std/*.xo`, so the push that caused it (it changed std/native/ files)
never ran it.

Cause: compiler/check marks a module `native` only when it loads it as
the implementation of a std path (load.xo). std/native/norm.xo and
idna.xo, which the Go backend compiles from their native source, gained
`use native/rt`; reached that way native/rt is an ordinary std module,
and compiler/gen's std_sigs (stdtypes.xo), which takes `is_std &&
!native`, treated its bodyless functions as hand written Go runtime.

Fix: std_sigs also skips modules whose path starts with `native/`. A
check that means "an implementation source" tests the path, not
`native` alone. Lang 4 found the commit with git bisect run over std
trees with one fixed xo (XO_CLI_STALE_OK=1, a private
XO_BACKEND_CACHE_DIR).
