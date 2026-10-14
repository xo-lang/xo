# The windows-amd64 seed cannot build xo on Windows (XO1306)

Status: Observed (2026-10-14, the lead on the Windows warm machine).

Symptom: on Windows, `go run ./scripts/xobuild` with the 0.11.0
windows-amd64 seed fails with XO1306, "module of
github.com/xo-lang/xo/selfhost/check is not required by xo.mod", for
every import between selfhost packages. The same seed version builds xo
on macOS and Linux. CI run 38626156792 (2026-10-14) showed it is not
the seed alone: a Go backend xo.exe cross built from current main fails
the same way when xobuild uses it as the seed on Windows. Any xo running
on Windows fails to build selfhost/xo; xo.exe itself works for other
work (the LLVM goldens, runtime tests, and `xo test` all passed in that
run with it as XO_CLI).

Cause (found 2026-10-14): internal/xotool's BuildXo (and the seed
chain's hop) passed native Windows paths to xo: the build directory
`filepath.Join(root, "selfhost", "xo")` = `C:\...\selfhost\xo`, `--std`,
`-o`, and `PWD`. The checker's path helpers (selfhost/check load.xo
`clean`, `parent`, `up`) split only at `/` (std.md 3.1: paths reach xo
with `/` on Windows), so the walk up from `C:\a\b\selfhost\xo` never
finds the repository's xo.mod, the main module is unknown, and every
`use github.com/xo-lang/xo/...` looks like another module's package
(XO1306). The error positions show it: `C:\...\selfhost\xo/tz.xo`.
Git Bash runs worked because the shell passes `C:/...`.

Fix: xotool passes paths through `xoPath` (filepath.ToSlash) and sets
`PWD` the same way, so every seed, including 0.11.0, builds xo on
Windows. Still open: the driver itself should accept `C:\x` arguments
(spec 3.1 calls them absolute), by converting a drive path argument's
`\` to `/` before the checker sees it; that is a selfhost/ change and
reaches seeds only from the next one.

Old workaround (no longer needed once the above is in the seed's
caller): build xo.exe elsewhere. A Go backend cross build from
macOS or Linux works: `xo build --backend=go --targets=windows/amd64
--std=$PWD/std -o xo.exe selfhost/xo`, put it in the checkout's
`bin/xo.exe` (so it finds std at bin/../std; an xo.exe elsewhere fails
with "std library not found" unless XO_STD is set), and set
`XO_CLI` to it. CI's `suite=windows` does this (`scripts/ci/suite.sh
xcross` in the plan job, knowledgebase/infrastructure/ci.md); the lead
uses scripts/lead/xcross.sh. Next step: reproduce with `xo check
selfhost/xo` under the seed on Windows and trace the module lookup.
