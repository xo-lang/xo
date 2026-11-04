# std twin walkdir: DirEntry rows (2026-11-03)

Host 1 (linux/arm64), `xo bench --ab`, n=6, Xo at 66c897688 (decision 0483). Ratio is Xo wall over Go wall.

| workload | Xo does | Go does | Go ms | Xo ms | ratio | interval |
|---|---|---|---|---|---|---|
| walk | walk_dir, FileInfo (a stat each) | WalkDir, no Info | 269.2 | 400.3 | 1.49 | 1.48-1.53 |
| walk_info | walk_dir, FileInfo | WalkDir + d.Info() | 428.2 | 404.5 | 0.94 | 0.91-0.96 |
| walk_entries | walk_dir_entries, no stat | WalkDir, no Info | 272.1 | 248.7 | 0.91 | 0.91-0.94 |
| walk_entries_info | walk_dir_entries + info on each | WalkDir + d.Info() | 431.3 | 480.4 | 1.11 | 1.10-1.13 |

`walk_entries` is Proven (at most 1.05 of Go, decision 0461). `walk_entries_info` is 1.11: `info` runs `fs_check` and a path resolution per entry, where Go's `d.Info()` reuses the open directory; a cheaper `info` (stat relative to the parent) is the next step.
