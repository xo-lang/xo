# 0613: `var self` interface methods

Status: Accepted (owner, 2026-11-11; staged: bounds, then values). Proposed by
Xo Lang 1 (2026-11-11); gap found by
Azure 1 porting BigQuery. Step 4 built 2026-11-11 (new code XO0439).

## Context

- Spec 2.4 says interface methods take `self` first and are satisfied
  structurally "as in Go". It says nothing about `var self`.
  compiler/check decl.xo accepts only a plain `self` parameter, so
  `interface Loader { fn load(var self, n: Int) }` is XO0403 ("must take
  `self` first"), plus the parser's XO0160 for the `var`.
- The checker lets `fn (var b: Box) load(n: Int)` satisfy
  `fn load(self, n: Int)`. A call through the interface then reaches
  compiler/ssa iface.xo, which rejects any `recv_var` method with
  XO1201 "through an interface ... (planned)". That covers generic
  bounds (`fn fill[T: Loader](var t: T) { t.load(5) }`) and interface
  values. The "planned" is decision 0100's leftover: "interface methods
  with `var` parameters are still XO1201 when used through a value". 0494
  part 3 (`Next[T]`, `fn next(var self) -> T?`) is waiting on the same
  gap.
- Accepting a `var` method for a `self` interface method is unsound as
  specified. Through the interface the caller passes a value it does
  not expect to change, so the method's update is lost or must be
  invented.
- Go's model is a pointer receiver: a `*T` method is in the method set
  of `*T` only, so `T` does not satisfy an interface that needs it, and
  the interface holds a pointer, so the update is shared. Xo has value
  semantics and no pointers, so a `var self` call updates the place the
  receiver came from, as a direct `var` method call does (spec 3.3,
  call.xo: the place is read, passed, and written back).

## Decision (proposed)

1. **Declaring.** An interface method may take `var self`. XO0403 keeps
   its meaning for a method with no `self`.
2. **Satisfying** (Go's method sets, with `var self` in the role of the
   pointer receiver):
   - a `var self` interface method is satisfied by a `var` method or by
     a plain one (a method that only reads needs no write back);
   - a plain `self` interface method is satisfied only by a plain
     method. A `var` method there becomes a new error, XO04xx (numbered
     when built), naming the method and offering two fixes: make the
     interface method `var self`, or the method plain. This breaks code
     that the checker accepts today but that fails on every native
     build (XO1201), so it gets a CHANGELOG **Breaking** line.
3. **Calling.** A call of a `var self` interface method needs a `var`
   place receiver: a `var` local or parameter, or a field path of one.
   Any other receiver is the existing "cannot mutate" error, as with a
   direct `var` method call.
4. **Generic bounds** (`T: Loader`, statically dispatched, since bodies
   are instantiated per type): the call lowers as a direct `var` method
   call does. The place is read, the method is called, and the place is
   written. iface.xo drops its `recv_var` rejection for this path.
5. **Interface values** (`let l: Loader = box`; decision 0100's
   environment of closures): a `var self` method's closure takes the
   interface value's environment and returns the updated one, and the
   call writes it back to the interface value's place. A shared
   environment (count > 1) is copied first, as any shared value is
   before a write, so two interface values made from one `Box` never
   see each other's updates (value semantics, unlike Go's shared
   pointer). Step 5 may come after step 4 if the owner wants it staged.

## Consequences

- Backend cost. Bounds: none beyond a direct `var` call (the bodies are
  monomorphized). Values: a `var` method's slot returns the
  environment, there is one write back per call, and the copy runs only
  when the environment is shared. Non-`var` methods are unchanged.
- 0494 part 3 (`Next[T]`, `TryNext[T, E]`) can be built on step 4.
- Spec 2.4 gains the rules above. spec 10.6 gains the new XO04xx row.
  XO1201's "planned" for `var` receivers goes away.
- Go backend: bounds are already generic Go code with `*T` receivers,
  if the Go backend lowers `var` methods that way (to check when
  built).

