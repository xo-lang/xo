# A wasm retain or release helper past the 1,000 parameters an engine accepts

Status: Observed (2026-11-11, Xo Lang 2, TestSelfhostWasm on main)

## Symptom

`xo build --backend=wasm` of frame_large_value, rc_large_offsets and
wide_store failed with "the wasm backend made $drop198 with 2000 parameters
and 0 results: more than the 1000 a WebAssembly engine accepts". The goldens
had passed (v0.48.0 builds them); TestSelfhostWasm listed them as "became
unsupported". Bisected to 4fa722d68.

## Cause

4fa722d68 (decision 0583 (e)) packed values in memory but kept an enum's
payload in words, so rc_addr_fn's enum branch stopped calling mem_rc (which
takes a wide struct field by field through its address) and loaded each
payload field's leaves to pass them to the leaf helper rc_fn. A payload field
of a struct of 2,000 leaves (Result[D, Str]) then got a 2,000-parameter
helper.

## Fix

home.xo word_rc: a struct or tuple past WIDE_RC leaves in word layout is
retained and released field by field (offsets in words), as rc_addr_fn does
in a packed one; only a narrow field goes to the leaf helper.

## How to avoid

A change to a value's layout in memory runs the wide goldens (frame_large_value,
rc_large_offsets, wide_store) on wasm in its gate:
`XO_WASM_GOLDENS=frame_large_value,rc_large_offsets,wide_store go test ./internal/selfhost -run '^TestSelfhostWasm$'`.
Any helper that takes a value's leaves as parameters needs the wide path
beside it.
