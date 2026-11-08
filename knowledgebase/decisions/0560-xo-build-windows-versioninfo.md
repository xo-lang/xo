# 0560: xo build writes full Windows version information, icons and a manifest

Status: Proposed (Xo Lang 2, 2026-11-07, at the lead's request)

## Context

A windows/amd64 program built by the native backend links with Xo's PE
linker (decision 0254 6b step 4, compiler/macho pelink.xo), which always
writes a `.rsrc` section (compiler/macho peres.xo): Xo's application
manifest (UTF-8 code page, long paths, Windows 10 and 11) and a
VS_VERSIONINFO whose file and product version come from `--stamp
VERSION=A.B.C.D` (compiler/xo ld.xo, any stamp named `version` in any
case), with a fixed FixedFileInfo and the strings FileDescription,
InternalName, ProductName and OriginalFilename from the program's name. An icon
is possible only through the linker tool itself, `macho pelink
--icon=FILE.ico`; `xo build` has no way to set one, nor the strings
Windows Explorer shows (CompanyName, FileDescription, LegalCopyright,
ProductName, OriginalFilename and the rest), the FixedFileInfo flags,
the translation, or a program's own manifest.

Go programs get these from github.com/josephspurrier/goversioninfo: a
`versioninfo.json` (FixedFileInfo, StringFileInfo, VarFileInfo,
IconPath, ManifestPath) turned into a `resource_windows_amd64.syso`
that `go build` links. Xo Lang 2 ported it to Xo
(~/godev/claude-space/xolang/goversioninfo, 2026-11-07): Go's 24 tests
pass, and on 1,920 corpus cases (configs with keys in any case, numbers
out of range, bad hex, nulls, wrong types, syntax errors, corrupt icons,
every arch and flag) every file written, the exit code and the output
are byte identical to Go's tool; on a Windows host the port's .syso links
into a Go program and PowerShell reads its VersionInfo.

Xo has no `.syso` mechanism: a package directory's objects are not linked.

## Decision

`xo build` takes the version information as goversioninfo's JSON, so a
project moving from Go keeps its file:

- `--versioninfo=FILE.json`: goversioninfo's schema and parsing rules
  (keys matched ignoring case as encoding/json does, its defaults, its
  warnings and its error texts), from the port's versioninfo.xo.
  FixedFileInfo, StringFileInfo and VarFileInfo replace the stamped
  ones; its IconPath and ManifestPath are read relative to the JSON
  file, as goversioninfo reads them relative to the working directory
  it runs in.
- `--icon=FILE.ico` (repeatable, or comma separated as goversioninfo's
  `-icon`): the icon group, overriding IconPath. The first icon is the
  application icon Explorer shows.
- `--manifest=FILE.manifest`: the program's manifest, replacing Xo's
  default one (and overriding ManifestPath). Without it every program
  keeps Xo's default manifest, as today.
- Without `--versioninfo`, `--stamp VERSION=` still sets the versions;
  with both, the JSON's FixedFileInfo versions win when they are not
  zero, else the stamp fills them (goversioninfo's fill rule between the
  fixed and string versions applies after).
- The flags apply to windows targets only. In a build for several
  targets they are used for the windows ones; a build with no windows
  target and one of the flags is a usage error (exit 2), as an unknown
  flag is.
- Errors (a bad JSON, an unreadable or corrupt icon, a missing manifest)
  are diagnostics with new XOnnnn codes (spec section 10), carrying
  goversioninfo's message text.

Implementation, reusing the port:

- The port's JSON model and parser (versioninfo.xo), its
  VS_VERSIONINFO writer (structbuild.xo) and the icon reader
  (rsrc/ico.xo) move into the compiler as compiler/winres (attributed,
  MIT); compiler/macho peres.xo keeps its resource tree writer
  (pe_rsrc) and takes the version and icon resources from winres,
  replacing pe_version and pe_icon so there is one writer.
- Native link (pelink): the resources go into the `.rsrc` section as
  today.
- lld-link path (`use c` programs, XO_WIN_NOCRT=0): the same resources
  as a COFF object with a `.rsrc` section (the port's syso.xo writer),
  passed to lld-link, so both links write the same resource bytes.

## Consequences

- A Windows program built by Xo can carry its company, product,
  description, copyright, file and product versions, an icon and its
  own manifest, from the file a Go project already has.
- New code in compiler/ (the fixed point set): the self build, codegen
  and wasm checks of pushq --needs. The JSON parser uses std/json, which
  compiler/xo already links.
- Tests: winres unit tests from the port's ported Go tests; a cmd/xo
  test that builds a windows/amd64 hello with the port's
  testdata/resource/versioninfo.json, its icon and manifest, and compares
  the image's `.rsrc` resources with the ones goversioninfo's .syso holds
  for the same inputs (types, ids, languages and data byte for byte; the
  section's layout may differ); a run on the amd64 offload host's
  Windows under the owner's rules, reading the VersionInfo with
  PowerShell as RESULTS.md did.
- Docs: toolchain.md (xo build flags, the pelink section), spec section
  10's new codes, CHANGELOG, and the port's FEEDBACK and RESULTS.

## Alternatives

1. Link `.syso` files from the package directory as Go does, and keep
   goversioninfo as a separate tool: a generated binary file in the
   repository and a second tool to install, and pelink would have to
   merge `.rsrc` sections from objects. Kept as possible later work for
   projects that already commit a .syso.
2. Settings in xo.mod (a `windows` section with the strings): a second
   schema to document, and no path for a Go project's existing JSON.
   The flags can be read from xo.mod later if wanted.
3. Compile Windows `.rc` scripts (rc.exe's language): a larger grammar
   (dialogs, menus, string tables) than a CLI program needs.
4. Keep only `macho pelink --icon`: no version strings, and the linker
   tool is not something users call.
