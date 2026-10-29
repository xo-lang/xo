# Two modules with the same last path segment shared their SSA names

Status: Observed (2026-10-29; xo-lang-5 through std/x509 and std/native/llrt/x509)

**Symptom.** A program using app/a/util and app/b/util (`use .../b/util as bu`), each with `fn parse(s: Str) -> Int`, printed a's result from b's body on the native and wasm backends; LLVM refused the IR ("invalid redefinition of function x.util.parse"); the Go backend was right. With different signatures: XO1510 "a stale cached piece ... result type does not match the callee" (nothing was stale), or `_xo_f_util_2eparse is defined twice` from the assembler.

**Cause.** compiler/ssa func_name (and the constants' xo.const[...] names) qualify a function with its module's default name, the last path segment, so both modules' functions were `util.parse`. Types already took the module path when ambiguous (ambiguous_type_names); functions did not.

**Fix.** ssa path_named_mods: a program module whose default name another module of the build has is named by its path (`github.com/x/app/a/util.parse`). std modules keep their names, which the std store holds whatever a build loads; two std modules sharing a name may not declare the same function (internal/repotest TestStdSameNameModulesDisjoint; std/trace and std/native/llrt/trace share one, disjoint). xo's own compiler/gen and compiler/debug are path named now (llrt gen, std debug). Golden same_basename_modules.

**Avoid.** A name the backends treat as an identity must be unique by construction, not by the convention that paths' last segments differ.
