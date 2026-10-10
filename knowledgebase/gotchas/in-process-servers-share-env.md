# Test servers in the test process share its environment

Status: Observed (2026-10-10, decisions 0124, 0126)

Symptom: `TestModProxy` pointed XO_GIT_BASE at a missing directory to
prove the client fetched through the proxy, then ran `xo get` of a
version the proxy had not cached: XO1302, "repository does not exist",
from the proxy.

Cause: the caching proxy (`mod.ProxyHandler` with fetch) and the
checksum log's lookup (`mod.LogRecord`) run in the test process under
`httptest`, so they read the same XO_GIT_BASE, XO_MODCACHE, and
XO_PROXY as the client under test. Changing an environment variable
for the client changes it for the servers too.

Fix: give the servers their own cache root (`ProxyHandler(root, ...)`,
`LogRecord(root, ...)` take one), fill them before cutting the client off
from git, and never let a server go through XO_PROXY (`download` with
`directOnly`), or a proxy would ask itself. A related trap: a test that
moves a tag to "other content" must check the new tree differs; money
v1.3.0 in testdata already holds `n * 1000`.
