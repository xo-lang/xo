package rt

import (
	"encoding/json"
	"os"
	"strings"
)

// RegressInfo describes how to call a contract fuzzed function from Xo
// source, so `xo test` can write a shrunk failure as a regression test
// (core 9.2).
type RegressInfo struct {
	Call   string // Xo call with {i} for parameter i
	Result string // "result", "value", or "unit"
	Names  []string
	Caps   []string // TestOs field for a capability parameter, "" otherwise
	Vars   []bool
}

// RegressParam is one argument of a recorded regression.
type RegressParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Cap   string `json:"cap"`
	Var   bool   `json:"var"`
}

// RegressRecord is written, one JSON object per line, to the file named by
// XO_REGRESSION_OUT; `xo test` turns each into Xo source.
type RegressRecord struct {
	Module string         `json:"module"`
	Dir    string         `json:"dir"`
	Test   string         `json:"test"`
	Seed   int64          `json:"seed"`
	Call   string         `json:"call"`
	Result string         `json:"result"`
	Params []RegressParam `json:"params"`
}

// recordRegression appends the shrunk input of a failing contract fuzz test.
// input is PropInput's text: one "name = value" line per generated value.
func recordRegression(tc TestCase, seed int64, input string) {
	path := os.Getenv("XO_REGRESSION_OUT")
	if path == "" || tc.Regress == nil {
		return
	}
	vals := map[string]string{}
	for _, ln := range strings.Split(strings.TrimRight(input, "\n"), "\n") {
		if name, val, ok := strings.Cut(ln, " = "); ok {
			vals[name] = val
		}
	}
	rec := RegressRecord{Module: tc.Module, Dir: tc.Dir, Test: tc.Name, Seed: seed, Call: tc.Regress.Call, Result: tc.Regress.Result}
	for i, n := range tc.Regress.Names {
		rec.Params = append(rec.Params, RegressParam{Name: n, Value: vals[n], Cap: tc.Regress.Caps[i], Var: tc.Regress.Vars[i]})
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}
