# A Windows ThinLTO link names promoted locals by the modules' source paths

Status: Observed (2026-10-28, Xo Lang 1, after the Windows cross toolchain was rebuilt).

Symptom: `xo build --backend=llvm --target=windows/amd64` with `XO_LLVM_SPLIT=4 XO_LLVM_LTO=thin`, built twice in
one cache (a cold build, then a warm one with no edit), gave two .exe files 512 bytes apart (1,590,784 and
1,591,296). The sections and their sizes were the same; only the COFF header's time stamp (/Brepro: a hash of
the output) and the COFF string table (504,285 against 504,689 bytes) differed. darwin's ThinLTO link of the
same build was byte for byte the same.

Cause: ThinLTO promotes a module's local symbols to globals named `NAME.llvm.HASH`, HASH from the module's
bitcode, which records the name of the source file clang compiled. compiler/xo split.xo compiled each split
module by its full path in the build's private directory, whose name is random per build (`w-<hex>`), so two
builds of the same IR wrote different bitcode and named the promoted locals differently. /debug:symtab (which
the profiler and the fault traceback read) keeps those names in the .exe, and the 512 bytes are the string
table crossing a file alignment boundary. The code is the same; the binary is not. (Running the link in the
objects' directory with their base names did not help: the names come from the bitcode, not the link.)

Fix: split_build compiles each module in its directory by its base name, so its bitcode, and its promoted
locals' names, are the same in every build. Checked: a cold and a warm XO_LLVM_SPLIT=4 XO_LLVM_LTO=thin build,
windows/amd64 and darwin/arm64, are byte for byte the same.

Observed again (2026-11-21, Xo Lang 1, task #607, linux/arm64): the
same rule in a second place. std's own LLVM objects (decision 0738)
are made by distributed ThinLTO, and its steps named each module by
its full path in the build's randomly named directory. There the
effect went past symbol names: the import order follows the module
paths too, so four builds of the same std gave four different
disassemblies of std and of the program linked with it, and about one
build in three a fault report whose traceback named every frame `?`
(gotchas/llvm-std-objects-roots-and-named-section.md, symptom 4). Fix,
the same: every step (bitcode, the index, each backend) runs in that
directory by base name (compiler/xo link.xo ir_objects_par cwd, 0919cb5f3);
TestSepcompLLVM/std_objects_reproducible builds std twice and compares
the code and symbol names. Any new place that hands clang a ThinLTO
module takes the rule: a path that differs between builds is an input.
