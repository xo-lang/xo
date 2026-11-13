# Windows on ARM: Git Bash reports AMD64, and WinDbg's cdb.exe will not run in place

Status: Observed (2026-11-12, the winarm VM, Xo Lang 2)

## Symptom

- scripts/ci/windows-sepcomp.sh on Windows 11 ARM chose `--backend=amd64`
  from `PROCESSOR_ARCHITECTURE` and every explicit separate build failed
  ("relocation type 2/14"), because the std store it linked was arm64's.
- Every cdb step failed, the transcript one line:
  `.../WindowsApps/Microsoft.WinDbg_1.2611.4001.0_arm64__.../arm64/cdb.exe: Permission denied`.

## Cause

- Git Bash's runtime is x64 under emulation; its processes see
  `PROCESSOR_ARCHITECTURE=AMD64` (and `uname -m` x86_64) on an arm64 host.
- Store and winget packages under `C:\Program Files\WindowsApps` cannot be
  executed by their file path (only through an app execution alias); `[ -x ]`
  still says yes.

## Fix

- Take the architecture from the program under test: `xo version` prints
  `windows/arm64`.
- Copy WinDbg's `arm64` directory out (`~/tools/windbg-arm64`) and run cdb
  from there; check a candidate by running it (`cdb -version`), not with `-x`.
