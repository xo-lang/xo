# A PDB without /LinkInfo is "file system or network error reading pdb"

Status: Observed (2026-11-09, WinDbg 1.2611's arm64 cdb on the winarm VM)

**Symptom.** cdb finds PROG.pdb beside PROG.exe but loads no symbols:
`!sym noisy` prints `DBGHELP: ...\PROG.pdb - file system or network error
reading pdb`, for windows/arm64 and windows/amd64 PDBs alike, while
llvm-pdbutil reads them without complaint.

**Cause.** compiler/macho pdb.xo wrote the PDB info stream's named stream
map with `/names` only (capacity 1) and no niMac word. This dbghelp
expects the layout MSVC's and lld's linkers write: `/LinkInfo` (an empty
stream) and `/names`, capacity 4, then niMac 0, then the VC140 feature.
Found by bisection: LLVM's `pdb2yaml`/`yaml2pdb` rewrite loaded, xo's
streams in a fresh MSF did not, LLVM's hash, symbol and type streams
grafted in did not, an lld-shaped info stream did; niMac alone did not.

**Fix.** info_stream writes lld's map, with an empty PS_LINKINFO stream
(pdb_test.xo checks the bytes). To check a PDB on Windows:
`cdb -y DIR -z PROG.exe -c "!sym noisy; .reload /f PROG.exe; lm m PROG; q"`
must say `private symbols & lines`.
