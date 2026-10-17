# The TLS library's objects hold common symbols

Status: Observed (2026-10-12, decision 0182) Obsolete since 2026-10-15: Mbed TLS removed (decision 0202).

Symptom: the self hosted linker (`compiler/macho`) linked every arm64
golden but the std/http ones, which failed with `libxotls.a(...
cipher_wrap.o): common symbol _mbedtls_cipher_supported is not
supported`.

Cause: Mbed TLS declares some globals without an initializer in more
than one file (tentative definitions), and the pack's clang builds keep
them as common symbols: in the object's symbol table an undefined
external whose value is its size and whose `n_desc` bits 8 to 11 hold
the log2 alignment. The program's objects and the runtime have none, so
a linker tested only on them never sees one.

Fix: the linker allocates every common symbol no object defines in a
zero filled `__DATA,__common` (the largest size and alignment among its
declarations), as a synthetic object whose symbols are ordinary global
definitions, so the rest of the link needs no special case. A real
definition anywhere wins over the commons.
