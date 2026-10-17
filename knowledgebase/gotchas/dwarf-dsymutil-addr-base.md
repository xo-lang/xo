# DWARF 5 location lists: three ways to get them silently wrong

Status: Observed (2026-10-17, decision 0252 stage 2, selfhost/arm64
debug.xo)

Symptoms and causes:

- `dwarfdump --verify`: "Offset into the .debug_abbrev section is not
  valid" for every unit, while the unit headers look right. Cause: an
  attribute code above 127 written as one byte in the abbreviation
  table (DW_AT_loclists_base is 0x8c). Abbreviation codes, attributes,
  and forms are ULEB128: 0x8c is `0x8c, 0x01`.
- The assembler: "label ..._dv8 is used but not defined". Cause:
  pair_mem (selfhost/arm64 lsr.xo) joins two loads or stores across
  labels nothing in the text refers to and drops those labels; the
  debug sections' references are not in the text. Fix: pair_mem keeps
  `_dl` and `_dv` labels (is_row_label).
- `--ld=cc` (dsymutil): "unable to resolve indirect address N for:
  DW_LLE_base_addressx", and lldb shows no variables. Cause: dsymutil
  rewrites location lists to address indexes into a .debug_addr it
  writes, but sets DW_AT_addr_base only on units that already have one.
  Fix: every unit carries DW_AT_addr_base (an empty .debug_addr of its
  own); selfhost/macho's dSYM relocates it with loclists_base.

Check every DWARF change with `dwarfdump --verify` on both the macho
and the cc link, and with llvm-dwarfdump --verify on Linux.
