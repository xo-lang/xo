# 0543: Map key order in JSON output

Status: Accepted, option (a) (owner, 2026-11-07; Xo Lang 3; raised by xo-lead from Xo Azure 1's findings). A Map's keys are written sorted by their bytes, as Go's encoding/json; a `json.Value` object keeps its order.

## Context

`json.encode`, `json.to_value` and `json.json_writer` write a `Map[Str, V]`'s keys in insertion order (Map is insertion ordered,
decision 0091). Go's `encoding/json` sorts map keys. Xo Azure 1's azsystemevents vectors hit it: 21 of 178 spec examples differ, and
every Azure and AWS module with a map field differs from Go when the input's keys are not already sorted. spec/std.md's toml section
already documents the same divergence ("a map's keys come in its JSON order (Go sorts them)"). Std follows Go's standard library
where it fits (CLAUDE.md), and the differential corpora compare bytes with Go's.

The scope is every place std/json turns a Map into an object: a derived `Json` struct's Map field, a `Map` given to `encode`, the
Map branch of `to_value`, and `json_writer().value(Value)` when the Value came from one. `json.Value`'s own object form
(parsed documents) is a separate question below.

## Options

(a) **Sort Map keys on encode, as Go.** Byte identical to Go for every Map; output deterministic whatever the insertion order.
 Cost: an O(k log k) sort of the keys per Map per encode (Str compare; a small Map of up to about 16 keys can use an insertion sort
 on the key list with no allocation beyond the list), no cost for structs. Changes: every golden or corpus whose expected text has a
 Map in non sorted insertion order (the 77 goldens that name a `Map[Str` are the upper bound, of which the ones that also encode
 JSON, about 43 use `json.encode` or `derive Json`, are the candidates; the real set comes from running them), the json corpora
 regenerate from Go and may already be sorted (they pass today, so their Maps are sorted or single key). A round trip no longer
 keeps the order of an input document's object when read into a Map and written back. Toml stays unsorted unless changed with it:
 toml's decoder path reads "in its JSON order", so it moves with the choice.

(b) **Keep insertion order (document the divergence).** No code change, no cost; round trips an input's order; every module that
 compares with Go's bytes keeps working around it by sorting its own input (what aws-19's ports do today) or by repinning vectors.
 The divergence stays a standing item in every port's change log.

(c) **Opt in: `derive Json(sort_keys: true)` or an encode option.** Go parity where asked, no change for existing code. Costs a flag
 on the derive and on `encode`/`json_writer`, a second code path to test, and every port still has to remember it; the default
 stays the divergent one.

## `json.Value` objects

A `Value` object holds its pairs as parsed (a document model, like `encoding/json`'s `map[string]any` only if sorted on encode).
Go sorts a `map[string]any` too, so a Value built from a Go-like map would match Go sorted. But a Value parsed from text keeps
the text's order for a faithful round trip (`json.RawMessage` in Go keeps bytes as they are). Suggested split if (a) is chosen:
`Map` fields and `to_value(Map)` sort; a `Value` object keeps its order whether it was parsed or built, and `json_writer().value`
writes it as held; `Value` gains `sorted()` to sort recursively when wanted.

## Recommendation

(a) for Map, with the Value split above: it removes a class of differences that every Azure and AWS port currently hits, it follows
Go as std does elsewhere, and it makes output deterministic. Measure first: run the golden and corpus suites with the sort on and list
what changes (expected: only Maps written out of order), and time `json.encode` of a 10 and a 1000 key Map before and after
(`xo bench --ab`, decision 0340). Toml and yaml map output move together with it (one decision for the three), their corpora
regenerate against Go's and are the check.

## Measurements (2026-11-07)

Sorted keys with an in-place insertion sort for up to 16 keys (else a sort of the index list), built at 04d5c3ae9 and
run on the first linux/arm64 offload host (bench/perf/jsonmap, descending-key Maps; the 10-key map is the "small" row):

| case | Xo, sorted (us) | Go encoding/json (us) | Xo / Go |
|------|-----------------|-----------------------|---------|
| none (no Map) | 678 | 845 | 0.80 |
| small | 9266 | 15958 | 0.58 |
| large | 38298 | 56163 | 0.68 |

Xo with sorted keys is faster than Go's `encoding/json` on all three. Against Xo's own unsorted insertion order
(`ab-host`, 3 rounds, unloaded host) sorting costs 2.3x on the small case (4.4 ms to 10.2 ms); the no-Map case is within
noise (0.94x). The goldens (arm64, llvm, stdtest) passed with the sorted order. The code stays on a branch until this
decision is Accepted.

## Consequences

- Needs the owner's decision between (a), (b) and (c) and on the Value split; nothing is implemented until then.
- If (a): std/changes entries for json (and toml, yaml if they follow), spec/std.md lines (the toml text), goldens updated in the
  same change.
