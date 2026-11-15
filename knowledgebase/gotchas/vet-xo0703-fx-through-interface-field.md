# xo vet says `uses fx` is unused when the only use is a call through an interface field

Status: Observed (2026-11-15, std/sql tracing, decision 0648 item 16)

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

Not traced yet: vet's unused effect check (compiler/cli/core/vet.xo)
does not count the effect variable performed by a call through an
interface field, which the checker does.

## Workaround

Keep `uses (..., fx)` (the checker is right) and leave the warning; the
Traced, TracedStmt and TracedCursor wrappers in std/native/sql.xo carry
eight of them. A wrapper whose methods return no `[fx]` type can use the
bare receiver and `uses net.dial` as `Noticed` does, which neither
reports.
