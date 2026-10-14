# xo-ci runner boot for Windows (decision 0225), the counterpart of
# boot/linux.sh. Terraform renders this template to
# s3://<seed bucket>/ci/boot/windows.ps1; every Windows runner machine
# fetches and runs it as SYSTEM on every boot (user data with
# <persist>true</persist>, EC2Launch v2). It is safe to run again: it
# installs only what is missing.
#
#   1. the 8 hour lifetime cap per boot (as scripts/warm-run.sh)
#   2. the idle stop: shut down (stop) after ${idle_minutes} minutes without a job
#   3. toolchain (Git for Windows, Go, LLVM, the MSVC build tools) on a new machine
#   4. the GitHub Actions runner as a service running as SYSTEM, registered
#      as the ci-runner tag's name with the ci-labels tag's labels, through
#      the xo-ci-register Lambda, so the GitHub App key never lives here
#
# Template note: Terraform interpolates dollar-brace sequences, so this
# script never uses them for PowerShell variables.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$D = 'C:\ProgramData\xo-ci'
New-Item -ItemType Directory -Force $D | Out-Null
Start-Transcript -Append -Path "$D\boot.log" | Out-Null
"== boot $((Get-Date).ToUniversalTime().ToString('s'))Z"
# the idle stop waits while this script runs (a new machine installs for an hour)
Set-Content "$D\booting" (Get-Date -Format o)
$env:AWS_REGION = '${region}'
$R = 'C:\actions-runner'
$GOVER = '1.26.1'

