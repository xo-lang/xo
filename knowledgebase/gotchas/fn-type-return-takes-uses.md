# A function type return value takes the `uses` clause

Status: Anticipated (seen in bench/paper/v0.2/05-rate-limiter/limiter.xo)

In `pub fn rate_limit(l: Limiter, clock: Clock) -> fn(http.Request, http.Next) -> http.Response uses clock {`
the parser attaches `uses clock` to the returned function type, because a
function type may end with `uses` (core 15, FnType) and the parser is greedy.
The declaration itself then has no `uses` clause and is pure.

That matches the author's intent here (the returned middleware uses the
clock), but a writer who meant the declaration's effect gets the wrong
meaning silently. To give the declaration an effect, parenthesize the return
type: `-> (fn(A) -> B) uses clock { ... }`, or write both:
`-> fn(A) -> B uses clock uses clock`. The spec does not mention this; it is a
candidate clarification for core 6.2.
