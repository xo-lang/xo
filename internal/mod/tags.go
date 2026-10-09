package mod

import (
	"sort"
	"strings"
)

// Git runs git in dir the way module fetching does (no prompts, no
// hooks, no system config) and returns its standard output (`xo
// publish`).
func Git(dir string, args ...string) (string, error) {
	out, err := git(dir, args...)
	return string(out), err
}

// LocalTags lists the vX.Y.Z tags of the git repository at dir, lowest
// first.
func LocalTags(dir string) ([]string, error) {
	out, err := git(dir, "tag", "--list", "v*")
	if err != nil {
		return nil, err
	}
	var vs []string
	for _, t := range strings.Fields(string(out)) {
		if ValidVersion(t) {
			vs = append(vs, t)
		}
	}
	sort.Slice(vs, func(i, j int) bool { return Compare(vs[i], vs[j]) < 0 })
	return vs, nil
}

// ExtractTag writes the tree of tag in the git repository at dir into dst
// (`git archive`, as fetching does: nothing in it runs).
func ExtractTag(dir, tag, dst string) error {
	data, err := git(dir, "archive", "--format=tar", "refs/tags/"+tag)
	if err != nil {
		return err
	}
	return untar(data, dst)
}
