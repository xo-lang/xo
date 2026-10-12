# A JSON key that is an Xo keyword cannot be a `derive Json` field

Status: Observed (2026-10-12, xo1 regress.xo, decision 0178 stage 1)

## Symptom

Porting cmd/xo regress.go to Xo, the struct for rt.RegressRecord did not
parse: `error XO0160: expected a field name, found keyword \`test\``, and
the same for the parameter field `var`.

## Cause

`derive Json` maps each field name to the JSON key of the same name, and
there is no rename attribute. `test`, `var`, `fn`, `match`, `uses` and the
other keywords cannot be field names, so a record written by Go with such
keys cannot be decoded field for field.

## Fix

Rename the keys in the text before decoding when the producer's encoding
is known: regress.xo replaces `,"test":` with `,"test_name":` and
`,"var":` / `{"var":` with the `is_var` forms (Go writes the struct fields
in order, so the key always follows `,` or `{`). A real fix would be a
field attribute naming the JSON key (a std/json decision).
