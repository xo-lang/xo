# A front end cache entry passed its checks and then did not replay

Status: Observed (2026-10-20), open: the root cause fix is in progress

Symptom: `xo build ./compiler/xo` (and scripts/bootstrap-xo.sh at s1)
failed with "the front end cache's entry for ssa.Builder.unbox_type does
not replay (XO_FE_CACHE=off builds without it)", against the shared
user cache, after building a branch that changed only
compiler/cli/servers. With XO_FE_CACHE=off it built.

Cause (suspected, decision 0271 step 1a's entries): an entry's commit
checks the types the body looked up by name ("t NAME" reads), but a type
the body made records its fields', variants', elements' and result's
types by name without logging them as reads, and so does a type the
body only reached structurally (a field's type, an operand's). An entry
recorded in a build where an earlier body had made such a type passes
the checks in a build where it does not exist yet, and the commit finds
the name missing after it has started changing the module. Sharing
entries more widely (std bodies across programs, 0271 step 1b; entries
across xo builds of one code generation key, 0275) makes it likelier.

Fix: the driver no longer fails the build: an entry whose commit fails
is dropped with the half committed build, the program is built again
without it (a warning names the entry), the body built fresh replaces
the entry, and the pack is rewritten without the old one (compiler/xo
bcache.xo fe_cached_build, ssa FeCache.bad). The root cause fix logs
every type name an entry needs as a read, so such an entry misses
before the commit starts.
