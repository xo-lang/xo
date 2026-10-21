package xotool

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// CheckerDirs are the sources that decide what `xo check` reports for a
// tree (with the tree's own std, --std): the checker and what it imports,
// and the command itself. XO1201 (`xo check --no-gc`) also runs the SSA
// builder (compiler/ssa); CheckerXo's caller adds it then.
var CheckerDirs = []string{
	"compiler/check", "compiler/parser", "compiler/lexer", "compiler/cli/bind",
	"compiler/cli/core/check.xo", "compiler/xo.mod",
}

// CheckerXo is an xo whose `xo check` of root's sources reports what an
// xo built from root's current sources would, without building one
// (owner, 2026-10-21: an agent checked compiler edits all day with an old
// xo, since a fresh build takes a minute): root's own bin/xo when it is
// current (Key), else the newest cached build of a clean tree whose
// recorded commit has the same files as root's working tree under dirs
// (CheckerDirs, uncommitted and untracked files included). ok is false
// when there is none; the caller builds (Ensure) then.
func CheckerXo(root string, dirs []string) (path, why string, ok bool) {
	key, err := Key(root)
	if err != nil {
		return "", "", false
	}
	bin := filepath.Join(root, "bin", exeName("xo"))
	if k, err := os.ReadFile(filepath.Join(root, "bin", "xo.key")); err == nil && strings.TrimSpace(string(k)) == key && isFile(bin) {
		return bin, "the tree's current bin/xo", true
	}
	if untracked(root, dirs) {
		return "", "", false
	}
	base, err := cacheBase()
	if err != nil {
		return "", "", false
	}
	ms, _ := filepath.Glob(filepath.Join(base, "checkout", "*", exeName("xo")))
	sort.Slice(ms, func(i, j int) bool { return modTime(ms[i]).After(modTime(ms[j])) })
	for i, m := range ms {
		if i == 30 {
			break
		}
		d := filepath.Dir(m)
		if _, err := os.Stat(filepath.Join(d, "clean")); err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(d, "commit"))
		c := strings.TrimSpace(string(b))
		if err != nil || c == "" {
			continue
		}
		args := append([]string{"-C", root, "diff", "--quiet", c, "--"}, dirs...)
		if exec.Command("git", args...).Run() == nil {
			return m, "the cached build of " + c[:min(12, len(c))] + " (its checker sources are the tree's)", true
		}
	}
	return "", "", false
}

// untracked reports whether root has untracked files under dirs (git
// diff does not see them).
func untracked(root string, dirs []string) bool {
	args := append([]string{"-C", root, "ls-files", "--others", "--exclude-standard", "--"}, dirs...)
	out, err := exec.Command("git", args...).Output()
	return err != nil || len(strings.TrimSpace(string(out))) > 0
}
