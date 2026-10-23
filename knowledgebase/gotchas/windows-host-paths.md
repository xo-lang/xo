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

Last verified: 2026-10-23
