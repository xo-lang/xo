# 0316: an input-keyed program cache (XO_PROG_CACHE)

Status: Proposed (stub, xo-fa, 2026-10-21; being drafted)

An unchanged warm `xo test` or `xo build` reuses its linked program, keyed
on every input of the build, and skips the check, the SSA passes and the
link. Draft to follow.
