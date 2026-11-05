# 0522: Closure ergonomics (amends 0003)

Status: Proposed (Xo Lang 1, 2026-11-05, for the owner; from the agents'
survey of 2026-11-05).

## Context

The survey of 2026-11-05 (aws-19, Xo Lang 2, Xo Lang 3; notes in
~/xo-lead/notes/lead/survey-2026-11-05/) named three closure frictions.
Each was checked against `xo check` on main (99ee25b38) before this draft,
since two of them turned out to work already in the form first described.

1. **Effects of a stored closure (aws-19, the ranked first).** A type that
   holds an effectful closure cannot implement an interface method whose
   `uses` is fixed:
   ```
   pub interface Provider { fn retrieve(self) -> Str ! Str uses (net.dial, clock) }
   pub struct ProcProvider[fx: Effects] { pub run: fn() -> Str ! Str uses fx }
   pub fn (p: ProcProvider[fx]) retrieve() -> Str ! Str uses (net.dial, clock) { (p.run)() }
   // XO0701: ProcProvider.retrieve declares uses clock, net.dial, but performs fx
   ```
   That is correct today: the interface promises its callers no other
   effect. The cost is real: AWS's credential_process runs in a background
   task fed through a channel (aws process.xo:125, :148), SSO token
   write-back moved out of `retrieve` (ssocreds.xo:7-13), and Azure's
   `TokenCredential.get_token` and `policy.Policy.send` have the same shape.
   What already works: a struct with an effect parameter whose omitted
   argument becomes an implicit effect variable of the function taking it
   (core 6.2; `fn upload(b: Body)` calling `b.reopen()`), and an interface
   with an effect parameter, `interface Provider[fx: Effects] { fn
   retrieve(self) -> ... uses (net.dial, clock, fx) }`, used as
   `List[Provider[fx]]` in a function generic over `fx`. What does not: a
   value of `ProcProvider[env]` used as `Provider[env]` is refused, XO0701
   "`ProcProvider[env]` is used as `Provider[env]`, whose method `retrieve`
   allows only `clock, fx, net.dial` ... but performs `fx`": the
   interface's `fx` is not substituted by the type argument when the
   method's effects are compared, a checker bug, and the message repeats
   itself. A pure or smaller implementation (an `EnvProvider` without `fx`)
   is accepted in the same list, so subsumption works.

2. **`?` inside a closure (Xo Lang 2).** `?` already propagates to the
   closure when its expected or declared result is a `Result`: `fn(s: Str)
   -> Int ! E { parse(s)? + 1 }`, and `m.with(fn(var j) { j.n +=
   parse(s)?; j.n })` in a function returning `Int ! E` (with's `R ! E`
   gives the closure its error type) both check. XO0205 remains where the
   expected function type cannot fail (`map`, `filter`, any `fn(T) -> U`),
   where `?` would need an error the callee never returns. (Lang 2's own
   snippet was requested on 2026-11-05; this section is amended if it shows
   another form.)

3. **Closures that mutate captured `var` bindings (Xo Lang 3).** XO0502 by
   design (core 3.3, decision 0003: closures capture by value; shared
   mutable state goes through `Mutex` or `Atomic`). Lang 3 wrapped a test
   fake's counter in a Mutex for it.

## Decision (proposed)

1. **Effect-polymorphic interfaces are the answer to stored effectful
   closures; fix what stops them.** No new syntax: an interface that must
   admit implementations with extra effects takes an effect parameter
   (`Provider[fx: Effects]`), as structs do. The checker substitutes an
   interface's effect arguments into its methods' declared sets when it
   compares an implementation (the bug above), and XO0701's message for
   it is rewritten. A library that offers one (AWS's credentials.Provider,
   Azure's TokenCredential and Policy) chooses it in its API; callers that
   use only pure providers write `Provider[()]` or leave the argument to
   inference.
2. **Teach the existing form with fix-its**, from aws-19's two reports:
   XO0701 "performs Request.fx" inside a function that takes `req:
   Request` and calls its closure in a nested closure suggests
   `send[fx: Effects](req: Request[fx]) uses (..., fx)`; "unknown effect
   gx" in a method's parameter type suggests adding `[gx: Effects]` to
   the method. A section in docs (writing an SDK) shows the threaded form.
3. **No change to `?` in closures.** It works where the closure can fail;
   XO0205's note, where the expected type cannot fail, names the
   alternatives (`try_map`, a `for` loop, or a failing function type).
4. **No change to captured `var`s (0003 stands).** XO0502's note already
   names `Mutex` and `Atomic`; it adds the local alternative: return the
   new value from the closure, or fold.

## Consequences

- AWS and Azure can hold credential_process, SSO write-back and a
  file-logging policy inside their providers once the substitution bug is
  fixed, at the cost of one effect parameter on the interface and on the
  types that store it (aws-19: it must reach every Config, Client and
  Request in 435 generated services; generated code carries it).
- The language gains no new effect rule; 0003's value semantics and 6.2's
  exact binding of effect arguments in types are unchanged.

## Alternatives

- **Effects charged at the closure's creation** (aws-19's first wish): a
  stored closure whose effects the creating call pays. Rejected: the
  effect happens when the closure runs, and a function that runs it would
  perform an effect its signature does not show, which is what effects
  exist to prevent.
- **An interface method with an implicit `Self.fx`** (`uses (net.dial,
  clock, Self.fx)`): each implementation contributes its own effects.
  Equivalent to 1 for one implementation; for a list of providers it needs
  an existential effect, which the type system does not have. Kept as a
  follow-up if 1's explicit parameter proves too heavy in generated SDKs.
- **Mutable captures by reference** (closures mutating a captured `var`):
  reintroduces shared mutable aliasing, against 0003.

## Evidence

- `xo check` runs on 2026-11-05 at 99ee25b38 of each form above (repros in
  ~/xo-lead/notes/lang1/0522-repros/).
- aws-19's report of 2026-11-05 (xo 842bf3f5c) and docs/reports/xo-feedback.md
  in the aws repository (2026-11-04: upload_file, pager, credential_process).
