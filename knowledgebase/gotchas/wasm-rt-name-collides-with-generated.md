# A WAT runtime function named like a generated helper

Status: Observed (2026-10-13, decision 0201).

**Symptom.** TestSelfhostWasm: the golden newtypes trapped at load with
`Compiling function #62:"bytes_eq16" failed: call[0] expected type i32,
found i64.load of type i64`.

**Cause.** selfhost/wasm/rt2.xo had a helper `$eq8` (a SWAR byte test);
the wasm backend also generates `$eq<N>` functions, the equality of type
number N, and newtypes had a type 8. The assembler bound `call $eq8` to
the generated one (params i32 i32). Generated names are `$f<N>`,
`$eq<N>`, `$sig<N>`, `$dropf<N>`, `$dropm<N>`, `$lcopy<N>`,
`$boxdrop<N>`, `$envdrop<N>`.

**Fix.** Runtime helpers take a prefix no generated name has
(`$swar_eq8`, `$swar_nib8`). A runtime function that implements
`rt.<name>` keeps `$<name>` (rt2_direct maps the call by name).
