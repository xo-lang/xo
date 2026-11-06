# std/native/sqlite

Origin: sqlite the SQLite amalgamation (sqlite3.c, sqlite3.h), copied unchanged

The C that std/sqlite binds with `use c` (decision 0534).

- [sqlite3.c](sqlite3.c), [sqlite3.h](sqlite3.h): SQLite's amalgamation, vendored unchanged (THIRD_PARTY_NOTICES item 8).
- [VERSION](VERSION): its release, the zip's URL and its SHA3-256.
- [xo_sqlite.c](xo_sqlite.c), [xo_sqlite.h](xo_sqlite.h): Xo's own adapter over SQLite's C API, the functions std/native/sqlite.xo binds.

To update, follow decision 0534's update procedure: replace the two files from the new amalgamation zip, update VERSION and THIRD_PARTY_NOTICES in the same commit, and rerun std/sqlite's tests.
