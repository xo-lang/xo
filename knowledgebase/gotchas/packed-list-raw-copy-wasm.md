# A raw copy of a List[UInt8]'s bytes is wrong on wasm

Status: Observed (2026-10-28, wasm archive_formats; bisected to 124e378a9)

Symptom: on the wasm backend every gunzip failed with "flate: corrupt input
before offset 7" and unzip with "zip: checksum error"; arm64, LLVM and the
Go backend passed the same goldens (archive_formats, and crc32_vectors added
for it). The CRC-32 values were right: the inflated bytes were not.

Cause: flate.list_bytes copied a List[UInt8] to Bytes with one rt.mem_copy
from rt.obj_addr(l) + rt.layout(2), which assumes the native layout: a
List[UInt8] is packed there, one byte an element. The wasm backend keeps
every list element as 8 bytes a leaf (compiler/wasm gen.xo size), so the
copy took the first n bytes of n words: mostly zeros. rt.layout folds to
the wasm runtime's offsets, but nothing folds the element size.

Fix: list_bytes copies a byte at a time on wasm (target.arch ==
TargetArch.Wasm32) and keeps the one copy on the native targets. Any raw
read or copy of a list's elements by address (rt.obj_addr plus
rt.layout(2)) needs the same wasm branch, or a portable form; run
TestSelfhostWasm with XO_SELFHOST_TIER=merge and XO_WASM_GOLDENS on the
goldens that reach it before pushing (the same rule as a new rt intrinsic).
