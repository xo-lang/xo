# The Windows import table can name an API set DLL

Status: Observed (2026-11-10, Lang 3, decision 0584 stage E)

## Symptom

Adding `CoTaskMemFree` to std/native/sys_windows.xo and regenerating
compiler/macho/peimports.xo with scripts/win-import-table.py mapped it to
`api-ms-win-core-com-l1-1-0.dll`, not `ole32.dll`, which Go's
golang.org/x/sys/windows and every other caller name.

## Cause

The generator reads every import library of the SDK in an order that puts
the libraries the runtime links first (its `order` list, the same as
compiler/xo link.xo's `-l` flags) and the rest by name, and the first
library that names a symbol wins. ole32 was not in the list, so an
umbrella library that forwards to the API set sorted before ole32.lib.

## Fix

Add the DLL to the generator's `order` list in the same change as the
link flags (link.xo sys_link_args and internal/xotool WindowsLinkLibs,
which TestWindowsLinkLibs keeps equal), then regenerate. Check the diff
of peimports.xo for an `api-ms-win-*` name before committing.
