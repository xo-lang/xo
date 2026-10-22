# scripts/ci/win-crash-cdb.ps1: a native windows/amd64 xo.exe's crash,
# caught by a debugger. A native xo writes no .xocrash (only Go backend
# programs do), so release-smoke.sh's failures (CI 39800969338: every
# `xo build` exit 139, no output) carry no stack. This unpacks the release
# archive, runs its xo.exe building a hello program, and for each run
# records the exact command line, its standard output and error, and the
# exit code in decimal and hex (0xC0000005 is an access violation,
# 0xC00000FD a stack overflow, 0xC0000409 a fail fast). It runs the build
# under cdb when the Windows SDK's debugger is installed (the exception
# record, registers, every thread's stack, modules, the TEB at the first
# access violation, stack overflow or fail fast), else under Sysinternals'
# procdump (downloaded; a dump of xo.exe at its first unhandled exception,
# which lldb on the Mac opens with the same xo.exe). WER LocalDumps
# (full) of a plain run are kept too. With XO_THREADS unset and with 1.
# For suite=script on the xo-windows runner:
#   gh workflow run ci.yml -f ref=BRANCH -f suite=script -f runner=xo-windows \
#     -f packages="scripts/ci/win-crash-cdb.ps1 ARCHIVE SHA256"
# ARCHIVE is a path in the checkout, else a name under ci/release/ of the
# seed bucket ($XO_SEED_BUCKET, as release-smoke.sh); SHA256 its digest.
# Output in $XO_CI_OUT/win-crash: summary.txt (every step), run-*.out and
# run-*.err, cdb-*.txt; the dumps and the xo.exe they need go to
# s3://$XO_SEED_BUCKET/dev/ci-out/$GITHUB_RUN_ID/win-crash/ (ci-out.ps1:
# artifacts carry text only), their keys in summary.txt. (Xo Lang 4, 2026-10-22: the 0.25.1
# Windows smoke; CI 39805650922 found no cdb and logged no output.)
param([string]$Archive, [string]$Sha256)
$ErrorActionPreference = 'Continue'
if (-not $Archive -or -not $Sha256) { throw 'usage: win-crash-cdb.ps1 ARCHIVE SHA256' }
$base = if ($env:XO_CI_OUT) { $env:XO_CI_OUT } else { Join-Path $PWD '.tools' }
$out = Join-Path $base 'win-crash'
New-Item -ItemType Directory -Force $out | Out-Null
$sum = Join-Path $out 'summary.txt'
Set-Content $sum "win-crash-cdb $Archive on $([Environment]::OSVersion.VersionString), PowerShell $($PSVersionTable.PSVersion)"
function Log([string]$s) { Add-Content $sum $s; Write-Output $s }
. (Join-Path $PSScriptRoot 'ci-out.ps1')
$tmp = if ($env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { $env:TEMP }

# Run NAME EXE ARGS DIR: the command line, standard output and error in
# run-NAME.out and .err, the exit code in decimal and hex
function Run([string]$name, [string]$exe, [string[]]$argv, [string]$dir) {
  $o = Join-Path $out "run-$name.out"
  $e = Join-Path $out "run-$name.err"
  Log "run $name in ${dir}: `"$exe`" $($argv -join ' ')"
  Push-Location $dir
  try {
    # a native call sets $LASTEXITCODE (Start-Process's ExitCode can be
    # empty in Windows PowerShell 5.1)
    & $exe @argv > $o 2> $e
    $c = $LASTEXITCODE
    Log ("run $name exit {0} (0x{1:X8})" -f $c, $c)
  } catch {
    Log "run $name could not start: $_"
  }
  Pop-Location
  foreach ($f in @($o, $e)) {
    if ((Test-Path $f) -and (Get-Item $f).Length -gt 0) { Log "--- $(Split-Path $f -Leaf)"; Get-Content $f -TotalCount 40 | ForEach-Object { Log "  $_" } }
  }
}

$a = $Archive
if (-not (Test-Path $a)) {
  $a = Join-Path $tmp (Split-Path $Archive -Leaf)
  Log "fetching s3://.../ci/release/$(Split-Path $Archive -Leaf)"
  & aws s3 cp --only-show-errors "s3://$($env:XO_SEED_BUCKET)/ci/release/$(Split-Path $Archive -Leaf)" $a 2>&1 | ForEach-Object { Log "  $_" }
  if (-not (Test-Path $a)) { Log "archive not found"; exit 2 }
}
$got = (Get-FileHash -Algorithm SHA256 $a).Hash.ToLower()
Log "sha256 $got"
if ($got -ne $Sha256.ToLower()) { Log "SHA-256 differs from $Sha256"; exit 2 }
# unpack outside XO_CI_OUT (not uploaded twice), with tar.exe's own output
$unpack = Join-Path $env:RUNNER_TEMP 'xo-win-crash'
if (-not $env:RUNNER_TEMP) { $unpack = Join-Path $env:TEMP 'xo-win-crash' }
Remove-Item -Recurse -Force $unpack -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $unpack | Out-Null
$tar = Join-Path $env:SystemRoot 'System32\tar.exe'
Log "unpack with $tar into $unpack"
& $tar -xzf $a -C $unpack 2>&1 | ForEach-Object { Log "  $_" }
Log "tar exit $LASTEXITCODE"
$rel = Get-ChildItem $unpack -Directory | Where-Object Name -like 'xo-*' | Select-Object -First 1
if (-not $rel) { Log "no xo-* directory after unpacking:"; Get-ChildItem -Recurse $unpack | Select-Object -First 20 | ForEach-Object { Log "  $($_.FullName)" }; exit 2 }
$xo = Join-Path $rel.FullName 'bin\xo.exe'
Log "xo.exe: $xo ($((Get-Item $xo).Length) bytes)"
Remove-Item Env:XO_STD, Env:XO_CLI -ErrorAction SilentlyContinue

$hello = Join-Path $out 'hello'
New-Item -ItemType Directory -Force $hello | Out-Null
Set-Content (Join-Path $hello 'main.xo') "fn main(os: Os) {`n  os.stdio.println(`"hello`")`n}"
Run 'version' $xo @('version') $hello
Run 'check' $xo @('check', '.') $hello

# WER LocalDumps for xo.exe: full dumps (DumpType 2), at most 4
$dumps = Join-Path $out 'dumps'
New-Item -ItemType Directory -Force $dumps | Out-Null
try {
  $key = 'HKLM:\SOFTWARE\Microsoft\Windows\Windows Error Reporting\LocalDumps\xo.exe'
  New-Item -Force $key | Out-Null
  New-ItemProperty -Force -Path $key -Name DumpFolder -PropertyType ExpandString -Value $dumps | Out-Null
  New-ItemProperty -Force -Path $key -Name DumpType -PropertyType DWord -Value 2 | Out-Null
  New-ItemProperty -Force -Path $key -Name DumpCount -PropertyType DWord -Value 4 | Out-Null
  Log "WER LocalDumps set to $dumps"
} catch { Log "WER LocalDumps not set: $_" }

# a debugger: the SDK's cdb, else procdump
$cdb = @("${env:ProgramFiles(x86)}\Windows Kits\10\Debuggers\x64\cdb.exe", "$env:ProgramFiles\Windows Kits\10\Debuggers\x64\cdb.exe") | Where-Object { $_ -and (Test-Path $_) } | Select-Object -First 1
$procdump = $null
if ($cdb) { Log "cdb: $cdb" } else {
  Log "no cdb.exe; fetching procdump"
  try {
    $zip = Join-Path $env:TEMP 'Procdump.zip'
    Invoke-WebRequest -UseBasicParsing -Uri 'https://download.sysinternals.com/files/Procdump.zip' -OutFile $zip
    Expand-Archive -Force $zip (Join-Path $env:TEMP 'procdump')
    $procdump = Join-Path $env:TEMP 'procdump\procdump64.exe'
    Log "procdump: $procdump"
  } catch { Log "procdump not fetched: $_" }
}

foreach ($t in @('', '1')) {
  $tag = if ($t) { 't1' } else { 'tdefault' }
  if ($t) { $env:XO_THREADS = $t } else { Remove-Item Env:XO_THREADS -ErrorAction SilentlyContinue }
  Log "== XO_THREADS=$t"
  Run "plain-$tag" $xo @('build', '-o', "p-$tag.exe", '.') $hello
  if ($cdb) {
    Run "cdb-$tag" $cdb @('-g', '-G', '-sxe', 'av', '-sxe', 'c00000fd', '-sxe', 'c0000409', '-c', '.echo XO-CRASH; .exr -1; r; .ecxr; kn 80; ~*kn 40; lm; !teb; q', $xo, 'build', '-o', "c-$tag.exe", '.') $hello
  } elseif ($procdump) {
    # -e: dump at an unhandled exception; -ma: full; -x: launch the program
    Run "procdump-$tag" $procdump @('-accepteula', '-e', '-ma', '-x', $dumps, $xo, 'build', '-o', "d-$tag.exe", '.') $hello
  }
}
Start-Sleep -Seconds 5
Get-ChildItem $dumps -ErrorAction SilentlyContinue | ForEach-Object { Log "dump $($_.Name) $($_.Length)" }
Copy-Item $xo $out
# the dumps and xo.exe to the bucket, their keys in summary.txt
Send-CiOut -Dir $out -Summary $sum
Log 'done'
