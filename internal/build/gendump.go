package build

import (
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/gen"
)

// GoGenDump is what `xo gendump` prints for one run (decision 0167): dirs
// checked as `xo build` checks them for the Go backend, and the files
// gen.Generate makes with opts (LineRoot set to lineRoot(dirs)), sorted by
// path, each after a line `== file <path>`; `== rejected` when the check
// fails, `== error <why>` when the generator does. The build information
// and the zone data that Build adds are not part of it.
func GoGenDump(dirs []string, opts gen.Options) string {
	res, ok, err := Check(dirs, opts.Holes)
	if err != nil {
		return "== error " + strings.ReplaceAll(err.Error(), "\n", " ") + "\n"
	}
	if !ok {
		return "== rejected\n"
	}
	opts.LineRoot = lineRoot(dirs)
	prog, err := gen.Generate(res, opts)
	if err != nil {
		return "== error " + strings.ReplaceAll(err.Error(), "\n", " ") + "\n"
	}
	var paths []string
	for p := range prog.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, p := range paths {
		b.WriteString("== file " + p + "\n")
		b.Write(prog.Files[p])
	}
	return b.String()
}
