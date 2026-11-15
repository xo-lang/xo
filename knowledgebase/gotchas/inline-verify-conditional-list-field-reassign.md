# A conditional reassignment of a struct's List field fails the inliner's verify

Status: Observed (2026-11-15, decision 0644 T13, std/native/llrt/tlsio.xo
`client_with`; not yet diagnosed, handed to Lang 1)

## Symptom

Building xo (which links std/native's TLS client) failed with every seed:

```
xo: the inlined arm64 module does not verify (1 errors; the first:
tlsio.client_with: b113: edge to b121 holds {v123.7:1 v483.22:1
v483.23:1 v483.24:1 v483.25:1 v483.26:1}, another predecessor
{v123.7:1}); failing the build (XO_INLINE_STRICT=1)
```

`client_with` builds `var cf = tls13.conf(...)`, sets its fields, and
then had

```
cf.legacy_suites = if allow12 { tlsneg.only(...) } else { [] }
...
if ech_list.is_some() {
  cf.legacy_suites = []
}
```

The function verified before the `if` was added. Replacing a struct
parameter with plain parameters did not help.

## Cause

Unknown. The error says that after inlining, one predecessor of a join
holds references to fields 22 to 26 of a value and the other does not,
so the ownership states of the two edges disagree. A field reassigned
on only one branch is the likely trigger. The failing variant is
~/xo-lead/notes/lang3/ech/tlsio-inline-verify-fail.xo (its diff against
the passing file is tlsio-inline-verify-fail.diff beside it).

## Fix

Work around it by folding the condition into the one assignment:
`cf.legacy_suites = if allow12 && ech_list.is_none() { ... } else { [] }`.
The compiler fix is Lang 1's: the join should drop or carry the field's
references the same way on both edges.
