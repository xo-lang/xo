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

// LocalTags lists the versions tagged in the git repository at dir,
// lowest first: the tags prefix+vX.Y.Z with the prefix removed (prefix
// is "" for a module at the repository root, "cli/" for one in cli/).
func LocalTags(dir, prefix string) ([]string, error) {
	out, err := git(dir, "tag", "--list", prefix+"v*")
	if err != nil {
		return nil, err
	}
	var vs []string
	for _, t := range strings.Fields(string(out)) {
		t, ok := strings.CutPrefix(t, prefix)
		if ok && ValidVersion(t) {
			vs = append(vs, t)
		}
	}
	sort.Slice(vs, func(i, j int) bool { return Compare(vs[i], vs[j]) < 0 })
	return vs, nil
}

// ExtractTag writes the tree of directory subdir ("" for the root) at tag
// in the git repository at dir into dst (`git archive`, as fetching does:
// nothing in it runs), without nested modules.
func ExtractTag(dir, tag, subdir, dst string) error {
	treeish := "refs/tags/" + tag
	if subdir != "" {
		treeish += ":" + subdir
	}
	data, err := git(dir, "archive", "--format=tar", treeish)
	if err != nil {
		return err
	}
	if err := untar(data, dst); err != nil {
		return err
	}
	return pruneNested(dst)
}
