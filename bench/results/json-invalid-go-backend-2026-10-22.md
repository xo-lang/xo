# Invalid JSON on the Go backend, 2026-10-22

`json.decode_str[User]` of text that is not valid JSON, on the Go backend
(`--backend=go`), before and after this change, and a Go twin
(`bench/perf/json/invalid_go`, go1.26.1 `json.Unmarshal` into the same
struct). Workload `invalid` of `bench/perf/json`: 1,000 texts (a document
with a byte after it, a document cut in half, a missing colon, a bad number),
decoded 40 times each. Instructions retired (`/usr/bin/time -l`), the run of
a mode that only builds the texts subtracted (Xo 73 M, Go 25 M); three runs,
spread under 1%. Both Xo binaries come from one xo (main 5ffea13f2) with the
std of main and of this change; the checksum is the same.

## Before

A failed decode ran `json.Valid` (the fast path's check), then the fast
reader up to the error, then `json.Decoder.Token` over the text again, which
builds the value up to the error with several allocations a token.

## After

`jsonDecodeFast` reports a text that is not valid, and `Json_decode` answers
with the scanner's own error (`jsonSyntaxErr`: `json.Unmarshal` into an empty
struct, which checks the syntax before it decodes) without the reader or the
token decoder. The texts of the error are the token decoder's: the scanner's
for a bad byte, `EOF` or `unexpected EOF` for a text that ends early
(`jsonEOFErr` tells between tokens from inside one). One case still goes to the token decoder because it words it differently: a
bad object key.

*Update, same day (ec2857303):* a byte after a complete value is now an error
on every backend (decision by the lead: RFC 8259, Go rejects it; std/changes/json.md),
so the text of the first of the four kinds (a document with a byte after it)
is answered by the scanner too, and the fast reader no longer keeps such a
value. The table below was measured before that change, when this kind
decoded the whole document and succeeded; the numbers after it are not
measured yet.

| | instructions | vs Go |
|---|---|---|
| Go `json.Unmarshal` | 205 M | 1.00 |
| Xo, Go backend, before | 1,707 M | 8.3 |
| Xo, Go backend, after | 670 M | 3.3 |

`xo bench --ab --rounds=7` (before against after, `invalid`): instructions
0.420 (interval 0.413 to 0.423), wall 0.537 (0.384 to 0.732), cpu 0.400,
RSS 0.949, all better. Valid text is untouched (`xo bench --ab` of `small`,
`strings`, `parse`: instructions 1.000, 0.999, 0.989, within noise).

Proof of the same texts: `TestJsonSyntaxErrMatchesDecoder` (std/rt, 50,000
random and 28,000 mutated texts, seven seeds run by hand) compares every text
`jsonSyntaxErr` answers with `jsonParseSlow`'s; `TestJsonParseFastTrailingJunk`
the byte after a value.

## What is left (3.3x, before the update above)

Two scans of the text on the error path (`json.Valid`, then `Unmarshal`'s
own) and the allocations of `Unmarshal` itself. Next: an error path that keeps
the scanner's state from the first scan (a copy of `checkValid`'s loop).
