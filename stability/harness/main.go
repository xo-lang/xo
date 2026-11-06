// harness: the stability program's checks (decision 0520), one Go program
// so that it does not share the runtime under test.
//
//	harness diff -xo XO -app DIR [-backends go,arm64,llvm] [-out OUT]
//	harness soak -xo XO -app DIR -out OUT [-backend arm64] [-rate 500] [-d 2h] (soak.go)
//
// diff builds the app once per backend, runs each build through the app's
// scenario (DIR/app.json names it), and compares the responses: status,
// Content-Type, and the body with the app's redacted JSON fields replaced.
// A scenario step may restart the app (gracefully or with SIGKILL) on the
// same data directory, so restart recovery is part of the comparison.
// OUT/BACKEND.jsonl gets each backend's normalized responses; the exit
// status is 1 when two backends differ or a step's expected status is not
// met, 2 for a usage or setup error.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

// App is DIR/app.json.
type App struct {
	Name string `json:"name"`
	// Env is the app's environment; {addr} is replaced by the address it
	// should listen on and {data} by its data directory.
	Env map[string]string `json:"env"`
	// Ready is the path polled until it answers, after each start.
	Ready string `json:"ready"`
	// Tokens name the bearer tokens a step may use.
	Tokens   map[string]string `json:"tokens"`
	Scenario string            `json:"scenario"`
	// Redact lists JSON object keys whose values differ from run to run
	// (times, generated tokens); their values compare as "<redacted>".
	Redact []string `json:"redact"`
	// Stats is a path answering `name value` lines with heap_live (std/debug's
	// /sched), read by soak with the token StatsToken names.
	Stats      string `json:"stats"`
	StatsToken string `json:"stats_token"`
	// Twin is the directory, relative to the app's, of its Go twin (a main
	// package with the same routes and environment), which load measures
	// against.
	Twin string `json:"twin"`
}

// bodyText is a step's request body: a JSON string is sent as its text,
// any other JSON value (an object, say) as that value's JSON.
type bodyText string

func (b *bodyText) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		*b = bodyText(s)
		return nil
	}
	var c bytes.Buffer
	if err := json.Compact(&c, data); err != nil {
		return err
	}
	*b = bodyText(c.String())
	return nil
}

// Step is one line of the scenario.
type Step struct {
	Method string   `json:"method,omitempty"`
	Path   string   `json:"path,omitempty"`
	Body   bodyText `json:"body,omitempty"`
	Token  string   `json:"token,omitempty"`
	// Expect is the status the step must get (0: any).
	Expect int `json:"expect,omitempty"`
	// Save stores fields of the JSON response under names that later
	// steps use as {name} in Path and Body.
	Save map[string]string `json:"save,omitempty"`
	// Restart is "term" (SIGTERM, then wait) or "kill" (SIGKILL); the app
	// starts again on the same data directory.
	Restart string `json:"restart,omitempty"`
}

// Result is one step's normalized response.
type Result struct {
	Step        int    `json:"step"`
	Request     string `json:"request"`
	Status      int    `json:"status"`
	ContentType string `json:"content_type,omitempty"`
	// Location is a redirect's target (the client does not follow it)
	Location string `json:"location,omitempty"`
	Body     string `json:"body,omitempty"`
	Err      string `json:"err,omitempty"`
}

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "soak" {
		os.Exit(soakMain(os.Args[2:]))
	}
	if len(os.Args) >= 2 && os.Args[1] == "load" {
		os.Exit(loadMain(os.Args[2:]))
	}
	if len(os.Args) < 2 || os.Args[1] != "diff" {
		fmt.Fprintln(os.Stderr, "usage: harness diff -xo XO -app DIR [-backends go,arm64,llvm] [-out OUT]\n       harness soak -xo XO -app DIR -out OUT [-backend arm64] [-rate 500] [-d 2h] [-every 10s] [-warmup 0.1]\n       harness load -xo XO -app DIR -out OUT [-backend arm64] [-rates 1000,4000] [-sat 64] [-d 30s] [-warmup 5s] [-rounds 3] [-drop-env K,...] [-env K=V,...]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	xo := fs.String("xo", "xo", "the xo binary")
	appDir := fs.String("app", "", "the app's directory (holds app.json)")
	backends := fs.String("backends", "go,arm64,llvm", "backends to compare, the first is the reference")
	out := fs.String("out", "", "output directory (default: a new temporary directory)")
	fs.Parse(os.Args[2:])
	if *appDir == "" {
		fmt.Fprintln(os.Stderr, "harness diff: -app is required")
		os.Exit(2)
	}
	code, err := diff(*xo, *appDir, strings.Split(*backends, ","), *out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "harness diff:", err)
		os.Exit(2)
	}
	os.Exit(code)
}

