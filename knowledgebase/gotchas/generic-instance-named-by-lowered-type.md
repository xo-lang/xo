# A generic instance named by its lowered types: two bodies, one symbol

Status: Observed (2026-11-20, task #469; found by xw-1 under #459)

Symptom: `json.encode_str("aé")` returned `"YcOp"` (the base64 of its
bytes), and any Str with a non-ASCII byte likewise, in every program
linked against std's store. In one program with no store, a function
value `fn(Int) -> Str` of a generic printed a Duration (`<2ns>` for 2)
when the Duration instance had been met first.

Cause: compiler/ssa named an instance `name[T1, T2]` by the SSA types
its arguments lower to (build.xo declare0), and Str, Bytes, StrBuilder
and BytesBuilder lower to one type, as do Int, Duration and Time. The
instance key told them apart; the name did not, so the second instance
met was `name[Str]#2`. Which body the bare name held was the builder's
order: std's store met encode_str[Bytes] first, a program
encode_str[Str], and the linker kept std's weak copy. The thunk of a
generic function value (call.xo) was keyed by the same names, so two
instances shared one thunk inside a single program. 19e3f5961 (task
#369) only exposed it by making encode_str a real function.

Fix: the instance's name and the thunk's key hold the checker's type
names (`b.type_name(b.impl_named(t))`), as the instance key's second
part did. Golden generic_lowered_names (arm64 and LLVM) runs the three
shapes. A name that a linker or a cache resolves must be a function of
what the program wrote, never of the order a builder met things in:
`#2` suffixes are only safe inside one object.
