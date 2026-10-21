# A constant's template text is not its value

Status: Observed (2026-10-21).

Symptom: after 87059821 (hoist_consts gave constants of one text in a
loop's preheader one register), clean main failed four arm64 goldens:
native_json, codex_gaps and text_table with `fault: 5e+08 is out of range
for Int at std/native/llrt/text.xo:375:15`, list_field_licm printing
`aliased 15.0` for `aliased 20.0`.

Cause: a MIR constant (Inst.konst) is identified by its text only when it
reads no registers. A Float constant is `fmov $d0, $u0` of the integer
register holding its bits (isel.xo fimm), the same text for every value;
merged by text, different Float constants in one loop read one register.
The other suspects (copy_prop, reuse_loads, the range folds) were ruled
out in minutes with `XO_PASSES=-NAME` on the failing goldens.

Fix: hoist_consts keys a constant by its text, the registers it reads,
and the class of the register it defines (konst_key). Golden
hoist_const_merge. Any pass that treats equal templates as equal values
must include Inst.us and the definition's class.
