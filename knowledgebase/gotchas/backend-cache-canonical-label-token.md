# A text pass that parses labels misreads the back-end cache's canonical labels

Status: Observed (2026-11-19, task #356)

## Symptom

A peephole that reads local labels in a function's emitted text (here
`x86_r11_reuse`, which keeps r11's stack slot across a block label
nothing jumps to) worked in a build with `XO_BACKEND_CACHE=off` and
miscompiled with the cache on, the default. Nine amd64 goldens
(iface_values, lists, maps, json_text, and others) died with SIGSEGV in
`xo.map.put` on the linux/amd64 host. The same compiler cross building
on the Mac with the cache off gave correct binaries, which hid it: only
the run's warm-cache builds failed.

## Cause

With the cache on, compiler/arm64 fcache.xo generates each function as
a canonical text whose label prefix is the token `\x01F\x01`
(`.L\x01F\x01_b466`); place_text substitutes the real prefix only
later. The passes in x86_emit_once run on that canonical text. The
label scanner (`x86_local_label_refs`) read a label as `.L` followed by
identifier bytes, so it stopped at `\x01`: every reference became `.L`,
no label definition matched a reference, and every join point looked
like a fall through. The pass then kept a slot's value in r11 across a
label that another path, one that changed r11, jumps to.

## Fix

Count byte 1 as part of a label when scanning (x86emit.xo
`x86_local_label_refs`), with a unit test on canonical labels
(x86r11reuse_test.xo). To check that a text pass sees both forms, build
the same program with `XO_BACKEND_CACHE=off` and with a fresh
`XO_BACKEND_CACHE_DIR`, keeping the assembly (`XO_KEEP_ASM=1`), and
compare them function by function: they must match up to the label
prefix.
