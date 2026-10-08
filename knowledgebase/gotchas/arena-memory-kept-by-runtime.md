# Runtime code that keeps a request or response must copy arena memory

Status: Observed (2026-10-08, arena audit of decision 0066)

Symptom: with request scoped arenas on (the default since decision 0066),
a value the runtime kept from a request changed later. Two cases:

- A recorded trace (`XO_RECORD`) or crash bundle ring (`XO_RING`) held
  `{"name":"b"}st-and-much-longer"}` for a request whose body was
  `{"name":"first-and-much-longer"}`, and `xo replay` diverged. Even a
  route that was not confined showed it: the recorder gets the request
  before dispatch makes the heap copy.
- A cache middleware of a mounted router (`api.wrap(...)`, `r.mount`)
  stored the responses it saw; all 72 of 72 cached bodies were wrong by
  the end of `arena_holes`.

Cause: the request body (`Arena.readBody`) and an arena response's JSON
body (`Http_json_arena`) are `unsafe.String`s over arena buffers that the
next request reuses. The escape analysis covers Xo code (what a handler can
name), not runtime code: `fxEncode` keeps strings as they are, and a
mounted router's middleware runs behind a server without top level
middleware, so `Arena.resp` was still on when its handler built the
response.

Fix: the recorder gets `recCopyReq`/`recCopyResp` copies, and a mounted
route with middleware runs inside `noArenaResp` (as `r.call` does). Any
new runtime code that keeps a request, a response, or their bodies past
the request must copy them first (the list is the comment on `Arena` in
`std/rt/arena.go`) and get a case in golden `arena_holes` or
`TestReplayServer`.
