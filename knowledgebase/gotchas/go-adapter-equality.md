# Go backend: a converted interface value compared unequal

Status: Observed (2026-11-12, tier 71 at 400183a7d)

## Symptom

`TestGoldenGoParity/iface_wrap_eq` failed: on the Go backend, an interface
value converted from another interface (`fn widen(p: Plain) -> Named { p }`)
was `!=` the same concrete value made directly (`let b: Named = Sq{s: 1}`),
and two converted values differed whenever the conversions differed. The
native backends and want.txt say they are equal (core 2.9: interface values
are equal when their concrete values are).

## Cause

The Go backend writes a conversion between interfaces whose methods differ
as an adapter struct (`xoAdapt_N{V: inner}`), and a `var self` holder as a
cell (`xoCell_N`). Their `XoEq` compared only against the same adapter type,
so `xoAdapt_0{V: Sq{1}}` never equaled `xoAdapt_1{V: Sq{1}}` or a bare
`Sq{1}`. The golden landed with decision 0613 stage 2, whose gate ran
TestGoldenGoParity only for compiler/gen and std/rt changes, so nothing ran
it on the Go backend until the full tier.

## Fix

Adapters and cells implement `XoInner() any`; `rt.EqAny` unwraps both sides
(`rt.UnwrapAny`) before comparing. scripts/gate.sh's llvm step now runs
TestGoldenGoParity for every internal/llvm golden a branch adds or edits
(`llvmgo`), whatever else it changes. Any new Go-backend wrapper type must
implement `XoInner` too.
