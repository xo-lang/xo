# A tuple sret result read in place returned zero words (arm64)

Status: Observed 2026-11-16 (xo-lang-2's std/amqptest rewrite; fixed in 258bd9d26 on
main by xo-lang-1).

## Symptom

On arm64 a program faulted with SIGSEGV at address 0 in `drop Str`
under `dropm M`, after `List.remove_at` of a large element (about 30 or
more words). The LLVM backend printed the right answer. It crashed only
when the out of line `xo.list.remove_at[List[M]]` was used (two call
sites, so it was not inlined), and only at the list's last element.

## Cause

`xo.list.remove_at` (compiler/ssa/listmove.xo) sends its slow path to
the prelude's `list_remove_at`, whose result is a tuple: the element and
the `var` list's new value. The helper returns that tuple whole
(`ret v11`). `plan_sret_lazy` (compiler/arm64/byref.xo) marked the call
"read in place", so `sret_set` gave the call zero words for the element
(`sret_res`): only `field_op`'s Field 0 reads the slot. The `ret` copied
the zero words, and the caller later dropped a null `Str`.
`plan_sret_slots` already refused a tuple result used whole
(`a == r && g.sret_tuple(r) && y.op != ssa.Op.Field`); the lazy plan did
not.

## Fix

`plan_sret_lazy` marks a tuple result used directly (not through a
Field or Payload) as bad, as `plan_sret_slots` does. Regression test:
internal/arm64 `TestRun/sret_lazy_tuple`.

## How to spot it

The caller's result buffer holds zero words with only the trailing
`var` parameter words set. Look at the callee's asm for a copy of the
zero image (`Lxo_zimgN`) where the call's result should be.
