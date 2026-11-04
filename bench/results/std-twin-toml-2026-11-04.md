# std/toml twin rows after json.to_value and json.from_value (2026-11-04)

Host 1 (linux/arm64, `--early`, n=6), bench/perf/toml at dd0aaf1d6 (includes 9305dc493: to_value and from_value with no JSON text in between).

| workload | Go ms | Xo ms | ratio | interval |
|---|---|---|---|---|
| decode | 46.77 | 53.1 | 1.135 | 1.115 to 1.148 |
| encode | 33.39 | 16.13 | 0.483 | 0.481 to 0.501 |

Before (std-twin rows of 2026-11-03): decode 1.174, encode 1.323. Encode's gain is `json.to_value` building the Value directly; decode still writes JSON text from the TOML parse and decodes it, the remaining gap.
