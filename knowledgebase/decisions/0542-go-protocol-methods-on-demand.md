# 0542: Go backend emits protocol methods only for the types that can reach them

Status: Accepted (owner, 2026-11-07)

## Context
aws-19's armnetwork (Azure SDK, 1132 models, 303 enums, 838 operations) generates one
585,001-line Go file; Go's compile needs about 11.5 GB (10.3 GB with GOGC=25 -c=1), and its
build coincided with the 2026-11-06 23:40 memory event that stopped every agent's jobs.
By family (gosplit/plan.md): per-type protocol methods 172k lines (29%: XoEq 47k, XoEqT 33k,
XoGenAny 31k, XoDebug 29k, XoHash 23k, XoMark 9k), JSON 131k (22%), operations 133k (23%),
generic copies 41k (7%, capped at 64 per function). gen_derived (compiler/gen/decl.xo) writes
XoEq, XoHash, XoDebug, M_debug, XoEqT for every struct and newtype, XoGenAny in test mode.
One gen run sees every non-std module of the program (gen.xo: g.pkgs over c.order), so a
whole-program analysis is available.

## Decision
1. compiler/gen/proto.xo walks, after every module and generic copy is
   generated, from each protocol's roots to the user type objects they
   reach (fields, enum payloads, newtype targets, type arguments, with the
   instance's substitution):
   - XoGenAny (test builds only): the parameter types of property tests
     and fuzz targets, and the type arguments of `testing.gen[T]` calls.
   - XoEq, XoEqT and XoHash: the operand type of every `==` and `!=` the
     generator writes (compare(): `expect(a) == b` too), the subject of a
     literal pattern it compares with rt.Eq, the element type of the
     prelude's `contains` and `index_of` (std/rt List_contains and
     List_index_of compare with rt.Eq), every Map key and Set element
     type instance, every argument of a constrained type parameter of a
     std function or type (std's Go is pregenerated, so its bodies are not
     seen; a program's own generic needs no root at its calls, since each
     specialized copy roots what it compares), and every user type that
     derives Eq, Hash or Ord with what its fields reach.
   A type parameter as a root (in a key, a compare, a search or a std
   call) is resolved where it is written: in a specialized copy it is
   concrete; in a function's generic form it is recorded on that function
   and keeps every type's methods only if the generic form runs (a call
   not specialized: past MAX_SPECS_PER_FUNC or MAX_SPEC_DEPTH, or
   --no-mono, and what such a form calls); in a generic type (a field
   `Set[T]`) it makes that type's parameter keyed, so each instance roots
   its argument. An interface type reached by a protocol reaches every
   type the program converts to it (each conversion the generator writes
   is recorded); a type parameter converted to an interface, or one
   reached by XoGenAny, keeps that protocol's methods on every type.
   (Two drafts were too wide: one kept every method for any interface or
   type parameter reached, which std's own `Map[K, V]` did in every
   program; the next rooted every constrained argument, so azure's
   `marshal_json[T: Json]` kept XoEq on 1,437 of armnetwork's 3,290
   types. One was too narrow: List `contains` and `index_of` were not
   roots and faulted, fixed in 1cd6984d8 with golden go_proto_prelude.)
   std's types in the runtime package keep every method.
2. Each such method is written between two marker chunks of the package's
   output; pkg_file keeps a method only when its object is reached (or the
   protocol is kept for all). XoDebug, M_debug, XoCmp and the JSON methods
   are unchanged (Debug's roots include every function's parameters through
   error frames, so pruning it would keep nearly everything).
3. Safety: every generated type has an empty `XoProto()` method; rt.EqAny
   and rt.HashAny fault with "xo internal: XoEq (XoHash) not generated for
   T (decision 0542; XO_GO_PROTO=all writes every one)" for a value with
   XoProto and without the method, instead of falling back to
   reflect.DeepEqual or the Debug text; rt.Gen faults the same way for a
   generated struct without XoGenAny. A missed root is a loud fault, never
   a wrong comparison.
4. XO_GO_PROTO=all writes every method, as before (a bisect switch).

## Consequences
- armnetwork, measured by aws-19 on 2026-11-07 (xo c2f2c5617, `xo test --backend=go` of
  resourcemanager/network/armnetwork, so XoGenAny counts; the tests pass in both modes): the
  generated package is 507,373 lines (20.7 MB) by default against 576,481 (22.7 MB) with
  XO_GO_PROTO=all, 69,108 fewer (-12.0%). Protocol methods fall from 118,172 lines (20.5%: XoEq
  3,290 functions, XoEqT 3,114, XoHash 3,290, XoGenAny 3,290) to 55,782 (11.0%: XoEq 1,437,
  XoEqT 1,431, XoHash 1,437, XoGenAny 0). The armnetwork package's Go compile, measured exactly
  on 2026-11-07 (linux/arm64 host, 12 CPUs, go 1.26.1, xo's flags `-trimpath -mod=mod
  -gcflags=all=-dwarf=false`, a fresh GOCACHE, /usr/bin/time -v through -toolexec, 3 rounds):
  max RSS 14.83 GB median (14.78, 15.60, 14.83) against 17.40 GB (17.72, 17.36, 17.40) with
  XO_GO_PROTO=all, -14.8%; compile time 48.96 s median against 54.74 s, -10.6%. (aws-19's 1 s
  samples on the Mac, 8.8 against 8.3 GB, were within their noise.) 44% of the types still keep XoEq: which roots reach them
  is the next refinement.
- Golden internal/llvm/testdata/golden/go_proto_roots reaches a type through each root kind only;
  Go parity fails if the walk misses one.
- A new root kind in the language must be added to the analysis (a test lists the roots).

## Alternatives
- Split into several Go packages: Go holds one package in memory, so only packages lower the
  peak; needs exported names across chunks; a larger change for the same or less (kept as
  lever D in plan.md).
- Reflection-driven shared XoEq/XoHash for all types: slower on hot comparisons and a rt
  change with its own semantics risk.
- GOGC/-c=1 for big packages: 10% less peak, no fix.
