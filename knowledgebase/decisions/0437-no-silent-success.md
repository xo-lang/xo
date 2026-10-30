# 0437: no silent success in the tools

Status: Proposed (xo-lang-4, 2026-10-30; owner's queue item from the agent survey of 2026-10-30)

Stub: being filled in. A command that did nothing must not look like it passed: `xo test` names its skips in the summary line; `--std-module` resolves bare names or exits 2 with a did-you-mean, and fails when it runs no test; XO_* debug variables that need the backend are not silently served by the run cache; `xo query` outside a module exits 2; the Go test helpers fail a `-run` that matches nothing in a package with goldens.
