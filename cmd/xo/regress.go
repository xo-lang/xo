package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
)

// regressFile is where `xo test` writes regression tests for shrunk contract
// fuzz failures, in the module itself so they can call its private
// functions (core 9.2).
const regressFile = "regressions_test.xo"

// regressRecord mirrors rt.RegressRecord.
type regressRecord struct {
	Module string `json:"module"`
	Dir    string `json:"dir"`
	Test   string `json:"test"`
	Seed   int64  `json:"seed"`
	Call   string `json:"call"`
	Result string `json:"result"`
	Params []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
		Cap   string `json:"cap"`
		Var   bool   `json:"var"`
	} `json:"params"`
}

// writeRegressions turns the records in path into regression tests. A test
// is written only if the module still type checks with it; otherwise the
// reason is reported and the seed remains the way to reproduce.
func writeRegressions(path string, out io.Writer) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r regressRecord
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		name := fmt.Sprintf("regression: %s (seed %d)", strings.TrimPrefix(r.Test, "contract fuzz: "), r.Seed)
		dst := filepath.Join(r.Dir, regressFile)
		old, _ := os.ReadFile(dst)
		if strings.Contains(string(old), strconv.Quote(name)) {
			continue
		}
		src, why := regressionSource(r, name)
		if why != "" {
			fmt.Fprintf(out, "xo test: no regression test for %s: %s\n", r.Test, why)
			continue
		}
		next := string(old)
		if next == "" {
			next = "// Regression tests written by `xo test` from shrunk contract fuzz\n// failures (core 9.2). Keep them; edit or delete them like any test.\n"
		}
		next += "\n" + src
		if why := checksWith(r.Dir, next); why != "" {
			fmt.Fprintf(out, "xo test: no regression test for %s: generated test does not check: %s\n", r.Test, why)
			continue
		}
		if err := os.WriteFile(dst, []byte(next), 0o644); err != nil {
			fmt.Fprintf(out, "xo test: no regression test for %s: %v\n", r.Test, err)
			continue
		}
		fmt.Fprintf(out, "xo test: wrote %q to %s\n", name, dst)
	}
}

// regressionSource renders the regression test, or the reason it cannot.
func regressionSource(r regressRecord, name string) (string, string) {
	var b strings.Builder
	fmt.Fprintf(&b, "// Found by contract fuzzing with seed %d.\ntest %s {\n", r.Seed, strconv.Quote(name))
	call := r.Call
	for i, p := range r.Params {
		var arg string
		switch p.Cap {
		case "os":
			return "", "the function takes `os: Os`, which a test cannot pass"
		case "":
			if p.Value == "" {
				return "", "no recorded value for `" + p.Name + "`"
			}
			arg = "arg_" + p.Name
			kw := "let"
			if p.Var {
				kw = "var"
			}
			fmt.Fprintf(&b, "  %s %s = %s\n", kw, arg, p.Value)
		default:
			arg = "os." + p.Cap
		}
		call = strings.ReplaceAll(call, "{"+strconv.Itoa(i)+"}", arg)
	}
	switch r.Result {
	case "result":
		fmt.Fprintf(&b, "  %s.ignore_err(\"regression: only a fault fails this test\")\n", call)
	case "value":
		fmt.Fprintf(&b, "  let _ = %s\n", call)
	default:
		fmt.Fprintf(&b, "  %s\n", call)
	}
	b.WriteString("}\n")
	return b.String(), ""
}

// checksWith reports the first error when the module in dir is checked
// with regressFile replaced by src, or "" if it checks clean.
func checksWith(dir, src string) string {
	tmp, err := os.MkdirTemp("", "xo-regress-")
	if err != nil {
		return err.Error()
	}
	defer os.RemoveAll(tmp)
	mod := filepath.Join(tmp, filepath.Base(dir))
	if err := os.CopyFS(mod, os.DirFS(dir)); err != nil {
		return err.Error()
	}
	if err := os.WriteFile(filepath.Join(mod, regressFile), []byte(src), 0o644); err != nil {
		return err.Error()
	}
	res, _, err := build.Check([]string{mod}, false)
	if err != nil {
		return err.Error()
	}
	for _, d := range res.Diagnostics {
		if d.Severity == diag.Error {
			return string(d.Code) + " " + d.Message
		}
	}
	return ""
}
