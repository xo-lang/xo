# The worktree's path in a cached object made every worktree assemble again

Status: Observed (2026-10-22), fixed the same day.

## Symptom

A build of xo in a second worktree at the same commit (a gate's
worktree) reused every front end, function, inlining and reference
counting entry of the back-end cache but none of the objects (`cache
objects 0/117 reused`, 115 MB written again) or parts.

## Cause

The arm64 emitter wrote the compile directory and std/'s root into each
part's DWARF as inline strings (two line table directories and the
unit's DW_AT_comp_dir), so the assembly text, the object's key, and the
part's key (partcache.xo hashed both paths) named the worktree. An
inline string cannot be replaced by a longer or shorter one without
moving unit_length, header_length and every unit relative reference.

## Fix

compiler/arm64 debug.xo: the three are DW_FORM_strp into two fixed
slots of the part's .debug_str (DW_SLOT, 1024 bytes, NUL padded),
which hold placeholders (DW_SLOT_COMP, DW_SLOT_STD) in the generated
text; the comp_dir attribute sits at byte 46 of the unit and the
directories at bytes 34 and 38 of the line table. compiler/xo puts the
real paths in as each object leaves compiler/macho's assembler
(dw_slots_obj) and in any text written for another assembler or a file
(dw_slots_text); compiler/macho dsym.xo joins __debug_str and rebases
the three offsets; the ELF linker joins .debug_str as every debug
section. A path of 1024 bytes or more is written as ".". The readers
(compiler/debug, std/native/llrt dwline.xo) already read strp. Test:
internal/arm64 TestBackendCacheObjectsAcrossDirs.
