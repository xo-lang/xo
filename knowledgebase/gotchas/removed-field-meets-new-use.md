# A push that removes a struct field meets a new use of it on main

Status: Observed (2026-10-31, xo-lang-2: Inst bits ce6a728b7 and Lang 1's 0cb73d571; fixed in pushq e3630dd65)

Symptom: main stopped building (XO0601, unknown field) right after two
pushes that each built alone: ce6a728b7 replaced arm64 Inst's Bool
fields by bits of Inst.fl, and 0cb73d571, pushed minutes earlier, added
compiler/arm64/zimage.xo reading x.has_fault and x.poll.

Cause: pushq rebased the field removal onto main, found the incoming
commits overlapped compiler/arm64, and pushed without a gate (by
policy: hourly.sh checks main). A rename or removal is the one kind of
change whose rebase can break code it never touched.

Fix: pushq now type-checks the rebased tree with xocheck when the
incoming commits overlap compiler/ (the lead's e3630dd65, 2026-10-31),
so such a push stops before it reaches main. By hand, before a removal
or rename of a declaration (a field, a function, a variant): rebase onto
origin/main, run `go run ./scripts/xocheck compiler/xo` (seconds), and
look for new uses with `xo refs NAME` on the rebased tree.
