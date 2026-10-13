# 0211: noalias for list parameters on LLVM

Status: Accepted (owner, 2026-10-13, as part of the zero-C work); being implemented on a branch.

Xo value semantics guarantee distinct List parameters do not alias; the LLVM emitters mark them noalias so loops can vectorize. Compiler internal, no user visible change. Stub claimed by the lead; the noalias agent fills it in.
