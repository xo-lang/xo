# Passing the authenticated user in a request header is spoofable

Status: Resolved (2026-10-08, decision 0055). Anticipated 2026-10-08 while
writing `examples/notes-service`; never hit in practice.

Resolution: std/http now has typed request values. A middleware calls
`next(req.with_value(user_key(), user))` and handlers read
`req.value(user_key())`, where `fn user_key() -> http.Key[User]`. Values
are process local: a client cannot send one, so there is nothing to
overwrite. Route scoped middleware (`Router.mount`) also removes the
"every route sits behind it" condition below. notes-service uses both. The
rest of this entry describes the old workaround.

Symptom: a client sends `X-Notes-User: 1` with no token (or with another
user's token) and reads user 1's data.

Cause: `http.Request` has no request scoped values, so the only way for a
middleware registered with `Router.wrap` to hand the resolved user to a
handler is to rewrite the request, in practice a header. Headers also come
from the client. If the middleware only adds the header when it is missing,
or a route is reachable without the middleware, the client's value wins.

Fix: in the middleware, always overwrite the header after a successful
token lookup (`headers.put(...)` then `next(req with {headers: headers})`),
and reject the request before `next` when the lookup fails. Test the spoof
case explicitly (`examples/notes-service/main_test.xo` keeps such a test:
"identity comes only from the token, never from the client"). The real fix is typed request locals
in std/http (gap 1 in `bench/results/real-app-2026-10-08.md`).
