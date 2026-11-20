package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// TestCLIMatrix is the class guard for the CLI and toolchain bugs (task
// #361; 96 past fixes in the lead's bug class report, 2026-11-18): the
// working directory or PWD, Windows drive paths, unreadable files and
// wrong exit codes. Each subcommand runs on one small module under every
// condition below, and each run checks its exit code and that no file
// of the tree changed and none appeared outside the build output.
//
//   - root: from the module root, the reference run (exit 0 expected).
//   - subdir: from a directory inside the module, paths relative to it.
//   - outside: from the directory above the module, paths through it.
//   - abs: absolute paths (on Windows these are C:\ paths).
//   - stalepwd: from the root with PWD naming another directory (a shell
//     that cd'd without exporting PWD, a test driver, an editor): the
//     output must match the root run's, since the process's directory
//     is the root (8ce127284, 67afa0c5a, 6999b0568).
//   - unreadable: util/util.xo cannot be read: an error naming it, no
//     crash, and no write anywhere, above all no file emptied or
//     replaced (057a60966).
//   - drive: on Unix, a C:\ path names no file: an error, no crash, and
//     no file named after it created.
func TestCLIMatrix(t *testing.T) {
	t.Parallel()
	type cmdCase struct {
		name string
		// args with {m} for the module directory as the condition writes
		// it and {out} for a build output outside the module
		args []string
		// edits files on success (the root run and paths reaching the
		// module): the no-change check applies to the error conditions
		// only
		writes bool
	}
	cmds := []cmdCase{
		{name: "check", args: []string{"check", "{m}/..."}},
		{name: "parse", args: []string{"parse", "{m}/..."}},
		{name: "fmt-check", args: []string{"fmt", "--check", "{m}/..."}},
		{name: "fmt", args: []string{"fmt", "{m}/..."}},
		{name: "vet", args: []string{"vet", "{m}/..."}},
		{name: "test", args: []string{"test", "{m}/util"}},
		{name: "build", args: []string{"build", "-o", "{out}", "{m}"}},
		{name: "run", args: []string{"run", "{m}"}},
		{name: "def", args: []string{"def", "--dir={m}", "greet"}},
		{name: "refs", args: []string{"refs", "--dir={m}", "greet"}},
		{name: "symbols", args: []string{"symbols", "{m}/util"}},
		{name: "type", args: []string{"type", "{m}/util/util.xo:2:8"}},
		{name: "doc", args: []string{"doc", "--dir={m}", "greet"}},
		{name: "edit-dry", args: []string{"edit", "--dry-run", "--dir={m}", "rename", "greet", "hello"}},
		{name: "edit", args: []string{"edit", "--dir={m}", "rename", "greet", "hello"}, writes: true},
	}
	files := map[string]string{
		"m/xo.mod":            "module example.com/m\n",
		"m/main.xo":           "use example.com/m/util\n\nfn main(os: Os) {\n  os.stdio.println(util.greet())\n}\n",
		"m/util/util.xo":      "/// The greeting main prints.\npub fn greet() -> Str {\n  \"hi\"\n}\n",
		"m/util/util_test.xo": "test \"greet\" {\n  expect(greet()) == \"hi\"\n}\n",
	}
	type cond struct {
		name string
		skip string
	}
	conds := []cond{{name: "root"}, {name: "subdir"}, {name: "outside"}, {name: "abs"}, {name: "stalepwd"}, {name: "unreadable"}, {name: "drive"}}
	// the root runs, for the stalepwd comparison and the expected codes
	type result struct {
		code   int
		stdout string
	}
	rootRuns := map[string]result{}
	for _, c := range cmds {
		top := writeFiles(t, files)
		chdir(t, filepath.Join(top, "m"))
		r := matrixRun(t, top, matrixArgs(c.args, ".", filepath.Join(top, "out", "prog")), "")
		if r.code != 0 {
			t.Fatalf("root %s: exit %d, want 0 (the fixture is wrong):\n%s", c.name, r.code, r.output)
		}
		if !c.writes {
			matrixUnchanged(t, "root "+c.name, top, r)
		}
		rootRuns[c.name] = result{code: r.code, stdout: strings.ReplaceAll(r.stdout, top, "{top}")}
	}
	for _, cd := range conds[1:] {
		cd := cd
		switch {
		case cd.name == "unreadable" && (runtime.GOOS == "windows" || os.Geteuid() == 0):
			cd.skip = "chmod 000 does not stop this user's reads"
		case cd.name == "drive" && runtime.GOOS == "windows":
			cd.skip = "abs covers drive paths on Windows"
		}
		t.Run(cd.name, func(t *testing.T) {
			if cd.skip != "" {
				t.Skip(cd.skip)
			}
			t.Parallel()
			for _, c := range cmds {
				top := writeFiles(t, files)
				m := filepath.Join(top, "m")
				dir, mArg, pwd := m, ".", ""
				switch cd.name {
				case "subdir":
					dir, mArg = filepath.Join(m, "util"), ".."
				case "outside":
					dir, mArg = top, "m"
				case "abs":
					dir, mArg = top, m
				case "stalepwd":
					pwd = top
				case "drive":
					mArg = `C:\xo-matrix\m`
				}
				if cd.name == "unreadable" {
					bad := filepath.Join(m, "util", "util.xo")
					if err := os.Chmod(bad, 0); err != nil {
						t.Fatal(err)
					}
				}
				t.Run(c.name, func(t *testing.T) {
					chdir(t, dir)
					args := matrixArgs(c.args, mArg, filepath.Join(top, "out", "prog"))
					r := matrixRun(t, top, args, pwd)
					if r.code < 0 || r.code >= 64 {
						t.Fatalf("%s: exit %d (a crash or fault, never a result):\n%s", strings.Join(args, " "), r.code, r.output)
					}
					switch cd.name {
					case "subdir", "outside", "abs":
						if r.code != rootRuns[c.name].code {
							t.Errorf("%s from %s: exit %d, the root run's %d:\n%s", strings.Join(args, " "), dir, r.code, rootRuns[c.name].code, r.output)
						}
						if !c.writes {
							matrixUnchanged(t, cd.name+" "+c.name, top, r)
						}
					case "stalepwd":
						if stdout := strings.ReplaceAll(r.stdout, top, "{top}"); r.code != rootRuns[c.name].code || stdout != rootRuns[c.name].stdout {
							t.Errorf("%s with PWD=%s: exit %d stdout %q, the root run's exit %d stdout %q", strings.Join(args, " "), top, r.code, r.stdout, rootRuns[c.name].code, rootRuns[c.name].stdout)
						}
						if !c.writes {
							matrixUnchanged(t, cd.name+" "+c.name, top, r)
						}
					case "unreadable":
						if r.code == 0 {
							t.Errorf("%s with util/util.xo unreadable: exit 0, want an error:\n%s", strings.Join(args, " "), r.output)
						}
						if !strings.Contains(r.output, "util.xo") {
							t.Errorf("%s with util/util.xo unreadable: the error does not name util.xo:\n%s", strings.Join(args, " "), r.output)
						}
						matrixUnchanged(t, cd.name+" "+c.name, top, r)
					case "drive":
						if r.code == 0 {
							t.Errorf("%s: exit 0 for a C:\\ path that names nothing here:\n%s", strings.Join(args, " "), r.output)
						}
						matrixUnchanged(t, cd.name+" "+c.name, top, r)
					}
				})
			}
		})
	}
}

