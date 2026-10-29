# The build that loaded the std record had another build id than the one that wrote it

Status: Observed (2026-10-29; Lang 2's Windows run 40820335875, 130 bytes; darwin 190).

## Symptom

Two builds of one program by one xo differed by 130 bytes on Windows (the
default build, which wrote the std record, against an explicit separate
build, which loaded it) and 190 on darwin against a build without the
record. Stripped binaries differed the same way, symbols and section
sizes did not.

## Cause

The build info's `<std>` entry (compiler/xo binfo.xo binfo_sources) is the
hash of every std-directory file's source. A file made from the std
record (decision 0388 stage 2) is unloaded until a tree is needed: no
source. The loader hashed empty bytes, got another `<std>` value and so
another build id, and the id is in the binary three times (the JSON,
the `<std>` value, the fault line's `id`): 112 bytes, plus the UUID and
the code signature's page hashes on darwin, the PDB GUID and stamp on
Windows. Code and data were identical.

## Fix

The record carries each file's hash (StdFileRec.sha, the same sha256 of
its bytes); load_std_record keeps it on the SrcFile (`sha`) and
binfo_sources uses it for an unloaded file. internal/arm64
TestStdRecordIdentity builds three times (the record written, loaded,
and XO_STD_EXPORT=0) under one output name and requires identical bytes.

## Rule

Anything the build derives from a source file's content must have a
source for it: a path that leaves files unparsed (records, lazy trees)
hands the derived value along instead. Compare whole binaries, not
outputs, when a new path replaces a parse; and compare builds under one
output name (the code signature identifier is the file's name).
