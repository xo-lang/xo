# dsymutil relocates an address in a location list as the function's

Status: Observed (2026-10-17, constant variable locations, Xo Lang 3)

Symptom: a variable's location list naming a string literal's static
object (DW_OP_addr of its label, then DW_OP_addrx of a .debug_addr
entry) was right in the dSYM that compiler/macho writes (lldb printed
`bytes = "box"`), but after a cc link and dsymutil:

- with DW_OP_addr of the assembler local L label, the operand stayed the
  object file's address (0x11c48), and `dwarfdump --verify` failed:
  `Name Index ... Entry for DIE ... (DW_TAG_variable) with name it
  missing` (a variable with an address operand counts as a static one);
- with a local symbol (in dsymutil's debug map) or with DW_OP_addrx,
  dsymutil rewrote the operand as a DW_OP_addr of the object file's
  address plus the enclosing function's offset (0x1000122c8; the string
  was at 0x100060060).

Cause: dsymutil's linker treats an address in a location list as one in
the function the list belongs to and applies that function's relocation
to it; a data address in another section cannot be expressed there.

Fix: the driver (compiler/xo main.xo) sets ssa.Module dw_data_addrs only
for ELF links and compiler/macho's link (the default on macOS when no C
source is linked); dwvars.xo describes a string literal's word only when
it is set, and an Int constant's (DW_OP_constu, no address) always.
TestDebugTypes checks the string on the macho link and skips it on cc.

Observed again 2026-11-06 (xo-lang-4): a separately compiled darwin
build (decision 0388, the default once std's variants are cached)
links through a debug map (decision 0401), which lldb and dsymutil read
as they read a cc link's objects: lldb showed another address's bytes
for a Str literal local (TestDebugTypes `bytes = "box"`), and
dsymutil's dSYM failed `dwarfdump --verify` with 54 .debug_names
errors, in std's units. The driver now sets dw_data_addrs off on darwin
for the program's side of a separately compiled build and for std's
object (gm.sepcomp_program, gm.std_obj), and STD_CACHE_VERSION went to
2 so cached variants rebuild; TestDebugTypes and TestDebugVars check
the separately compiled build (macho-sep) through dsymutil and lldb.
