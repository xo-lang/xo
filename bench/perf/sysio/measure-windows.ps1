# measure-windows.ps1: bench/perf/sysio's measure.sh for Windows, which has
# no perf and no /usr/bin/time -l. Runs one built sysio binary for each
# workload N times (default 15) in a fresh scratch directory and prints
# "<workload> <min cycles> <min ms> <checksum>": the minimum of
# QueryProcessCycleTime (the CPU cycles of every thread of the process,
# user and kernel; the analogue of measure.sh's instruction counts) and of
# the wall time (bench/results/clocks-2026-10-14.md's method).
#
# usage: powershell -ExecutionPolicy Bypass -File measure-windows.ps1 `
#          -Exe C:\path\t-c.exe -Out C:\path\out [-N 15] [-Workloads dir,lines]
param(
	[Parameter(Mandatory = $true)][string]$Exe,
	[Parameter(Mandatory = $true)][string]$Out,
	[int]$N = 15,
	[string[]]$Workloads = @('none', 'dir', 'lines', 'handle', 'rename', 'lock', 'tree', 'copy', 'mode')
)
$ErrorActionPreference = 'Stop'
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class XoCycles {
	[DllImport("kernel32.dll", SetLastError = true)]
	public static extern bool QueryProcessCycleTime(IntPtr h, out ulong cycles);
}
'@
$Exe = (Resolve-Path $Exe).Path
New-Item -ItemType Directory -Force -Path $Out | Out-Null
$Out = (Resolve-Path $Out).Path
foreach ($w in $Workloads) {
	$best = [ulong]::MaxValue
	$bestMs = [double]::MaxValue
	$sum = ''
	for ($k = 0; $k -lt $N; $k++) {
		$scratch = Join-Path $Out 'scratch'
		if (Test-Path $scratch) { Remove-Item -Recurse -Force $scratch }
		New-Item -ItemType Directory -Path $scratch | Out-Null
		$si = New-Object System.Diagnostics.ProcessStartInfo
		$si.FileName = $Exe
		$si.Arguments = "$w scratch"
		$si.WorkingDirectory = $Out
		$si.UseShellExecute = $false
		$si.RedirectStandardOutput = $true
		$sw = [System.Diagnostics.Stopwatch]::StartNew()
		$p = [System.Diagnostics.Process]::Start($si)
		$o = $p.StandardOutput.ReadToEnd()
		$p.WaitForExit()
		$sw.Stop()
		if ($p.ExitCode -ne 0) { throw "$w exited $($p.ExitCode)" }
		$c = [ulong]0
		if (-not [XoCycles]::QueryProcessCycleTime($p.Handle, [ref]$c)) { throw 'QueryProcessCycleTime failed' }
		$p.Dispose()
		if ($c -lt $best) { $best = $c }
		if ($sw.Elapsed.TotalMilliseconds -lt $bestMs) { $bestMs = $sw.Elapsed.TotalMilliseconds }
		$sum = ($o.Trim() -split '\s+')[1]
	}
	'{0} {1} {2:F1} {3}' -f $w, $best, $bestMs, $sum
}
$scratch = Join-Path $Out 'scratch'
if (Test-Path $scratch) { Remove-Item -Recurse -Force $scratch }
