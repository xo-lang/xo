# Xo native builds: updating a collection inside a list element copies it

Status: Observed (2026-10-11, the arm64 build of selfhost/xo); fixed by decision 0169 (2026-10-11)

Symptom: the arm64 build of the self hosted compiler driver
(selfhost/xo) ran for over 20 minutes compiling itself, where the go
build of the same code emits selfhost/parser in a few seconds. `sample`
showed nearly every sample in `xo_rl_copy` and `xo_rl_free` under
`G.add`, whose body was `g.blocks[g.cur].insts.push(x)`.

Cause: on the arm64 and LLVM backends, an update of a list (or map)
that is a field of a list element (`xs[i].f.push(x)`, `xs[i].f[j] = v`,
`xs[i].m[k] = v`, and a field of an element of such a list,
`xs[i].f[j].g = v`) reads the element with its own reference to the
inner collection, so the collection's count is two when it is updated
and the update copies all of it. A micro benchmark (20,000 operations,
a 20,000 element inner list): push 1,018 ms (arm64) and 757 ms (LLVM),
`xs[0].f[j].g = v` 2,169 ms, a map put in an element 182 ms; the same
updates on a field of a field (`s.sub.l.push(x)`, `s.sub.m[k] = v`) or a
top level list (`s.l[i] = v`) are in place (0 ms). The go backend
updates in place. Decision 0147's note already lists "box contents and
list elements under a borrowed root" as not done.

Fix (in the code, until the backends update in place): move the inner
collection out, update it, and put it back:

    var ins = g.blocks[b].insts
    g.blocks[b].insts = []
    ins.push(x)
    g.blocks[b].insts = ins

selfhost/arm64 does this for its instruction lists (take_insts,
put_insts), keeps the current block's memo out of the memo list (g.cm),
and pushes onto lists of lists through push_rng and push_int. Next
experiment for the compiler: InsertRC (or the backends) should treat
`list.index` followed by a field update and `list.set` of the same
index as a move of the element (load without retain, store back), the
in place path decision 0147 describes for borrowed roots.

Fixed 2026-10-11 (decision 0169): the SSA builder moves the element out
of the list (`list.take`, a placeholder in its slot) and back, so these
updates are in place on both native backends when the outer list is
unique (bench/results/nested-update-2026-10-11.md: 20,000 pushes 63.6
ms to 0.1 ms on arm64). What still copies: the contents of a box (a
field of a recursive type) under an element, `Map.update` on a map in
an element, and any update whose outer list is shared (value
semantics). The hand workarounds in selfhost/arm64 are no longer
needed for speed (see the results file for the experiment without
take_insts and put_insts).
