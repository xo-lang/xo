# CodeView inline site annotations: ChangeCodeOffset is 3, not 2

Status: Observed (2026-10-22, decision 0344 stage 3)

Symptom: a PDB's S_INLINESITE records dumped correctly with
`llvm-pdbutil dump --symbols` ("code 0x59D (+0x59D)", "code end 0x615"),
and llvm-pdbutil found no error, but llvm-symbolizer named almost no
inlined frame from the PDB (and once a frame that was not there): 145 of
268 addresses had DWARF's inline chain.

Cause: cvinfo.h's BinaryAnnotationOpcode has ChangeCodeOffsetBase = 2
and ChangeCodeOffset = 3. compiler/macho pdb.xo wrote 2. llvm-pdbutil
prints both as a code offset; LLVM's reader
(NativeFunctionSymbol.cpp inlineSiteContainsAddress) handles only
CodeOffset (1), ChangeCodeOffset (3) and the combined ops, so every
range started at the wrong offset. Also: a site whose first entry
changes no line and moves more than 15 bytes was written as a bare
ChangeCodeOffset, and NativeInlineSiteSymbol::getLineOffset then has no
current line, so the frame (and, in PDBContext, every frame after it)
is dropped.

Fix: opcode 3, and a ChangeLineOffset (0 if need be) before the first
entry's code offset. Check with a reader's behavior, not a dump: replay
the reader's algorithm (LLVM's source is short), and compare
llvm-symbolizer's chains from the PDB alone with DWARF's over many
addresses (decision 0344 stage 3: 265 of 268 after the fix).
