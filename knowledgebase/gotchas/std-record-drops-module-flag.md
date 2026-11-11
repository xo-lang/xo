# std's declaration record drops a new module flag

Status: Observed (2026-11-10, Lang 3; found by Lang 2 and the lead in the
full tier)

## Symptom

After std/base64 was merged with its implementation (decision 0588), a
program using base64 and another std module failed on the native backends:

```
main.xo:9:45: error XO1201: not available on the native backends (arm64, amd64, LLVM) yet: the std function `base64.encode`
```

only when the build logged `cache stdx record loaded` (internal/arm64
TestSepcompStdxKeepsFrontCache, TestDebugInfoSameCode). With
`XO_STD_EXPORT=0` or a private cache it built. A base64 program alone, and
a base32 one with crc32, built: base64 was the only merged module inside
the record's closure (std/native's cbor, crypto, protojson and websocket
import it).

## Cause

compiler/check/export.xo's StdModRec carried path, name, dir,
is_prelude, native and implements, not `Module.merged`. A build that loads
std from the record made base64 an ordinary std declaration module, and
compiler/ssa call.xo lowers a std function from its body only for a root or
merged module, so the call went to std_call and XO1201.

## Fix

`StdModRec.merged`, written and read with the others, restored by
load_std_record; EXPORT_VERSION v11 so no older record is read. Any new
`Module` field that a later phase reads for std modules needs the same three
lines and a version bump. Related: compiler/xo stdcache.xo std_source_set
and tree_without had the same blind spot for `xo test --std-module` (they
read only std/native), now fixed to read std/merged.txt.