func loadApp(dir string) (*App, []Step, error) {
	b, err := os.ReadFile(filepath.Join(dir, "app.json"))
	if err != nil {
		return nil, nil, err
	}
	var app App
	if err := json.Unmarshal(b, &app); err != nil {
		return nil, nil, fmt.Errorf("app.json: %v", err)
	}
	sort.Strings(app.Redact)
	f, err := os.Open(filepath.Join(dir, app.Scenario))
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	var steps []Step
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		var s Step
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			return nil, nil, fmt.Errorf("%s:%d: %v", app.Scenario, n, err)
		}
		steps = append(steps, s)
	}
	return &app, steps, sc.Err()
}

func diff(xo, appDir string, backends []string, out string) (int, error) {
	app, steps, err := loadApp(appDir)
	if err != nil {
		return 2, err
	}
	if out == "" {
		if out, err = os.MkdirTemp("", "harness-"+app.Name+"-"); err != nil {
			return 2, err
		}
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return 2, err
	}
	runs := map[string][]Result{}
	for _, be := range backends {
		bin := filepath.Join(out, app.Name+"-"+be)
		cmd := exec.Command(xo, "build", "--backend="+be, "-o", bin, ".")
		cmd.Dir = appDir
		if b, err := cmd.CombinedOutput(); err != nil {
			return 2, fmt.Errorf("build %s: %v\n%s", be, err, b)
		}
		res, err := runScenario(app, steps, bin, filepath.Join(out, be))
		if err != nil {
			return 2, fmt.Errorf("%s: %v", be, err)
		}
		runs[be] = res
		if err := writeResults(filepath.Join(out, be+".jsonl"), res); err != nil {
			return 2, err
		}
	}
	code := 0
	ref := backends[0]
	for _, r := range runs[ref] {
		if s := steps[r.Step]; s.Expect != 0 && r.Status != s.Expect {
			fmt.Printf("step %d %s: %s got %d, want %d\n", r.Step+1, r.Request, ref, r.Status, s.Expect)
			code = 1
		}
	}
	for _, be := range backends[1:] {
		a, b := runs[ref], runs[be]
		if len(a) != len(b) {
			fmt.Printf("%s ran %d steps, %s %d\n", ref, len(a), be, len(b))
			code = 1
			continue
		}
		for i := range a {
			if a[i] != b[i] {
				fmt.Printf("step %d %s: %s and %s differ\n  %s: %d %s %s %s\n  %s: %d %s %s %s\n", a[i].Step+1, a[i].Request, ref, be,
					ref, a[i].Status, a[i].ContentType, a[i].Body, a[i].Err, be, b[i].Status, b[i].ContentType, b[i].Body, b[i].Err)
				code = 1
			}
		}
	}
	verdict := "same"
	if code != 0 {
		verdict = "DIFFER"
	}
	fmt.Printf("harness diff %s: %d steps on %s: %s (output in %s)\n", app.Name, len(steps), strings.Join(backends, ","), verdict, out)
	return code, nil
}

func writeResults(path string, res []Result) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, r := range res {
		enc.Encode(r)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// proc is one running copy of the app.
type proc struct {
	cmd  *exec.Cmd
	addr string
	log  *os.File
}

func freeAddr() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	return l.Addr().String(), nil
}

func start(app *App, bin, work string, n int) (*proc, error) {
	addr, err := freeAddr()
	if err != nil {
		return nil, err
	}
	data := filepath.Join(work, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		return nil, err
	}
	logf, err := os.Create(filepath.Join(work, fmt.Sprintf("run%d.log", n)))
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin)
	cmd.Dir = work
	cmd.Env = os.Environ()
	for k, v := range app.Env {
		v = strings.ReplaceAll(v, "{addr}", addr)
		v = strings.ReplaceAll(v, "{data}", data)
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		logf.Close()
		return nil, err
	}
	p := &proc{cmd: cmd, addr: addr, log: logf}
	cl := &http.Client{Timeout: time.Second}
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		resp, err := cl.Get("http://" + addr + app.Ready)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return p, nil
		}
	}
	p.stop("kill")
	return nil, fmt.Errorf("not ready at %s within 20 s (log %s)", addr, logf.Name())
}

