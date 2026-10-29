# A child started through env(1) does not start on Windows

Status: Observed (2026-10-28)

## Symptom

On the xo-windows CI runner, `xo dev stdcache --conc=0` printed `cache std
... conc=false failed: the whole program is built in 30 ms` (CI
40810568719), so separate compilation's std variants were never built on
a Windows host. `--backend=go` had the same call: it worked under Git Bash
(the 0.33.1 release smoke passed) and not from PowerShell or cmd.

## Cause

`os.proc.run("env", ["NAME=VALUE", ..., program, args...])` sets a child's
variables through the env program, which is on PATH on darwin and linux
and on Windows only inside Git Bash (its usr/bin). stdcache.xo's
std_variant_build, dev.xo's stdobj_run and gobackend.xo's go_build all
started their children that way.

## Fix

Set the variables on the command: compiler/xo tools.xo `env_command(os,
args)` takes env(1)'s arguments and returns `os.proc.cmd(program,
args).set_env(...)` for each `NAME=VALUE` (`NAME=` sets an empty variable,
as env(1) does; sepcomp_default reads an empty XO_SEPCOMP as the user's
choice). Test a Windows child spawn with a PATH that has no env program
(scripts/ci/windows-sepcomp.sh's clean PATH: go's directory and
System32), not from Git Bash alone.
