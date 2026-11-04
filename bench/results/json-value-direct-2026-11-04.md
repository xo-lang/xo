# json.to_value and from_value with no text (2026-11-04)

Xo Lang 2, the lead's queue item after decision 0488: generate direct
T to json.Value and json.Value to T paths, no public API change.

## Why

Mac XO_PROF of bench/perf/toml encode (1000 Hz): json.to_value was 76%
of toml.encode, of which parse_value 55% and encode_str 21%: to_value was
parse_value(encode_str(v)). from_value was decode(value_encode(v).bytes()).

## What

- to_value: a codec per type, compiler/ssa jsonval.xo (xo.json.val[T]),
  through native/rtjson's json_val; where the text changed a value on its
  way back, std/native/json helpers give what the parse gave (a whole
  Float above Int's minimum and below 2^63 is an Int, -2^63 a Num, NaN
  and the infinities null, a UInt64 above Int's range a Num, a json.Value
  field read back from its text, Bytes, Duration and Time as their text).
- from_value: a tape in jsontext's format written from the Value (its
  strings placed as they are, its numbers' text), read by the decoder
  generated for T, as decode reads the text.

## Host 1 (linux/arm64, 12 CPUs), `ab-host.sh A B NAME --rounds=5 -- PROG W`, n=5

| program | workload | A wall ms | B wall ms | ratio | interval | verdict |
|---|---|---|---|---|---|---|
| jsonvalue | to | 163.9 | 23.58 | 0.144 | 0.138 to 0.145 | better |
| toml | encode | 40.06 | 16.46 | 0.411 | 0.399 to 0.427 | better |
| toml | decode (control, unchanged path) | 50.48 | 50.45 | 0.999 | 0.979 to 1.022 | within noise |
| jsonvalue | from | 152.3 | 115.3 | 0.757 | 0.749 to 0.764 | better |

A: b54838d90 (toml) and 52d2ebad7 (jsonvalue, the same base plus the
bench); B: 5428b2ac6 (toml), 770cfbb72 (jsonvalue `to`), 41c147392 (jsonvalue `from`).

The first tape writer set STR_HIGH on every string: from 1.074 (1.048
to 1.092), worse, the decoder matching every key on its slow path
(XO_PROF on the Mac: jsontext.keys_match_slow 34%, str_at 31%); the flag
is now set only for a Str with a byte from 0x80.
Max RSS within noise or lower on every row.

## Go twin

Against Go (host 1, 2026-11-04, `xo bench --ab --rounds=9 --early`, Go twin
vs Xo `--release` at 41c147392; Go's twin of jsonvalue marshals the Config and
unmarshals into `any`, then back, which is what to_value and from_value
replace; toml's twin is the vendored Go module of std-twin-rows3):

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| jsonvalue | none | 1.136 | 0.479 | 0.422 | 0.386 to 0.436 | 0.263 |
| jsonvalue | to | 460.0 | 24.4 | 0.053 | 0.053 to 0.054 | 0.146 |
| jsonvalue | from | 679.9 | 122.5 | 0.180 | 0.178 to 0.185 | 0.145 |
| toml | none | 0.602 | 0.341 | 0.568 | 0.518 to 0.59 | 0.274 |
| toml | decode | 46.54 | 52.7 | 1.132 | 1.099 to 1.152 | 0.154 |
| toml | encode | 33.42 | 16.39 | 0.490 | 0.466 to 0.507 | 0.155 |

toml encode was 1.323 of Go on 2026-11-03 (std-twin-rows3); decode does not
use these paths (1.174 then, 1.132 now, a different day's host load).

## Checks

Golden json_to_value: each value's to_value structurally equal to
parse_value(encode_str(v)), and each from_value (values and errors)
equal to decode of the Value's text, on arm64 and LLVM, with Go parity.
