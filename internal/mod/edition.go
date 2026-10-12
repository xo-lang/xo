package mod

// The edition names (core 13), shared by the `edition` line of xo.mod
// (read by internal/check) and the `xo <edition>` line of xo.work (read
// here): one resolver for both.

// CurrentEdition is the newest edition this toolchain knows (core 13). A
// module whose xo.mod has no `edition` line is on this edition. Until 1.0
// the only edition is `preview` (decision 0155); the owner names the first
// frozen edition at 1.0.
const CurrentEdition = "preview"

// Editions lists every released edition, oldest first. The compiler supports
// all of them (core 13). Tests may append a future edition temporarily.
var Editions = []string{"preview"}

// EditionAliases maps an old edition name to the edition it stands for.
// `2026` was the name of `preview` before decision 0155; it keeps building
// during the 0.x releases, and `xo check` gives the fixable note XO1315 on
// it (in xo.mod and in xo.work).
var EditionAliases = map[string]string{"2026": "preview"}

// ResolveEdition returns the edition that s names: a released edition, or
// the edition an alias stands for. ok is false for an unknown name.
func ResolveEdition(s string) (string, bool) {
	if a, ok := EditionAliases[s]; ok {
		s = a
	}
	if EditionIndex(s) < 0 {
		return "", false
	}
	return s, true
}

// EditionIndex is the position of e in Editions, or -1.
func EditionIndex(e string) int {
	for i, x := range Editions {
		if x == e {
			return i
		}
	}
	return -1
}
