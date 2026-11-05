# A program type named like a runtime handle took the handle's place

Status: Observed (2026-11-04)

Symptom: aws's generated DynamoDB client (its `enum Select`) and a
full duplex HTTP test in one build failed on arm64 and LLVM with "in
http.PipeWriter.write: the runtime call rt.select_new returns a Select"
(aws issue 10). Either alone built. A 25 line program with `enum
Select` (or a struct, or a newtype, named Select) and a `select`
statement fails the same way on 0.38.1.

Cause: the compiler interns some runtime handles under their bare names
(ssa Module.handle: Scope, Select, Listener, Conn), and Module.handle
returns whatever type is already interned under that name.
ambiguous_type_names gives a program's type its module path only when
another module or the prelude has a type of that name; Select is not a
prelude type, so the program's Select was interned under "Select" and
select_new's result took its type. Every case found fails at build
time; none was seen to build.

Fix: build.xo RT_HANDLE_NAMES lists the bare handle names, and a program
type with one of them counts as ambiguous (its module path names it).
A new Module.handle name goes into that list. Regression test:
internal/arm64 TestProgramTypeNamedLikeRuntimeHandle.
