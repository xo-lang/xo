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
