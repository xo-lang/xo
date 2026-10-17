# Building xo on a Windows instance took hours

Status: Observed (2026-10-14, verifying the windows-amd64 seed)

## Symptom

On a fresh EC2 Windows Server 2025 instance, `go run ./scripts/xobuild`
from the windows-amd64 seed (a Go-backend build of compiler/xo) used one
core for over two hours (8,285 s of CPU in xo.exe), and the chain build
from go-compiler-final ran past the instance's 240 minute lifetime. The
same Go-backend build cross-compiled on the Mac
(`xo build --backend=go --targets=windows/amd64`) took 35 s. MsMpEng
(Windows Defender) was the busiest process on the instance.

## Cause

Defender's real-time protection scans every file the build writes and
reads: the generated Go of xo, the Go build cache, and the runtime
objects, thousands of files.

## Fix

- scripts/remote-windows.sh's user data adds Defender exclusions for the
  build trees and tools on these throwaway instances (Add-MpPreference
  -ExclusionPath / -ExclusionProcess).
- For Windows test jobs, cross-build xo.exe on the Mac for the job's
  commit (about 5 to 35 s), upload it to
  s3://xo-seeds-ACCOUNT_ID/dev/<commit>/windows-amd64/xo.exe, and set
  XO_CLI to it on the instance instead of building xo there.
- Windows seeds are cross-built the same way (aws-remote-runs memory,
  decision 0216 consequences).
