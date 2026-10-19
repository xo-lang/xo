# 0270: Native backend as the default on amd64 hosts

Status: Proposed (stub, xo-fa, 2026-10-19; claimed before the checks).
Filled in once the checks are done: the full golden pass count with the
native amd64 backend on linux/amd64 and darwin/amd64, and instruction
counts against LLVM at release settings. If both pass, this record
supersedes decision 0248's x86_64 line (the default becomes native
whenever the host has a native backend, --race still choosing LLVM;
Windows stays on LLVM until decision 0254 stage 6c). If not, it lists
the gaps, ranked, for the owner.
