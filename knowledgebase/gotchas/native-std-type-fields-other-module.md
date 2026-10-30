# Another std module's native fields are not visible

Status: Observed (Xo lang 5, 2026-10-30)

## Symptom

After `TlsCert` moved from std/http to std/tls (decision 0433),
std/native/http.xo's `c.c` on a `tls.TlsCert` failed only in the native
std build (the Go backend passed):

```
xo: warning: separate compilation is off: std's store build failed (xo dev stdobj exit 1: std/native/http.xo:2160:27: error XO0601: `tls.TlsCert` has no field or method `c`)
```

## Cause

A std type's native fields (std/native/tls.xo's `struct TlsCert { c:
tlssrv.Cert }`) are visible in its own native module only. Another
module sees the type as std/tls.xo declares it, an empty handle, as
prelude handles are seen outside the native prelude
([native-prelude-handles-opaque.md](native-prelude-handles-opaque.md)).

## Fix

Give the owning native module a std-internal accessor that casts the std
type to its native struct, and import that module under another name:

```
// std/native/tls.xo
pub fn cert_data(t: tls.TlsCert) -> tlssrv.Cert { rt.cast[tls.TlsCert, TlsCert](t).c }
// std/native/http.xo
use native/tls as ntls
hserve.srv_tls(srv, ntls.cert_data(c))
```

std/native/websocket.xo reaches std/native/http.xo the same way
(`use native/http as nhttp`, `nhttp.handoff_response`). Run the try on
`--backend=arm64`, not only Go, when a type moves between std modules.
