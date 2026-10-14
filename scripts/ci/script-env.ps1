# scripts/ci/script-env.ps1: the Windows smoke test of `suite=script`
# (scripts/ci/suite.sh), as script-env.sh:
#   gh workflow run ci.yml -f suite=script -f runner=xo-windows -f packages="scripts/ci/script-env.ps1 a b"
$ErrorActionPreference = 'Stop'
"args: $args"
foreach ($v in 'XO_CLI', 'XO_CI_STD', 'XO_CI_BASE_STD', 'XO_CI_OUT') { "$v=" + [Environment]::GetEnvironmentVariable($v) }
& $env:XO_CLI version
if ($LASTEXITCODE -ne 0) { throw "xo version failed ($LASTEXITCODE)" }
if (-not (Test-Path $env:XO_CI_STD) -or -not (Test-Path $env:XO_CI_BASE_STD)) { throw 'a std directory is missing' }
Set-Content (Join-Path $env:XO_CI_OUT 'script-env.txt') "script-env ok $args"
