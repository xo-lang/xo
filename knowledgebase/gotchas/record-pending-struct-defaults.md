# A std struct's field defaults are empty until its trees are joined

Status: Observed (2026-11-16)

## Symptom

`json.decode_str[trace.Span]` of a Span without `trace_id` failed with
"missing field `trace_id`" in a build that compiled std separately (the
default for `xo build` and `xo run` once std's object is in the cache),
and decoded fine with `XO_SEPCOMP=off`, `XO_FE_CACHE=off` or
`XO_BACKEND_CACHE=off`. It came and went with the cache: the first
builds after a cold cache decoded fine, and every build after the one
that wrote std's declaration record (`stdx`) failed. It looked like a
stale front end cache entry (aws-43's fecache-span repro, task #126), but
no cache key was missing an input.

## Cause

A build that declares std from its record (decision 0388 stage 2,
`trees_lazy`) leaves each std object's trees pending: its body, its
parameters' defaults and its fields' defaults are joined only where
something asks (`Checker.need_tree`). The checker asks before a struct
literal, but derive Json's decoder (compiler/ssa json.xo) read
`objs[obj].fields[].default` directly, found no defaults, and lowered
every defaulted field of a std struct as required.

## Fix

The decoder sets `Builder.trees_missing` when the struct's trees are
pending, as `Builder.body` does for a std body, and the driver builds
again with every tree joined (`--phase-times` names it in
`std_trees_rebuild first body: derive Json of std/trace.Span`). Test:
internal/arm64 TestStdRecordJsonDefaults.

The other derives (checked 2026-11-16): Toml decodes through derive Json
(decision 0622), so the fix covers it (the test has a Toml case). Proto,
Xml, Yaml and Cbor expand to source in the checker (check derive.xo) and
build the struct with a literal, whose defaults the checker joins first
(need_tree before a struct literal). Derive SqlRow's decoder (json.xo
sql_body) reads a `-` field's default the same way the Json decoder did,
but no std struct derives SqlRow and the record holds std only, so it
cannot meet a pending tree today (Anticipated: a std struct deriving
SqlRow with a defaulted `-` field would decode it as its zero; sql_body
runs inside add_landings, which is pure, so it can set trees_missing but
not join the tree).

How to avoid: SSA code that reads a std object's trees (`default`,
`fn_decl`, `const_val`) checks `Checker.tree_pending` first.
