# A decoded payload copies its name on every read

Status: Observed (2026-11-12, decision 0448, Value size)

**Symptom.** Storing SSA payloads in two words (names as an index into
the function's table, Func.aux decoding them) lowered the compiler's peak
memory 2.3% but cost 8.9% CPU and 4.6% wall on aiplatform/apiv1's build;
`drop Str` in the CPU profile went from 4.1 to 9.6 s.

**Cause.** Func.aux builds an Aux.S holding the table's Str: a counted
copy, released after each read. The compiler reads payloads hundreds of
millions of times, mostly to test a kind, an integer or one name.

**Fix.** Read the stored AuxW where no name is needed (ssa.auxw_int, a
match or `if let` that binds no S, Bs or P payload) and test or read names
in place (Func.name_is, name_starts, name_ends, name_of); decode with
Func.aux only where a whole Aux is needed. After: CPU 0.952 against the
one word smaller layout. Check a new hot read with XO_PROF=cpu: `drop
Str` and ssa.Func.aux_w should stay small.
