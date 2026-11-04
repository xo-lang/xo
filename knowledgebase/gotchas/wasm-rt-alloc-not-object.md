# Wasm runtime text: `$alloc` is not an object constructor

Status: Observed (2026-10-31, decision 0447 step 1, compiler/wasm rt5.xo)

## Symptom

A wasi:http component built by the wasm backend answered every request
404: the router saw the method as "\x07\0\0" and the path as "\x07"
(right lengths, wrong bytes). A second request on the same instance, or
any request whose router freed the request, trapped in `free` (memory
access out of bounds).

## Cause

rt5.xo's hand written WAT made Strs with `$alloc(n)` and wrote the bytes
at +8, and gave the canonical ABI `$alloc(n) + 8` and freed `p - 8`.
In compiler/wasm rt.xo, `$alloc(n)` returns the first usable byte of a
block of n bytes (its 8 byte block header is below the address, and
`$free(p)` subtracts it itself). A Str or Bytes object is `$rs_new(n)`:
{rc, cap, bytes}, the bytes at +8. So the objects had no refcount or
capacity (the bytes Xo read were the header's), the copies ran 8 bytes
past their blocks, and the frees named the wrong block.

## Fix

Runtime WAT that returns a Str or Bytes builds it with `$rs_new(n)` and
copies to +8; a raw block (cabi_realloc's, a scratch area) is
`$alloc(n)` used at its address and freed with `$free(p)` unchanged.
Check: the node stub host (internal/selfhost testdata/wasm_http/host.mjs,
TestWasmComponentHTTP) runs a component's handle export under Node, with
an instance per request and one instance for every request.

## Seen again (2026-11-04, decision 0447 stage 5 step 2, rt10.xo)

rt10.xo's WASI 0.3 client passed a Str parameter (its object) straight
to `[method]request.set-authority` and `set-path-with-query` as the
string's address. The host read the object's refcount and capacity as
text and refused both, so every outgoing request failed as "the host
refused the request". A Str or Bytes parameter of hand-written runtime
WAT is the object: its bytes, which the canonical ABI wants, are at +8
(rt5.xo adds 8 before every such call). To find which call failed, a
temporary build returned a bit per setter as the error-code case.
