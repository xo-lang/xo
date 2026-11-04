# JSON own form and tagged enums (decision 0478 stage 4), 2026-11-03

Xo Lang 2. The measurement plan of decision 0478, items 1 to 5, on main
after stage 3 (93600eb6a).

## 3. Code size (native __text, darwin/arm64, xo at 93600eb6a plus bb59293e3)

Programs: bench/perf/jsonsize (each encodes and decodes one value).

| program | __text bytes | against its twin |
|---|---|---|
| tagged20: 20 variant enum, `derive Json(tag: "type", case: snake)` | 342772 | +816 (+0.24%) |
| external20: the same enum, `derive Json(case: snake)` | 341956 | |
| ledger_via: Money `derive Json(via: Str, to:, from:)` in a DTO | 342680 | +88 (+0.03%) |
| ledger_str: Str fields converted by hand (the workaround) | 342592 | |

## 1, 2. Timing

Host 1 (linux/arm64, 12 CPUs, go1.26.1), xo at 11089eb18 (main after stage 3
plus these benches), `xo bench --ab --rounds=6 A B -- W`; raw output in
the lead's notes (census/twin0478-host1.txt). Ratio is B/A with its 95%
interval.

### 1. Own form cost: bench/perf/jsonown (100,000 entries, two amounts each, encoded and decoded, 10 times)

| workload | Go wall ms | Xo wall ms | Xo/Go | interval | Go max RSS KB | Xo max RSS KB |
|---|---|---|---|---|---|---|
| strfield (a: Str fields, hand conversion) | 781.8 | 294.1 | 0.376 | 0.361 to 0.417 | 74464 | 80276 |
| via (b: `via: Str`; Go: MarshalJSON/UnmarshalJSON) | 1302 | 268.3 | 0.206 | 0.198 to 0.221 | 54812 | 54304 |
| value (c: json.Value fields, hand conversion; Go: any) | 1000 | 328.8 | 0.329 | 0.303 to 0.358 | 63450 | 78264 |

Xo alone, via against strfield: wall 0.923 (0.905 to 0.929), max RSS
0.676 (54304 KB against 80276 KB): the own form is faster than the
workaround and keeps no Str copies of the amounts. The plan expected (b)
within noise of (a); it is 8% better. Go's own form (d) is its slowest
case (MarshalJSON goes through reflection and a second Unmarshal per
value).

### 2. Tagged decode cost: bench/perf/jsontag (1,000,000 decodes of 185 byte messages, five variants)

| workload | Go wall ms | Xo wall ms | Xo/Go | interval | Go max RSS KB | Xo max RSS KB |
|---|---|---|---|---|---|---|
| external (Go: map of RawMessage) | 1962 | 218.6 | 0.111 | 0.104 to 0.115 | 11068 | 1244 |
| tagfirst (Go: two pass, RawMessage) | 1699 | 229.7 | 0.135 | 0.13 to 0.14 | 11074 | 1244 |
| taglast (Go: two pass) | 1772 | 235.2 | 0.133 | 0.131 to 0.142 | 11036 | 1244 |
| onepass (Xo: tagfirst again; Go: one struct of every field) | 965.2 | 231.5 | 0.240 | 0.227 to 0.25 | 11044 | 1244 |

Xo alone: taglast against tagfirst 0.992 (0.975 to 1.021), within noise,
as expected (the tag is found by lookup, wherever it is). Tagfirst
against external 1.089 (1.045 to 1.118): worse, where the plan expected
within noise. A tagged decode reads the object's tag first and then its
fields with the struct rules; the external form's single key selects the
variant directly. A Mac XO_PROF (1000 Hz, 2026-11-04) of the two: jsontext.str_end
38 ms against 18 ms, and alloc.alloc and obj.count (12 and 13 ms) only
in tagfirst. Both forms read the variant name into a Str (dec_str) and
compare it per variant with str.eq; the difference is the field pass,
which in the tagged form scans the one object holding the tag and the
fields, and reads the tag's string a second time. Follow-up: match the
tag in place (key_eq_padded on the tape, no Str) and skip the tag member
in dec_fields.

Follow-up done 2026-11-04 (jsontext.str_is: the tag compared in place,
no Str unless the variant is unknown), host 1, `xo bench --ab`, n=6, A
11089eb18, B the change: tagfirst wall 0.857 (0.842 to 0.889), better,
220.7 to 189.0 ms; external 0.984 (0.88 to 1.006), within noise (its
path is unchanged). A tagged decode now takes less time than the
external form's (189.0 against 208.5 ms). The tag member is still read
by the field pass as an unknown key; skipping it is not needed for this.

## 4. Correctness, every backend

- golden json_via (arm64, LLVM, Go parity, wasm): struct via Str with a
  failing `from`, enum via Int, newtype via Str, in fields, `List`, `Map`,
  `T?`, `Patch[T]`, and a tagged enum's content; errors at their paths.
- golden json_tagged_enum, json_variant_names (stages 1 and 2).
- golden sql_json_tagged: `r.json[T]` on a tagged enum and a via type.
- test/errorcheck/json_via, wire_names; cmd/xo TestFmtJsonVia,
  TestFmtEnumWireNames.

## 5. No compiler effect

`go run ./scripts/identity --targets=linux/arm64 586873428 NEW` on host 1
(2026-11-04), NEW built at 11089eb18 (stage 3, 93600eb6a, plus these
benches and goldens, which change nothing under compiler/ or std/):
137 programs compared, 0 differing, 0 failed on either side. Programs
that do not use via or the tagged forms build byte for byte as before.
The darwin/arm64 target is not compared on a Linux host (objdump cannot
read Mach-O; gotchas/identity-darwin-target-on-linux-host.md).
