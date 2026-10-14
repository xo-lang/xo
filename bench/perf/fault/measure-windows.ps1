# Measures bench/perf/fault on Windows (B06 part 3, the fault walk in
# Xo) through CI's script suite (knowledgebase/infrastructure/ci.md,
# "Suites"): builds main.xo with $env:XO_CLI against main's std
# ($env:XO_CI_BASE_STD, "base") and the branch's ($env:XO_CI_STD, "new")
# on the LLVM backend, runs each workload 5 times, and writes "<workload>
# llvm <base|new> <min ms>" lines to $env:XO_CI_OUT\results.txt (the
# run's script-out artifact). Windows has no instruction count at hand,
# so these are wall clock milliseconds: a best effort check, not
# decision 0180's bar (the Mac is).
#
# usage (CI): gh workflow run ci.yml -f ref=BRANCH -f suite=script
#   -f runner=xo-windows -f packages="bench/perf/fault/measure-windows.ps1"
$ErrorActionPreference = 'Stop'
$xo = $env:XO_CLI
$out = $env:XO_CI_OUT
$src = Split-Path -Parent $MyInvocation.MyCommand.Path
$res = Join-Path $out 'results.txt'
foreach ($v in @(@('base', $env:XO_CI_BASE_STD), @('new', $env:XO_CI_STD))) {
	$d = Join-Path $out "src-$($v[0])"
	New-Item -ItemType Directory -Force $d | Out-Null
	Copy-Item (Join-Path $src 'main.xo'), (Join-Path $src 'xo.mod') $d
	Push-Location $d
	& $xo build "--std=$($v[1])" --backend=llvm -o (Join-Path $out "fault-$($v[0]).exe") .
	if ($LASTEXITCODE -ne 0) { throw "build $($v[0]) failed" }
	Pop-Location
}
foreach ($w in 'none', 'isolate', 'defer', 'owned') {
	foreach ($v in 'base', 'new') {
		$exe = Join-Path $out "fault-$v.exe"
		$best = [double]::MaxValue
		for ($k = 0; $k -lt 5; $k++) {
			$t = (Measure-Command { & $exe $w | Out-Null }).TotalMilliseconds
			if ($t -lt $best) { $best = $t }
		}
		$line = "$w llvm $v $([math]::Round($best, 1))"
		Write-Output $line
		Add-Content -Path $res -Value $line
	}
}
