# A build's implicit std/testing load hid a missing prelude import

Status: Observed (2026-10-27, Xo Lang 1)

## Symptom

After builds stopped loading std/testing unless a program file has a test block, `xo build
--targets=all` of a program using `os.net` (internal/llvm golden conn_iface) failed to link its
static no-libc target: `xo: link: xo_x_nc_listen is not defined (a static link with no C
library)`.

## Cause

os.net's runtime calls (rt.net_listen, rt.net_dial, rt.conn_read and seven others) lower to C ABI
exports (compiler/ssa exports.xo rt_export) defined in native/llrt/cabi_net.xo. Only
std/native/http.xo imported that module. Every build used to load std/testing, which imports
std/http, so every program had cabi_net, and nothing showed that the native prelude itself did not
import it.

## Fix

std/native/prelude.xo imports native/llrt/cabi_net. internal/repotest
TestRtExportsImportedByPrelude checks in -short that every rt_export target is defined in a cabi
module the native prelude imports. When a load becomes conditional, look for what else only it
brought in: a link of every target (`xo build --targets=all`) of a program using each runtime
area finds the rest.
