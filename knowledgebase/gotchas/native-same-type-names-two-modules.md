# Two modules' types of the same name were one type on the native backends

Status: Observed (2026-10-10, building compiler/check with
`--backend=arm64`); fixed.

Symptom: `xo build --backend=arm64 compiler/check` (also LLVM) failed
with `XO1201 ... this field (support is planned)` on lines of
compiler/lexer and compiler/parser that read `d.span` of a `Diag`; the
Go backend built and ran the same program. A one file repro: module `a`
declares `pub struct Diag { span: Int }`, the main module declares
`struct Diag { file: Str, line: Int }`, and every field access of one of
them is "unsupported".

Cause: internal/ssa interns named types by their printed name
(`Named.String`, which qualifies std types but not user types), so the
second `Diag` met the first one's layout. Had the fields agreed by name,
the program would have built with the wrong layout. A type alias
(`type Type = parser.Type`) declares no type and is not a conflict.

Fix: `builder.typeName` qualifies a user type whose name another loaded
user module also declares by its module path (`ambiguousTypeNames`), for
interning and for the recursion check of boxed fields (which compared
the old names). Programs without such a clash get the same names as
before. Golden: `internal/gen/testdata/golden/same_type_names` (listed
for the native backends in `internal/llvm/testdata/gen_native.txt`).
Function symbols are qualified by the module's local name
(`funcName`); two modules with the same local name and a function of the
same name would still clash (not seen yet; next experiment: a golden
with `example.com/a/util` and `example.com/b/util` both declaring `fn
f`).
