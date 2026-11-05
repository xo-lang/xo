# A struct holding an Option of itself breaks a debug build

Status: Observed (2026-11-05, std/yaml's Node, branch yaml-s3)

## Symptom

`xo test` or `xo run` (any build with debug info, such as the xo built from
the checkout) of a program that reaches std/yaml fails while building std's
object store: `xo: warning: separate compilation is off: std's store build
failed (xo dev stdobj exit 70)`, then either `fatal error: SIGBUS` (a stack
overflow in arm64's `dv_type_ok`, compiler/arm64/dwvars.xo, which walks
`struct Node { alias: Node? }` through Option's variants and back to Node
without end) or `fault: index 1 out of range for List of length 1 at
arm64/isel.xo:3833`. The same shape in a small program compiles and runs
(only the std store build with debug info hits it).

## Cause

`dv_type_ok` stops at List and Box ("a recursive type stops there") but not
at an Option (an enum) or a struct that contains itself through one.

## Fix

std/yaml's `Node.alias` is a `List[Node]` of zero or one node, not a
`Node?`. A compiler fix (stop `dv_type_ok` on a type already being walked)
is Xo Lang 1's; when it lands, `Node.alias` can become `Node?` in a breaking
change.
