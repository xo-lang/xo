# --std-module after an edit ran std's old generic instances and thunks

Status: Observed (2026-11-15, P0 #37; aws-43's sql mutants).

## Symptom

`xo test --std-module=native/llrt/pg` passed with a mutant of
std/native/sql.xo that `XO_BACKEND_CACHE=off` failed. XO_BACKEND_CACHE=verify
pointed at the front end cache, but the cause was the link: the binary held
two copies of `xo.iface[sql.DriverConn].native.sql.Traced.pipeline`, the
program's (mutant, in `_ptext`) and std's test variant's (the original, in
`__text`), and every caller, the program's from-source `sql.Pool.take`
included, branched to the variant's.

## Cause

A `--std-module` run reuses a test variant of std built before the edit
(stdcache.xo std_variant_near) and builds the module and its std importers
from source (check.sep_source). The test variant defines every function
weak (Module.weak_all, decision 0596) so the from-source copies replace
its. But the program's side also made its generic instances and `xo.*`
helpers (interface thunks, closure env helpers, show and debug) weak
(Module.weak_shared, shared_name), and of two weak definitions the link
keeps the first: std's. A plain function of the edited module was strong
and won, so TestSepcompStdModuleEdits (which edits one) passed.

## Fix

A --std-module program makes no definition weak (main.xo gen_module:
weak_shared off when o.std_module is set): its store is always the all-weak test variant, so each
of its definitions replaces the variant's. TestSepcompStdModuleEdits edits
an interface method behind its thunk too (51 to 52).

## How to spot it

`nm -m BIN | grep NAME` shows the symbol in both `__TEXT,__text` and
`__TEXT,_ptext`; `objdump -d` shows which address the callers' adrp/add
pairs take.
