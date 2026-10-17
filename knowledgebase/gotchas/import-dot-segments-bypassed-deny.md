# Dot segments in import paths got past deny rules

Status: Observed (2026-10-10)

Symptom: with `deny example.com/app/main -> example.com/app/lib` in
xo.mod, `use example.com/app/lib` was `XO1312`, but
`use example.com/app/./lib` and `use example.com/app/x/../lib` checked
with 0 errors. `use ./lib` gave `XO1306` with a hint to `xo get ./lib`.

Cause: both lexers accepted `.` and `..` as whole path segments (only
leading, trailing, and doubled `/` were rejected). Module resolution
cleaned the path, so the package loaded, but `checkDeny` matched the
path as written, which no rule named.

Fix: the lexers (`internal/lexer/lexer.go` `hasDotSegment`,
`compiler/lexer/lexer.xo` `has_dot_segment`) report such paths as
`XO0158`, and both `loadImports` and `checkDeny` use `path.Clean` of the
path, so a package has one spelling everywhere even if a malformed path
reaches them. After the deny check, the loader binds such an import to
no module, so `XO0158` is the only error about it (no `XO0601` or
`XO1306` with a wrong `xo get ./lib` hint). Tests: `TestErrors` (lexer), the selfhost lexer cases,
`TestResolution` "dot segments do not get past a deny rule" and
"relative import reports only XO0158". Any new
check that compares import paths should compare cleaned paths.