## Alternatives

- **Go's sharing.** An interface value holding a `var` receiver would
  alias its source. Rejected: it breaks value semantics everywhere else
  in Xo.
- **Keep accepting `var` methods for `self` interface methods** and
  drop the update. Rejected: silent data loss.
- **Bounds only, never values** (as 0494 noted). This is possible as a
  first stage (step 4), but leaving values at XO1201 keeps the gap that
  Azure 1 hit for any SDK that stores a Loader in a struct.

## Amendment: step 5 needs a representation choice (Proposed, Xo Lang 1, 2026-11-12)

Status of this amendment: Proposed, for the owner. Step 4 (bounds) is on
main (17abec150); interface values with a `var self` method are still
XO1201 on the native backends and a clean "decision 0613 step 5" error on
the Go backend.

**Why step 5 cannot be built as written.** An interface value
(compiler/ssa iface.xo to_iface) is a struct of closures that all share
one captured environment holding the concrete value (Op.Closure for the
show function, Op.Rebind for every method slot onto the same
environment). No SSA operation writes a captured value, and the
environment's count is always above one (every slot's closure holds it),
so "copy the environment only when it is shared" cannot be tested at
run time.

**Option A: the `var` slot returns a rebuilt interface value.** The thunk
copies the concrete value out of the environment, calls the method, and
builds a new interface value; the caller writes it back to the place.
It needs no new operation. But the old environment still holds the value
while the method writes it, so every write copies the heap fields it
touches. Priced with a program that has A's reference pattern (the old
value alive across the write) against B's (the value held once, written
in place, which is what a call through a generic bound does since step
4); a loader appending N rows, darwin/arm64, one run each
(~/xo-lead/notes/lang1/ifv/main.xo):

| rows N | B: in place | A: copy per call |
|---|---|---|
| 1,000 | 7 us | 113 us |
| 10,000 | 31 us | 8.0 ms |
| 100,000 | 0.24 ms | 6.3 s |

A is quadratic in the size the method writes, which is the ValueLoader
case that motivated this decision. Not recommended.

**Option B: an itab form.** An interface value becomes {a box holding the
concrete value, a static table of the concrete type's methods, its type
id}, the table's functions taking the box. Only the interface value holds
the box, so a `var` slot takes the value out when the box's count is one
(in place, no copy) and copies it when the box is shared (two interface
values made from one value: value semantics, as written above). The work
it needs:

- compiler/ssa: an erased box type (the interface's one heap object for
  any concrete type), boxing and unboxing operations, and an operation
  that takes the box's value when its count is one and copies it
  otherwise; to_iface, iface_call, the eq, show and type id paths, and
  the interface readers in build.xo, expr.xo and show.xo move from slot
  closures to the table. This SSA lowering serves arm64, amd64, LLVM and
  wasm at once.
- each of those backends lowers the new operations (the take-if-unique
  check is a count test and a branch around a copy; whether LLVM and wasm
  can reuse an existing runtime copy call is to check when built).
- Go backend (compiler/gen, Go interfaces): the interface holds a pointer
  to a cell with `*T` methods; Go has no counts, so value semantics needs
  a copy when an interface value is copied (or a shared flag set on copy
  and tested before a `var` call). gen to_iface and the adapters (adapt.xo).
- dispatch cost (expected, not measured): a table call is one load more
  than today's slot closure call, and non-`var` methods no longer need a closure per method per
  value (an interface value is three words instead of n + 3 closures),
  so building interface values gets cheaper; the `--ab` of bench/perf's
  interface rows on linux/arm64 prices it, as the spike's first step.

**Recommendation: B**, built in stages behind the existing XO1201: (1) the
SSA representation and arm64 with the interface goldens and an `--ab` of
dispatch and of building interface values (the decision point: B ships
only if dispatch is within noise); (2) amd64, LLVM, wasm; (3) the Go
backend's cell; (4) `var self` slots on top, which then cost what a
direct `var` call costs. A stays rejected for the quadratic cost above.
