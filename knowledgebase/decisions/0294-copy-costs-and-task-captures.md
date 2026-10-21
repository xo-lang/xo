# 0294: Copy costs in the spec, and a vet note for large task captures

Status: Accepted (lead, from the agents' poll with the owner, 2026-10-20)

## Context

Value semantics (core 5.1, decision 0003) say what a copy means, not what
it costs. In the 2026-10-20 poll Xo lang 5 could not tell whether
capturing the compiler's `Checker` into a task copied it, or whether a
`push` copied the list. `xo vet` and `xo explain XO0531` explained one
shape (a large parameter only stored, decision 0263), but nothing said
the general rule. The answer is in the runtime: a copy shares each heap
field by a count (std/native/llrt obj.xo), and a write to a collection
something else still holds copies it first (copy on write; a native Map
the same, until decision 0291's log makes that copy O(1)).

## Decision

- spec/core.md 5.1 gains an informative paragraph, "What a copy costs":
  a copy costs its heap fields (counted, never deep); a write to a shared
  collection copies it once, then writes in place; a task's capture shares
  every heap field at the spawn and releases them at its end, and the
  first change on either side copies the collection; a `Mutex.with` body
  writes a value the mutex still holds.
- `xo vet` notes `XO0534` (note severity, code group of XO0530 to
  XO0532) at each local a closure passed to `Scope.spawn` or
  `spawn_background` captures when its type has 32 or more heap fields
  (vet.xo `TASK_CAPTURE_THRESHOLD`; XO0531's 8 would flag everyday config
  structs). The note says to capture the fields the task uses.
  `XO0533` stays reserved by decision 0266.

## Consequences

- An agent can read the cost model in the spec instead of profiling, and
  vet points at the expensive capture while writing it.
- The note is advisory and best effort, like decision 0263's: vet sees
  the AST and types, not whether the RC pass moves the value.
- When decision 0291 lands, the Map part of the paragraph changes from
  O(n) to O(1); the rule for lists and strings stays.

## Alternatives

- A warning: the program is correct, and many captures are cheap enough.
- Using XO0531's threshold of 8: too many notes on ordinary handlers'
  captured configs.
- Documenting it in a knowledgebase page only: agents read the spec for
  semantics, and this is where the copy rule already is.

## Addendum (2026-10-20): list elements copied out and stored back

The release dry run's windows/amd64 link of xo.exe ran for over 24
minutes in the PE linker: pelink.xo read each output section with
`var buf = bufs[p.out]`, applied one relocation, and stored it back with
`bufs[p.out] = buf`, so every relocation copied the whole section (fixed
in place, f104cabd). The lead asked for the general check: `xo vet` notes
`XO0536` when `var x = xs[i]` on a List whose elements are collections
(List, Bytes, Map, Set, Str) is followed in the same block by
`xs[i] = x`. Taking the element first (`xs[i] = []` between, lsr.xo's
idiom) is not reported. The grep of compiler/ and std/ found 29 copy-outs
stored back. Most are struct elements (a struct copy shares its fields,
and a change copies only the field it changes). The collection elements
are pelink.xo (fixed), dsym.xo's two (once per link), and lsr.xo (a
take). spec/core.md 5.1 states the rule.