// stop ends the process: "term" waits up to 20 s for it to exit by itself.
func (p *proc) stop(how string) error {
	defer p.log.Close()
	if how == "kill" {
		p.cmd.Process.Kill()
		p.cmd.Wait()
		return nil
	}
	p.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(20 * time.Second):
		// where it is stuck, before it goes: the task tree, then macOS's
		// sample (every thread's stack) or Linux's gdb when installed
		hang := strings.TrimSuffix(p.log.Name(), ".log") + ".hang.txt"
		pid := fmt.Sprint(p.cmd.Process.Pid)
		// an Xo program prints its task tree on SIGQUIT and goes on (core
		// 11.4): what each task waits on, into the run's log
		p.cmd.Process.Signal(syscall.SIGQUIT)
		time.Sleep(2 * time.Second)
		if runtime.GOOS == "darwin" {
			exec.Command("sample", pid, "3", "-file", hang).Run()
		} else if gdb, err := exec.LookPath("gdb"); err == nil {
			b, _ := exec.Command(gdb, "-p", pid, "-batch", "-ex", "thread apply all bt").CombinedOutput()
			os.WriteFile(hang, b, 0o644)
		}
		p.cmd.Process.Kill()
		<-done
		return errors.New("did not stop within 20 s of SIGTERM")
	}
}

var placeholder = regexp.MustCompile(`\{([a-z_][a-z0-9_]*)\}`)

func runScenario(app *App, steps []Step, bin, work string) ([]Result, error) {
	os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return nil, err
	}
	starts := 1
	p, err := start(app, bin, work, starts)
	if err != nil {
		return nil, err
	}
	defer func() { p.stop("term") }()
	vars := map[string]string{}
	fill := func(s string) string {
		return placeholder.ReplaceAllStringFunc(s, func(m string) string {
			if v, ok := vars[m[1:len(m)-1]]; ok {
				return v
			}
			return m
		})
	}
	// a redirect is a response to compare, not one to follow (shortener's
	// GET /{code} points at the internet)
	cl := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var res []Result
	for i, s := range steps {
		if s.Restart != "" {
			if err := p.stop(s.Restart); err != nil {
				return nil, fmt.Errorf("step %d: %v", i+1, err)
			}
			starts++
			if p, err = start(app, bin, work, starts); err != nil {
				return nil, fmt.Errorf("step %d: %v", i+1, err)
			}
			continue
		}
		path := fill(s.Path)
		// the step's path as written: a filled in {name} may be random (a
		// generated code), and the response that saved it is compared already
		r := Result{Step: i, Request: s.Method + " " + s.Path}
		var body io.Reader
		if s.Body != "" {
			body = strings.NewReader(fill(string(s.Body)))
		}
		req, err := http.NewRequest(s.Method, "http://"+p.addr+path, body)
		if err != nil {
			return nil, fmt.Errorf("step %d: %v", i+1, err)
		}
		if s.Token != "" {
			tok, ok := app.Tokens[s.Token]
			if !ok {
				tok = fill(s.Token)
			}
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := cl.Do(req)
		if err != nil {
			r.Err = err.Error()
			res = append(res, r)
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		r.Status = resp.StatusCode
		r.ContentType = resp.Header.Get("Content-Type")
		r.Location = resp.Header.Get("Location")
		var doc any
		if json.Unmarshal(b, &doc) == nil {
			for name, field := range s.Save {
				if m, ok := doc.(map[string]any); ok {
					vars[name] = fmt.Sprint(m[field])
				}
			}
			redact(doc, app.Redact)
			nb, _ := json.Marshal(doc)
			r.Body = string(nb)
		} else {
			r.Body = string(b)
		}
		res = append(res, r)
	}
	return res, nil
}

// redact replaces the values of the listed keys anywhere in doc. Maps
// marshal with sorted keys, so key order never differs.
func redact(doc any, keys []string) {
	switch v := doc.(type) {
	case map[string]any:
		for k, x := range v {
			if contains(keys, k) {
				v[k] = "<redacted>"
			} else {
				redact(x, keys)
			}
		}
	case []any:
		for _, x := range v {
			redact(x, keys)
		}
	}
}

func contains(keys []string, k string) bool {
	i := sort.SearchStrings(keys, k)
	return i < len(keys) && keys[i] == k
}
