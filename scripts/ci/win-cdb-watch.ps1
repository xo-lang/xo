# scripts/ci/win-cdb-watch.ps1: the 0.25.1 windows/amd64 xo.exe's corrupt
# Program, traced under cdb on the xo-windows runner (Xo Lang 4,
# 2026-10-22). `xo build` crashes in write_dumps(dumps=NULL) because
# generate_cached's G (g) holds wrong words at g+0x700 (dumps) and g+0x5c0
# (err's length) by the time Program is built (minidumps of CI
# 39806679573). This sets hardware write watchpoints on those two words as
# soon as generate_cached stores g's dumps, and logs every later write to
# them (the writing instruction, the registers, the stack) until
# write_dumps or the access violation. xo.exe has no PDB: the breakpoints
# are module offsets of the shipped xo.exe (sha256 of the archive below),
# found with lldb on the dumps; another build has other offsets.
#   gh workflow run ci.yml -f ref=BRANCH -f suite=script -f runner=xo-windows \
#     -f packages="scripts/ci/win-cdb-watch.ps1 ARCHIVE SHA256"
# Output: $XO_CI_OUT/win-cdb/summary.txt and cdb.txt, text only (no dumps,
# no archive copy: the artifact egress limit).
param([string]$Archive, [string]$Sha256)
$ErrorActionPreference = 'Continue'
if (-not $Archive -or -not $Sha256) { throw 'usage: win-cdb-watch.ps1 ARCHIVE SHA256' }
$base = if ($env:XO_CI_OUT) { $env:XO_CI_OUT } else { Join-Path $PWD '.tools' }
$out = Join-Path $base 'win-cdb'
New-Item -ItemType Directory -Force $out | Out-Null
$sum = Join-Path $out 'summary.txt'
Set-Content $sum "win-cdb-watch $Archive"
function Log([string]$s) { Add-Content $sum $s; Write-Output $s }
$tmp = if ($env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { $env:TEMP }

# the archive, from the checkout or the bucket's ci/release/, unpacked in
# the runner's temporary directory (not uploaded)
$a = $Archive
if (-not (Test-Path $a)) {
  $a = Join-Path $tmp (Split-Path $Archive -Leaf)
  & aws s3 cp --only-show-errors "s3://$($env:XO_SEED_BUCKET)/ci/release/$(Split-Path $Archive -Leaf)" $a 2>&1 | ForEach-Object { Log "  $_" }
}
$got = (Get-FileHash -Algorithm SHA256 $a).Hash.ToLower()
Log "sha256 $got"
if ($got -ne $Sha256.ToLower()) { Log "SHA-256 differs from $Sha256"; exit 2 }
$unpack = Join-Path $tmp 'xo-win-cdb'
Remove-Item -Recurse -Force $unpack -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $unpack | Out-Null
& (Join-Path $env:SystemRoot 'System32\tar.exe') -xzf $a -C $unpack 2>&1 | ForEach-Object { Log "  $_" }
$rel = Get-ChildItem $unpack -Directory | Where-Object Name -like 'xo-*' | Select-Object -First 1
$xo = Join-Path $rel.FullName 'bin\xo.exe'
Log "xo.exe: $xo"
Remove-Item Env:XO_STD, Env:XO_CLI -ErrorAction SilentlyContinue

# cdb: the SDK's, else winget's Debugging Tools, else the SDK installer
# with only the debuggers
function Find-Cdb {
  @("${env:ProgramFiles(x86)}\Windows Kits\10\Debuggers\x64\cdb.exe", "$env:ProgramFiles\Windows Kits\10\Debuggers\x64\cdb.exe") | Where-Object { $_ -and (Test-Path $_) } | Select-Object -First 1
}
$cdb = Find-Cdb
if (-not $cdb -and (Get-Command winget -ErrorAction SilentlyContinue)) {
  Log 'installing the debuggers with winget'
  & winget install --silent --accept-package-agreements --accept-source-agreements Microsoft.WindowsSDK.Debugging 2>&1 | Select-Object -Last 5 | ForEach-Object { Log "  $_" }
  $cdb = Find-Cdb
}
if (-not $cdb) {
  Log 'installing the debuggers from the Windows SDK installer'
  $sdk = Join-Path $tmp 'winsdksetup.exe'
  try {
    Invoke-WebRequest -UseBasicParsing -Uri 'https://go.microsoft.com/fwlink/?linkid=2272610' -OutFile $sdk
    Start-Process -Wait -FilePath $sdk -ArgumentList '/features', 'OptionId.WindowsDesktopDebuggers', '/quiet', '/norestart'
  } catch { Log "SDK installer failed: $_" }
  $cdb = Find-Cdb
}
if (-not $cdb) { Log 'no cdb.exe'; exit 2 }
Log "cdb: $cdb"

$hello = Join-Path $tmp 'xo-win-cdb-hello'
New-Item -ItemType Directory -Force $hello | Out-Null
Set-Content (Join-Path $hello 'main.xo') "fn main(os: Os) {`n  os.stdio.println(`"hello`")`n}"

# The command file. At either store of g's dumps in generate_cached (r10 is
# g there), two write watchpoints (ba w8): g+0x700 (dumps) and g+0x5c0
# (err's length); each hit prints the instruction that wrote, the new
# word, the registers and 12 frames, then goes on. write_dumps' entry and
# an access violation print the registers and the stack and quit. At most
# 400 watchpoint hits are logged (the counter in $t0).
$cmds = Join-Path $tmp 'xo-win-cdb.txt'
$watch = 'r $t0 = @$t0 + 1; .if (@$t0 > 400) { .echo TOO-MANY-HITS; q }; .echo WRITE; r rip; u @rip - 0x10 L6; dq @$t1 L1; dq @$t2 L1; r; kn 12; gc'
# the inner ba commands' quotes escaped (\") inside bp's string, below
$onstore = 'r $t1 = @r10 + 0x700; r $t2 = @r10 + 0x5c0; .echo G-STORE; r r10; dq @$t1 L1; dq @$t2 L1; ba w8 @$t1 "' + $watch + '"; ba w8 @$t2 "' + $watch + '"; gc'
@(
  '.echo WIN-CDB-WATCH',
  'r $t0 = 0',
  'sxe -c ".echo ACCESS-VIOLATION; .exr -1; r; kn 40; q" av',
  "bp xo+0x572adb `"$($onstore -replace '"', '\"')`"",
  "bp xo+0x578276 `"$($onstore -replace '"', '\"')`"",
  'bp xo+0x595cd4 ".echo AT-WRITE-DUMPS; r; dq @rdx L4; kn 20; q"',
  'g'
) | Set-Content -Encoding ASCII $cmds
Get-Content $cmds | ForEach-Object { Log "cmd: $_" }
$env:XO_THREADS = '1'
Push-Location $hello
& $cdb -G -cf $cmds $xo build -o h.exe . *> (Join-Path $out 'cdb.txt')
Log ("cdb exit {0} (0x{1:X8})" -f $LASTEXITCODE, $LASTEXITCODE)
Pop-Location
Log "cdb.txt lines: $((Get-Content (Join-Path $out 'cdb.txt')).Count)"
Get-Content (Join-Path $out 'cdb.txt') -TotalCount 60 | ForEach-Object { Log "  $_" }
