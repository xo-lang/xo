# The windows-amd64 seed cannot build xo on Windows (XO1306)

Status: Observed (2026-10-14, the lead on the Windows warm machine).

Symptom: on Windows, `go run ./scripts/xobuild` with the 0.11.0
windows-amd64 seed fails with XO1306, "module of
github.com/xo-lang/xo/selfhost/check is not required by xo.mod", for
every import between selfhost packages. The same seed version builds xo
on macOS and Linux.

Cause: not yet found. Probably a Windows path bug in how xo maps a
package directory to its module (backslashes or a drive letter in the
module root comparison; compare xo1-windows-drive-paths.md).

Fix (workaround): build xo.exe elsewhere. A Go backend cross build from
macOS or Linux works: `xo build --backend=go --targets=windows/amd64
--std=$PWD/std -o xo.exe selfhost/xo`, put it in the checkout's
`bin/xo.exe` (so it finds std at bin/../std; an xo.exe elsewhere fails
with "std library not found" unless XO_STD is set), and set
`XO_CLI` to it. CI's `suite=windows` does this (`scripts/ci/suite.sh
xcross` in the plan job, knowledgebase/infrastructure/ci.md); the lead
uses scripts/lead/xcross.sh. Next step: reproduce with `xo check
selfhost/xo` under the seed on Windows and trace the module lookup.
