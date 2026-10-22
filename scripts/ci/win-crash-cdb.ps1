# scripts/ci/win-crash-cdb.ps1: a native windows/amd64 xo.exe's crash,
# caught by the debugger. A native xo writes no .xocrash (only Go backend
# programs do), so release-smoke.sh's failures (CI 39800969338: every
# `xo build` exit 139, no output) carry no stack. This runs the release
# archive's xo.exe building a hello program under cdb (the Windows SDK's
# debugger on the runner) and prints, at the first access violation, stack
# overflow or fail fast: the exception record, the registers, the faulting
# thread's stack, every thread's stack, the modules and the TEB. Addresses
# are xo.exe's (no PDB): map them on the Mac against a build of the same
# commit. It also turns on WER LocalDumps for xo.exe (full dumps) and runs
# the build once without the debugger, the dumps kept in the output. With
# XO_THREADS unset and with 1. For suite=script on the xo-windows runner:
#   gh workflow run ci.yml -f ref=BRANCH -f suite=script -f runner=xo-windows \
#     -f packages="scripts/ci/win-crash-cdb.ps1 ARCHIVE SHA256"
# ARCHIVE is a path in the checkout, else a name under ci/release/ of the
# seed bucket ($XO_SEED_BUCKET, as release-smoke.sh); SHA256 its digest.
# Output in $XO_CI_OUT/win-crash: summary.txt, cdb-*.txt, plain-*.txt,
# dumps. (Xo Lang 4, 2026-10-22: the 0.25.1 Windows smoke.)
param([string]$Archive, [string]$Sha256)
$ErrorActionPreference = 'Continue'
if (-not $Archive -or -not $Sha256) { throw 'usage: win-crash-cdb.ps1 ARCHIVE SHA256' }
$base = if ($env:XO_CI_OUT) { $env:XO_CI_OUT } else { Join-Path $PWD '.tools' }
$out = Join-Path $base 'win-crash'
New-Item -ItemType Directory -Force $out | Out-Null
$sum = Join-Path $out 'summary.txt'
Set-Content $sum "win-crash-cdb $Archive"

$a = $Archive
if (-not (Test-Path $a)) {
  $a = Join-Path $out (Split-Path $Archive -Leaf)
  & aws s3 cp --only-show-errors "s3://$($env:XO_SEED_BUCKET)/ci/release/$(Split-Path $Archive -Leaf)" $a
  if (-not (Test-Path $a)) { throw "$Archive is not in the checkout or the bucket (ci/release/)" }
}
$got = (Get-FileHash -Algorithm SHA256 $a).Hash.ToLower()
if ($got -ne $Sha256.ToLower()) { throw "SHA-256 $got, want $Sha256" }
$unpack = Join-Path $out 'unpack'
Remove-Item -Recurse -Force $unpack -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $unpack | Out-Null
& tar -xzf $a -C $unpack
$rel = Get-ChildItem $unpack -Directory | Where-Object Name -like 'xo-*' | Select-Object -First 1
$xo = Join-Path $rel.FullName 'bin\xo.exe'
Remove-Item Env:XO_STD, Env:XO_CLI -ErrorAction SilentlyContinue
& $xo version | Add-Content $sum

# WER LocalDumps for xo.exe: full dumps (DumpType 2), at most 4
$dumps = Join-Path $out 'dumps'
New-Item -ItemType Directory -Force $dumps | Out-Null
$key = 'HKLM:\SOFTWARE\Microsoft\Windows\Windows Error Reporting\LocalDumps\xo.exe'
New-Item -Force $key | Out-Null
New-ItemProperty -Force -Path $key -Name DumpFolder -PropertyType ExpandString -Value $dumps | Out-Null
New-ItemProperty -Force -Path $key -Name DumpType -PropertyType DWord -Value 2 | Out-Null
New-ItemProperty -Force -Path $key -Name DumpCount -PropertyType DWord -Value 4 | Out-Null

$hello = Join-Path $out 'hello'
New-Item -ItemType Directory -Force $hello | Out-Null
Set-Content (Join-Path $hello 'main.xo') "fn main(os: Os) {`n  os.stdio.println(`"hello`")`n}"
$cdb = @('C:\Program Files (x86)\Windows Kits\10\Debuggers\x64\cdb.exe', 'C:\Program Files\Windows Kits\10\Debuggers\x64\cdb.exe') | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $cdb) { Add-Content $sum 'no cdb.exe on this runner' }
foreach ($t in @('', '1')) {
  $tag = if ($t) { 't1' } else { 'tdefault' }
  if ($t) { $env:XO_THREADS = $t } else { Remove-Item Env:XO_THREADS -ErrorAction SilentlyContinue }
  Push-Location $hello
  if ($cdb) {
    # -g skips the initial break, -G the exit break; break on the first
    # chance of an access violation, a stack overflow and a fail fast
    & $cdb -g -G -sxe av -sxe c00000fd -sxe c0000409 `
      -c '.echo XO-CRASH; .exr -1; r; .ecxr; kn 80; ~*kn 40; lm; !teb; q' `
      $xo build -o "h-$tag.exe" . *> (Join-Path $out "cdb-$tag.txt")
    Add-Content $sum "cdb $tag exit $LASTEXITCODE"
  }
  & $xo build -o "p-$tag.exe" . *> (Join-Path $out "plain-$tag.txt")
  Add-Content $sum "plain $tag exit $LASTEXITCODE"
  Pop-Location
}
Get-ChildItem $dumps | ForEach-Object { Add-Content $sum "dump $($_.Name) $($_.Length)" }
Get-Content $sum
