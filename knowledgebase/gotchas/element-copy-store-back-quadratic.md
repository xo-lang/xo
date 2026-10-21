# Copying a list element out and storing it back copied it at every pass

Status: Observed (2026-10-20, release dry run, compiler/macho/pelink.xo)

Symptom: cross-linking xo.exe for windows/amd64 ran for over 24 minutes
and never finished; a sample put the time in the PE writer's `put_le`.
Linking the same objects with the fix took 276 s.

Cause: the relocation loop did `var buf = bufs[p.out]`, patched `buf`
with `put_le`, then `bufs[p.out] = buf`, once per relocation. The copy
shares the section's bytes by count (core 5.1), so the first write to
`buf` copies the whole section while `bufs` still holds it; the store
back then drops the old one. Every relocation copied its whole output
section: quadratic in the section size for a multi-megabyte .text.

Fix: write through the element (`put_le(bufs[p.out], at, v, 4)`), which
changes the list's own value in place (f104cabd). Where a copy-out is
needed, take the element (`xs[i] = []` right after the read) so the
local is the only holder. `xo vet` notes the pattern as `XO0536`
(decision 0294 addendum).
