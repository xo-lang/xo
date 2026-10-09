# Every program has std/http loaded, used or not

Status: Observed (2026-10-09).

Symptom: deciding "the program does not use std/http" by
`info.Module("std/http") == nil` never fired: hello world still linked
net/http.

Cause: the checker always loads std/testing (it provides the predeclared
`os`, `expect`, and `testing` of test blocks, `loadPrelude` in
internal/check/load.go), and std/testing imports std/http, std/log,
std/sql, and std/task. So every program has those modules loaded.

Fix: ask whether the program's own modules (or the prelude) import the
module, directly or through other std modules: `reachesStd` in
internal/gen/gen.go walks `File.Imports` from the non-std modules and the
prelude. Test binaries count as importing std/testing, so they keep
everything. Used for the build tag `xo_nohttp` (gen.Program.Tags).
