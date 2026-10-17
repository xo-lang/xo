# A std/native/rt.xo declaration shares its name with the SSA builder's runtime calls

Status: Anticipated (2026-10-13, decision 0200)

Symptom: a new intrinsic declared in std/native/rt.xo under a name the
SSA builders already use for a runtime call (`atomic_add`, `atomic_cas`,
`atomic_swap`: Atomic's C operations) compiles, but every call of it is
a call of that C function (xo_atomic_add takes an Atomic handle and a
location, not a list and an index), or an emitter's inline lowering of
the new name catches the old calls too.

Cause: a call of an rt.xo function lowers to `Op.Call` with aux
`rt.<name>`, the same aux the SSA builders give the runtime calls they
make themselves (`b.rt("atomic_add", ...)`, internal/ssa `s.rt`), and
the emitters dispatch on that text alone.

Fix: grep `"rt.<name>"` and `rt("<name>"` in compiler/ssa and
internal/ssa (and `xo_<name>` in std/llrt) before naming an intrinsic.
The audit's `rt.atomic_load`/`atomic_swap`/`atomic_cas`/`atomic_add`
became `rt.slot_load`, `slot_swap`, `slot_cas`, `slot_add` for this
reason. Declaring an existing C function in rt.xo on purpose (as the C
path atomic_c.xo does with `atomic_add`) is fine: it is the same call.
