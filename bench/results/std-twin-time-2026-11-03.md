# std/time parse: field codes (2026-11-03)

Host 2 (linux/arm64, `ab-host.sh`, `xo bench --ab`, stopped early at n=6), bench/perf/time `parse`, before vs after "parse_stream: field arguments are integer codes and the literal is a range of the layout".

| workload | before ms | after ms | ratio | interval |
|---|---|---|---|---|
| parse (wall) | 54.97 | 44.80 | 0.815 | 0.791-0.828 |

Against Go: the last host 1 row was 3.75 of Go for parse; the new row waits for host 1. The profile before the change (Mac, 6M parses): `parse_stream` 56 percent self, `drop Str` 13, `field_code_b` 11, `field_ok` 3. Next: `field_code_b` and the per-call layout scan (a per-layout cache is not allowed by the no-package-level-cache rule, so scan once per call with a first-byte switch as `field_scan` does for format).
