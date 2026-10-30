# Unify bound effect sets without comparing them, and a pure function performs effects

Status: Observed (2026-10-30)

## Symptom

A pub function with no `uses` read the clock and checked with 0 errors:

```xo
struct Box[fx: Effects] { f: fn() -> Int uses fx }
fn run[fx: Effects](a: fn() -> Int uses fx, b: Box[fx]) -> Int uses fx { a() + (b.f)() }
fn one() -> Int { 1 }
fn p(b: Box[clock]) -> Int { run(one, b) }
pub fn q(b: Box[clock]) -> Int { p(b) }
```

(With a closure literal instead of `one` the program was sound: the
closure's effects were still open, so Box[clock] bound fx to clock.)

## Cause

compiler/check's unify (infer.xo, its Func and EffArg arms) called
unify_eff and returned true. unify_eff binds an open effect variable and
does nothing when both sets are already bound, so `Box[fx]` with fx bound
to {} by `one` unified with `Box[clock]`, p was inferred pure, and q
passed. Effects are checked
only through obligations (sub within sup), and unify pushed none.

## Fix

A function argument's effects are a lower bound on a call's own effect
variable (call_effarg, an open tail in `tails`); an exact use (a type's
effect argument) binds the tail, and call.xo closes the rest empty. A
later function argument is still checked against the first's effects,
as before (decision 0432 would make it a union). unify_eff records the two bound sets (eff_eqs); assignable, join and a
closure's annotated parameter turn them into obligations both ways at
their span (ObKind.Same: "the expected type `T` fixes these effects as
...", XO0701). A function parameter of function type is contravariant
(func_assignable(expected param, actual param)); a type's effect
argument is invariant. Every unify outside those sites is listed in
the commit with why it cannot pass an effect.

## How to avoid

A new unify site that can move a value into a typed place goes through
assignable, or calls same_effects for the eff_eqs it adds; test it with
an errorcheck case that hides an effect behind that place.

## Unify sites outside assignable (audited 2026-10-30)

Sites (compiler/check; outcomes from errorcheck runs of main's xo and the fixed one):
| site | what | can an effect slip? |
| call.xo:1088 call_method | receiver vs method recv_ty with fresh pre | binds the method's fresh variables from the receiver, so the call performs the receiver's effects: safe (errorcheck effect_recv_explicit; main and the fix agree) |
| call.xo:1187/1189/1191 call_func | result vs hint | hint only; the value is checked by assignable where it is used -> safe |
| call.xo:1533 type_call, 1581 construct, expr.xo:1817 struct_lit | type vs hint | safe: fields are checked by assignable against the hint's types (main already rejects; errorcheck effect_struct_hint keeps it) |
| dump.xo:224, stmt.xo:327, expr.xo:704/1969/2015 | unify with Never / Option of a fresh var | no effect sets -> safe |
| expr.xo:330/334/344 coerce_ret | returned Result vs declared | WAS A HOLE (main: 0 errors on `Ok(b)` of Box[clock] as Box[rand]); the value reaches assignable (expr.xo:358), which now checks it: errorcheck effect_ret_box |
| expr.xo:781 propagate | ? error type vs the function's | WAS A HOLE (0 errors): `?` on `! Retry[clock]` in a function `! Retry[rand]` passed, since err_converts' identical ignores effect sets; now a same-type error is unified and its sets checked, errorcheck effect_err_arg. FOLLOW-UP: other callers of identical (err_converts' Error and from-variant paths compare types only) |
| expr.xo:839 join | if/match branch types | WAS A HOLE (main: 0 errors): the joined type took the first branch's pure type; fixed (same_effects at the branch), errorcheck effect_join_branches |
| expr.xo:1411, 1531-1615 binary/binary_type | operand types (==, arithmetic) | no value flows into a typed slot -> safe |
| expr.xo:1933 fn_lit | annotated closure param vs expected param | a hint; WAS A HOLE (main: 0 errors) through assignable, whose func_assignable unified the parameters: now contravariant there (func_assignable(expected param, actual param)), errorcheck effect_closure_param |
| expr.xo:1953/1963 fn_lit | closure result/err vs its context | the body's values are checked by assignable/coerce_ret -> safe |
| pattern.xo:24/101 | literal and binding patterns vs scrutinee | bindings take the scrutinee's type -> safe |
