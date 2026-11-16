# xo vet says `uses fx` is unused when the only use is a call through an interface field

Status: Observed (2026-11-15, std/sql tracing, decision 0648 item 16); fixed 2026-11-15 (task #38)

## Symptom

`xo vet` warns `XO0703: Wrap.get declares uses fx but never performs it`
on a method of an effect generic struct whose body calls a method of a
field typed `Src[fx]` (an interface with `uses (clock, fx)`). Removing
`fx` as the fix says makes `xo check` fail: `XO0701: Wrap.get declares
uses clock, but performs fx`.

## Repro

```
interface Src[fx: Effects] {
  fn get(self) -> Int uses (clock, fx)
}

struct Wrap[fx: Effects] {
  s: Src[fx]
}

fn (w: Wrap[fx]) get() -> Int uses (clock, fx) {
  w.s.get() + 1
}
```

`xo check` is clean; `xo vet` gives the XO0703 above.

## Cause

vet's unused effect check (compiler/cli/core/vet.xo unused_effects)
matched an effect parameter against the performed parameters only when
it was a type parameter of the function itself. A method's `fx` here is
its receiver struct's, so the check looked for it among the effect
labels, where `performed_of` never lists a parameter, and reported it.

## Fix

An effect name that `performed_of` reports as a performed parameter now
counts as used, whether it is the function's or its receiver type's; an
`fx` the body never performs is still reported. Test: cmd/xo
TestVetUsesFxOfReceiver. Before the fix, the Traced, TracedStmt and
TracedCursor wrappers in std/native/sql.xo carried eight such warnings
(std modules are not vetted for XO0703, so only a user's copy showed
them).
