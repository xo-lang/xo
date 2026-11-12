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
