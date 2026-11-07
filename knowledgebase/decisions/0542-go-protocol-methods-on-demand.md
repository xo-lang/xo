# 0542: Go backend emits protocol methods only for the types that can reach them

Status: Proposed (Xo Lang 2, 2026-11-07; owner acceptance pending: the measured before and after for armnetwork is added before it)

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

## Decision (proposed)
1. Before generating, compute per protocol (eq, hash, debug, gen, mark) the set of user types
   it can reach: roots are the static types of `==`/`!=`, map keys and Set elements, `${x:?}`,
   debug(), expect's compared values, forall/gen, and mark; every type converted to `any`, to an
   interface, or used as a type argument is a root of every protocol (rt reaches it through
   EqAny, HashAny, DebugAny, Gen). The sets are closed over field types, enum payloads, newtype
   targets and type arguments.
2. gen_derived emits a protocol's methods only for a type in that protocol's set.
3. Safety: rt's EqAny, HashAny, DebugTo and Gen fall back to reflection today; for a generated
   struct type (one with an `XoProto` marker method) without the method they panic with
   "xo internal: <protocol> not generated for <type>", so a missed root fails loudly (Go
   parity goldens, xo test on the Go backend) instead of comparing by reflect.DeepEqual.
4. XO_GO_PROTO=all turns pruning off (a bisect switch), in the codegen cache key.

## Consequences
- Expected: armnetwork drops most of the 172k lines (its models are compared or debugged only
  in a few tests); measure lines and compile peak with aws-19 before and after.
- A new root kind in the language must be added to the analysis (a test lists the roots).

## Alternatives
- Split into several Go packages: Go holds one package in memory, so only packages lower the
  peak; needs exported names across chunks; a larger change for the same or less (kept as
  lever D in plan.md).
- Reflection-driven shared XoEq/XoHash for all types: slower on hot comparisons and a rt
  change with its own semantics risk.
- GOGC/-c=1 for big packages: 10% less peak, no fix.
