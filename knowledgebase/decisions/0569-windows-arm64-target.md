# 0569: windows/arm64 as a native target

Status: Accepted (owner, 2026-11-08, on the lead's recommendation; windows/arm64 stays out of releases until it runs in a VM). Proposed (Xo Lang 2, 2026-11-08; the owner decides). The owner
approved windows/arm64 as a new target on 2026-11-08, through the lead;
this ADR is the plan. Stages 1 to 4 need no Windows machine and are
verified by cross build. Running anything waits for the test host (a
Windows 11 on Arm VM on the owner's coming Mac).

Stage 1 progress (2026-11-08, Xo Lang 2): the assembly half is done.
`xo build --backend=arm64 --targets=windows/arm64` (only with the
backend named: `--targets=all` and the default build still leave the
target out, and it is in no release) writes assembly that `clang
-target aarch64-windows-msvc` assembles: hello, all 28 arm64 run
goldens and a sample of 64 of the 640 LLVM goldens (every tenth), then
stops with "does not link it yet". llvm-readobj --unwind (host 1)
decodes the .pdata and .xdata of every function with no warning. The
darwin, linux and windows/amd64 assembly is byte-identical to the
parent's on 8 programs (12 for arm64). What it has:

- the frame: the saved registers and the frame record at the top,
  allocated first, then `add_fp`, then the rest of the frame probed a page
  at a time (compiler/arm64 winarm.xo); each prologue and epilogue
  instruction carries one `.seh_*` directive, as llvm checks;
- shrink wrapping off (a later prologue would need its own fragment);
- a leaf's fault stubs as a function fragment of their own;
- landing pads: the personality as `.seh_handler`, the GCC call site
  table as its handler data (x86win.xo's form);
- TLS through `[x18, #88]`, `_tls_index` and secrel, an 18 word `.tls$`
  table, the TEB's stack bounds in the task switch (the LLVM emitter's
  arm64_win_switch);
- DWARF in COFF sections (`"dr"`, `.secrel32` offsets);
- x18 never written: the preemption stub no longer saves and restores it
  off Linux (the LLVM backend's windows/arm64 output changes too), and
  test/codegen/winx18 checks the whole program (`// arm64@windows:`
  patterns of `xo dev codegen`); cmd/xo TestWindowsArm64Assembly.

The link half (2026-11-08): `xo build --backend=arm64
--targets=windows/arm64` writes an ARM64 PE from any host. clang
(`--target=aarch64-pc-windows-msvc`) assembles each part, the zone
data, the build information and the outline atomics
(link.xo win_arm64_atomics: each tests xo_lse and takes the LSE
instruction or the load and store exclusive loop, as on Linux;
cabi_wincrt.xo xo_win_start sets xo_lse from IsProcessorFeaturePresent's
PF_ARM_V81_ATOMIC_INSTRUCTIONS_AVAILABLE, 34; no XO_LSE=0 override
there yet). Xo's PE linker links
them with no C runtime (link.xo nolibc is always true for
windows/arm64, so cabi_wincrt.xo's xo_win_start is the entry). pelink
reads 0xAA64 objects and applies the ARM64 relocations as lld-link does
(compiler/macho pearm64.xo: the field holds the addend; a page offset
keeps its 12 bits, the carry being the adrp's), with ADDR64 as a
DIR64 base relocation, and writes 16 byte import thunks (adrp x16; ldr
x16; br x16; brk). Variadic C calls pass every argument in x0 to x7 and
then the stack (vfix -4 in compiler/arm64 call_locs); no `use c` source
links yet, so no program reaches them.

`use c` (2026-11-08): with XO_WINSYSROOT set (an xwin sysroot with
arm64), clang compiles the adapters and `link` C files for
aarch64-pc-windows-msvc with `-isystem` the sysroot's MSVC and SDK
include directories (link.xo win_sdk_dirs; Apple's clang does too), the
C library functions they call being cabi_wincrt.xo's, and `link "NAME"`
is the sysroot's arm64 NAME.lib (the file name's case ignored), whose
short import objects of machine 0xAA64 pelink adds to its import table
(compiler/macho implib.xo; the SDK's ARM64X libraries also hold ARM64EC
members, skipped; a static library is refused). cmd/xo
TestWindowsArm64UseC links a program whose C calls user32's
GetSystemMetrics; llvm-readobj (LLVM 23.1.3 on the Mac) shows the
USER32.dll import and the C function's load from its IAT slot. A binding
is still made with the host's clang (`xo bind --c`), so a header's
`long` binds as Int though Windows' is 32 bits (Anticipated).

Stage 2 progress (2026-11-08, Xo Lang 2): `xo dev debug --unwind-check`
reads ARM64 images (compiler/debug a64unwind.xo, with no llvm-readobj):
each .pdata entry in order and apart; packed entries expanded into the
codes the canonical prolog stands for (the order llvm-readobj prints,
its dumpPackedARM64Entry); each .xdata record of version 0, its epilog
scopes in order inside the function, its codes known and ending, with
save_next resolved against the pair after it; every code matched with
its instruction (prolog codes newest first, an epilog's in instruction
order, the end code with ret, b or br; `__chkstk`'s `sub sp, sp, x15,
uxtx #4` with x15 from its movz and movk); each prolog's frame a
multiple of 16 with every save inside it; every epilog freeing the whole
frame (through x29 when it restores sp from it); and no instruction of
.text writing x18 (data processing destinations, loads, mrs). A cross
built hello (678 functions) and a clang built program (the C
runtime and a 133 KB function: 686, 10 of them packed) pass with no problem; with every
function's first instruction made a nop the check finds 533 prologs
that no longer match, and one more 16 bytes in an alloc_s code is
found in the prolog and in both its epilogs. cmd/xo
TestWindowsArm64Assembly runs it on hello, and a nop'd copy. Still to do: the
goldens in the gate, and the same reader in llrt's unwinder (with the
ARM64 CONTEXT offsets), which waits for the host. Codes the checker
refuses: alloc_z, save_any_reg and the custom stack cases (0xe8 to
0xec), none of which the emitter or clang's C output uses.

Stage 3 progress (2026-11-08, Xo Lang 2): Xo's own COFF writer for
windows/arm64 (compiler/macho coffa64.xo, coffseh64.xo), the default:
`xo build --backend=arm64 --targets=windows/arm64` needs no clang
except for `use c` (`--as=cc` keeps stage 1's clang assembler). It
reuses the Mach-O and ELF writers' arm64 parser and encoder, with
coff.xo's sections, symbols and writer, and writes the relocations as
LLVM's COFF streamer does: a temporary label's reference is to its
section's symbol with the offset in the instruction's field
(PAGEBASE_REL21, PAGEOFFSET_12A for an add, 12L scaled for a load or
store, SECREL_HIGH12A and LOW12A for `:secrel_hi12:` and `:secrel_lo12:`,
BRANCH26, 19 and 14, ADDR32NB); a branch to a symbol of its own section
that is not weak is resolved. The .seh_ directives become .xdata and
.pdata as LLVM 21's MCWin64EH.cpp writes them (its ARM64 half, ported):
the codes simplified to their one byte forms and save_next, a packed
.pdata word when the prolog and its one epilog are canonical, an epilog
sharing the prolog's codes or an earlier epilog's, padding with 0xE3,
the handler as an IMGREL word; each record at its `.seh_endproc` (a
`.seh_handlerdata` one at that directive, never packed), then .pdata
at the end. No mapping symbols ($x, $d: ELF's); LLVM's offset labels
(`$L<section>_N` at every 1 MiB of a larger section, a relocation
through a temporary label past 1 MiB moved to the label below it, so a
branch's or an adrp's addend fits its field). Objects are byte
identical to Apple clang 21's (`--target=aarch64-pc-windows-msvc`):
hello with its DWARF (426,508 bytes of .text, 678 .pdata entries,
2026-11-09) and 671 of 671 goldens' assembly (the arm64 run and LLVM
goldens, scripts/coff-compare-arm64.sh, 2026-11-09); compiler/macho
coffa64_test.xo holds two samples with clang's objects. The DWARF
sections need `.secrel32` (SECREL in a .debug section) and clang's
section header bits: a .debug section is discardable whatever its
flags, .data's default alignment is 4 and every section but .bss has a
data pointer, empty or not (ARM64 only: amd64's COFF bytes are
unchanged until its own compare with clang). Back-end cache keys of these
objects are their own (`coff=arm64`). The linked image (2026-11-09, main
f484fc4af, hello, 549,351 bytes, the same bytes with `--as=cc`) reads
cleanly with llvm-readobj 23.1.3 on the linux/arm64 host
(`--file-headers --sections --coff-imports --coff-debug-directory
--coff-load-config --coff-resources`, and `--unwind`): no warnings,
machine ARM64 (0xAA64), console subsystem 6.0, DYNAMIC_BASE,
HIGH_ENTROPY_VA, NX_COMPAT and TERMINAL_SERVER_AWARE, sections .text,
.rdata, .data, .pdata, .tls, .rsrc and .reloc, imports from KERNEL32,
api-ms-win-core-synch-l1-2-0, WS2_32, ntdll, SHELL32 and bcrypt, a TLS
directory, the version resource, no debug directory (no PDB yet) and no
load configuration, and all 678 .pdata entries decode with their
unwind codes. Whether Windows runs it waits for a windows/arm64 machine.

PDB progress (2026-11-09, Xo Lang 2): a windows/arm64 program with
debug information gets a PDB beside it, as windows/amd64's does
(decision 0344), and its image a debug directory naming it. The build
passed the link no debug flag (main.xo), so the link wrote no PDB;
pdb.xo, cvobj.xo and cvtypes.xo read DWARF, not machine code, so only
three parts were x86-64's: the DWARF relocations applied to an object's
sections (ARM64's ADDR64, SECREL and ADDR32NB), the CodeView registers
of the locals' locations (cvtypes.xo cv_reg_arm64: x0 to x28 as
ARM64_X0 to X28, 50 to 78, fp 79, lr 80, sp 81, a vector register as
S, D or Q by size, from LLVM's CodeViewRegisters.def) and the DBI
header's machine (0xAA64, the image's). A debugger maps lines with the
info stream (GUID and age, matching the image's RSDS record), the DBI
module with its section contributions, the module's S_GPROC32 records
with their C13 line blocks and file checksums, and the publics; the
types, locals and inline sites come with them. Unwinding needs nothing
from the PDB (.pdata and .xdata). Checked without Windows on the
linux/arm64 host (LLVM 23.1.3, 2026-11-09, a two function hello):
the image's RSDS GUID and age equal the PDB's (`llvm-readobj
--coff-debug-directory`, `llvm-pdbutil dump --summary`: debug info,
types, IDs, globals and publics), `llvm-pdbutil dump --all` reads its
13 streams with no error, and each of its 8,920 line rows (`dump -l`)
names the file and line `llvm-symbolizer` gives from the image's DWARF
for that address's outermost frame (the PDB, as MSVC's, puts inlined
code at its call's line and the callee's lines in S_INLINESITE). A debugger on Windows reading it waits for
the windows/arm64 machine, as running the image does.

Not done in stage 1 yet: --race for windows/arm64;
a split link and the std object; the C ABI exports beyond Str results;
running an image (stage 2's host). To
measure in stage 5: `save_regp` needs ascending pairs (`stp x19, x20`),
the order gotchas/arm64-stp-pair-order.md found 1.4x slower on recursive
fib on Apple M4 than Linux's `stp x20, x19`.

## Context

windows/arm64 is the one target of spec 12.1 that the native backends
do not build:

- `target_backend` (compiler/xo/main.xo:593) maps it to no backend, so
  `--targets=all` leaves it out with a note and naming it is an error
  (decision 0270's addendum).
- The LLVM backend cross-builds it as `aarch64-pc-windows-msvc`
  (decision 0197 stage 3; compiler/xo/clang.xo:108).
  scripts/lead/windows-batch.sh builds the LLVM goldens for it, but none
  has ever run ("not run: no host").

Windows on Arm PCs (Snapdragon X and later) are now a large part of new
Windows laptops. Go has had windows/arm64 since Go 1.17 (issue 36439).

Most of what the target needs already exists for another target:

| Part | Where it exists today | windows/arm64 |
|---|---|---|
| arm64 instruction selection, register allocation, frames, encoder | compiler/arm64 (darwin, linux) | reused; x18 is never allocated on any OS (regalloc.xo:17), which Windows requires |
| PE/COFF linker, no C runtime, imports table, resources, manifest | compiler/macho pelink.xo, peimports.xo, peres.xo, llrt cabi_wincrt.xo (decision 0254 6b step 4) | reused; machine field, relocations and import thunks gain an arm64 form |
| COFF object writer | compiler/macho coff.xo (x86 only: 0x8664, AMD64 relocations, x64 UNWIND_INFO) | new arm64 path |
| Runtime Windows layer: file I/O, IOCP, threads, processes, errno | std/native sys_windows.xo, llrt windows.xo (no architecture references) | reused as is |
| Windows arm64 fiber frame with the TEB stack fields | llrt sched.xo:199-210 (from the LLVM path) | reused |
| Windows arm64 CONTEXT offsets for raising | llrt unwind.xo:1174-1196 (`WA_*`, from the LLVM path) | reused, extended |
| Authenticode signing | scripts/sign-windows.sh (jsign, Azure Artifact Signing), verify-windows-sig.sh | machine neutral, reused |
| `.syso` resources with an ARM64 machine | compiler/winres/coff.xo:14 (0xaa64) | reused |

## What Windows on Arm64 asks of generated code

Sources, retrieved 2026-11-08:

- Microsoft's "Overview of ARM64 ABI conventions";
- Microsoft's "ARM64 exception handling";
- the PE format page (machine type and relocation types);
- Go's runtime/sys_windows_arm64.s;
- Go's race detector article.

### Calling convention

Non-variadic calls follow AAPCS64, the same as linux/arm64:

- x0-x7 and v0-v7 carry arguments;
- x8 holds the indirect result address;
- x19-x28 and the low 64 bits of v8-v15 are callee saved.

Xo's arm64 calls and the platform's are therefore the same convention.
Unlike windows/amd64 (decision 0254 6b: Xo stays System V, and Win64
appears only in generated thunks), arm64 needs no thunks for ordinary
OS calls or callbacks. The differences:

- **x18** holds the TEB pointer in user mode and must never be written.
  Xo never allocates it; hand-written runtime assembly must not touch
  it either (checked by stage 2's checker).
- **Frame chain.** x29 must point at the caller's {fp, lr} pair, for ETW
  and the stack walker. Xo's arm64 frames already chain this way on
  darwin and linux.
- **Variadic calls** pass every argument in x0-x7 and then on the
  stack, floats and HFAs included; SIMD registers are not used. Linux
  passes floats in v registers, and darwin passes variadic arguments on
  the stack. This affects only `use c` adapters and the variadic C
  calls cbind generates, a third case beside darwin's.
- **No red zone.** Only 16 bytes below sp are reserved, and those are
  for patching. Xo's arm64 code uses none (the survey found red-zone
  code only in the x86 files); stage 2 keeps that true.
- **Stack probes.** A frame of 4 KB or more touches its pages in order
  (`__chkstk`, with x15 holding the size / 16). With no C runtime, Xo
  supplies the probe in cabi_wincrt.xo, as `w64_chkstk` does for amd64
  (x86win.xo:446).
- SP alignment faults are always on, and FPCR's trap bits must stay 0.

### Unwind data

Every function that touches the stack needs a .pdata entry of 8 bytes:
its start RVA, then a second word whose low two bits are a flag. Leaf
functions with no frame and no saved registers have none.

- **Flag 0:** an .xdata RVA. The .xdata holds:
  - a header (function length / 4 in 18 bits, so at most 1 MB per
    fragment);
  - epilogue scopes;
  - unwind codes that map 1:1 to the prologue and epilogue
    instructions, so an exception mid-prologue or mid-epilogue still
    unwinds.
- **Flag 1:** packed. Usable when the function is under 8 KB, the frame
  under 8 KB - 16, and the prologue and epilogue are canonical. The
  fields are RegI (x19 upward), RegF (d8 upward), CR (the {fp, lr}
  chain), H (homed arguments) and FrameSize.
- **Flag 2:** a packed fragment with no prologue or epilogue.
- **Flag 3:** a link record for a jump island.

Functions over 1 MB split into fragments (compiler/macho coffseh64.xo
a64_segments, ported from LLVM's ARM64FindSegmentsInFunction on
2026-11-09: xo's own build for windows/arm64 has a 2.6 MB function;
each segment gets its own .xdata record and .pdata entry, a segment
without the prolog starts its codes with end_c, and the objects match
llvm-mc 23 byte for byte). Code generated at run time
would register with RtlAddFunctionTable; Xo generates none.

Xo's arm64 emitter writes `.cfi_*` directives (emit.xo:271-447,
2079-2162). Mach-O turns them into compact unwind (compiler/macho
cfi.xo), and ELF keeps eh_frame. Windows needs a third translation,
from the same frame description to ARM64 unwind codes:

- packed .pdata where the prologue is one of the canonical shapes;
- otherwise .xdata.

The emitter's prologue (`stp x29, x30, [sp, #-N]!`, `mov x29, sp`, then
callee-saved pairs) maps to `save_fplr_x`, `set_fp`, `save_regp` and
`save_fregp`. Where an Xo prologue has no unwind code (an order the
codes cannot express), the Windows path emits the canonical order
instead; stage 2 lists every such shape.

Xo's own unwinder (decision 0221: faults, contracts and panics) reads
the platform's tables on each OS. On Windows it already reads x64
.pdata for amd64 (`win_walk`, unwind.xo:2251). An ARM64 reader of the
same tables makes `xo debug`, profiles and crash traces work, and
doubles as the checker.

### Thread-local storage

The static TLS sequence goes through x18:

    ldr  x9, [x18, #0x58]          // TEB ThreadLocalStoragePointer
    adrp x8, _tls_index
    ldr  w8, [x8, :lo12:_tls_index]
    ldr  x8, [x9, x8, lsl #3]
    add  x8, x8, :secrel_hi12:var  // IMAGE_REL_ARM64_SECREL_HIGH12A
    add  x8, x8, :secrel_lo12:var  // IMAGE_REL_ARM64_SECREL_LOW12A

pelink already makes `_tls_index` and the TLS directory (pelink.xo:412,
630-640, mostly machine neutral). The emitter's TLS sites in sched.xo
(darwin's TLV descriptor at :129, ELF local-exec at :141) gain this
third form.

Go uses a TlsAlloc slot at TEB+0x1480 instead. Its bug #45138, the
slot offset read from the wrong register, worked by luck and broke only
on newer Windows. So the host runs the newest Windows 11 on Arm build,
and a test reads a thread local from several threads.

### COFF and PE

- **Machine:** 0xAA64.
- **Relocations** (PE format page):

  | Type | Value |
  |---|---|
  | ADDR32NB | 0x2 |
  | BRANCH26 | 0x3 |
  | PAGEBASE_REL21 | 0x4 |
  | PAGEOFFSET_12A | 0x6 |
  | PAGEOFFSET_12L | 0x7 |
  | SECREL | 0x8 |
  | SECREL_LOW12A | 0x9 |
  | SECREL_HIGH12A | 0xA |
  | ADDR64 | 0xE |
  | BRANCH19 | 0xF |
  | BRANCH14 | 0x10 |

- **Import thunks:** `adrp x16, __imp_f; ldr x16, [x16, :lo12:__imp_f];
  br x16`, in place of amd64's `jmp *IAT(%rip)` (pelink.xo:446).
- **Range thunks:** BRANCH26 reaches ±128 MB, so pelink needs them only
  past that. xo itself is 84 MB on darwin/arm64, so the limit is
  checked, not assumed.
- Base relocations stay DIR64, the image base stays 0x140000000, and
  the image stays PE32+.

## Decision

Build windows/arm64 on the arm64 backend in reviewable stages, each
verified by cross build until the host exists:

1. **Driver and assembly, objects from clang.** Steps:
   - `target_backend` maps windows/arm64 to arm64;
   - the emitter gets a Windows branch, wherever it now branches on
     darwin or linux:
     - COFF symbol and label names;
     - `:lo12:`-style page addressing;
     - `.rdata` and the other sections (gen.xo:360 already has the
       branch);
     - weak references through `/alternatename`, as amd64 does (done
       2026-11-09: rt.sym_present and weak C calls, the sys_c golden
       0101 on the winarm VM);
     - the TLS sequence above;
     - variadic C calls;
   - the emitter writes `.seh_*` directives beside its CFI.

   Objects come from `clang -target aarch64-windows-msvc`: Apple's clang
   assembles ARM64 COFF with SEH directives, checked 2026-11-08 on a
   test function with `.seh_save_fplr_x`, `.seh_save_reg` and
   `.seh_set_fp`. pelink gains the arm64 machine, relocations and
   import thunks, so the first images already link with no C runtime
   and no SDK, as windows/amd64 does by default. This is unlike amd64's
   step 1, which used lld-link first.
2. **The unwind checker.** Extend `xo dev debug --unwind-check`
   (compiler/debug/winunwind.xo, x64 only today) to ARM64 images. It:
   - decodes every .pdata and .xdata entry, packed and unpacked;
   - checks that each covers a whole function, with no overlaps, in
     order;
   - checks that the prologue and epilogue instructions match the
     codes 1:1, decoding the image's own prologue and epilogue bytes
     with a small decoder of the instruction forms they use (stp, ldp,
     str, ldr, add, sub, mov). compiler/debug has no arm64 disassembler
     today;
   - checks that no function writes x18 and none stores below sp.

   It runs on every golden's image in the gate. The same reader goes
   into llrt's unwinder for faults, crash traces and `xo debug`, beside
   the x64 `win_walk`. The ARM64 CONTEXT layout replaces the x64
   constants in unwind.xo, crash.xo, dbgwin.xo and prof.xo by target:
   X0 at 8, Fp 0xF0, Lr 0xF8, Sp 0x100, Pc 0x108, V at 0x110, 912
   bytes, as unwind.xo's `WA_*` constants already have them. The
   ContextFlags value comes from winnt.h's ARM64_NT_CONTEXT, and every
   offset is checked against that header.
3. **Xo's own COFF writer for arm64.** compiler/macho coff.xo gains an
   arm64 path that reuses the arm64 assembler front end and encoder of
   the Mach-O and ELF writers, writing ARM64 relocations and .xdata.
   Its objects must be byte identical to clang's on every golden, as
   the amd64 COFF writer's are to llvm-mc's (scripts/coff-compare.sh,
   199 of 199 on 2026-10-19). Then no clang is needed: windows/arm64
   cross-builds from any host with xo alone.
4. **Release and seed plumbing,** behind `XO_RELEASE_WINDOWS_ARM64=1`
   until the host passes:
   - release.sh gains a sixth default archive, the windows/arm64 one
     (today: darwin/arm64, darwin/amd64, linux/arm64, linux/amd64 and
     windows/amd64; wasip1/wasm only with XO_RELEASE_WASI=1);
   - SHA256SUMS, the signing step (release.sh:492) and the layout
     lines that name windows-amd64 cover it;
   - seed-cut.sh's platform loop (:276) builds a windows/arm64 seed
     only once a host would build xo itself, which the test host does
     not need, since it runs cross-built goldens;
   - `--targets=all` includes windows/arm64, and spec 12.1's note is
     removed. Stage 6 only.
5. **On the host (waits for it),** in order:
   - the Windows golden subset (scripts/wine-goldens-subset.txt: OS
     calls, fs, net, threads, TLS, faults and unwinding, callbacks,
     processes), then every golden. The LLVM build of the same goldens
     is the reference;
   - `xo test` of std's Windows modules;
   - a fault in a thread, a panic through a callback, and IOCP under
     load;
   - xo itself cross-built for windows/arm64, running `xo build` there;
   - one signed hello and http_server (billed, so the owner approves
     them).
6. **Default and release.** The owner decides whether the archive
   ships, after stage 5 passes on the host and on one real Windows on
   Arm machine, if the owner has one.

**Race runtime: not in this plan.** Go's race detector does not
support windows/arm64: windows/amd64 is its only Windows pair (go.dev
race detector article, 2026-11-08). Xo's windows/amd64 runtime is TSan's
Go flavor built with MinGW-w64 in Docker (scripts/race-runtime.sh,
race-windows.cpp). ThreadSanitizer has no upstream windows/aarch64
platform. `--race` for windows/arm64 stays the existing error for a
target with no runtime in lib/xo/race (ld.xo:274 `race_files`). A port
(llvm-mingw's aarch64 toolchain and the Go-flavor sources) is a later
ADR, with its own measurement.

**Not in scope:**
- ARM64EC and ARM64X (x64 emulation interop);
- PAC (`pacibsp`; unwind code `pac_sign_lr`);
- SVE state.

Windows does not require them for a normal arm64 process.

## Test host

A Windows 11 on Arm VM on the owner's coming Mac (Parallels or UTM),
reached and scripted like ~/xo-lead/offload-win.env (OFF_KIND=win). It
is for pass/fail runs only, never timing, since a VM on a busy Mac
measures nothing. Setup notes and a script come when the Mac exists,
with the VM's address and credentials kept out of tracked files, as for
the amd64 Windows host.

Until then:
- every stage is verified by cross build: images link, the unwind
  checker passes on every golden, the identity diff holds, and the
  COFF writer matches clang;
- every runtime claim is marked "waiting for the host" in this ADR and
  in CHANGELOG entries;
- the identity diff compares each golden's windows/arm64 code with its
  linux/arm64 code: only symbol names, TLS, probe, variadic and unwind
  sites may differ. It is a text test like internal/arm64
  TestWindowsAmd64Identity.

llvm-readobj (which decodes ARM64 unwind with `--unwind`) is not on the
Mac, and Apple's llvm-objdump decodes AMD64 unwind only. The checker of
stage 2 is Xo's own. llvm-readobj on an offload host is the second
opinion, cross-checked once per stage.

## `use c` goldens on the winarm VM (Xo Lang 2, 2026-11-09)

With XO_WINSYSROOT (xwin's sysroot, ~/xo-tools/winxc on the Mac) the four
`use c` goldens (use_c_add, use_c_bytes, use_c_enum, native_sqlite) pass
on the Windows 11 ARM VM (scripts/winarm-goldens.sh). native_sqlite took
four fixes, each the next link error:

- pelink.xo keeps the first COMDAT section of a symbol and leaves out
  later copies with their associative sections (coff_comdat_drops;
  coffread.xo reads each COMDAT section's selection and associated
  section): clang's MSVC string literals (`??_C@...`) are COMDAT any in
  every object that uses them ("is defined twice").
- `__chkstk` (compiler-rt's aarch64 chkstk.S) in the generated
  xo_atomics.s: clang probes the stack of a C frame over a page.
- kernel32.lib from the sysroot in every `use c` link, as MSVC's linker
  adds it (SQLite's RaiseException).
- std/sqlite's amalgamation compiled with its windows/amd64 prebuilt
  object's defines (cffi.xo sqlite_win_defines: SQLITE_WIN32_MALLOC, no
  worker threads, no SEH, no extension loading, THREADSAFE=2): with no C
  runtime there is no realloc, _msize, _beginthreadex or
  __C_specific_handler. A windows-arm64 prebuilt object
  (scripts/sqlite-prebuilt.sh) is a follow-up for releases.

## Consequences

- Every target of spec 12.1 gets a native backend. `--targets=all`
  means the same with and without `--backend`, and 0270's note goes
  away.
- The arm64 emitter gets a third OS. The Windows sites are mostly
  where darwin and linux already differ (symbol and label names,
  addressing, sections, TLS, unwind), so the change is breadth, not a
  new convention.
- No SysV/Win64-style boundary layer: arm64's convention is the
  platform's, so x86win.xo has no arm64 counterpart beyond the stack
  probe and the variadic case.
- pelink and coff.xo become two-architecture. Their tests run both
  machines.
- The release grows a sixth archive and the signing step a second
  Windows executable, once stage 6 is decided.
- The fault and debug paths (unwind.xo, crash.xo, dbgwin.xo, prof.xo)
  stop assuming x64 on Windows.

## Alternatives

- **Keep windows/arm64 on the LLVM backend only.** That needs clang and
  the Windows SDK sysroot for every build. It is the configuration that
  0254 6b step 4 removed for amd64, and it is still untested on
  hardware. Rejected as the end state; the LLVM build stays the
  reference while the native one is checked.
- **lld-link first, Xo's linker later,** as amd64 did. On arm64 the
  import, TLS and relocation work in pelink is small against stage 1,
  and lld-link would bring back the SDK dependency. Linking with
  pelink from the start avoids a step that would be deleted.
- **Hand-written .xdata only,** with no packed form. It is simpler, but
  it costs 8 to 16 bytes per function more than packed .pdata, for
  thousands of functions in xo. The checker handles both forms either
  way.
- **Wait for the host before any work.** Stages 1 to 4 are verifiable
  without it, and the host arrives with the Mac. Working now means the
  first host run tests finished code.
