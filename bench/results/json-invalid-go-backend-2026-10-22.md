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

## After the trailing byte change, and one more cut (2026-10-22, later)

Re-measured on main at 31d67996d (a byte after the value now an error, so the
first kind is answered by the scanner too): `invalid` 536 M instructions
total, 463 M less the 73 M setup, 2.3x Go's 205 M. `Json_decode` now converts
the text to bytes once (it did twice on the error path) and
`jsonSyntaxErr` type-asserts the error instead of `errors.As`: 486 M total
(413 M net, 2.0x Go), `xo bench --ab --rounds=7` instructions 0.908
(0.904 to 0.913), wall within noise.

## One scan: encoding/json's scanner copied into std/rt (2026-10-22, later)

`std/rt/jsonscan.go` is the scanner of encoding/json (go1.26, BSD, listed in
THIRD_PARTY_NOTICES) with its names prefixed `js`, and `jsCheck` is its
`checkValid`. `jsonDecodeFast` calls it in place of `json.Valid` and hands its
error to `Json_decode` (`jsonErrOf`), so a text that is not valid is scanned
once, and `json.Unmarshal` is not called for the error. Same texts:
`TestJsonSyntaxErrMatchesDecoder` and `TestJsonTrailingBytes` run on the copy.

`invalid`, instructions less the 73 M setup: 415 M (main, 488 M total) to 235 M
(308 M total), Go's `json.Unmarshal` 205 M: 2.0x to 1.15x Go.
`xo bench --ab --rounds=7`: instructions 0.631 (0.629 to 0.633), wall 0.674
(0.491 to 0.771), RSS 0.801, all better. Valid text: `small` instructions
0.986 (0.982 to 0.990, better), `strings` 0.991 and `parse` 0.999 within noise.

## What is left (1.15x)

The Go side is one scan and the error text; what remains is the Xo side of a
failed decode (the `Result`, the error value, the text concatenation).
