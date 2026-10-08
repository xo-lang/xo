# Xo defer is block scoped, Go defer is function scoped

Status: Observed (2026-10-07, P1 code generation)

When lowering to Go, a `defer` inside a nested block cannot use Go `defer`
directly. The generator runs a block that contains `defer` as an immediately
invoked closure `func() int { ... }()`, so Go's function scoped defer is the
block's; `return`, `break`, `continue`, and `?` inside it return a control
code that is dispatched after the call. Deferred code runs through
`tk.Shielded` (cancellation shield, core 3.5, decision 0022). A function
body's own defers use Go defer directly. Golden test: `defer_blocks`.

Agents with Go habits may also expect function scope; `xo explain` for
related diagnostics should mention it.
See [../decisions/0009-block-scoped-defer.md](../decisions/0009-block-scoped-defer.md).
