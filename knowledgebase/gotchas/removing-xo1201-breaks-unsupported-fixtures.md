# Making a construct native breaks the tests that used it as "unsupported"

Status: Observed (Xo Lang 1, 2026-11-05)

## Symptom

cmd/xo TestCheckSupportPerMember failed on main (`exit 0, want 1`: no
XO1201 for module v) after 174b0c648, a change to the SSA builder that
the test never touches.

## Cause

The test needed one XO1201 and took it from a construct the native
backends did not have, a function value with `var` parameters.
174b0c648 gave the native backends that construct, so the fixture
stopped producing the diagnostic. That push's gate ran the backend
goldens and the codegen patterns, not cmd/xo.

## Fix

The cmd/xo tests that need an XO1201 share one construct,
`nativeUnsupportedStmt` in cmd/xo/unsupported_test.go (a Map keyed by a
Set, planned), with its reason; when the native backends gain it,
change it there to another planned construct. A change that removes an
XO1201 (makes a construct native) greps the tests for the construct
before it pushes: cmd/xo, internal/selfhost, internal/llvm,
test/errorcheck. `use go` and `use c` stay unsupported natively for
good, but `xo check` binds a `use go` package (it needs the Go
toolchain), so they are poor fixtures for a check test.
