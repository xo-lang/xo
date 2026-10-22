# Parsers copy tokens of a long input (decision 0330 option 3), 2026-10-22

Since slices are views (0330), a token a parser returns from a large input
keeps all of that input alive. Probe: each parser reads about 1 MiB, 300
times, and one small token of each result is kept (peak resident set, darwin/arm64).

| parser | before | after |
|---|---|---|
| url.parse_query | 318 MB | 6.0 MB |
| mime.parse_media_type | 318 MB | 6.1 MB |
| csv.parse | 320 MB | unchanged (fields are views by design) |
| json.decode, toml.decode, multipart.parse, xml.parse | 6 to 29 MB | unchanged (tokens are built, not sliced) |

Copying every token costs instructions on the usual small input: 200,000
x (a three parameter media type and a four pair query), instructions
retired, one run (deterministic to 0.1%):

| variant | instructions | vs before |
|---|---|---|
| views, no copies (main) | 2,593 M | |
| copy every token | 2,973 M | +14.7% |
| copy when the input is over 4 KiB (a call per token) | 2,674 M | +3.1% |
| copy when the input is over 4 KiB (test in line) | 2,629 M | +1.4% |

The last is landed: `parse_query` and `parse_media_type` copy keys, values,
kind and parameters when the input is over 4 KiB (a kept token then retains
at most 4 KiB). csv.parse keeps views: a caller that keeps one field of a
large file calls `clone()`. Probe: `scratchpad/mb/ret2` (not committed).
