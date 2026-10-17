# xo1 hangs on a Windows drive path

Status: Observed (2026-10-12), fixed in compiler/check (path_volume,
is_rooted) and compiler/xo (is_abs_path).

Symptom: on Windows, `xo1 build --backend=llvm DIR` printed nothing and
ran until stopped (no `--phase-times` line: the checker never
returned), while `xo1 version` worked.

Cause: the working directory reaches xo1 as `PWD=C:/Users/...` (Git
Bash passes it with `/`). The checker's `up` walks to the module root
by "rooted at `/`" or "relative"; `C:/Users/x` is neither, so `up(".")`
went to `parent("C:")` = `.` and around again forever. The driver's
`abs_of`, `line_root`, and the test runner's `abs_path` had the same
assumption.

Fix: a path is absolute when it starts with `/` or, on Windows, with a
drive (`C:/`); `clean` keeps the drive and cleans the rest as rooted,
and `up` stops at `C:/`. Any new path walk in selfhost must use
`is_rooted` (check) or `is_abs_path` (xo), not `starts_with("/")`.