// matrixResult is one run of the CLI matrix: exit code, stdout, both
// streams, and the tree before and after.
type matrixResult struct {
	code          int
	stdout        string
	output        string
	before, after map[string]string
}

// matrixRun runs xo with args in t's directory, with PWD set to pwd
// when it is not "" (command sets PWD to the directory), snapshotting
// root before and after.
func matrixRun(t *testing.T, root string, args []string, pwd string) matrixResult {
	t.Helper()
	before := matrixSnapshot(t, root)
	var out, errOut bytes.Buffer
	cmd := command(t, xoBin(t), args...)
	if pwd != "" {
		for i, kv := range cmd.Env {
			if strings.HasPrefix(kv, "PWD=") {
				cmd.Env[i] = "PWD=" + pwd
			}
		}
	}
	cmd.Stdout, cmd.Stderr = &out, &errOut
	code := 0
	if err := cmd.Run(); err != nil {
		code = -1
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() >= 0 {
			code = ee.ExitCode()
		} else {
			fmt.Fprintf(&errOut, "%v\n", err)
		}
	}
	return matrixResult{code: code, stdout: out.String(), output: out.String() + errOut.String(), before: before, after: matrixSnapshot(t, root)}
}

// matrixArgs fills a command's {m} and {out}.
func matrixArgs(args []string, m, out string) []string {
	r := make([]string, len(args))
	for i, a := range args {
		a = strings.ReplaceAll(a, "{m}", m)
		r[i] = strings.ReplaceAll(a, "{out}", out)
	}
	return r
}

// matrixSnapshot maps every file under root (but out/, the build
// output) to its mode and the hash of its bytes ("unreadable" for one
// this user cannot read, compared by mode and size only).
func matrixSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			if rel == "out" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		sum := "unreadable"
		if data, err := os.ReadFile(p); err == nil {
			sum = fmt.Sprintf("%x", sha256.Sum256(data))
		}
		m[filepath.ToSlash(rel)] = fmt.Sprintf("%v %d %s", info.Mode(), info.Size(), sum)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// matrixUnchanged fails t when a file of the tree changed, went away or
// appeared during the run.
func matrixUnchanged(t *testing.T, what, top string, r matrixResult) {
	t.Helper()
	before := r.before
	var diffs []string
	for p, b := range before {
		if a, ok := r.after[p]; !ok {
			diffs = append(diffs, "removed "+p)
		} else if a != b {
			diffs = append(diffs, fmt.Sprintf("changed %s: %s -> %s", p, b, a))
		}
	}
	for p := range r.after {
		if _, ok := before[p]; !ok {
			diffs = append(diffs, "created "+p)
		}
	}
	if len(diffs) > 0 {
		sort.Strings(diffs)
		t.Errorf("%s: the run changed the tree (%s):\n  %s\noutput:\n%s", what, top, strings.Join(diffs, "\n  "), r.output)
	}
}
