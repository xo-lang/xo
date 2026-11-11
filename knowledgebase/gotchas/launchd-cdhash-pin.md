# launchd refuses a heavyd rebuilt in place

Status: Observed (2026-11-10)

## Symptom

After `scripts/lead/install.sh` rebuilt `~/xo-lead/agents/heavyd` (18:39,
2026-11-10), the owner's `launchctl kickstart` of `org.xolang.heavyd`
failed with "spawn failed" (exit 78), and heavy.sh fell back to the bash
legacy-heavy.sh.

## Cause

macOS Background Task Management records the code directory hash
(cdhash) of a LaunchAgent's binary when the job is registered. A new
build at the same path has a different cdhash, so launchd refuses to
spawn it until the job is registered again. install.sh rebuilt heavyd on
every run, even when its source had not changed.

## Fix

- install.sh builds heavyd only when the hash of its Go files, go.mod and
  `go version` differs from `agents/.heavyd.srchash`, and after a rebuild
  prints the command for the owner (restarting heavyd is the owner's):
  `launchctl bootout gui/$(id -u)/org.xolang.heavyd; launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/org.xolang.heavyd.plist`
- The first install after this change has no stored hash, so it rebuilds
  once and prints the command.
