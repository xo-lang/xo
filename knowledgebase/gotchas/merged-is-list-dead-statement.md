# A merge left two `name is ...` lines; the first was silently dead

Status: Observed (2026-10-13, decision 0207 stage 2)

Symptom: TestSelfhostWasm dropped from 132 to 127 passing goldens
(wasmMinPass 128) once std code reached on wasm called `rt.mem_store`
(raw.zero_ints in regexp, jsontext, archive): "the wasm backend does
not support the call `rt.mem_store` (stage 2)", although
selfhost/wasm/rt2.xo defines `$mem_store` and lists it in
`rt2_direct`.

Cause: the merge of decision 0207 stage 1 with decision 0201
(cb9207d3) kept both sides' one line bodies of `rt2_direct`: two
expression statements `name is "a" | ... | "mem_set" | "mem_cmp"` and
`name is "a" | ... | "rs_set_len"`. The function's value is the last
one; the first is computed and dropped with no diagnostic, so mem_load,
mem_store, mem_copy, mem_move, mem_set, and mem_cmp were never direct
calls on wasm. Nothing on wasm called them until stage 2 (cabi.xo,
their only caller in stage 1, is not built for wasm).

Fix: one list (both sides' names). When resolving a conflict in a one
expression function body, keep one expression; TestSelfhostWasm after
any change to code std runs on wasm (preamble UPDATE 6) catches the
drop.
