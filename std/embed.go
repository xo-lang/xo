// Package std embeds the xo standard library declaration files (std/*.xo):
// bodiless signature listings of spec/std.md, whose encoding is
// documented at the top of prelude.xo. Since decision 0215 its one user
// is internal/gobind (the prelude's type names); xo itself reads std/ as
// a directory (lib/xo/std of a release, std/ of a checkout).
package std

import "embed"

// Files holds every std/*.xo file. prelude.xo is the prelude; each other
// file `name.xo` is the module `std/name`.
//
//go:embed *.xo
var Files embed.FS
