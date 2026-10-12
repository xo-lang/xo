# SSA helper names hold unqualified type names

Status: Observed (2026-10-12, decision 0178 stage 2).

**Symptom.** Building xo1 after selfhost/cli/core added its own `Ctx`
struct failed in the SSA builder:

    xo.frameargs(ctx: Ctx, dirs: List[Str]): ... call xo.debug[Ctx], v5, v4:
    argument 1 has type github.com/xo-lang/xo/selfhost/cli/core.Ctx,
    want github.com/xo-lang/xo/selfhost/xo.Ctx

**Cause.** The builder names its generated helpers after checker types
(`xo.debug[Ctx]`, `xo.frameargs(c: Ctx)`, `xo.json.enc[...]`,
`xo.gen[...]`, `xo.sql.dec[...]`), and a checker type prints without its
module. Two types of one name in different modules got one helper: the
second module's frames called the first module's `xo.debug[Ctx]`.

**Fix.** internal/ssa `helperName` (and selfhost/ssa `helper_name`): a
helper whose name is taken by one of other parameter types (captures,
for `xo.frameargs`) takes the name with `#2`, `#3`, ... Names without a
collision are unchanged, so no golden changed.
TestHelperNamesAcrossModules (internal/ssa) builds the two `Ctx` case.
