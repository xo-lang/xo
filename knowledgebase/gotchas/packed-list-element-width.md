# A list element access must use the element's own width, not its register type

Status: Observed (2026-10-25)

Symptom: with packed lists on (decision 0369), a `List[UInt8]` swap
written as a raw LLVM load and store (`g.ll(et)`, which is `i64` for
every integer kind) wrote eight bytes over one byte elements: the values
came back wrong and the neighbouring elements were overwritten. The same
code was right on arm64, whose `load_elem`/`store_elem` are width aware,
so the packed rows failed on LLVM only (0374 step 1, `rt.list_swap`: all
four packed rows failed, `List[UInt8]` first).

Cause: LLVM's element address is packed (`elem_ptr` uses `elem_ll`, the
narrowed type), but the value in a register is `i64` or `double` for
every integer width and `Float32`. A load or store at the register type
therefore reads or writes a whole word at a 1, 2 or 4 byte element. arm64
hides the bug because its element load and store take the width from the
list's element type.

Fix: go through `elem_load` and `elem_store` (compiler/llvm/func.xo),
which load at the narrowed width and `zext`/`sext`, or `fpext` a
`Float32`, and narrow or `fptrunc` on the way back, exactly as `list.get`
and `list.set` do. On arm64 `load_elem`/`store_elem` already are. Since
packing is on by default, any new element access needs a row for a packed
type: `List[UInt8]` with 255 and 250 catches a word access at once, and
`List[Int16]` at -32768 catches an unsigned widening (32768), which a
`UInt8` row would not.
