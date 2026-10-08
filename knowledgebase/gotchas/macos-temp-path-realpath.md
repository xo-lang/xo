# macOS temp paths differ between /var and /private/var

Status: Observed (2026-10-07, planted bug detection runner)

`tempfile` returns `/var/folders/...`, but tools such as nilaway print the
resolved `/private/var/folders/...`. Stripping only the first form made every
nilaway finding look new, so every planted variant was wrongly "caught".
Fix in `bench/planted/run_detection.py`: strip both `root` and
`os.path.realpath(root)`, and reuse one fixed work directory so tool output
paths are identical between the base and each variant.