try {
  $tok = Invoke-RestMethod -Method Put -Uri http://169.254.169.254/latest/api/token -Headers @{ 'X-aws-ec2-metadata-token-ttl-seconds' = '300' }
  $h = @{ 'X-aws-ec2-metadata-token' = $tok }
  $NAME = Invoke-RestMethod -Headers $h http://169.254.169.254/latest/meta-data/tags/instance/ci-runner
  $LABELS = Invoke-RestMethod -Headers $h http://169.254.169.254/latest/meta-data/tags/instance/ci-labels

  # 1. lifetime cap: a shutdown scheduled 8 hours after boot
  schtasks /create /tn xo-warm-cap /sc onstart /tr "shutdown /s /f /t 28800 /c xo-warm-cap" /ru SYSTEM /f | Out-Null
  # (through cmd: shutdown fails when one is already scheduled, and a
  # native command's redirected stderr is an error under 'Stop')
  cmd /c 'shutdown /s /f /t 28800 /c xo-warm-cap 2>nul'

  # 2. idle stop, checked every minute (installed before anything that can fail)
  $idle = @'
# Shut down (stop) this runner machine after IDLE_MINUTES minutes without a job.
# Stamps older than this boot do not count, so every boot starts a fresh idle period.
$D = 'C:\ProgramData\xo-ci'
$S = "$D\busy"
$boot = (Get-CimInstance Win32_OperatingSystem).LastBootUpTime
function Fresh($f) { (Test-Path $f) -and ((Get-Item $f).LastWriteTime -gt $boot) }
function Busy { (Fresh "$D\booting") -or (Get-Process Runner.Worker -ErrorAction SilentlyContinue) }
function Stamp { Set-Content $S (Get-Date -Format o) }
if (Busy) { Stamp; exit 0 }
if (-not (Fresh $S)) { Stamp; exit 0 }
if (((Get-Date) - (Get-Item $S).LastWriteTime).TotalMinutes -lt IDLE_MINUTES) { exit 0 }
# stop taking jobs first, then look once more: a job assigned in between keeps the machine up
$svc = Get-Service 'actions.runner.*' -ErrorAction SilentlyContinue
if ($svc) { Stop-Service $svc.Name -Force }
if (Busy) { if ($svc) { Start-Service $svc.Name }; Stamp; exit 0 }
Write-EventLog -LogName Application -Source xo-ci -EventId 1 -Message "xo-ci: idle IDLE_MINUTES minutes, shutting down" -ErrorAction SilentlyContinue
# the lifetime cap's pending shutdown blocks a new one: cancel it first
cmd /c 'shutdown /a 2>nul'
shutdown /s /f /t 0 /c xo-ci-idle
'@
  Set-Content "$D\idle.ps1" $idle.Replace('IDLE_MINUTES', '${idle_minutes}')
  if (-not [Diagnostics.EventLog]::SourceExists('xo-ci')) { New-EventLog -LogName Application -Source xo-ci }
  $act = New-ScheduledTaskAction -Execute powershell.exe -Argument "-NoProfile -ExecutionPolicy Bypass -File $D\idle.ps1"
  $trg = New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(1) -RepetitionInterval (New-TimeSpan -Minutes 1)
  Register-ScheduledTask -TaskName xo-ci-idle -Action $act -Trigger $trg -User SYSTEM -RunLevel Highest -Force | Out-Null

  # 3. toolchain (as scripts/warm-run.sh's Windows user data; a machine
  # adopted from the warm pool has it already: C:\xo-ready)
  # Windows Defender's real-time scan made builds take hours
  # (gotchas/windows-defender-slows-builds.md): exclude the build trees
  Add-MpPreference -ExclusionPath 'C:\Users\Administrator', 'C:\Windows\System32\config\systemprofile', $R, 'C:\Program Files\Go', 'C:\Program Files\LLVM', 'C:\Windows\Temp'
  Add-MpPreference -ExclusionProcess 'go.exe', 'xo.exe', 'clang.exe', 'lld-link.exe', 'link.exe', 'compile.exe', 'git.exe', 'bash.exe', 'Runner.Worker.exe'
  if (-not (Test-Path C:\xo-ready)) {
    function Get-Asset($repo, $pattern) {
      $r = Invoke-RestMethod -UseBasicParsing "https://api.github.com/repos/$repo/releases/latest"
      ($r.assets | Where-Object { $_.name -match $pattern } | Select-Object -First 1).browser_download_url
    }
    $dl = 'C:\xo-dl'
    New-Item -ItemType Directory -Force $dl | Out-Null
    Invoke-WebRequest -UseBasicParsing (Get-Asset 'git-for-windows/git' '^Git-[0-9.]+-64-bit\.exe$') -OutFile $dl\git.exe
    Start-Process -Wait $dl\git.exe -ArgumentList '/VERYSILENT', '/NORESTART', '/SUPPRESSMSGBOXES'
    Invoke-WebRequest -UseBasicParsing "https://go.dev/dl/go$GOVER.windows-amd64.msi" -OutFile $dl\go.msi
    Start-Process -Wait msiexec -ArgumentList '/i', "$dl\go.msi", '/qn', '/norestart'
    Invoke-WebRequest -UseBasicParsing (Get-Asset 'llvm/llvm-project' '^LLVM-[0-9.]+-win64\.msi$') -OutFile $dl\llvm.msi
    Start-Process -Wait msiexec -ArgumentList '/i', "$dl\llvm.msi", '/qn', '/norestart'
    Invoke-WebRequest -UseBasicParsing https://aka.ms/vs/17/release/vs_BuildTools.exe -OutFile $dl\vsbt.exe
    Start-Process -Wait $dl\vsbt.exe -ArgumentList '--quiet', '--wait', '--norestart', '--nocache', '--add', 'Microsoft.VisualStudio.Workload.VCTools', '--add', 'Microsoft.VisualStudio.Component.VC.Tools.x86.x64', '--add', 'Microsoft.VisualStudio.Component.VC.Tools.ARM64', '--includeRecommended'
    if (-not (Get-Command aws -ErrorAction SilentlyContinue)) {
      Invoke-WebRequest -UseBasicParsing https://awscli.amazonaws.com/AWSCLIV2.msi -OutFile $dl\awscli.msi
      Start-Process -Wait msiexec -ArgumentList '/i', "$dl\awscli.msi", '/qn', '/norestart'
    }
    $p = [Environment]::GetEnvironmentVariable('Path', 'Machine')
    [Environment]::SetEnvironmentVariable('Path', "$p;C:\Program Files\LLVM\bin;C:\Program Files\Go\bin;C:\Program Files\Git\cmd;C:\Program Files\Amazon\AWSCLIV2", 'Machine')
    Set-Content C:\xo-ready ready
  }
  $aws = 'C:\Program Files\Amazon\AWSCLIV2\aws.exe'

  # 4. runner
  if (-not (Test-Path "$R\config.cmd")) {
    $v = (Invoke-RestMethod -UseBasicParsing https://api.github.com/repos/actions/runner/releases/latest).tag_name.TrimStart('v')
    New-Item -ItemType Directory -Force $R | Out-Null
    Invoke-WebRequest -UseBasicParsing "https://github.com/actions/runner/releases/download/v$v/actions-runner-win-x64-$v.zip" -OutFile "$D\runner.zip"
    Expand-Archive -Force "$D\runner.zip" $R
    Remove-Item "$D\runner.zip"
  }
  # job environment: Git Bash first on PATH (the workflow's shell: bash
  # steps), then the toolchain; the job's home is Administrator's, so the
  # warm machine's caches (Go, xo) stay in use under the SYSTEM service
  $path = 'C:\Program Files\Git\bin;C:\Program Files\Git\usr\bin;C:\Program Files\Go\bin;C:\Program Files\LLVM\bin;C:\Program Files\Amazon\AWSCLIV2;' + [Environment]::GetEnvironmentVariable('Path', 'Machine')
  $envs = @(
    "PATH=$path",
    'USERPROFILE=C:\Users\Administrator',
    'HOME=C:\Users\Administrator',
    'LOCALAPPDATA=C:\Users\Administrator\AppData\Local',
    'APPDATA=C:\Users\Administrator\AppData\Roaming',
    'GOPATH=C:\Users\Administrator\go',
    "AWS_REGION=$env:AWS_REGION"
  )
  # written before config.cmd, which starts the service; a restart right
  # after that start makes GitHub refuse the new session for a while
  # (Conflict: "A session for this runner already exists")
  $old = if (Test-Path "$R\.env") { Get-Content -Raw "$R\.env" } else { '' }
  Set-Content -Encoding ascii "$R\.env" $envs
  $envChanged = $old -ne (Get-Content -Raw "$R\.env")
  Set-Content -Encoding ascii "$D\register.json" ('{"name":"' + $NAME + '"}')
  & $aws lambda invoke --function-name ${register_fn} --cli-binary-format raw-in-base64-out --payload "file://$D/register.json" "$D\register.out" | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'register Lambda failed' }
  $reg = Get-Content "$D\register.out" -Raw | ConvertFrom-Json
  Remove-Item "$D\register.out"
  $configured = $false
  if (-not $reg.exists -or -not (Test-Path "$R\.runner")) {
    $configured = $true
    "registering $NAME ($LABELS)"
    # the old service and registration go first: config.cmd will not
    # configure over them, even with --replace
    Get-Service 'actions.runner.*' -ErrorAction SilentlyContinue | ForEach-Object { Stop-Service $_.Name -Force; sc.exe delete $_.Name | Out-Null }
    Remove-Item -Force -ErrorAction SilentlyContinue "$R\.runner", "$R\.credentials", "$R\.credentials_rsaparams", "$R\.service"
    Push-Location $R
    & .\config.cmd --unattended --replace --url ${repo_url} --token $reg.token --name $NAME --labels $LABELS --work _work --runasservice --windowslogonaccount 'NT AUTHORITY\SYSTEM'
    $rc = $LASTEXITCODE
    Pop-Location
    if ($rc -ne 0) { throw "config.cmd failed ($rc)" }
  }
  $svc = Get-Service 'actions.runner.*' -ErrorAction SilentlyContinue
  if (-not $svc) { throw 'no runner service after config.cmd' }
  if ($envChanged -and -not $configured) { Restart-Service $svc.Name -Force }
  elseif ($svc.Status -ne 'Running') { Start-Service $svc.Name }
  "== ready $NAME ($LABELS)"
} catch {
  "boot failed: $_"
} finally {
  Remove-Item -Force -ErrorAction SilentlyContinue "$D\booting"
  Stop-Transcript | Out-Null
}
