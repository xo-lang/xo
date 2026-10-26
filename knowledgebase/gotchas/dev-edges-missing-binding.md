# dev-edges-missing-binding: a `use go` program's edges are silently absent

Observed 2026-10-26.

Symptom: `xo dev edges DIR` on a program that says `use go "net/url"` prints
thousands of lines and exits 0, and every one of them is a standard library
reference: the program's own file names nothing, so a caller that reads the
output as "this program's resolved references" sees a program with no calls.
A sweep that runs the command once per program directory then measured every
`use go` golden as empty (the first std coverage audit, 2026-10-26, lost all
of them and the loss was invisible against 890 programs).

Cause: the program's `use go` package has no binding in the directory's
`.xo/gobind` (check, build and test bind missing packages on their own; the
`xo dev` sessions do not). The check stops before names resolve, the standard
library alone checks, and the session reported no error, so the caller could
not tell.

Fix: `compiler/cli/servers/edges.xo` returns the session's stopped reason
(Session.stopped, set from the check's unsupported message), so the command
exits 1 with `use go "net/url": ...` on standard error;
TestDevEdgesMissingBinding holds it and TestDevEdgesOtherCommandsKeepAnswering
holds the rest of the agent commands unchanged. The scope is the edge list
only: `xo def`, `xo doc`, `xo refs`, `xo query` and `xo symbols` still answer
from std and say that the check stopped, which is useful to them. Run
`xo bind --dir=DIR <go imports...>` in the module before the sweep, or use a
golden that commits its `.xo/gobind`; the binding can be deleted afterwards.
The rule and the sweep procedure are in
[agent-tools.md](../infrastructure/agent-tools.md) under `xo dev edges`.
