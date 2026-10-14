# A dangling `.` swallows the next statement

Status: Observed (2026-10-13, decision 0219)

Symptom: in a function body,

```xo
let r = http.new_request("GET", "/")
r.
let q = p.x
```

`xo check` reports XO0160 "expected a field or method name after `.`,
found keyword `let`" and XO0601 "`http.Request` has no field or method
``", and then reports `p` unused: the `let q = p.x` line is lost in the
recovery. A keyword after the `.` (`w.scope(...)` with the keyword
`scope` as a method name) also gives XO0407 "method `` mutates its
receiver and cannot be used as a value" with an empty name.

Cause: the selector's name is parsed with `expect_ident`, which on a
keyword returns an empty name without consuming it, and the statement
recovery then skips to the next statement boundary past the following
line.

Fix (for `xo lsp`): completion does not depend on the parser's recovery.
It inserts a placeholder name (`xo_complete_`) at the cursor before
checking, so `r.` parses as `r.xo_complete_` and the next line survives
(selfhost/cli/servers/lsp_complete.xo, as gopls's source fixing does).
`xo check` still loses the line; a parser fix would change diagnostics
and front end recordings (decision 0217), so it is left for its own
decision. Also: `scope` is a keyword, so it cannot name a method.
