# The CLI assumed Unix host paths

Status: Observed 2026-10-23 (field report on v0.26.0, Windows 11), fixed.

## Symptom

- `xo init` (and `xo test`) hung: the workspace file search walked up from the
  directory and stopped only at `/`, but `dir_of("C:/")` is `C:/`.
- Contract fuzz shrinking hung the same way (the module copy in the temporary
  directory), and its regression tests were never written: the temporary
  directory was `$TMPDIR ?? "/tmp"`, and Windows sets TEMP and TMP; Git Bash's
  TMPDIR is `/c/Users/...`, which xo.exe cannot open.
- A re-run of `xo test` with a Windows TMPDIR reported every test failed
  (NotFound): the cached test binary was copied without `.exe`.
- `xo vet C:/x` failed and `--race` reports doubled paths: `abs_path` treated
  `C:/...` as relative.
- `--tzdata=system` on Windows made every zone unknown, with no warning.

## Cause

Eight copies of "clean, dir, base, abs" and the `/tmp` fallback, each written
for a `/` root.

## Fix

`compiler/hostpath` is the one place for the host's path rules (volume, root,
is_abs, clean, dir, base, abs, rel_below, temp_dir, exe_suffix), each with a
`_w` form taking the OS so the Windows rules are tested on any host. The CLI
packages call it. `internal/repotest` TestNoHostTmpFallback fails when a
compiler or native runtime source names `TMPDIR` or a literal `/tmp` outside
the listed files. `--tzdata=system` for a Windows program embeds the data and
prints a note.

Missed copies (Windows tester, 0.33.0, bugs 13 and 15): `use go` looked for
`xo-gobind` with no `.exe`, took the program's path as a directory only when
it held `/`, and split PATH on `:` (a drive letter's colon); `xo mod tidy`'s
ModCtx.abs and is_local, and six other CLI sites, still tested
`starts_with("/")` for an absolute path, so `C:/w/x` became `C:/w/C:/w/x`. They
call hostpath now (gobind_candidates_w and is_local_w tested in Windows mode).
Before adding a path check, `grep -rn 'starts_with("/")' compiler/cli
compiler/xo`: what is left there is a slash path by design (archive entry
names, module paths, URLs, the Linux-only $XDG_CACHE_HOME).

Last verified: 2026-10-27
