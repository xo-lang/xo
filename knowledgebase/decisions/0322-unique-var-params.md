# 0322: unique `var` List parameters skip the entry copy-on-write check

Status: Proposed

Stub: an interprocedural proof that every call of a function passes a
unique list to a `var` List parameter removes the parameter's entry
`reuse` (copy-on-write check), with a checking mode (XO_RC_CHECK=1).
