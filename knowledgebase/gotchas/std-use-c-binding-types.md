# A std module's `use c` binding: types unnamed in signatures, paths read from the program's directory

Status: Observed (2026-11-06, while writing std/sqlite, decision 0534)

**Symptom.** `xo test --std-module=native/sqlite` reported `XO0601 module
\`c3\` has no type \`Sqlite3\`` on every struct field and signature that
named a handle type of the module's `use c "sqlite/xo_sqlite.h" ... as c3`
binding, while calls of the binding's functions in bodies checked. The same
header in a program (a struct field `db: xsq.Sqlite3`) checked and ran.

**Cause.** The checker declares and resolves std's tree first (modules with
`std_tree`, decision 0388), then the program's. A `use c` binding module is
`is_std` but was never `std_tree`, so a std module's binding went with the
program's modules: std's signatures resolved before the binding's types were
declared. Function bodies resolve later, so calls found the functions.

**Fix.** compiler/check/cimport.xo: a binding module takes its importer's
`std_tree` (2026-11-06), so a std module's binding is declared with std.

**Second symptom, same day.** Run from another directory (a golden's),
the module's `link "sqlite/xo_sqlite.c"` was `XO1104 no such file
<program dir>/std/native/sqlite/xo_sqlite.c`: a std module's `dir` is the
label `std/native`, which cimport joined to the program's directory. It
worked only from the repository root, where the label happens to be a real
path. cimport now maps a std-tree module's dir onto the std directory
(`c.std_dir`).

**Test.** cmd/xo TestStdModuleUseC: a copy of std gains std/ctiny, whose
native file names a `use c` handle type in a struct field and links a C
source; a program in a temporary directory runs it.

**Anticipated.** The binding cache of a std module is `std/native/.xo/cbind`,
inside the std tree. A release's std may be read only (a package manager's
install): the release ships the binding, or the cache moves to the main
module's `.xo/cbind` as a dependency's does (use-c-in-dependency-readonly-cache.md).
Decision 0534's packaging step decides.
