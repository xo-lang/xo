# scripts/ci/ci-out.ps1: ci-out.sh for the PowerShell CI scripts
# (win-crash-cdb.ps1, win-cdb-watch.ps1); dot-source it. Actions
# artifacts carry text only (GitHub's artifact egress limit, 2026-10-22):
# large outputs (dumps, binaries kept for debugging, text over 1 MiB)
# move to s3://$XO_SEED_BUCKET/dev/ci-out/$GITHUB_RUN_ID/<DIR's name>/,
# a line per file in the summary. Nothing expires under dev/: delete a
# run's prefix once read (knowledgebase/infrastructure/ci.md).
#   Send-CiOut -Dir DIR -Summary SUMMARY
function Send-CiOut([string]$Dir, [string]$Summary) {
  $max = if ($env:CI_OUT_MAX) { [int64]$env:CI_OUT_MAX } else { 1048576 }
  $text = @('.txt', '.log', '.out', '.err', '.json', '.md', '.csv')
  $run = $env:GITHUB_RUN_ID
  $bucket = $env:XO_SEED_BUCKET
  $aws = Get-Command aws -ErrorAction SilentlyContinue
  $n = 0
  $root = (Resolve-Path $Dir).Path
  foreach ($f in Get-ChildItem -Recurse -File $Dir) {
    if ($f.Name.StartsWith('.')) { continue }
    if ($text -contains $f.Extension.ToLower() -and $f.Length -le $max) { continue }
    $rel = $f.FullName.Substring($root.Length).TrimStart('\', '/').Replace('\', '/')
    if (-not $run -or -not $bucket -or -not $aws) {
      Add-Content $Summary "large output kept locally (no CI bucket): $rel ($($f.Length) bytes)"
      continue
    }
    $dest = "s3://$bucket/dev/ci-out/$run/$(Split-Path $root -Leaf)/$rel"
    & aws s3 cp --only-show-errors $f.FullName $dest
    if ($LASTEXITCODE -eq 0) {
      Add-Content $Summary "large output in the bucket: $dest ($($f.Length) bytes)"
      $n++
    } else {
      Add-Content $Summary "large output NOT uploaded (aws s3 cp exit $LASTEXITCODE), left out of the artifact: $rel ($($f.Length) bytes)"
    }
    Remove-Item -Force $f.FullName
  }
  if ($n -gt 0) { Add-Content $Summary "large outputs: $n under s3://$bucket/dev/ci-out/$run/ (delete the prefix once read)" }
}
