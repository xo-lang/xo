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
Windows. The driver accepts native paths too (2026-10-14): on Windows
selfhost/xo opts.xo `front` turns `\` into `/` in directory arguments
and the values of path flags (`-o`, `--std`, `--wd`, `--tzdata`,
`--cc`, ...), never in a `run` program's arguments or after `--`
(scripts/ci/winpaths.sh checks it on the Windows runner: passes with
the change, run 38635797157; main's xo without it fails XO1306, run
38637803546). It reaches
seeds from the next one cut; xotool's `/` paths cover older seeds.

Old workaround (used until the fix, 2026-10-14): a Go backend cross
built xo.exe from macOS or Linux in the checkout's `bin/xo.exe` (so it
finds std at bin/../std) with `XO_CLI` set to it. CI's plan job did
this (`suite.sh xcross`) and the lead's scripts/lead/xcross.sh; both are
deleted.
